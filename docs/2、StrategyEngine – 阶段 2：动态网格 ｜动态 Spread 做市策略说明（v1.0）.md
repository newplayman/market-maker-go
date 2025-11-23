


这是阶段 1 的增强版，所有接口保持一致，只增加“智能化”和“多层挂单”能力。

---

# 目录

1. 文档目的与范围
    
2. 策略设计理念
    
3. 模块定位与接口
    
4. 输入数据与新增指标
    
5. 参数配置（新增参数）
    
6. 核心策略逻辑（自然语言 + 伪代码）
    
7. 网格挂单层级设计
    
8. 动态 Spread 计算
    
9. 盘口不平衡（Orderbook Imbalance）逻辑
    
10. Inventory（库存）偏移策略
    
11. 异常行情的特殊处理
    
12. 可观测性与调参指标
    
13. 与阶段 1 的差异总结
    

---

# **1. 文档目的与范围**

**目的：**

- 定义阶段 2 策略（智能化轻量做市）的逻辑，让开发者能基于阶段 1 版本扩展。
    
- 支持更高成交率、更稳定收益、更低 inventory 风险。
    

**范围：**

- 单交易对（symbol），后续可扩展到多交易对调度。
    
- 使用 Binance USDC 合约行情（盘口 + trade 事件）。
    
- 数据来源：MarketDataService、InventoryManager、RiskService 等。
    

---

# **2. 策略设计理念**

阶段 2 与阶段 1 的区别：  
**阶段 1：单挂单 + 靠 spread 边缘做无风险剥头皮**  
**阶段 2：智能挂单 + 多层挂单 + 对行情反馈**

主要增强点：

1. **动态 Spread 调整**（随波动率变化）
    
2. **多层挂单（网格）**
    
3. **盘口不平衡（OB imblance）驱动决策**
    
4. **库存偏移与柔性目标**
    
5. **微趋势检测与避免逆势挂单**
    
6. **异常行情快速撤退机制**
    

简言之：

> 阶段 2 = “轻量做市 + 智能网格 + 风险自适应”

---

# **3. 模块定位与接口**

与阶段 1 完全一致：

```go
type Phase2Strategy interface {
    Init(config StrategyConfig, symbol string) error  
    OnTick(ctx StrategyContext) ([]OrderIntent, error)
    OnTradeUpdate(trade TradeEvent)
    OnOrderUpdate(order OrderEvent)
}
```

阶段 2 只是增强策略决策层逻辑，不改变系统架构。

---

# **4. 输入数据与新增指标**

阶段 2 除阶段 1 输入外，需要 MarketDataService 提供：

---

## **4.1 新增必要指标**

### ① 短周期波动率（Rolling Volatility）

例如用最近 1–5 秒的 mid price 标准差：

```
vol = StdDev(mid_price_t, t in last 3 seconds)
```

### ② 盘口不平衡（Orderbook Imbalance）

用于判断市场力量偏向：

```
imbalance = (BidDepth(0~N) - AskDepth(0~N)) / (BidDepth + AskDepth)
```

范围：`[-1, +1]`。  
接近 +1 → 买盘强  
接近 -1 → 卖盘强

### ③ Short-term trend（微趋势）

例如：

```
price_slope = (mid_price_now - mid_price_200ms_before)
```

为避免逆势挂单。

### ④ Spread 历史均值 + 分布

用于判断当前 spread 是否“正常”还是“异常宽/窄”。

---

# **5. 参数配置（新增参数）**

在阶段 1 基础上新增如下字段：

```go
type StrategyConfig struct {

    // =========  继承阶段1 =========
    BaseOrderSize
    MinSpreadTicks
    QuoteOffsetTicks
    MaxInventory
    ...

    // =========  阶段2新增 =========

    // 网格层级
    GridLevels            int     // 挂多少层，例如 2~5
    GridStepTicks         int     // 每层间隔多少 tick
    SizeMultiplier        float64 // 每层挂单量递增比例（1.0=同量）

    // 动态 spread 调整
    VolatilityFactor      float64 // 波动率越高，spread 越宽
    MinDynamicSpreadTicks int     // 动态spread下限
    MaxDynamicSpreadTicks int     // 动态spread上限

    // 盘口不平衡偏移
    ImbalanceBiasFactor   float64 // 偏移 mid 价格
    ImbalanceThreshold    float64 // 超过这个不平衡值启动偏移

    // 微趋势控制
    TrendAvoidThreshold   float64 // 避免逆势挂单的幅度（每秒变化率）
    TrendPauseMs          int     // 避免逆势期的暂停时间

    // 风控增强
    VolatilityPanicRatio  float64 // vol 超多少倍时全撤单
    MaxGridTotalSize      float64 // 所有网格挂单的总最大规模

}
```

