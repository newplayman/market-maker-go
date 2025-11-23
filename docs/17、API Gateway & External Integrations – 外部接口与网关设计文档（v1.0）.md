
好的，继续为你撰写下一个关键模块文档——你整个做市系统与外部世界（交易所）的连接层。这部分决定系统的稳定性、延迟、异常处理能力，是做市商最容易“翻车”的地方之一。


API Gateway 是策略系统与交易所通信的统一入口。  
它负责：

- 高频稳健地与 Binance API / WebSocket 通信
    
- 隔离交易逻辑和网络细节
    
- 提供一致的接口（方便未来接 Bybit / OKX / 多交易所）
    
- 处理交易所的异常、限速、断线
    
- 提供高可用 WS 重连机制
    
- 保证 OMS（订单管理系统）始终拥有最新数据
    
- 多策略共用一个底层网关
    
- 提供本地 cache 与 snapshot
    

这一层设计得越专业，实盘越稳。

本篇文档将输出一个专业量化团队使用的 API Gateway 设计方案。

---

# **目录**

1. 模块目标
    
2. 为什么需要 API Gateway
    
3. 整体架构
    
4. 功能模块拆分
    
5. 与 Binance 的通信设计
    
6. REST 客户端设计
    
7. WebSocket 客户端设计
    
8. 限频（Rate Limit）设计
    
9. 重试机制（Retry Policy）
    
10. 行情订阅模型（MarketData Subscription）
    
11. 账户与仓位同步（Account Sync）
    
12. 下单、撤单接口（Trade API）
    
13. 错误处理与异常恢复
    
14. 统一接口（统一多交易所）
    
15. 扩展能力
    

---

# **1. 模块目标**

API Gateway 的目标：

- 让策略不用直接处理 Binance 的 API 细节
    
- 提供统一可靠的接口供 OMS、策略使用
    
- 实现高性能、稳健、低延迟交易
    
- 自动处理市场数据流
    
- 自恢复（重连、自动恢复 snapshot）
    
- 保持状态一致（account / orders / depth / trades）
    

一句话：

> **API Gateway 是整个做市系统的“交易所适配器”，决定外部稳定性和内部一致性。**

---

# **2. 为什么需要 API Gateway？**

不用 Gateway，策略会直接调用 Binance API，有很多问题：

1. 交易所 API 风格复杂（REST vs WS）
    
2. 多 API 数据不一致（snapshot vs stream）
    
3. WS 经常断线、卡顿、lag
    
4. REST 频限会导致策略完全失效
    
5. 订单事件处理分散 → bug 多
    
6. 下单、撤单需要复杂错误处理
    
7. 多交易所接入时极其痛苦
    
8. 策略需要简单 API，而不是复杂协议
    

所以必须引入专门的 API Gateway 把复杂清洗掉。

---

# **3. 整体架构**

API Gateway 建议拆分 4 层：

```
        +---------------------------+
        |       Strategy Layer      |
        +------------+--------------+
                     |
        +------------+--------------+
        |   OMS (Order Management)  |
        +------------+--------------+
                     |
        +------------+--------------+
        |         API Gateway       |
        |   (统一接口 & 状态管理)    |
        +------------+--------------+
                     |
           +---------+---------+
           | BINANCE REST API  |
           | BINANCE WebSocket |
           +-------------------+
```

Gateway 负责底层通信、重试、数据修正。  
OMS 和策略永远只调用：

- `gateway.PlaceOrder(...)`
    
- `gateway.CancelOrder(...)`
    
- `gateway.GetPosition()`
    
- `gateway.GetOrderBook()`
    

---

# **4. 功能模块拆分**

API Gateway 包括：

### **A. REST Client 模块**

用于：

- 下单
    
- 撤单
    
- 查询订单
    
- 获取账户余额
    
- 获取持仓
    

### **B. WebSocket Client 模块**

用于：

- 深度行情
    
- 成交行情
    
- 账户订单回报
    
- position 更新
    

### **C. 状态缓存（State Cache）**

缓存：

- 最新订单簿（OrderBook）
    
- 最新余额
    
- 最新仓位
    
- 最新持仓风险
    
- 当前挂单（pending orders）
    

### **D. 同步器（Syncer）**

在系统启动时/WS 断线后同步：

- 全量订单状态
    
- 当前持仓
    
- 当前余额
    

### **E. Rate Limit 模块**

管理 Binance 的限频。

### **F. Retry & Backoff 模块**

重试策略：

- 立即重试
    
- 指数退避
    
- 特殊错误分类处理
    

### **G. Snapshot 管理**

恢复 orderbook snapshot。

### **H. Event Dispatcher**

把 WS 事件转发给 OMS / Strategy。  
防止策略直接面向 Binance 数据。

---

# **5. 与 Binance 的通信设计**

Binance REST + WebSocket 存在一些“人尽皆知的问题”：

- WS 深度流与 REST snapshot 的时间可能不一致
    
- 部分回报事件可能延迟
    
- WS 会随机断线（你已观测到）
    
- WS 甚至会卡住不发数据
    
- API 有隐藏限频（未文档化）
    
- 某些错误不会清晰表达（如 -1021 时间偏差）
    
- 有时候返回超时但订单却成功了（要查补单）
    

你的 API Gateway 必须处理好这些。

---

# **6. REST 客户端设计**

### REST 客户端必须实现：

- retry（带 backoff）
    
- idempotent（支持幂等）
    
- 对 Binance 时间偏差自动校准
    
- 签名缓存
    
- HTTP 连接池
    
