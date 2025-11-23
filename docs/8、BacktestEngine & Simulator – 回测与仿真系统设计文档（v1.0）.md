

做市策略如果没有高质量的回测与仿真系统，就无法安全上线实盘。  
这是整个“轻量化做市商系统”的最终基础层之一。

此文档会定义：

- 回测引擎架构
    
- 数据格式
    
- 仿真撮合模型
    
- 策略与回测的交互
    
- 如何模拟挂单、撤单、滑点
    
- 如何模拟 latency（延迟）
    
- 如何模拟 inventory、风控
    

本设计可直接用于真实可运行的回测平台。

---

# **目录**

1. 模块目的
    
2. 为什么做市策略必须有回测系统
    
3. 系统架构与模块关系
    
4. 数据源要求
    
5. 撮合模型设计（核心）
    
6. 回测模式：K线 / 盘口 / 成交 驱动
    
7. 仿真模式（实时模拟）
    
8. 延迟模型
    
9. 订单生命周期模拟
    
10. Inventory 与资金计算
    
11. 风控模拟
    
12. 指标输出（PnL、Sharpe、fill rate 等）
    
13. 多 symbol、多策略回测
    
14. 系统接口设计（Go 示例）
    
15. 可扩展能力
    

---

# **1. 模块目的**

BacktestEngine（离线回测）+ Simulator（实时仿真）主要作用：

- **在实盘前验证策略逻辑是否正确**
    
- **评估策略收益与回撤**
    
- **评估不同参数对结果的影响**
    
- **模拟真实撮合，寻找策略在极端行情下的漏洞**
    
- **调试 OMS / 风控 / Inventory 模块**
    
- **训练未来 AI 多模型决策结构（你的项目需要）**
    

本质是：

> 构建一个“虚拟交易所”，让策略在其中跑，  
> 得到非常接近真实世界的成交行为。

---

# **2. 为什么做市策略必须有回测系统？**

因为做市策略的难点不是“预测价格”，  
而是：

- 挂单是否会被吃？
    
- 会吃多少？
    
- 什么行情会导致 inventory 爆炸？
    
- 动态 spread 是否合理？
    
- 参数是否过拟合？
    

没有高质量回测 → 实盘很快爆仓。

---

# **3. 系统架构（推荐）**

```
+-----------------------------+  
|       BacktestEngine       |
|   - DataLoader             |
|   - MatchingEngine         |
|   - SimulatorClock         |
|   - ExecutionLogger        |
+-------------+---------------+
              |
              v
+-----------------------------+
|      StrategyEngine         |
|  (Phase1 / Phase2 / Future) |
+-------------+---------------+
              |
              v
+-----------------------------+
| Local OMS (Simulated OMS)  |
+-----------------------------+
              |
              v
+-----------------------------+
| InventoryManager (sim)     |
| RiskControl (sim)          |
+-----------------------------+
```

**特点：策略层完全不需要修改代码即可接入回测引擎。**

这是专业 HFT/做市系统的标准架构。

---

# **4. 数据源要求**

要模拟做市，需要**高质量盘口数据**，不是 K 线。

优先级：

### **1）真实 Orderbook 深度（Level 1 ~ Level 10）**

来源：

- Binance Futures Historical Depth
    
- 流（手动录制）
    

### **2）真实成交数据（aggTrade）**

用于：

- 成交方向（taker vs maker）
    
- 成交量大小
    
- 价格变动速度与趋势
    

### **3）如无深度数据，至少需要：**

- Best bid/ask 历史序列
    
- Spread 分布
    
- Trade tick
    

否则无法测试 Phase2 网格策略。

---

# **5. 撮合模型设计（核心）**

回测器的核心就是：**一个真实交易所的撮合引擎模型**

你的系统需要**价格驱动撮合**模型，模拟：

- 价格变化
    
- 挂单是否吃到
    
- 吃单顺序
    
- 挂单剩余数量
    
- 撤单延迟
    

---

## **5.1 撮合规则：Maker/Taker 逻辑**

做市商是 Maker：

- 你挂单 → 如果市场价格触达你的价格 → 你的挂单会被成交
    

撮合规则：

### **买单撮合条件：**

```
trade.price <= limitBuy.price
```

### **卖单撮合条件：**

```
trade.price >= limitSell.price
```

**成交量按真实成交 tick 来模拟。**

---

## **5.2 分配成交量（按成交量比例分配）**

真实交易所按价格优先、时间优先匹配。  
但在回测中无法模拟“整个市场所有挂单”，  
所以需要：

### 两种模型可选：

### **模型 A：比例成交（推荐）**

```
你的挂单成交量 = (你的挂单在该价位的量 / 盘口总量) * trade.qty
```

优点：

- 完全可控
    
- 模拟正确
    
- 不会过拟合
    
- 适用于做市策略
    

### **模型 B：优先成交（简单）**

如果 trade.qty >= 你的挂单 qty → 全成交  
否则 → 部分成交

适用于粗略测试，但不够真实。

---

# **6. 回测驱动模式**

推荐支持 3 种模式：

---

## **6.1 盘口驱动（最真实）**

事件序列：

```
深度变化事件 → 撮合 → 调用策略 → OMS → inventory
```

适用于做市策略最佳（强推）。

