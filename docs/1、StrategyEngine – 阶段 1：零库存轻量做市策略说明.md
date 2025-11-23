

你可以直接拿这个给 codex / 程序员做实现，后面阶段 2 会在这个基础上增强。

---

## 1. 文档目的与范围

**目的**

- 定义“阶段 1：零库存轻量做市”策略的业务逻辑与接口。
    
- 让开发者可以据此实现 `Phase1Strategy` 模块，而不产生歧义。
    
- 为后续阶段 2、专业做市策略留出扩展空间。
    

**范围**

- 仅包含“单 symbol、零库存目标”的轻量做市逻辑。
    
- 只假设一个交易所（Binance USDC 合约），后续多交易所通过上层调度扩展。
    
- 不包含：
    
    - 回测逻辑
        
    - 多品种资产间协同（portfolio 层）
        

---

## 2. 策略目标与原则

### 2.1 目标

1. **核心收益来源**：捕捉盘口天然价差（spread），依托挂单 0 手续费优势。
    
2. **风险控制目标**：策略本身尽量不暴露方向性风险，保持 `inventory ≈ 0`。
    
3. **实现复杂度**：逻辑尽量简单，便于验证、调优和监控。
    

### 2.2 原则

- 不做中长期预测，只做超短期流动性提供者。
    
- 单笔下单量小，频率可高，但需要服从风险模块和交易所限频。
    
- 一旦价格波动异常或盘口异常，优先保护资金安全 → 立即撤单 / 暂停挂单。
    

---

## 3. 模块定位与对外接口

### 3.1 在整体架构中的位置

- 模块名称建议：`Phase1Strategy`（或 `StrategyPhase1`）
    
- 上层调用者：`StrategyEngine`
    
- 依赖模块：
    
    - `MarketDataService`
        
    - `InventoryManager`
        
    - `RiskControlService`
        
    - `OrderManager`
        
    - `ConfigService`
        

### 3.2 对外核心接口（建议）

用伪 Go 结构说明接口签名（可以调整）：

```go
type Phase1Strategy interface {
    // 初始化策略，加载参数
    Init(config StrategyConfig, symbol string) error

    // 每次行情或成交事件触发时调用
    OnTick(ctx Context) ([]OrderIntent, error)

    // 成交 / 订单状态变更回调（可选拆分）
    OnTradeUpdate(trade TradeEvent)
    OnOrderUpdate(order OrderEvent)
}
```

其中：

- `Context` 由上层封装，包括：
    
    - 当前行情快照（best bid/ask、spread、最近成交等）
        
    - 当前 inventory（从 InventoryManager 获取）
        
    - 当前风险状态（RiskControl 提供的只读信息）
        
    - 当前已有挂单列表（从 OrderManager / 本地缓存获取）
        
- `OrderIntent` 是“策略意图层”的订单指令，还没真正发到交易所：
    

```go
type OrderIntent struct {
    Symbol      string
    Side        Side        // Buy / Sell
    Price       float64
    Quantity    float64
    Type        OrderType   // Limit / Market
    Action      ActionType  // New / Cancel / Replace
    ClientOrderID string    // 用于和当前挂单匹配
    Meta        map[string]interface{} // 可选：策略内部标记
}
```

---

## 4. 输入数据与内部状态

### 4.1 策略依赖的输入（来自 Context）

必需字段（建议）：

```go
type MarketSnapshot struct {
    Symbol       string
    BestBid      float64
    BestBidSize  float64
    BestAsk      float64
    BestAskSize  float64
    LastPrice    float64
    MarkPrice    float64
    Timestamp    time.Time
}

type StrategyContext struct {
    Market       MarketSnapshot
    Inventory    InventoryInfo
    OpenOrders   []OrderInfo         // 当前本策略挂在盘口上的订单
    RiskState    RiskSnapshot        // 只读，标识是否允许下单等
    Config       StrategyConfig      // 当前生效策略配置
}
```

`InventoryInfo` 示例：

```go
type InventoryInfo struct {
    PositionQty      float64 // >0 多，<0 空
    PositionValue    float64 // 按 markPrice 估算
    AvgEntryPrice    float64
}
```

`RiskSnapshot` 可以是：

```go
type RiskSnapshot struct {
    CanTrade         bool    // 全局是否允许交易
    MaxPositionQty   float64
    MaxOrderNotional float64
    DailyPnL         float64
    MaxDrawdownHit   bool
}
```

### 4.2 策略内部状态（Strategy 内部维护）

例如：

```go
type Phase1State struct {
    LastDecisionTime time.Time
    LastBidOrderID   string
    LastAskOrderID   string

    // 用于简单风控节流
    RecentOrderCount int
    RecentCancelCount int

    // 可选：记录最近一次挂单价，用于避免频繁改单
    LastBidPrice     float64
    LastAskPrice     float64
}
```

---

## 5. 参数配置（StrategyConfig）

### 5.1 核心参数

建议结构：

