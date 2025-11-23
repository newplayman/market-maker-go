
这是做市商系统最重要的“执行层”。  
策略决定什么单要挂、撤、改单，**OrderManager + ExchangeGateway 是让策略真正能跑起来的核心基础设施**。

本说明文档会给出：

- 模块职责
    
- 典型架构
    
- 状态同步机制
    
- 本地订单簿（Local Order Book）
    
- API 调用规范
    
- 下单/撤单流程
    
- 异步回报处理
    
- 错误处理与重试策略
    
- 性能要求
    

可以直接用于实盘开发。

---

# **1. 模块目的**

## **OrderManager（OMS，订单管理）**

主要目标：

1. 统一管理系统内所有订单的生命周期。
    
2. 为策略提供一致且实时的订单状态。
    
3. 将策略意图（OrderIntent）转换为交易所 API 调用。
    
4. 避免订单状态乱序 / 丢失 / 重复，保证最终一致性。
    
5. 控制 QPS / 防止触发交易所限频。
    

本质是：

> **策略与交易所之间的交易中间件（middleware）**  
> **负责可靠、可控、可监控的订单操作**

---

## **ExchangeGateway（交易所网关）**

主要目标：

1. 封装 Binance Futures（USDC-M）REST + WebSocket
    
2. 标准化所有输入输出
    
3. 提供安全、可重试、带序列号的 API 调用
    
4. 处理节流（rate-limit）
    
5. 保证订单回报的顺序与完整性
    

本质是：

> **策略与交易所 API 的适配层 + 稳定通信层**

---

# **2. 模块架构（推荐）**

```
 +------------------------------+
 |     StrategyEngine           |
 +------------------------------+
             |
             | OrderIntent[]
             v
 +------------------------------+        +--------------------------+
 |       OrderManager (OMS)     |<------>|   ExchangeGateway (API)  |
 | - Intent diffing             |        | - REST 下单/撤单         |
 | - 本地订单簿                 |        | - WS 成交/订单回报       |
 | - 状态机（New/Part/Fill）    |        | - 错误重试/限流保护       |
 +------------------------------+        +--------------------------+
             ^                                     |
             | 状态更新                             | 交易所（Binance）
             +-------------------------------------+
```

---

# **3. OrderManager 的核心职责**

### **（1）维护本地订单状态（Local Order Book）**

本地维护每个 symbol 的订单列表：

```go
type LocalOrder struct {
    ClientOrderID string
    ExchangeOrderID string
    Symbol string
    Price float64
    Quantity float64
    Side Side
    Status OrderStatus

    FilledQty float64
    RemainingQty float64
    CreateTime time.Time
    UpdateTime time.Time

    Meta map[string]interface{}
}
```

状态如：

- NEW
    
- PARTIALLY_FILLED
    
- FILLED
    
- CANCELED
    
- REJECTED
    

本地状态必须**100% 与交易所 WS 回报保持同步**。

---

### **（2）将 OrderIntent 变成真实 API 调用**

策略只生成意图：

```
Buy, price=123.45, qty=0.01, Action=New
```

OMS 负责：

- 判断是否需要执行
    
- 匹配现有订单（diff 过程）
    
- 调用 REST API
    
- 将 API 消息转为本地 LocalOrder
    

---

### **（3）diff 行为：减少无效操作**

核心功能：**避免频繁撤单/重挂**

伪代码：

```go
diff(newIntents, localOrders):
    for each localOrder:
        if it does not appear in newIntents:
            issue Cancel

    for each intent in newIntents:
        if corresponding localOrder not exists:
            issue NewOrder
        else if price/qty differ too much:
            cancel + new
        else:
            do nothing
```

diff 是做市系统能否通过交易所风控的关键。

---

### **（4）执行风控校验**

每个 OrderIntent 必须先过 RiskControl：

```
allowed, reason = rc.Validate(intent)
if !allowed:
    reject
```

OMS 不允许绕过风控。

---

### **（5）聚合批量执行**

OMS 应按时间窗口（例如 10 ms）聚合 Intent：

- 合并多次 Cancel
    
- 限制 QPS
    
- 降低 REST 调用次数
    

---

# **4. ExchangeGateway 的核心职责**

### **（1）REST API 下单/撤单/改单**

典型 Binance 需要实现：

- POST `/order`
    
- DELETE `/order`
    
- GET `/openOrders`
    
- GET `/positionRisk`
    
- GET `/account`
    

要求：

- 自动添加 timestamp / recvWindow
    
- 自动签名（HMAC）
    
- 支持重试（退避）
    

---

### **（2）WS API 订单回报处理**

必须订阅：

- `USER_DATA_STREAM`
    
- `OrderUpdate`
    
- `TradeUpdate`
    

必须保证：

- 回报顺序（binance 内置 seq）
    
- 缺包自动补齐（主动拉取 OpenOrders 对账）
    
- 断开自动重连
    

---

### **（3）错误处理与限流（非常重要）**