---

## **6.2 成交驱动（tick-level）**

事件序列：

```
trade event → 撮合 → 调用策略
```

用于模拟高频趋势检查。

---

## **6.3 K线驱动（低精度）**

仅用于压力测试，不适合做市。

---

# **7. 仿真模式（实时模拟）**

你的系统需要一个“实时模拟交易所”环境：

```
实时行情（WebSocket） → Simulator → 策略 → Simulated OMS → 仿真成交
```

可应用于：

- 实盘前演练
    
- 低资金测试
    
- 新策略测试
    
- 多模型 AI 的 debate 模拟环境
    

Simulator 在实时仿真下：

- 使用真实 depth WS
    
- 但不去交易所下单
    
- 而是本地模拟成交
    
- 输出与实盘几乎相同的成交回报
    

这是专业做市商标配。

---

# **8. 延迟模型（Latancy Simulation）**

延迟是回测必须模拟的因素之一。

可以配置多个延迟参数：

```
StrategyLatencyMs         // 策略计算耗时
OMSLatencyMs              // OMS → Gateway 耗时
NetworkLatencyMs          // 交易所网络延迟
ExchangeMatchDelayMs      // 成交回报延迟
```

建议分段模拟：

### 下单流程延迟：

```
t_send = now + OMSLatency + NetworkLatency
```

### 撤单延迟：

```
t_cancel_effective = t_cancel + latency
```

### 成交回报延迟：

```
t_trade_event = real_trade_time + ExchangeMatchDelayMs
```

这些延迟决定策略是否在快行情中“接飞刀”，  
非常关键。

---

# **9. 订单生命周期模拟（关键）**

订单状态必须完全仿照真实 OMS + Exchange：

```
NEW → PARTIAL_FILLED → FILLED
       |                  ^
       +----→ CANCELED ---+
```

Simulated OMS 必须支持：

- New
    
- Cancel
    
- Replace
    
- Partial fill
    

### 示例：

```
订单挂出 → 50 ms 后被吃掉 10% → 策略看到部分成交 → 第二次决策 → 剩余部分被取消
```

必须严格模拟 timing。

---

# **10. Inventory 与资金计算**

Inventory 模块在模拟器中与实盘一致（见上一份文档）。

资金计算：

```
uPnL = PositionQty * (mark_price - entry_price)
realizedPnL += 处理成交的PnL
equity = balance + uPnL
```

整理：

- 成交费用（挂单免费、吃单收费）
    
- 资金费率（funding）
    
- 强平（模拟风险）
    

此部分必须等同于 Binance 规则。

---

# **11. 风控模拟**

Simulator 应复用相同的 RiskControl 模块（不修改一行代码）。

回测期间：

- 若达到 MaxDailyLoss → 停止策略
    
- 若超过 MaxInventory → 自动对冲（仿真）
    
- 若 trade error rate 过高 → 出发 panicstop（仿真）
    

**回测必须使用与实盘完全相同的风控代码**，避免偏差。

---

# **12. 指标输出（核心性能指标）**

模拟器应导出：

### 绩效：

- 总盈亏（PnL）
    
- 年化收益
    
- 最大回撤
    
- Sharpe 比率
    
- Sortino 比率
    

### 做市商特有指标：

- Maker Fill Rate（被动成交比例）
    
- Spread Capture 分布
    
- Inventory Risk（最大 inventory）
    
- Cancel/Order Ratio
    
- 每层网格成交情况（phase2）
    
- 多空成交比例（buy fills / sell fills）
    

### 行为数据：

- 平均延迟
    
- 平均每秒下单数
    
- 平均每秒撤单数
    

这是策略调参必备。

---

# **13. 多 symbol、多策略支持**

BacktestEngine 必须支持：

```
for each symbol:
    load symbol-specific data
    run strategy
```

以及：

- 多策略对同一个符号执行 A/B 测试
    
- 多策略竞争同一套 MarketData（训练 AI debate）
    

未来你可以：

- 同一交易对，多个权重策略同时运行
    
- 让 AI 策略互相对抗（强化学习环境）
    

---

# **14. 系统接口设计（Go 示例）**

### **主入口**

```go
type BacktestEngine struct {
    Loader       DataLoader
    Matcher      MatchingEngine
    Simulator    SimulatorClock
    Strategy     StrategyEngine
    OMS          SimulatedOMS
    Inventory    InventoryManager
    Risk         RiskControl
}
```

### **执行函数**

```go
func (bt *BacktestEngine) Run(start, end time.Time) BacktestResult
```

---

# **15. 扩展能力**

未来可扩展：

- GPU 加速回测（用历史 tick 训练 AI）
    
- 大规模参数扫描（数万个组合）
    
- 蒙特卡洛模拟
    
- 高频市场冲击模型（根据挂单深度调整成交概率）
    
- 多市场数据同步（跨交易所交易）
    
- 全量逐笔（tick-by-tick L3）回测
    

此设计已经为你未来构建更高级的 **AI 做市系统** 预备好了全部基础设施。

---

# 下一份文档

按顺序，下一份是：

> **《Monitoring & Alerting – 监控与告警模块设计文档（v1.0）》**

用于保障实盘稳定运行，是实现稳定盈利的核心环节。

我继续为你写这一份吗？