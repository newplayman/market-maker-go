
适用于阶段 1 + 阶段 2 策略，可无缝扩展到阶段 3（专业做市）。

---

# 目录

1. 模块目的
    
2. 风控设计总原则
    
3. 风控总体架构
    
4. 风控输入数据
    
5. 风控核心逻辑分类
    
6. 订单级风控（Order-Level Risk）
    
7. 仓位级风控（Inventory / Position Risk）
    
8. 行情级风控（Market Condition Risk）
    
9. 行为级风控（Action Rate / Cancel Limit Risk）
    
10. 账户级风控（Account-Level Risk）
    
11. Panic-Stop / Emergency Stop
    
12. 对上层与下层的接口交互
    
13. 可观测性（监控指标）
    
14. 后续扩展点
    

---

# **1. 模块目的**

RiskControlService 是整个做市商系统最核心的安全组件，目标：

- 避免交易系统因异常行情、交易所风控、网络延迟、bug 造成重大亏损
    
- 保护账户不过度暴露仓位
    
- 保证订单行为合法合规，不触发交易所的限流或风控
    
- 在 panic 情况自动执行紧急停机与撤单
    

你可以把 RiskControlService 理解成：

> **策略的大脑 —— 的监护人。  
> 策略可以大胆，风控必须保守。**

---

# **2. 风控设计总原则**

- **最优先：活下来**  
    做市不是高风险策略，回撤应极小，异常情况必须迅速结束交易。
    
- **风控优先级高于策略**  
    策略输出的 OrderIntent 必须先通过风控才能提交。
    
- **风控分层，每层独立**  
    Order、Position、Market、Behavior、Account 五层互不干扰，各做各的事。
    
- **实时性要求高**  
    风控需要毫秒级决策，绝不能阻塞。
    
- **所有规则均可配置**  
    例如：
    
    - 最大撤单频率
        
    - 最大持仓
        
    - 波动率上限  
        等等必须通过配置调整以适应不同市场状态。
        

---

# **3. 风控总体架构**

```
              +------------------------------+
              |   StrategyEngine (Phase1/2)  |
              +------------------------------+
                             |
                             | OrderIntent[]
                             v
              +------------------------------+
              |   RiskControlService         |
              |  - Order Risk                |
              |  - Position Risk             |
              |  - Market Risk               |
              |  - Behavior Risk             |
              |  - Account Risk              |
              +------------------------------+
                             |
                             | ApprovedOrderIntent[]
                             v
              +------------------------------+
              |        OrderManager          |
              +------------------------------+
```

核心交互点：

- **策略 → 风控：Intent**
    
- **风控 → 订单管理：过滤后的 Intent**
    
- **风控提供状态（RiskSnapshot）给策略**  
    策略会基于 risk.CanTrade 决定是否继续挂单。
    

---

# **4. 风控输入数据**

RiskControlService 需要：

### （1）策略拟执行的 OrderIntent

来自 `StrategyEngine`。

### （2）当前真实账户信息

- 头寸（Position）
    
- 可用余额
    
- 未实现盈亏（uPnL）
    
- 账户权益（Equity）
    

### （3）当前市场信息

- 盘口价格
    
- 盘口深度
    
- 短期波动率
    
- Spread
    
- 成交量变化
    

### （4）订单行为信息

- 每秒下单数
    
- 每秒撤单数
    
- 被拒单数（交易所拒绝）
    

### （5）策略状态

- 当前 inventory
    
- 当前网格大小
    
- 最近成交情况
    

---

# **5. 风控核心逻辑分类**

风控分为五类：

|类别|含义|
|---|---|
|**订单级**|单笔订单是否合法、安全？|
|**仓位级**|当前头寸是否超限？|
|**行情级**|当前市场是否危险？|
|**行为级**|当前你的操作是否会触发交易所风控？|
|**账户级**|账户资金是否风险过高（回撤、杠杆率等）？|

我们逐个拆解。

---

# **6. 订单级风控（Order-Level Risk）**

针对每一条 OrderIntent 做验证：

### ① 检查订单类型是否合法

- 是否允许使用 Market Order？
    
- 阶段 1 / 阶段 2 中，默认仅允许：
    
    - Limit maker 单（主力）
        
    - Market 单仅允许用于对冲（inventory hedge）
        

### ② 检查订单价格是否合法（价格保护）

保护措施：

```go
if price < best_bid - MaxSlippageTicks*tick
    reject()

if price > best_ask + MaxSlippageTicks*tick
    reject()
```

避免策略误认为“盘口价”，实际下错价位，导致瞬间亏损。

### ③ 检查订单数量是否合法

- 单笔订单不能超过 `MaxOrderSize`
    
- 单笔订单金额不能超过 `MaxOrderNotional`
    

### ④ 检查订单重复 / “废单”

- 不允许重复挂已经存在的订单（价格相同且位置相同）
    

### ⑤ 检查订单方向是否导致超仓位

例如：

```go
if (new_order_side == Buy && position + order_qty > MaxInventory)
    reject()
```

---

# **7. 仓位级风控（Position Risk）**

### ① MaxInventory 限制（绝对上限）

```
PositionQty ∈ [-MaxInventory, MaxInventory]
```

超出 → 触发强制对冲模式（taker）。