处理常见错误：

|错误码|含义|行为|
|---|---|---|
|**-1021**|时间差错误|立即校时（NTP）|
|**429**|限流|降低 qps / 冷却几百 ms|
|**-2010**|下单失败|记录原因，拒单|
|**-2013**|找不到订单|强制与本地订单簿对账|
|**5xx**|交易所异常|重试 + fallback|

所有错误都必须进入 retry pipeline：

```
retry schedule: 0ms → 30ms → 100ms → 300ms → give up
```

并向风控上报 API 异常率。

---

# **5. 本地订单簿（Local Order Book）设计**

## **5.1 数据结构**

维护一个 map：

```
symbol → clientOrderID → LocalOrder
```

最核心能力：

- 状态一致性
    
- 可追溯性（写入 log）
    
- 回报幂等（多次回报也不改变一致性）
    

---

## **5.2 状态更新机制：三条来源**

BN 的订单状态更新来自三处：

### **来源 1：REST 下单成功的同步返回**

结果包括：

- orderId
    
- clientOrderId
    
- status
    

OMS 需立即更新本地订单对象。

---

### **来源 2：WS 成交/订单回报（异步）**

必须优先信任 WS 回报。  
例如服务器重启后通过 `listenKey` 恢复。

状态为：

- NEW
    
- PARTIALLY_FILLED
    
- FILLED
    
- CANCELED
    
- REPLACED
    

---

### **来源 3：定期对账（fallback reconciliation）**

每 10–60 秒使用：

```
GET /openOrders
GET /allOrders
```

确保本地状态与交易所一致。

必要性：

- WS 丢包
    
- listenKey 断开
    
- 程序崩溃重启
    

---

# **6. 下单流程（顺序图）**

```
StrategyEngine  
      |
      | OrderIntent[]  
      v
OrderManager  
      |
      | validate via RiskControl  
      v
ExchangeGateway (REST)
      |
      | order response  
      v
OrderManager  
      |
      | update local order book  
      v
WS 回报  
      |
      v
OrderManager  
      |
      v
更新最终状态（最终写入 LocalOrder）
```

---

# **7. 撤单流程（顺序图）**

```
Strategy → OMS → Gateway → Binance
                          ↓ WS
                     状态回报（Canceled）
                          ↓
                     OMS更新状态
```

**撤单是做市系统最敏感的环节，需要最强的稳定性。**

---

# **8. 异步回报处理（关键机制）**

### OrderUpdate 收到时：

```go
func OnOrderUpdate(event OrderEvent) {
    order := localBook[event.ClientOrderID]

    // 幂等更新
    if event.UpdateTime < order.UpdateTime {
        return
    }

    order.Status = event.Status
    order.FilledQty = event.FilledQty
    order.RemainingQty = order.Quantity - order.FilledQty
    order.UpdateTime = event.UpdateTime

    if event.Status == FILLED:
        removeFromOpenOrderList()
}
```

保证：

- 时间戳排序
    
- 不回退状态
    

---

# **9. 重试与错误处理**

### 情况 1：网络超时

→ 重试 0ms、30ms、100ms、300ms  
→ 超过上限 → 标记失败 → 风控介入

### 情况 2：REST 成功但未收到 WS 回报

→ 需在 `OpenOrders` 对账时修复

### 情况 3：WS 回报到达但 REST 返回错误

→ 优先以 WS 为准

---

# **10. 性能要求**

### **下单链路延迟**

（策略 → OMS → Gate → Binance）

目标：

- **5–15ms**  
    可接受：20ms  
    过高：> 50ms（需要优化）
    

### **并发**

- 支持每秒 30–50 次下单/撤单处理（策略自己会限频）
    

### **稳定性**

- WS 自动重连：< 500ms
    
- REST 重试：< 300ms
    

稳定性高于速度。

---

# **11. logging（日志要求）**

OMS 必须记录：

- 每条下单请求（intent→request）
    
- 每条撤单请求
    
- 每条 WS 回报
    
- 每次状态变化
    
- 每次对账行为
    
- 每次风控拒单
    
- 每次 API 错误
    

这是实盘复盘的核心数据。

---

# **12. 未来扩展能力（为阶段 3/4 做准备）**

无需修改架构即可支持：

- 多交易所网关（OKX / Bybit）
    
- 决策套利（跨交易所价差）
    
- 现货/永续对冲同步下单
    
- 更高密度网格
    
- 高频 cancel/replace（使用 ModifyOrder API）
    

你现在的架构已经是专业 HFT 的基础。

---

# 下一份文档

你现在已经有：

- 策略（阶段1/2）
    
- 风控
    
- OMS + Gateway
    

接下来应该写：

**《MarketDataService（行情服务）详细说明文档）》**

行情服务质量决定：

- 做市系统能否低延迟
    
- 是否能应对盘口变化
    
- 是否能产生精确的波动率、盘口不平衡指标
    

我可以继续写下一份吗？