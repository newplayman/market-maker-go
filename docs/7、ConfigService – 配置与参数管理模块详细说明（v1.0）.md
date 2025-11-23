

这是做市系统中极其重要的一层，因为：

- 策略参数必须**可配置**
    
- 风控阈值必须**可动态调整**
    
- 不同市场、不同交易对需要**不同参数组合**
    
- 以后要支持“**热更新参数/在线调参**”
    
- 模型与程序员必须从**同一个参数源**读取，避免分歧
    

ConfigService 就是整个系统的 **统一参数中枢**。

---

# 《ConfigService – 配置与参数管理模块详细说明（v1.0）》

# 目录

1. 模块目的
    
2. 配置类型分类
    
3. 配置加载路径（优先级）
    
4. 配置结构设计
    
5. 参数热更新机制
    
6. 适用于 Phase1/Phase2/风控/OMS 的参数集
    
7. 多 symbol 配置
    
8. 错误处理与回滚机制
    
9. 监控与版本管理
    
10. 扩展能力
    

---

# **1. 模块目的**

ConfigService 的目标是：

- 提供统一的配置输入源
    
- 屏蔽底层存储（YAML、JSON、env、DB）
    
- 在运行时为所有模块提供参数
    
- 支持热更新（无重启）
    
- 支持不同 symbol 不同参数
    
- 支持不同策略版本的参数模板（未来可能有 Phase3/Phase4）
    

本质上是：

> 做市系统的“参数调度中心”，通过它可以快速调参、切换运行模式、精细化控制风险和策略行为。

---

# **2. 配置类型分类**

整体参数分为 5 类：

|类型|模块|示例|
|---|---|---|
|**系统级配置**|全局系统|日志级别、WS URL、API key|
|**symbol 级配置**|每个交易对|tick size、step size、资金投入量|
|**策略级配置**|phase 1/2|spread、网格层数、趋势阈值|
|**风控级配置**|RiskControl|daily max loss、撤单上限|
|**执行层参数**|OMS/Gateway|retry 策略、rate limit|

ConfigService 必须支持按此分类加载和合并。

---

# **3. 配置加载路径（优先级体系）**

ConfigService 采用“**多级覆盖**”体系（类似 Kubernetes config）：

```
默认参数（default config）  
  → 系统级配置文件（system.yaml）  
    → 交易对配置（symbols/btc.yaml）  
      → 策略配置（strategy_phase1.yaml / phase2.yaml）  
        → 环境变量（ENV override）  
          → 热更新（live override）  
```

优先级从低到高（高覆盖低）：

```
default < system < symbol < strategy < env < live_override
```

这样你可以方便地：

- 快速在线改参数
    
- 一键调整所有交易对
    
- 为某个 symbol 配置特殊值（如 BTC 波动大，ETH 波动小）
    

---

# **4. 配置结构设计**

建议采用 YAML，但 Go 内部使用 struct。

### **顶层结构（Go struct 示例）：**

```go
type Config struct {
    System      SystemConfig
    Exchange    ExchangeConfig
    Symbols     map[string]SymbolConfig
}
```

---

## （1）SystemConfig（系统级）

```go
type SystemConfig struct {
    LogLevel       string
    Environment    string // dev/test/prod
    HeartbeatMs    int
    EnableProfiling bool
}
```

---

## (2) ExchangeConfig（交易所相关）

```go
type ExchangeConfig struct {
    ApiKey             string
    SecretKey          string
    BaseUrl            string
    WsUrl              string

    // Rate limits
    MaxOrdersPerSec    int
    MaxCancelsPerSec   int

    RecvWindow         int
    RetryPolicy        RetryPolicyConfig
}
```

---

## (3) SymbolConfig（每个 symbol 独立）

```go
type SymbolConfig struct {
    Symbol            string
    TickSize          float64
    StepSize          float64
    MinNotional       float64

    StrategyPhase     string // phase1 or phase2
    Strategy          StrategyConfig  // 嵌套策略参数
    Risk              RiskConfig      // 嵌套风控参数
    Funds             FundsConfig     // 本交易对资金规模
}
```

---

## (4) StrategyConfig（Phase1/2 通用）

此结构已在策略文档出现，这里归档：

```go
type StrategyConfig struct {
    // Phase 1/2 公共参数
    BaseOrderSize         float64
    MaxOrderSize          float64
    MinSpreadTicks        int
    QuoteOffsetTicks      int
    MaxInventory          float64

    // ----- Phase 2 增强参数 ------
    GridLevels            int
    GridStepTicks         int
    SizeMultiplier        float64

    VolatilityFactor      float64
    MinDynamicSpreadTicks int
    MaxDynamicSpreadTicks int

    ImbalanceBiasFactor   float64
    ImbalanceThreshold    float64

    TrendAvoidThreshold   float64
    TrendPauseMs          int

    VolatilityPanicRatio  float64
    MaxGridTotalSize      float64
}
```

