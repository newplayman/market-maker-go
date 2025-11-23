

Inventory（库存 / 持仓）是做市商的核心风险来源之一。  
InventoryManager 负责实时、准确、严格地管理持仓状态，  
确保策略不会因为“被动积累方向性仓位”而在行情异动时产生巨大亏损。

此模块对阶段 1（零库存）和阶段 2（动态库存偏移）都至关重要，  
也是构建阶段 3/4（专业做市）模型的基础。

---

# **目录**

1. 模块目标
    
2. 核心设计理念
    
3. 输入与输出
    
4. 数据结构
    
5. 持仓更新流程
    
6. Inventory 风险判断
    
7. 对冲与库存调整建议接口
    
8. Inventory 对策略的偏移影响
    
9. 多币种、多策略支持
    
10. 与风控 & OMS 的协作
    
11. 可观测性（监控指标）
    
12. 扩展能力
    

---

# **1. 模块目标**

InventoryManager 的主要目标：

1. **实时维护当前持仓（Position）和策略内部库存（Inventory）**
    
2. **统一对外提供持仓信息，保证状态一致**
    
3. **监控持仓风险**（反向挂单、taker 对冲等）
    
4. **为策略提供库存方向偏移数据**（用于智能做市）
    
5. **支持强制对冲的机制**（配合 RiskControl）
    

本质：

> InventoryManager 是“持仓真相源”。  
> 所有策略、风控、OMS 都必须依赖它才能运行正确。

---

# **2. 核心设计理念**

Inventory 管理按照以下思想设计：

### **（1）持仓 = 交易所仓位 + 本地挂单未完成部分**

对做市商来说，未成交挂单也具有方向性风险，因此：

```
EffectiveInventory = RealPositionQty + PendingMakerExposure
```

### **（2）绝不能依赖策略或 OMS 自己维护仓位**

因为：

- 交易所可能拒单
    
- WS 回报可能延迟
    
- 程序可能重启
    
- 有部分成交未同步
    

InventoryManager 必须定期对账，确保一致性。

### **（3）统一格式，不暴露交易所内部细节**

对策略暴露统一结构，屏蔽交易所 API 格式。

---

# **3. 输入与输出**

## 输入（来自系统各层）

1. **TradeUpdate（成交事件）**
    
    - 来自 WS
        
    - 是持仓变化最重要来源
        
2. **OrderUpdate（订单部分成交）**
    
    - 来自 WS
        
    - 用于更新 Pending Exposure
        
3. **REST 对账数据**
    
    - positionRisk
        
    - openOrders（补齐遗漏）
        

---

## 输出（提供给策略与风控）

InventoryManager 对外提供统一接口：

```go
type InventoryInfo struct {
    PositionQty      float64 // 实际持仓（合约张数）
    AvgEntryPrice    float64 // 持仓成本
    PositionValue    float64 // 估值（含 mark price）
    
    // 未成交挂单的方向性风险（非常重要）
    PendingBuyQty    float64 
    PendingSellQty   float64

    EffectiveInventory float64 // 总净库存（Position + Pending）
}
```

策略和风控都使用 `EffectiveInventory` 进行判断。

---

# **4. 数据结构**

内部维护两类数据：

---

## （1）真实持仓（Real Position）

来自交易所的 positionRisk：

```go
type RealPosition struct {
    Symbol     string
    Qty        float64   // >0 多仓，<0 空仓
    EntryPrice float64
}
```

---

## （2）挂单方向性敞口（Pending Exposure）

做市商挂着大量 maker 单时，**未成交量会突然被吃掉**，  
造成 inventory 飙升，而此时策略来不及撤单，就会巨大亏损。

所以必须追踪：

```go
type PendingExposure struct {
    PendingBuyQty  float64 // 所有 Buy 限价单剩余可成交量
    PendingSellQty float64 // 所有 Sell 限价单剩余可成交量
}
```

---

## （3）综合结构

```go
type InventoryState struct {
    RealPosition     RealPosition
    PendingExposure  PendingExposure

    EffectiveInventory float64  // RealPosition.Qty + PendingExposure.Buy - PendingExposure.Sell

    AvgEntryPrice float64
    LastUpdate    time.Time
}
```

---

# **5. 持仓更新流程**

InventoryManager 由三个来源更新：

---

## **（1）OnTradeUpdate（成交事件）– 最核心**

来自 ws 的 trade 回报：

```go
func OnTradeUpdate(trade TradeEvent) {
    // 根据 side 调整 RealPosition.Qty
    if trade.Side == Buy {
        position.Qty += trade.FilledQty
    } else {
        position.Qty -= trade.FilledQty
    }

    // 更新平均持仓价
    updateAvgEntryPrice(...)
}
```

---

## **（2）OnOrderUpdate（订单部分成交 / 撤单）**

来自订单事件：