---

# **6. 核心策略逻辑（阶段 2 总流程）**

> 注意：此流程是阶段 1 的超集。

---

# **6.1 逻辑概览（自然语言版本）**

每次 OnTick：

1. **基础检查（同阶段 1）**
    
    - risk.CanTrade?
        
    - spread 是否足够？
        
2. **计算动态 spread**
    
    - 使用 `vol * VolatilityFactor`
        
    - 限制在 `[MinDynamicSpreadTicks, MaxDynamicSpreadTicks]`
        
3. **应用盘口不平衡偏移**
    
    - imbalance > threshold → 抬高 bid，抬高 ask（倾向做多）
        
    - imbalance < -threshold → 压低 ask，压低 bid（倾向做空）
        
4. **微趋势规避**
    
    - 若短期价格快速上行 → 暂停挂大量 ask
        
    - 若短期价格快速下行 → 暂停挂大量 bid
        
5. **库存偏移**
    
    - inventory > 0 → bid 较小、ask 较多
        
    - inventory < 0 → ask 较小、bid 较多
        
6. **生成网格**
    
    - 根据 dynamic spread 决定第一层 bid/ask
        
    - 再按 GridLevels 生成第二层、第三层、…
        
    - 遵循 GridStepTicks
        
    - 各层数量按 SizeMultiplier 增长或保持
        
7. **与现有挂单 diff**
    
    - 保留价格相近的单子
        
    - 修改或撤销过时的单子
        
8. **限制总挂单规模**
    
    - 单层、单方向、总网格金额均需受上限限制
        
9. **生成 OrderIntent 列表并返回**
    

---

# **6.2 阶段 2 OnTick 伪代码**

这里给完整伪代码：

```go
func (s *Phase2Strategy) OnTick(ctx StrategyContext) ([]OrderIntent, error) {
    intents := []OrderIntent{}
    m := ctx.Market
    cfg := ctx.Config
    inv := ctx.Inventory

    // 0. 基础风控
    if !ctx.RiskState.CanTrade {
        return CancelAll(ctx.OpenOrders), nil
    }

    // 当前 spread 若太窄，不挂单
    spread := m.BestAsk - m.BestBid
    if spread < float64(cfg.MinSpreadTicks)*TickSize(m.Symbol) {
        return CancelAll(ctx.OpenOrders), nil
    }

    // 1. 计算动态 spread
    vol := GetShortVol(ctx)
    dynamicSpreadTicks := int(vol * cfg.VolatilityFactor)
    dynamicSpreadTicks = Clamp(dynamicSpreadTicks,
        cfg.MinDynamicSpreadTicks, cfg.MaxDynamicSpreadTicks)

    tickSize := TickSize(m.Symbol)
    baseOffset := float64(dynamicSpreadTicks) * tickSize

    bidBase := m.BestBid - baseOffset
    askBase := m.BestAsk + baseOffset

    // 2. 盘口不平衡偏移
    imbalance := CalcOrderbookImbalance(ctx)
    if math.Abs(imbalance) > cfg.ImbalanceThreshold {
        bias := imbalance * cfg.ImbalanceBiasFactor * tickSize
        bidBase += bias
        askBase += bias
    }

    // 3. 微趋势控制
    trend := CalcShortTrend(ctx) // mid_now - mid_Nms_before
    if trend > cfg.TrendAvoidThreshold {       // 快速涨
        // 暂停挂 sell
        askBase = math.NaN()    // 禁用 ask 挂单
    }
    if trend < -cfg.TrendAvoidThreshold {      // 快速跌
        // 暂停挂 buy
        bidBase = math.NaN()
    }

    // 4. Inventory 偏移
    if inv.PositionQty > 0 {
        // 多头 → 少挂 bid，多挂 ask
        bidBase -= tickSize * 2
        askBase += tickSize * 1
    } else if inv.PositionQty < 0 {
        // 空头 → 少挂 ask，多挂 bid
        askBase += tickSize * 2
        bidBase -= tickSize * 1
    }

    // 5. 生成网格层级
    newOrders := []OrderIntent{}
    totalSize := 0.0

    for level := 0; level < cfg.GridLevels; level++ {
        levelOffsetTicks := cfg.GridStepTicks * level
        offset := float64(levelOffsetTicks) * tickSize

        // BUY 网格
        if !math.IsNaN(bidBase) {
            price := RoundToTick(bidBase - offset, tickSize)
            qty := cfg.BaseOrderSize * math.Pow(cfg.SizeMultiplier, float64(level))
            totalSize += qty
            if totalSize <= cfg.MaxGridTotalSize {
                newOrders = append(newOrders, NewLimitIntent(
                    m.Symbol, Buy, price, qty,
                ))
            }
        }

        // SELL 网格
        if !math.IsNaN(askBase) {
            price := RoundToTick(askBase + offset, tickSize)
            qty := cfg.BaseOrderSize * math.Pow(cfg.SizeMultiplier, float64(level))
            totalSize += qty
            if totalSize <= cfg.MaxGridTotalSize {
                newOrders = append(newOrders, NewLimitIntent(
                    m.Symbol, Sell, price, qty,
                ))
            }
        }
    }

    // 6. 与当前挂单 diff
    diffIntents := DiffOrders(ctx.OpenOrders, newOrders)
    intents = append(intents, diffIntents...)

    return intents, nil
}
```