- 请求限频（自动排队）
    

示例（伪 Python）：

```go
resp, err := RetryRequest(maxTries=3, backoff=10ms, fn=func() {
    return httpClient.Do(req)
})
```

### 特殊错误分类：

|错误码|含义|应对|
|---|---|---|
|-1021|时间不同步|自动同步时间，加偏移重试|
|-2013|无此订单|执行补单检查|
|-2010|保证金不足|通知风控|
|429 或 418|限频|进入限频队列|

---

# **7. WebSocket 客户端设计**

WebSocket 是做市系统生死关键。

必须支持：

- 心跳（pong 检查）
    
- 自动重连
    
- 自动重新订阅（含 listenKey）
    
- 自动恢复 orderbook snapshot
    
- 延迟测量
    

### WebSocket 生命周期：

```
连接 → 订阅 → 接收数据 → 
ping/pong → 循环 → 断线 → 重连 → 恢复数据
```

Gateway 的 WS 模块必须具备：

### ✔ OnDisconnect → 全部撤单

（策略停顿期间不能大胆挂单）

### ✔ OnReconnect → 执行：

```
resubscribe
fetch snapshot
apply missed updates
resume strategy
```

### ✔ 对 depth 必须做“顺序校验”：

Binance 深度有 U（更新 ID），必须校验：

```
if update.U > snapshot.lastUpdateId:
    data inconsistent → drop & refetch snapshot
```

---

# **8. 限频（Rate Limit）设计**

Binance 有**显性限频**和**隐藏限频**。

你的 Gateway 必须统一管理。

### 方法：

### ① 维护一个令牌桶（Token Bucket）

例如最大 1200 / min。

### ② 每个请求前检查是否可发

不可发 → 等待 → 排队。

### ③ 限频通知 OMS

延迟过高时 OMS 会自动减频。

---

# **9. 重试机制（Retry Policy）**

重试必须分类处理：

### A. 网络超时 → retry

### B. Binance 系统繁忙 → retry

### C. 时间偏差错误 → 校准时间 → retry

### D. 限频错误 → sleep → retry

### E. 下单超时（未知订单状态）→ 查订单状态（补单）

补单逻辑非常关键：

```
send order → timeout → unknown result
→ query openOrders
→ if found → return success
→ else → retry send with new clientOrderId
```

---

# **10. 行情订阅模型（MarketData Subscription）**

Gateway 必须允许：

- 订阅多个 symbol
    
- 每个 symbol 分开 WS 通道
    
- 统一转发到 Strategy / MD Handler
    
- 保留本地最新 snapshot
    
- 支持多策略共享行情（未来扩展）
    

使用 Channel fan-out：

```
WS → Gateway → Channel → Strategy A
                         → Strategy B
```

---

# **11. 账户与仓位同步（Account Sync）**

在以下情况必须重新同步：

- 系统启动
    
- WS 断线
    
- listenKey 过期
    
- API reject（某些情况下）
    
- OMS 发现状态不一致
    

同步内容：

- 全量余额
    
- 全量持仓
    
- 全量挂单
    
- 资金费率
    
- 杠杆设置
    

Gateway 必须保证：

> OMS 和策略永远能读到一致的账户状态。

---

# **12. 下单、撤单接口（Trade API）**

Gateway 对策略提供统一接口：

```
PlaceOrder(symbol, side, price, qty, type)
CancelOrder(orderId)
CancelAll(symbol)
ReplaceOrder(...)
QueryOrder(...)
```

特点：

- 自动生成 clientOrderId
    
- 策略永远不知道 REST / WS 的状态
    
- OMS 统一路由
    

PlaceOrder 需要 latency 监控：

```
send_time = now
resp = rest.send()
latency = now - send_time
report to metrics
```

撤单（cancel）也必须监控。

---

# **13. 错误处理与异常恢复**

必须处理所有可能出现的严重错误：

### ✔ WS 数据乱序

自动丢弃 + 重新 snapshot

### ✔ 部分订单状态丢失

执行补单查询

### ✔ WS 卡住（无数据）

3 秒无 tick → 自动重连

### ✔ REST 突然全部超时

网关降频 + 进入安全模式

### ✔ listenKey 过期

自动刷新 → 自动 resubscribe

---

# **14. 统一接口（支持未来多交易所）**

你未来会接 OKX、Bybit，为此需要统一接口：

```
Exchange interface:
    SubscribeDepth(...)
    PlaceOrder(...)
    CancelOrder(...)
    QueryOrder(...)
    GetBalance(...)
    GetPosition(...)
```

每家交易所实现自己的适配器：

```
BinanceAdapter
OKXAdapter
BybitAdapter
```

策略不关心交易所。

---

# **15. 扩展能力**

未来可扩展：

- 底层使用 WebSocket expert 技术（gzip、压缩、批处理）
    
- 行情分发服务（MD Gateway 与 Trading Core 分离）
    
- 多交易所套利策略（Gateway 最重要）
    
- 统一 Order ID 标准化
    
- API 性能自动优化（无锁队列/zero-copy）
    
- 使用 L3 行情（纳秒级）
    

最终形成一个可扩展的：

> **交易所无关的低延迟、可靠、高性能交易网关。**

---

# **下一份文档**

这是基础设施的倒数第二部分：

> **《System Architecture Summary – 系统架构总览与依赖关系图（v1.0）》**

它会把你所有模块汇总，绘制完整架构图、依赖链、数据流图、控制流图、部署拓扑图，并给你最终的“做市商系统全景图”。

需要我继续？