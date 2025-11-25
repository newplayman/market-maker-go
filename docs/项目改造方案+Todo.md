
一个"**项目升级总体蓝图 + 按重要程度排好序的细节化 TODO**"，完全围绕 `market-maker-go` 现有框架来设计。**
    

---

## 一、升级改造的总体目标

把当前 `market-maker-go` 从"参数化网格/简单做市骨架"升级为：

> **专业级 HF 做市引擎：  
> Avellaneda-Stoikov 风格核心 + 微观结构信号 + 订单流毒性过滤（VPIN）
> 
> - 动态 spread & spacing + delta-neutral inventory control + 自适应风控 + 事后学习反馈 + 完整监控。**
>     

核心落地点都集中在这些目录：

- `market/`：盘口快照、波动率、Imbalance、VPIN、Regime（市场状态）
    
- `strategy/`：新建 `asmm` 策略（AS 做市），引入动态 spread/spacing、多档挂单、Inventory Skew
    
- `inventory/` + `risk/`：仓位跟踪 + AdaptiveRiskManager（随波动/毒性/逆向选择自动收缩仓位）
    
- `posttrade/`（新建）：事后逆向选择分析 & 策略自适应调参
    
- `metrics/` 或现有 Prometheus 集成：把 VPIN、Regime、AdverseSelection 等打点
    
- `cmd/backtest` / `cmd/runner`：接入新策略 & 新风控 & 日志
    

---

## 二、模块级设计方案（高层）

### 1. strategy：新增 ASMM 策略（项目的"新大脑"）

**目标：**  
用一个 `strategy/asmm` 模块替代原始"网格思路"，核心行为：

- 每个时刻输出若干档 **bid/ask 报价**（连续做市而非死网格）
    
- spread & 间距随 **波动率/Regime** 变化
    
- 报价中心价随 **仓位（inventory skew）** 偏移，长期保持 delta-neutral
    
- 遇到 **高波动/高 VPIN** 自动拉宽 spread 或只减仓
    

建议接口（伪代码）：

```go
// strategy/asmm/types.go
type ASMMConfig struct {
    QuoteIntervalMs int
    MinSpreadBps    float64
    MaxSpreadBps    float64
    MinSpacingBps   float64
    MaxLevels       int
    BaseSize        float64
    SizeVolK        float64

    TargetPosition  float64
    InvSoftLimit    float64
    InvHardLimit    float64
    InvSkewK        float64

    VolK                   float64
    TrendSpreadMultiplier  float64
    HighVolSpreadMultiplier float64
    AvoidToxic             bool
}

type ASMMStrategy struct {
    cfg    ASMMConfig
    spreadAdj *VolatilitySpreadAdjuster
}

type Quote struct {
    Price      float64
    Size       float64
    Side       Side   // Bid / Ask
    ReduceOnly bool
}
```

策略核心逻辑：

1. 从 `market.Snapshot` 拿：`Mid`, `RealizedVol`, `Regime`, `VPIN/Toxic`, `Imbalance`
    
2. 从 `inventory.Position` 拿：`NetPosition`
    
3. 计算：
    
    - `reservationPrice`（mid + inventory skew）
        
    - `halfSpread`（用 VolatilitySpreadAdjuster + Regime 调整）
        
    - `spacingBps`（vol-adjusted spacing，多档挂单）
        
4. 根据 VPIN/Regime 决定是否只减仓、减少档数或直接不报价
    

---

### 2. market：微观结构信号 + VPIN + Regime

在 `market/` 上扩展现有的 `Snapshot`：

```go
type Snapshot struct {
    Mid         float64
    BestBid     float64
    BestAsk     float64
    Spread      float64

    BidVolumeTop float64
    AskVolumeTop float64
    Imbalance    float64   // (BidVol - AskVol)/(BidVol + AskVol)

    LastTrades   []TradeSummary

    RealizedVol  float64   // 短窗波动率
    Regime       Regime    // Calm / TrendUp / TrendDown / HighVolChaos

    VPIN         float64   // 订单流毒性
    Toxic        bool

    StalenessMs  int64
}
```

新增几个文件：

- `market/volatility.go`：滚动计算短窗 realized vol
    
- `market/imbalance.go`：用前 N 档盘口算 Imbalance
    
- `market/regime.go`：基于 vol + Imbalance + 价格偏离判定 Regime
    