```go
type StrategyConfig struct {
    Symbol                string

    // 基本挂单参数
    BaseOrderSize         float64   // 每次基础下单数量（合约张数或币）
    MaxOrderSize          float64   // 单笔最大下单
    MinSpreadTicks        int       // 最小挂单价差（以 tick 数计）
    QuoteOffsetTicks      int       // 相对 best bid/ask 的偏移 tick 数

    // 频率控制
    MinRequoteIntervalMs  int       // 最小重报价间隔（避免频繁改单）
    MaxOrderPerSecond     int       // 本策略下单节流
    MaxCancelPerSecond    int       // 撤单节流

    // inventory 控制
    MaxInventory          float64   // 允许的最大净仓（绝对值），阶段1可设非常小
    UseTakerToHedge       bool      // 是否允许用吃单平仓
    MaxTakerHedgeQty      float64   // 单次吃单最大数量

    // 触发暂停条件
    VolatilityThreshold   float64   // 超过则暂停挂单（简单波动率指标）
    SpreadTooNarrowTicks  int       // spread 小于该值时不挂单
}
```

### 5.2 参数含义示例

- `MinSpreadTicks`：  
    若盘口当前价差（`best_ask - best_bid` 换算成 tick 数）小于该值，则不挂单（没有足够利润空间覆盖 taker fee/风险）。
    
- `QuoteOffsetTicks`：  
    挂单价不直接贴在最佳价，而是稍微向内或向外偏移。例如：
    
    - bid: `best_bid - QuoteOffsetTicks * tickSize`
        
    - ask: `best_ask + QuoteOffsetTicks * tickSize`
        
- `MaxInventory`：  
    阶段 1 推荐设为一个很小的值，例如占用总资金 1% 以内对应的币数。
    

---

## 6. 核心决策逻辑（自然语言 + 伪代码）

### 6.1 总体流程（OnTick）

每次行情更新 / 成交回报触发时，大致流程：

1. 读取行情快照 `MarketSnapshot`。
    
2. 基础检查：
    
    - risk.CanTrade 是否为 true。
        
    - 当前 spread 是否足够大。
        
3. 检查当前 inventory 是否超出容忍范围：
    
    - 若超出，优先平仓（使用 taker 或调整挂单方向）。
        
4. 根据策略规则计算目标挂单价位与数量：
    
    - 目标 bid（price & qty）
        
    - 目标 ask（price & qty）
        
5. 对比当前已有挂单，生成：
    
    - 需要撤销的订单列表
        
    - 需要新建的订单列表
        
6. 返回 `[]OrderIntent` 给上层，由 `OrderManager` 执行。
    

### 6.2 伪代码示例

> 注意：这是**策略意图层**伪代码，不包含风控的再次过滤。

```go
func (s *Phase1StrategyImpl) OnTick(ctx StrategyContext) ([]OrderIntent, error) {
    intents := []OrderIntent{}

    m := ctx.Market
    inv := ctx.Inventory
    cfg := ctx.Config
    now := time.Now()

    // 1. 基础风控 & 环境检查
    if !ctx.RiskState.CanTrade {
        // 全撤
        for _, o := range ctx.OpenOrders {
            intents = append(intents, CancelIntent(o))
        }
        return intents, nil
    }

    spread := m.BestAsk - m.BestBid
    spreadTicks := spread / TickSize(m.Symbol)
    if spreadTicks < float64(cfg.SpreadTooNarrowTicks) {
        // spread 太小，不挂单，撤掉已有
        for _, o := range ctx.OpenOrders {
            intents = append(intents, CancelIntent(o))
        }
        return intents, nil
    }

    // 2. Inventory 检查
    if math.Abs(inv.PositionQty) > cfg.MaxInventory {
        // 超库存，优先平仓
        hedgeIntents := s.generateHedgeIntents(ctx)
        // 这里可以选择先 return，或者继续有限挂单
        intents = append(intents, hedgeIntents...)
        return intents, nil
    }

    // 3. Requote 控制（避免频繁改单）
    if now.Sub(s.state.LastDecisionTime) < time.Duration(cfg.MinRequoteIntervalMs)*time.Millisecond {
        return intents, nil // 不做任何更改
    }
    s.state.LastDecisionTime = now

    // 4. 计算目标挂单价位
    tickSize := TickSize(m.Symbol)

    targetBidPrice := m.BestBid - float64(cfg.QuoteOffsetTicks)*tickSize
    targetAskPrice := m.BestAsk + float64(cfg.QuoteOffsetTicks)*tickSize

    // 四舍五入到 tick
    targetBidPrice = RoundToTick(targetBidPrice, tickSize)
    targetAskPrice = RoundToTick(targetAskPrice, tickSize)

    // 5. 计算下单数量（可简单用 BaseOrderSize）
    bidQty := cfg.BaseOrderSize
    askQty := cfg.BaseOrderSize

    // 6. 根据当前已有挂单进行 diff 计算（避免无谓撤单）
    // 这里建议有一个小的价格差阈值，低于阈值不改单
    priceTolerance := 0.5 * tickSize

    needNewBid := true
    needNewAsk := true

    for _, o := range ctx.OpenOrders {
        switch o.Side {
        case Buy:
            if math.Abs(o.Price - targetBidPrice) <= priceTolerance {
                // 价格足够接近，不动
                needNewBid = false
            } else {
                intents = append(intents, CancelIntent(o))
            }
        case Sell:
            if math.Abs(o.Price - targetAskPrice) <= priceTolerance {
                needNewAsk = false
            } else {
                intents = append(intents, CancelIntent(o))
            }
        }
    }

    // 7. 若需要新挂单，则生成订单意图
    if needNewBid {
        intents = append(intents, NewLimitIntent(
            m.Symbol, Buy, targetBidPrice, bidQty,
        ))
    }
    if needNewAsk {
        intents = append(intents, NewLimitIntent(
            m.Symbol, Sell, targetAskPrice, askQty,
        ))
    }

    return intents, nil
}
```