---

## (5) RiskConfig（风控参数）

对应风控文档：

```go
type RiskConfig struct {
    MaxInventory          float64
    MaxOrderNotional      float64
    MaxDailyLossPercent   float64
    CancelRateLimit       float64
    PanicVolRatio         float64
    MaxApiErrorRate       float64
    MaxPositionQty        float64
}
```

---

## (6) FundsConfig（账户在本 symbol 投入的资金）

```go
type FundsConfig struct {
    MaxCapital        float64  // 最大允许使用资金
    MaxOpenNotional   float64
    Leverage          float64
}
```

---

# **5. 参数热更新机制（核心）**

做市商需要在不重启程序的情况下改参数。  
示例：将 `GridLevels` 从 3 改为 4。

流程如下：

```
运维 / Web UI → 修改配置 → 写入 live_override.json
    ↓
ConfigService watcher 检测变更
    ↓
解析新参数
    ↓
校验（tickSize、max值等）
    ↓
更新 runtime config （带版本号）
    ↓
通知策略下次 OnTick 采用新参数
```

### **API 示例：**

```go
func (cs *ConfigService) HotUpdate(symbol string, newCfg SymbolConfig) error
```

策略每次 OnTick 读取的是最新 config。

### **变更通知机制（推荐）**

使用 channel：

```go
ConfigUpdateChan chan ConfigUpdateEvent
```

策略、风控、OMS 监听此事件决定是否刷新本地缓存的参数。

---

# **6. 适用于 Phase1 / Phase2 / 风控 / OMS 的参数集**

### Phase1 关键参数：

- BaseOrderSize
    
- MinSpreadTicks
    
- QuoteOffsetTicks
    
- MaxInventory
    
- MinRequoteIntervalMs
    

### Phase2 新增关键参数：

- GridLevels / GridStepTicks
    
- SizeMultiplier
    
- VolatilityFactor
    
- ImbalanceBiasFactor
    
- TrendAvoidThreshold
    
- VolatilityPanicRatio
    

### 风控：

- MaxDailyLossPercent
    
- CancelRateLimit
    
- MaxOrderNotional
    
- PanicVolRatio
    

### OMS：

- MaxOrdersPerSec
    
- RetryPolicy
    

ConfigService 必须保证这些全部可以 **统一来源 + 热更新**。

---

# **7. 多 symbol 配置**

结构：

```yaml
symbols:
  BTCUSDC:
    tickSize: 0.1
    stepSize: 0.001
    strategyPhase: "phase2"
    strategy:
      gridLevels: 3
      volatilityFactor: 1.2
  ETHUSDC:
    tickSize: 0.01
    strategyPhase: "phase1"
```

ConfigService 自动管理 mappings：

```
config.Symbols["BTCUSDC"].Strategy
config.Symbols["ETHUSDC"].Risk
```

---

# **8. 错误处理与回滚机制**

加载配置或热更新时：

1. YAML/JSON 解析失败 → 拒绝更新
    
2. 类型不匹配 → 拒绝更新
    
3. 数值违反限制（如负 tickSize） → 拒绝更新
    
4. 风控矛盾（如 MaxInventory < BaseOrderSize） → 拒绝更新
    

**如果发生半更新状态必须回滚到上个版本。**

运行版本采用：

```go
ActiveConfigVersion
LastGoodConfigVersion
```

---

# **9. 监控与版本管理**

监控内容：

|指标|含义|
|---|---|
|config_active_version|当前版本号|
|config_reload_total|重载次数|
|config_reload_errors|失败次数|
|config_last_update_ts|最近更新时间|
|config_overrides_total|live override 次数|

同时需要日志：

```
INFO: Config updated: symbol=BTCUSDC version=23
ERROR: Config update failed: reason=...
```

---

# **10. 扩展能力**

未来可加入：

- Web UI 修改参数
    
- 参数 A/B 测试（两个策略运行不同参数）
    
- 参数版本对比回溯
    
- 从数据库拉取统一参数（Postgres / Redis）
    
- 多策略共用参数组
    
- 自动化调参系统（模型自动调 spread 或 grid 层数）
    

ConfigService 的设计已支持你之后构建“多策略、多交易对、多模型协作”的高级系统。

---

# 下一份文档

按顺序，下一个模块是：

> **《BacktestEngine & Simulator – 回测与仿真系统设计文档（v1.0）》**

这个模块对于策略验证、参数调参、避免实盘踩坑 **极其重要**。  
尤其是你未来准备做 AI debate 决策系统，回测器会变成核心基础。

我可继续写这一份吗？