- `market/toxicity.go`：
    
    - 用 aggTrade 流 + volume bucket 算 VPIN
        
    - 大于阈值时设置 `Toxic = true`
        

作用：

- Calm Regime + 低 VPIN：积极紧密报价、多档、较大 size
    
- Trend / HighVol / 高 VPIN：减少档数、拉宽 spread 或只减仓
    

---

### 3. risk + inventory：AdaptiveRisk + Inventory Control

利用现有 `risk.Manager` 的 `singleMax/netMax/pnlMin/shockPct` 基础上增强：

**新增：**

```go
// risk/adaptive.go
type AdaptiveRiskManager struct {
    base *Manager
    cfg  AdaptiveRiskConfig
}

type AdaptiveRiskConfig struct {
    BaseNetMax float64
    MinNetMax  float64
}

func (a *AdaptiveRiskManager) EffectiveNetMax(snap market.Snapshot, stats posttrade.Stats) float64 {
    netMax := a.cfg.BaseNetMax

    if snap.Regime == market.RegimeHighVol || snap.Toxic {
        netMax *= 0.4
    }
    if stats.AdverseSelectionRate > 0.6 {
        netMax *= 0.5
    }

    if netMax < a.cfg.MinNetMax {
        netMax = a.cfg.MinNetMax
    }
    return netMax
}
```

**策略层的 Inventory Skew：**

在 `ASMMStrategy` 内部：

```go
pos  := inv.NetPosition
soft := cfg.InvSoftLimit
ratio := clamp(pos/soft, -1, 1)

inventorySkewBps := cfg.InvSkewK * ratio * cfg.MinSpreadBps
centerPrice := snap.Mid * (1 + inventorySkewBps*1e-4)
```

inventorySkewBps := cfg.InvSkewK * ratio * cfg.MinSpreadBps
centerPrice := snap.Mid * (1 + inventorySkewBps*1e-4)
```

然后所有档位围绕 `centerPrice` 报价，而不是裸 mid。  
配合 `risk` 的 `reduceOnlyThreshold/netMax`，实现"双层"库存管理。

---

### 4. posttrade：逆向选择分析（事后学习）

新增 `posttrade/` 包：

```go
// posttrade/analyzer.go
type FillRecord struct {
    FillPrice    float64
    FillTime     time.Time
    Side         string
    PriceAfter1s float64
    PriceAfter5s float64
}

type Analyzer struct {
    fills map[string]*FillRecord
}

func (a *Analyzer) OnFill(orderId string, price float64, side string) { ... }
func (a *Analyzer) trackPriceMovement(orderId string) { ... } // 1s/5s 后记录 mid

func (a *Analyzer) Stats() Stats {
    // 统计逆向选择率、平均 slippage 等
}
```

`Stats` 中有：

- `AdverseSelectionRate`
    
- `AvgPnL1s` / `AvgPnL5s`
    

`strategy/asmm` & `risk/AdaptiveRisk` 定期读取 `Stats`，做自适应调整：

- 逆向选择率太高 → 自动增大 spread / 减小 size / 降低 netMax
    
- 逆向选择率恢复正常 → 慢慢恢复原配置
    

这是一个简单但很有用的"自我反省机制"。

---

### 5. metrics / 监控：Prometheus 指标

在现有 Prometheus 集成（如果已有）上增加：

- `mm_vpin_current`
    
- `mm_volatility_regime`（0/1/2/3）
    
- `mm_adverse_selection_rate`
    
- `mm_inventory_net`
    
- `mm_reservation_price`
    
- `mm_inventory_skew_bps`
    
- `mm_adaptive_netmax`
    

这样你能在 Grafana 实时看到：

- 现在市场状态如何？
    
- VPIN 是否飙升？
    
- 策略是否自动缩手？
    
- 被人吃亏的频率如何？
    

---

### 6. config：扩展 `config.yaml` 支持新策略与新参数

在 `symbols.<symbol>.strategy` 下支持：

```yaml
symbols:
  ETHUSDC:
    ...
    strategy:
      type: asmm
      quoteIntervalMs: 150
      minSpreadBps: 6
      maxSpreadBps: 40
      minSpacingBps: 4
      maxLevels: 3
      baseSize: 0.01
      sizeVolK: 0.5

      targetPosition: 0
      invSoftLimit: 3
      invHardLimit: 5
      invSkewK: 1.5

      volK: 0.8
      trendSpreadMultiplier: 1.5
      highVolSpreadMultiplier: 2.0
      avoidToxic: true

    risk:
      singleMax: 1
      dailyMax: 10
      netMax: 5
      reduceOnlyThreshold: 3
      pnlMin: -5
      pnlMax: 10
      stopLoss: -20
      haltSeconds: 30
      shockPct: 0.02