---

## 7. Inventory 处理与 Taker 平仓逻辑

### 7.1 策略原则

- 阶段 1 的核心是 **“不持方向性仓位”**。
    
- 因此允许两种方式控制 inventory：
    
    1. 调整挂单方向（偏向 inventory 相反方向）
        
    2. 在超出阈值时使用 taker 单立即减仓
        

### 7.2 简单实现思路

```go
func (s *Phase1StrategyImpl) generateHedgeIntents(ctx StrategyContext) []OrderIntent {
    intents := []OrderIntent{}
    inv := ctx.Inventory
    cfg := ctx.Config
    m := ctx.Market

    hedgeQty := math.Min(math.Abs(inv.PositionQty), cfg.MaxTakerHedgeQty)
    if hedgeQty <= 0 {
        return intents
    }

    if inv.PositionQty > 0 {
        // 当前多头超限，用市价卖出平仓
        intents = append(intents, NewMarketIntent(
            m.Symbol, Sell, hedgeQty,
        ))
    } else {
        // 当前空头超限，用市价买入平仓
        intents = append(intents, NewMarketIntent(
            m.Symbol, Buy, hedgeQty,
        ))
    }

    // 可选：同时撤掉所有挂单，避免对冲中再次被成交
    for _, o := range ctx.OpenOrders {
        intents = append(intents, CancelIntent(o))
    }

    return intents
}
```

---

## 8. 异常行情与暂停逻辑

### 8.1 spread 太小

- 当 `spreadTicks < SpreadTooNarrowTicks` 时，不挂任何单，撤掉所有旧单。
    

### 8.2 波动率异常

- `MarketDataService` 可以提供简单波动率指标，如过去 N 秒价格标准差。
    
- 若 `vol > VolatilityThreshold`：
    
    - 暂停新挂单一段时间（例如 1–5 秒）。
        
    - 保守处理：撤掉旧单。
        

### 8.3 交易所 / 网络异常

- 当 ExchangeGateway 报错率异常：
    
    - RiskControl 设置 `CanTrade=false`。
        
    - 策略看到后不再挂新单，仅撤单。
        

---

## 9. 与风险控制、订单管理协作约定（简要）

1. **策略不直接调用交易所 API**，只返回 `OrderIntent`。
    
2. **RiskControlService** 在 OrderManager 执行前再做一次校验：
    
    - 超过最大下单资金 / 持仓上限 → 拒绝执行该 Intent。
        
3. **OrderManager** 负责：
    
    - Intent → 实际 REST / WS 下单指令。
        
    - 保证订单状态与本地缓存一致。
        
4. **策略对自己的行为负责**：
    
    - 避免每 tick 都全撤重挂。
        
    - 控制自身的节流，如 `MinRequoteIntervalMs`。
        

---

## 10. 可观测性与日志要求（针对阶段 1）

策略模块至少需要输出以下关键日志或指标（方便你以后调参）：

- 每次决策：
    
    - 当前 `best_bid / best_ask / spread`。
        
    - 当前持仓 `inventory`。
        
    - 目标挂单价格、数量。
        
- 每次 hedge（用 taker）：
    
    - hedge 方向、数量、价格、对冲前后持仓。
        
- 每次暂停挂单的原因：
    
    - spread 太小？
        
    - volatility 太高？
        
    - 风控禁止交易？
        

这些信息建议：

- 同时写入文件日志（方便复盘）。
    
- 部分关键指标做成监控面板（之后我们在监控文档里再细化）。
    

---

如果这份阶段 1 策略说明你觉得清晰，我们下一份就可以写：

> **《StrategyEngine – 阶段 2：动态网格 / 动态 spread 做市策略说明》**

那一份会在当前接口不变的前提下，增加：

- 多层挂单（L1-LN 网格）
    
- 动态 spread（随波动率调整）
    
- 盘口不平衡（orderbook imbalance）对策略的影响
    

你要是确认“OK，继续阶段2”，我就直接写下一份。