### ② MaxDirectionalExposure（方向性风险上限）

例如：

```
max long exposure = 3% of equity
max short exposure = 3% of equity
```

### ③ AvgEntryPrice 偏离保护

如果持仓偏离当前价格过远（超过设定 %），强制减仓。

---

# **8. 行情级风控（Market Condition Risk）**

### ① 波动率超限停机

如果最近 3 秒波动率超过：

```
vol > baseVol * VolatilityPanicRatio
```

→ 立即：

- 全撤单
    
- 暂停挂单 X 毫秒
    
- 风控设置：CanTrade = false
    

### ② Spread 异常缩窄

如果 spread 低于 N tick：

- 取消全部挂单
    
- 暂停一段时间
    

### ③ 盘口异常（深度崩塌）

如果：

- 最优买单深度突然减少 80%+
    
- 最优卖单深度消失
    
- 出现巨大吃单
    

→ 停止挂单，等待恢复。

### ④ 急速趋势（微型闪崩）

短期趋势变化：

```
trend > TrendPanicThreshold
```

→ 不挂逆向订单。

---

# **9. 行为级风控（Action Risk）**

避免被交易所风控（非常重要）。

### ① 每秒下单次数限制

例如 token limit：

```
orders_per_second < 20
```

### ② 每秒撤单次数限制

币安对撤单占比过高账户会记分（罚分机制）。  
所以需要控制撤单率：

```
cancel_rate <= CancelRateLimit
```

### ③ API 错误率 / 被拒单率

如果出现：

- 429（限流）
    
- -1021（timestamp 错误）
    
- 5xx 交易所异常
    

达到阈值 → 进入：

- 全撤安全模式
    
- 暂停下单
    

### ④ 网格规模限制

避免策略疯狂扩展网格：

```
total_grid_notional <= MaxGridTotalSize
```

---

# **10. 账户级风控（Account-Level Risk）**

### ① 最大回撤限制（Daily Max Drawdown）

例如：

```
dailyLoss > MaxDailyLossPercent * equity
```

触发行为：

- 全撤单
    
- 平掉所有持仓（taker）
    
- 设置 CanTrade = false
    
- 推送告警
    

### ② 杠杆率 / 维持保证金风险

如果：

```
maintenance_margin / equity > 0.8
```

说明接近强平，必须：

- 减仓
    
- 停止挂单
    

### ③ 未实现亏损（uPnL）超标

```
|uPnL| > UpnlPanicRatio * equity
```

说明行情剧烈 → 暂停挂单。

---

# **11. Panic-Stop / Emergency Stop（紧急停机）**

这是风控最重要的功能。

触发条件（任一即可）：

- 回撤超过阈值
    
- inventory 超过数倍阈值
    
- volatility 超阈值
    
- MarketDataService 断开
    
- API 异常率超过上限
    
- 订单行为异常（短时间过多订单）
    
- 交易所风控信号（交易所拒单）
    

行为：

1. CancelAll()
    
2. MarketHedgeAll()（对冲清掉仓位）
    
3. RiskState.CanTrade = false
    
4. 上报告警
    
5. 等待人工介入或冷却时间
    

---

# **12. 对上层与下层的接口交互**

### （1）策略读取风控状态

RiskControl 提供：

```go
type RiskSnapshot struct {
    CanTrade             bool
    MaxPositionQty       float64
    MaxOrderNotional     float64
    DailyPnL             float64
    MaxDrawdownHit       bool
    VolatilityPanic      bool
    CancelRateHigh       bool
}
```

策略根据这些状态：

- 不挂单（CanTrade = false）
    
- 偏向对冲（Position 高时）
    
- 缩窄网格（波动高时）
    

---

### （2）订单管理 → 风控 → 交易所

订单请求流：

```
Strategy → RiskControl → OrderManager → ExchangeGateway → Binance
```

风控的核心函数：

```go
func (rc *RiskControl) Validate(intent OrderIntent, ctx RiskContext) (allowed bool, reason string)
```

批处理：

```go
func (rc *RiskControl) ValidateBatch(intents []OrderIntent, ctx RiskContext) ([]OrderIntent, []RejectedIntent)
```

---

# **13. 可观测性（监控）**

所有风控应暴露 Prometheus 指标（或 log 指标）：

- 每秒下单数 / 撤单数
    
- 被拒订单数
    
- 账户权益、保证金比例
    
- volatility
    
- imbalance
    
- 当前仓位
    
- inventory 临界预警
    
- panic stop 次数
    
- 每次拒单原因分类统计
    

这些信息用于调参与日常监控。

---

# **14. 后续扩展点**

未来阶段 3/4 需加入：

- 高频库存优化（Avellaneda–Stoikov γ 模型）
    
- 多交易所风险敞口
    
- 基差风险（现货/永续套利）
    
- 组合风险管理（Portfolio Risk）
    

这些都能与你目前架构无缝兼容。

---

# 下一步文档

如果你愿意，我可以继续完成下一份：

> **《OrderManager & ExchangeGateway – 下单网关与订单管理模块说明（v1.0）》**

这是策略能否执行成功的关键模块，  
也是你用 Go 写时“最容易踩坑”的部分（如异步回报不同步、状态不一致、撤单 race condition 等）。

你需要继续吗？