---

# **7. 网格挂单层级设计**

### Example（示例）：

假设：

```
GridLevels = 3
GridStepTicks = 15
BaseOffsetTicks = 10
SizeMultiplier = 1.2
```

则：

|层级|买单价格|买单量|卖单价格|卖单量|
|---|---|---|---|---|
|L1|bidBase (−10 ticks)|1.0|askBase (+10 ticks)|1.0|
|L2|bidBase − 15 ticks|1.2|askBase + 15 ticks|1.2|
|L3|bidBase − 30 ticks|1.44|askBase + 30 ticks|1.44|

总挂单量受 `MaxGridTotalSize` 限制。

---

# **8. 动态 Spread 计算**

### 基础公式：

```
dynamicSpreadTicks = clamp(volatility * VolatilityFactor)
```

volatility 常见算法：

```
vol = StdDev(mid_price over last 3 seconds) / TickSize
```

贝塔参数调节控制“智力程度”：

- 波动高 → 挂得更宽
    
- 波动低 → 挂得更窄
    

这是做市商最核心逻辑之一。

---

# **9. 盘口不平衡（OB Imbalance）逻辑**

定义：

```
imbalance = (bid_depth - ask_depth) / (bid_depth + ask_depth)
```

用于价格偏移：

- 买盘强 → bid 与 ask 向上偏移
    
- 卖盘强 → 两侧向下偏移
    

这是“对冲 inventory + 提高成交率”的灵魂。

---

# **10. Inventory（库存）偏移策略**

阶段 2 的 key：

```
大方向：inventory > 0 → 优先卖单
        inventory < 0 → 优先买单
```

可做如下：

- bidBase -= 2 ticks * sign(+)
    
- askBase += 1 tick * sign(+)
    

按情况偏移 1–3 ticks，抵消库存积累倾向。

---

# **11. 异常行情处理**

|情况|行为|
|---|---|
|波动率 5 秒内暴涨 3 倍|全撤单 + 暂停 1 秒|
|高频爆量成交（鲸鱼）|暂停双向挂单|
|盘口瞬间被打穿（闪崩）|停止挂 bid，暂停 2 秒|
|交易所 WS 断开|立即撤单，禁挂单|
|inventory 超出 max 倍数|强制 taker 对冲|

阶段 2 停单逻辑比阶段 1 更严格，因为网格挂单量更多。

---

# **12. 可观测性与调参指标**

阶段 2 建议新增以下监控指标：

- 实时报价：每层网格价格与间距
    
- 成交分布：各层成交情况（L1/L2/L3…）
    
- spread 分布 vs 动态 spread 参数
    
- volatility vs 动态 spread
    
- imbalance vs inventory 偏移效果
    
- 被动成交（maker fill）比例
    
- 被逆势吃单（被动亏损）的比例（关键指标）
    

这些指标对调参极其关键。

---

# **13. 阶段 1 vs 阶段 2 差异总结**

|项目|阶段 1|阶段 2|
|---|---|---|
|是否多层挂单|❌|✔ 多层网格|
|Spread 模型|固定|动态（基于波动）|
|是否考虑 orderbook imbalance|❌|✔|
|是否考虑 Trend（微趋势）|❌|✔|
|库存管理|保持 0|容忍偏移，智能反向挂单|
|对异常行情的适应|低|高|
|预期收益|40%–120%|80%–150%|

这是轻量化做市商到智能化做市商的最重要一步。

---

# 下一步

如果你确认“阶段 2 策略文档 OK”，我们可以继续写下一份：

> **《RiskControlService – 风控模块详细说明》**

风控会直接影响做市商是否能在大行情中活下来，是 **关键模块**。

你要不要继续？