```

---

### 7. backtest / runner：支持新策略与新指标

- `cmd/backtest`：
    
    - 接入 `ASMMStrategy` + `market.Snapshot` 扩展字段（vol/imbalance/VPIN 可从历史数据离线计算或简化）
        
    - 输出回测报告：PnL、回撤、仓位分布、逆向选择率
        
- `cmd/runner`：
    
    - 通过 `type: asmm` 选择新策略
        
    - 注入 `market`, `inventory`, `risk.AdaptiveRiskManager`, `posttrade.Analyzer`
        
    - 暂时可以在日志或 Prometheus 中输出关键指标
        

---

## 三、按优先级 + 顺序的细节化 TODO List

### ✅ Phase 0：基线整理（1 天）

1. **TODO 0.1**：
    
    - 在本地拉取最新 `market-maker-go`
        
    - 跑通当前 `cmd/backtest` & `cmd/runner -dryRun` 确认基线
        
2. **TODO 0.2**：
    
    - 整理当前 `market/`, `strategy/`, `risk/`, `inventory/` 的核心结构
        
    - 写一份 `docs/ARCHITECTURE.md`，只要简单模块关系（方便后面 AI 对话用）
        

---

### ✅ 🥇 Phase 1（最高优先级）：ASMM 策略 + 动态 spread/spacing（约 3–5 天）

1. **TODO 1.1**：
    
    - 在 `strategy/` 新增目录 `asmm/`
        
    - 创建 `config.go`, `strategy.go`, `types.go` 文件
        
    - 定义 `ASMMConfig`, `ASMMStrategy`, `Quote` 结构体和构造函数
        
2. **TODO 1.2**（动态 spread）
    
    - 仿照"可行性评估"里提到的 `VolatilitySpreadAdjuster`：
        
        - 新建 `strategy/dynamic_spread.go` 或放入 `asmm/spread.go`
            
        - 实现 `GetHalfSpread(vol, regime)`：
            
            - `baseSpread` + `volK * vol`
                
            - clip 到 `[minSpread, maxSpread]`
                
            - 不同 Regime 乘不同 multiplier
                
3. **TODO 1.3**（vol-adjusted spacing + 多档报价）
    
    - 在 `ASMMStrategy.Quote` 中：
        
        - 根据 `RealizedVol` 计算 `spacingBps`
            
        - 根据 `MaxLevels` 循环生成多档报价
            
        - 注意用 tickSize 对齐价格
            
4. **TODO 1.4**（与现有主流程接线）
    
    - 在策略工厂处（通常在某个 `strategy/factory.go` 或 `cmd/runner`）：
        
        - 支持 `type: asmm` 从 config 解析 `ASMMConfig` 并返回策略实例
            

---

### ✅ 🥈 Phase 2：市场波动率 & Imbalance & Regime（约 3–4 天）

1. **TODO 2.1**（realized vol）
    
    - 新建 `market/volatility.go`：
        
        - 用滚动窗口存 mid 价
            
        - 计算短窗 log-return 的标准差作为 `RealizedVol`
            
        - 输出到 `Snapshot.RealizedVol`
            
2. **TODO 2.2**（Imbalance）
    
    - 新建 `market/imbalance.go`：
        
        - 从订单簿前 N 档汇总买卖量
            
        - 计算 `(BidVol - AskVol)/(BidVol + AskVol)`，填入 `Snapshot.Imbalance`
            
3. **TODO 2.3**（Regime 判定）
    
    - 新建 `market/regime.go`：
        
        - 根据 `RealizedVol`, `Imbalance`, `短期价格偏离` 定义简单规则
            
        - 设置 `RegimeCalm / RegimeTrendUp / RegimeTrendDown / RegimeHighVol`
            
    - 在 `ASMMStrategy` 中根据 Regime：
        
        - 调整 `spreadMultiplier` / 档数 / 是否 conservative
            

---

### ✅ 🥉 Phase 3：Inventory skew + 基础风控对接（约 2–3 天）

1. **TODO 3.1**（策略层 inventory skew）
    
    - 在 `ASMMStrategy` 中加入：
        
        - `InvSoftLimit`, `InvHardLimit`, `InvSkewK`
            
        - 根据 `NetPosition` 计算 `inventorySkewBps`，偏移中心价
            
2. **TODO 3.2**（与风险配置对齐）
    
    - 在 `risk.Manager` 中确认：
        
        - 已有 `netMax`, `reduceOnlyThreshold` 是否在 config 可配
            
    - 在 `ASMMStrategy` 的报价中：
        
        - 当 `|pos| >= InvHardLimit` → 不再生成会增加仓位的单
            
        - 当 `|pos| >= reduceOnlyThreshold` → 将 `Quote.ReduceOnly = true`，只允许减仓
            
3. **TODO 3.3**（回测验证）
    
    - 用简单 mid price 序列（平盘/单边）在 `cmd/backtest` 模式下：
        
        - 验证仓位是否在软硬限制附近自动收敛
            
        - 验证单边行情下不会无限加仓
            

---

### ✅ Phase 4：VPIN 毒性检测 + High Toxic 行为（约 4–5 天）

1. **TODO 4.1**（VPIN 计算）
    
    - 新建 `market/toxicity.go`：
        
        - 在接收 aggTrade 时维护 volume buckets
            
        - 统计买卖 volume 差异，计算 VPIN（可以先用简化版）
            
2. **TODO 4.2**（Toxic 标志）
    
    - 在 config 中添加 VPIN 阈值：
        
        - `vpinThresholdHigh`
            
    - 当 VPIN 超过阈值：
        
        - 在 `Snapshot` 中设置 `Toxic = true`
            
3. **TODO 4.3**（策略对毒性的响应）
    
    - `ASMMConfig.AvoidToxic = true` 时：
        
        - `Toxic == true` →
            
            - 只挂减仓单
                
            - 或 spread 加大一倍
                
            - 或短暂冷却（例如不报价 N 秒）
                
4. **TODO 4.4**（结合 Regime）
    
    - 高 VPIN + HighVol Regime 同时出现时：
        
        - 直接交给 `risk` 模块触发更小的 `netMax` 或 halt
            

---

### ✅ Phase 5：PostTrade Analyzer + 自适应风控（约 4–6 天）

1. **TODO 5.1**（posttrade 包）
    
    - 新建 `posttrade/analyzer.go`：
        
        - `OnFill` 记录成交价、时间、方向
            
        - 用 goroutine 在 1s、5s 后从 `market` 取 mid 记录
            
        - 统计 adverse selection（成交后价格向你不利方向走的比例）
            
2. **TODO 5.2**（与 AdaptiveRiskManager 整合）
    
    - 在 `risk/adaptive.go` 中接入 `posttrade.Stats`：
        
        - `AdverseSelectionRate` > 阈值 → 缩小 `EffectiveNetMax`
            
        - 同时让 `ASMMStrategy` 提高 `MinSpreadBps` 或减小 `BaseSize`
            
3. **TODO 5.3**（回测 & 仿真验证）
    
    - 在 `cmd/backtest` 加入简单市场模型，模拟：
        
        - 某一段时间内故意"恶意对手盘" → 看策略是否自动变保守
            
        - 正常期 → 能否恢复到较积极状态
            

---

### ✅ Phase 6：监控指标 & 可视化（约 2–4 天）

1. **TODO 6.1**：
    
    - 在 metrics 模块（或新增 `metrics/prometheus.go`）里注册：
        
        - VPIN / Regime / AdverseSelectionRate / NetPosition / ReservationPrice / AdaptiveNetMax 等
            
2. **TODO 6.2**：
    
    - 更新 `cmd/runner`：
        
        - 周期性采集并输出这些指标
            
3. **TODO 6.3**（可选）：
    
    - 编写一份 `docs/monitoring.md`：
        
        - 给出 Grafana dashboard layout 建议（几个面板、各监控曲线）
            

---

### ✅ Phase 7：文档 & 清理（约 2 天）

1. **TODO 7.1**：
    
    - 更新 `README`：
        
        - 增加 `asmm` 策略介绍
            
        - 增加配置样例
            
2. **TODO 7.2**：
    
    - 新增 `docs/strategy_asmm.md`：
        
        - 说明理论来源（AS 模型）、主要参数如何调、风险点在哪
            
3. **TODO 7.3**：
    
    - 新增 `docs/roadmap.md`，记录已完成阶段 & 下一步优化点（例如将来接入 RL、更多交易所等）
        

---