```go
func OnOrderUpdate(order OrderEvent) {
    // 调整 PendingExposure
    if order.Side == Buy {
        exposure.PendingBuyQty = calcRemainingBuyQty()
    } else {
        exposure.PendingSellQty = calcRemainingSellQty()
    }
}
```

策略挂了 10 单，每单 10 张，这就是 100 张 exposure。

---

## **（3）定期对账（REST）**

每 10~60 秒：

- GET /positionRisk → 修复 RealPosition
    
- GET /openOrders → 修复 PendingExposure
    

这些用于修复 WS 丢包导致的差异。

---

# **6. Inventory 风险判断**

暴露给风控：

```go
type InventoryRisk struct {
    OverLimit         bool
    OverLimitFactor   float64
    ShouldHedge       bool
}
```

判断逻辑：

```
if abs(EffectiveInventory) > MaxInventory:
        OverLimit = true
        ShouldHedge = true
```

对阶段 1：

```
MaxInventory = 很小（例如资金 1%）
```

对阶段 2：

```
MaxInventory = 可以扩张，例如 3–10 倍 baseOrderSize
```

---

# **7. 对冲与库存调整建议（策略辅助接口）**

非常关键：  
InventoryManager **不自己下单，而是给策略“建议”如何对冲**。

提供接口：

```go
type HedgeAction struct {
    NeedHedge bool
    HedgeSide Side
    HedgeQty  float64
    Reason    string
}

func (im *InventoryManager) SuggestHedge() HedgeAction
```

实现示例：

```
if EffectiveInventory > MaxInventory:
    return {NeedHedge: true, HedgeSide: Sell, Qty: EffectiveInventory - MaxInventory}

if EffectiveInventory < -MaxInventory:
    return {NeedHedge: true, HedgeSide: Buy, Qty: ...}
```

策略收到后在下一次 OnTick 做：

- 立即使用 taker 限量对冲
    
- 或缩边挂单（减少单侧挂单）
    

---

# **8. Inventory 对策略的偏移影响（阶段 2 使用）**

策略根据库存大小偏移报价，例如：

```
bidBase -= inv(Qty) * InventoryBiasFactor
askBase += inv(Qty) * InventoryBiasFactor
```

InventoryManager 必须严格提供：

- 精确的 `EffectiveInventory`
    
- L1~L3 网格每层的 pendingExposure
    

策略根据 exposure 自动调节挂单才能真正“智能化”。

---

# **9. 多币种、多策略支持**

InventoryManager 必须支持：

- 多 symbol（BTCUSDC, ETHUSDC...）
    
- 每个 symbol 独立状态
    
- 每个策略（phase1/2/3）共享同一真实 position
    

建议数据结构：

```
symbol → InventoryState
```

策略运行在 symbol 层面，不会冲突。

---

# **10. 与风险控制 & OMS 的协作**

## OMS → InventoryManager

订单事件更新 pending：

```
OnOrderUpdate()
OnTradeUpdate()
```

## 风控 → InventoryManager

风控会查询：

- EffectiveInventory
    
- 是否超限
    
- Hedge 建议
    

直接影响策略是否继续挂单。

## 策略 → InventoryManager（读）

策略每次需要：

- PositionQty
    
- PendingExposure
    
- EffectiveInventory
    
- AvgEntryPrice
    

不允许写。

---

# **11. 可观测性（监控指标）**

应该输出：

|指标|含义|
|---|---|
|inventory_effective|当前 effective inventory|
|inventory_real|真实持仓|
|inventory_pending_buy|未成交买单敞口|
|inventory_pending_sell|未成交卖单敞口|
|inventory_over_limit_total|超限次数|
|inventory_hedge_count|自动对冲次数|
|avg_entry_price|平均仓位成本|

特别监控：

- 若 pending exposure 不断变大 → 说明订单撤单失败或 diff 出错
    
- 若 EffectiveInventory 频繁突破 max → 策略参数不足或行情剧烈
    

---

# **12. 扩展能力**

未来可扩展：

- **Gamma / Vega 风险管理**（适用于期权做市）
    
- **Inventory 的动态限制**（根据 volatility 自动调整）
    
- **Inventory 多策略分摊**
    
- **跨交易所 inventory 对冲**（Binance 对 OKX）
    
- **基差套利 inventory 管理（现货 + 永续）**
    

InventoryManager 是专业做市中非常关键的模块，  
你的设计已经即将到达交易所 MM（official market maker）级别。

---

# **下一份文档建议**

现在我们已经完成：

✔ 阶段 1 策略  
✔ 阶段 2 策略  
✔ 风控  
✔ OMS & Gateway  
✔ 行情服务  
✔ Inventory 管理

接下来可以写：

> **《ConfigService 配置系统 + 参数管理（允许热更新）说明》**  
> 或  
> **《BacktestEngine & Simulator（回测与仿真系统）设计文档》**  
> 或  
> **《Monitoring & Alerting（监控告警体系）说明》**

你想继续哪一份？