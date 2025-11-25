# Phase 3-4 工程交接文档

> **交接日期**: 2025-11-23  
> **上一阶段**: Phase 2-3（策略+告警+对账）已完成  
> **本阶段目标**: 策略增强、集成测试、性能优化

---

## 📋 给下一个工程师的指令

```
你好！接手做市商系统重构项目的Phase 3-4阶段。

项目背景：
1. 这是一个Golang做市商系统重构项目
2. Phase 1-3已完成：
   - 基础设施（日志+监控+容器）✅
   - 订单状态机 ✅
   - 风控核心（PnL监控、熔断器、监控中心）✅
   - 基础做市策略 ✅
   - 告警系统 ✅
   - 订单对账机制 ✅
3. 所有代码编译通过，102个单元测试全部通过

请阅读本文档了解：
- 已完成的工作内容和代码结构
- 下一步需要实现的功能
- 具体的实现指导

现在开始Phase 3-4工作，按优先级实现：
1. 集成测试套件 (test/integration/) - P0
2. 波动率计算器 (internal/strategy/volatility.go) - P1
3. 动态Spread模型 (internal/strategy/spread_model.go) - P1
```

---

## ✅ 已完成工作总结（截至2025-11-23）

### 🎯 Phase 1-2: 基础设施和订单管理 ✅

已由前几个阶段完成：
- ✅ 日志系统 (infrastructure/logger/)
- ✅ 监控系统 (infrastructure/monitor/)
- ✅ 容器管理 (internal/container/)
- ✅ 订单状态机 (order/state_machine.go)
- ✅ 订单管理器 (order/manager.go)

### 🎯 Phase 2: 风控核心模块 ✅

**1. PnL监控器** (internal/risk/pnl_monitor.go)
- 代码: 214行
- 测试: 305行，13个测试，100%覆盖率
- 功能: 实时PnL计算、回撤监控、日内限制、每日重置

**2. 三状态熔断器** (internal/risk/circuit_breaker.go)
- 代码: 238行
- 测试: 457行，17个测试，100%覆盖率
- 功能: Closed/Open/HalfOpen状态控制、失败计数、超时恢复

**3. 风控监控中心** (internal/risk/monitor.go)
- 代码: 350行
- 测试: 550行，20个测试，100%覆盖率
- 功能: 整合所有风控、四级风险状态、紧急停止

### 🎯 Phase 3: 策略+告警+对账 ✅

**1. 基础做市策略** (internal/strategy/basic_mm.go)
- 代码: 250行
- 测试: 470行，17个测试，94.5%覆盖率
- 功能: 对称报价、库存倾斜、动态参数、并发安全

**使用示例**:
```go
// 创建策略
config := strategy.Config{
    Symbol:       "ETHUSDC",
    BaseSpread:   0.0005,  // 0.05%
    BaseSize:     0.01,
    MaxInventory: 0.05,
    SkewFactor:   0.3,
}
strategy := strategy.NewBasicMarketMaking(config)

// 生成报价
ctx := strategy.Context{
    Symbol:       "ETHUSDC",
    Mid:          2000.0,
    Inventory:    0.02,    // 当前持仓
    MaxInventory: 0.05,
}
quotes, err := strategy.GenerateQuotes(ctx)
// quotes[0]: BUY @ 1999.50, qty: 0.01
// quotes[1]: SELL @ 2000.50, qty: 0.01

// 成交回调
strategy.OnFill("BUY", 1999.50, 0.01)

// 动态更新参数
strategy.UpdateParameters(map[string]interface{}{
    "base_spread": 0.001,  // 调整为0.1%
})
```

**2. 告警系统** (infrastructure/alert/)
- 代码: 350行（manager.go + channels.go）
- 测试: 550行，18个测试，98.9%覆盖率
- 功能: 多通道告警、智能限流、日志/控制台通道

**使用示例**:
```go
// 创建告警管理器
channels := []alert.Channel{
    alert.NewLogChannel(logger),
    alert.NewConsoleChannel(),
}
alertMgr := alert.NewManager(channels)

// 发送告警
alertMgr.SendAlert(alert.Alert{
    Level:   "ERROR",
    Message: "PnL超过日亏损限制",
    Fields: map[string]interface{}{
        "daily_pnl": -150.0,
        "limit":     -100.0,
    },
})

// 限流机制自动工作：相同告警5分钟内只发送一次
```

**3. 订单对账机制** (order/reconciler.go)
- 代码: 240行 + Manager扩展
- 测试: 440行，16个测试，80.1%覆盖率
- 功能: 定期对账、冲突解决、按Symbol对账

**使用示例**:
```go
// 创建对账器
config := order.ReconcilerConfig{
    Interval: 30 * time.Second,
}
reconciler := order.NewReconciler(gateway, orderManager, config)

// 启动对账服务
ctx := context.Background()
reconciler.Start(ctx)

// 立即执行一次对账
reconciler.ForceReconcile()

// 按交易对对账
reconciler.ReconcileBySymbol("ETHUSDC")

// 获取统计信息
stats := reconciler.GetStatistics()
fmt.Printf("对账次数: %d, 解决冲突: %d\n", 
    stats.TotalReconciliations, stats.ConflictsResolved)

// 停止对账
reconciler.Stop()
```

### 📊 整体测试结果

```
✅ internal/risk:           93.7% 覆盖率
✅ internal/strategy:       94.5% 覆盖率  
✅ infrastructure/alert:    98.9% 覆盖率
✅ order:                   80.1% 覆盖率

总计：102个单元测试全部通过 ✅
整体编译：通过 ✅
```

---

## 📁 当前代码结构

```
market-maker-go/
├── internal/
│   ├── risk/                    # ✅ 风控模块（已完成）
│   │   ├── pnl_monitor.go       # PnL监控器
│   │   ├── pnl_monitor_test.go
│   │   ├── circuit_breaker.go   # 熔断器
│   │   ├── circuit_breaker_test.go
│   │   ├── monitor.go           # 风控监控中心
│   │   └── monitor_test.go
│   │
│   ├── strategy/                # ✅ 策略模块（基础完成）
│   │   ├── basic_mm.go          # 基础做市策略
│   │   ├── basic_mm_test.go
│   │   └── (待添加 volatility.go, spread_model.go)
│   │
│   ├── container/               # ✅ 容器（已有）
│   └── engine/                  # ⏳ 交易引擎（待实现）
│
├── infrastructure/
│   ├── alert/                   # ✅ 告警（已完成）
│   │   ├── manager.go
│   │   ├── channels.go
│   │   └── manager_test.go
│   │
│   ├── logger/                  # ✅ 日志（已有）
│   └── monitor/                 # ✅ 监控（已有）
│
├── order/                       # ✅ 订单（已完成）
│   ├── state_machine.go
│   ├── manager.go
│   ├── reconciler.go
│   └── reconciler_test.go
│
├── test/                        # ⏳ 测试（待添加）
│   ├── integration/             # 需要创建
│   └── benchmark/               # 需要创建
│
├── gateway/                     # ✅ 网关（已有）
├── market/                      # ✅ 行情（已有）
├── inventory/                   # ✅ 库存（已有）
└── config/                      # ✅ 配置（已有）
```

---

## 🎯 下一步工作清单

### 优先级 P0-Critical（必须完成）

#### 任务1: 集成测试套件 (test/integration/)

**目标**: 建立完整的集成测试框架

**需要创建的文件**:
- `test/integration/trading_flow_test.go`
- `test/integration/risk_integration_test.go`
- `test/integration/reconcile_integration_test.go`
- `test/integration/mock_gateway.go`

**测试场景**:
1. **完整交易流程测试**
   ```go
   // 场景1: 正常下单-成交-更新库存
   func TestNormalTradingFlow(t *testing.T) {
       // 1. 初始化所有组件
       // 2. 策略生成报价
       // 3. 订单管理器下单
       // 4. 模拟成交回报
       // 5. 验证库存更新
       // 6. 验证PnL计算
   }
   
   // 场景2: 下单-部分成交-撤单
   func TestPartialFillAndCancel(t *testing.T) {
       // 测试部分成交后撤单流程
   }
   
   // 场景3: 风控拒单
   func TestRiskRejection(t *testing.T) {
       // 测试风控限制触发
   }
   ```

2. **风控集成测试**
   ```go
   func TestRiskCircuitBreaker(t *testing.T) {
       // 测试熔断器在交易流程中的作用
   }
   
   func TestPnLLimitTrigger(t *testing.T) {
       // 测试PnL限制触发后的系统行为
   }
   ```

3. **对账集成测试**
   ```go
   func TestReconciliationFlow(t *testing.T) {
       // 测试对账流程和冲突解决
   }
   ```

**验收标准**:
- [ ] 至少5个集成测试场景
- [ ] Mock Gateway正确模拟交易所
- [ ] 所有测试可重复运行
- [ ] 测试文档完整

**预计工时**: 8-10小时

---

### 优先级 P1-High（重要）

#### 任务2: 波动率计算器 (internal/strategy/volatility.go)

**目标**: 实现波动率计算，为动态Spread提供支持

**参考设计**:
```go
package strategy

import (
    "container/ring"
    "math"
    "sync"
    "time"
)

// VolatilityCalculator 波动率计算器
type VolatilityCalculator struct {
    window   time.Duration    // 计算窗口（如5分钟）
    samples  *ring.Ring       // 价格样本环形缓冲区
    alpha    float64          // EWMA平滑系数
    variance float64          // 当前方差
    mu       sync.RWMutex
}

// VolatilityConfig 配置
type VolatilityConfig struct {
    Window     time.Duration
    SampleSize int     // 样本数量
    Alpha      float64 // EWMA系数（如0.1）
}

// NewVolatilityCalculator 创建计算器
func NewVolatilityCalculator(cfg VolatilityConfig) *VolatilityCalculator {
    return &VolatilityCalculator{
        window:  cfg.Window,
        samples: ring.New(cfg.SampleSize),
        alpha:   cfg.Alpha,
    }
}

// Update 更新价格样本
func (v *VolatilityCalculator) Update(price float64, timestamp time.Time) {
    v.mu.Lock()
    defer v.mu.Unlock()
    
    // 添加样本到环形缓冲区
    v.samples.Value = PriceSample{
        Price:     price,
        Timestamp: timestamp,
    }
    v.samples = v.samples.Next()
    
    // 更新EWMA方差
    if v.samples.Len() >= 2 {
        v.updateVariance()
    }
}

// Calculate 计算当前波动率（标准差）
func (v *VolatilityCalculator) Calculate() float64 {
    v.mu.RLock()
    defer v.mu.RUnlock()
    
    if v.variance <= 0 {
        return 0
    }
    return math.Sqrt(v.variance)
}

// GetAnnualized 获取年化波动率
func (v *VolatilityCalculator) GetAnnualized() float64 {
    vol := v.Calculate()
    // 假设一天有1440分钟
    periodsPerDay := 1440.0 / v.window.Minutes()
    periodsPerYear := periodsPerDay * 365
    return vol * math.Sqrt(periodsPerYear)
}

// updateVariance 使用EWMA更新方差
func (v *VolatilityCalculator) updateVariance() {
    var returns []float64
    var prevPrice float64
    
    v.samples.Do(func(val interface{}) {
        if val == nil {
            return
        }
        sample := val.(PriceSample)
        if prevPrice > 0 {
            ret := (sample.Price - prevPrice) / prevPrice
            returns = append(returns, ret)
        }
        prevPrice = sample.Price
    })
    
    if len(returns) == 0 {
        return
    }
    
    // 计算最新收益率的平方
    latestReturn := returns[len(returns)-1]
    squaredReturn := latestReturn * latestReturn
    
    // EWMA更新方差
    if v.variance == 0 {
        v.variance = squaredReturn
    } else {
        v.variance = v.alpha*squaredReturn + (1-v.alpha)*v.variance
    }
}

type PriceSample struct {
    Price     float64
    Timestamp time.Time
}
```

**测试要点**:
- [ ] 波动率计算准确性（与标准公式对比）
- [ ] EWMA算法正确性
- [ ] 年化计算正确
- [ ] 并发安全
- [ ] 边界条件处理

**验收标准**:
- [ ] 代码实现完成
- [ ] 单元测试覆盖率 > 90%
- [ ] 性能满足要求（计算延迟<1ms）
- [ ] 文档完整

**预计工时**: 6小时

---

#### 任务3: 动态Spread模型 (internal/strategy/spread_model.go)

**目标**: 根据波动率动态调整Spread

**参考设计**:
```go
package strategy

// DynamicSpreadModel 动态Spread模型
type DynamicSpreadModel struct {
    baseSpread    float64  // 基础Spread（如0.0005）
    volMultiplier float64  // 波动率乘数（如2.0）
    minSpread     float64  // 最小Spread（如0.0003）
    maxSpread     float64  // 最大Spread（如0.002）
    
    volCalculator *VolatilityCalculator
}

// SpreadModelConfig 配置
type SpreadModelConfig struct {
    BaseSpread    float64
    VolMultiplier float64
    MinSpread     float64
    MaxSpread     float64
}

// NewDynamicSpreadModel 创建模型
func NewDynamicSpreadModel(
    cfg SpreadModelConfig, 
    volCalc *VolatilityCalculator,
) *DynamicSpreadModel {
    return &DynamicSpreadModel{
        baseSpread:    cfg.BaseSpread,
        volMultiplier: cfg.VolMultiplier,
        minSpread:     cfg.MinSpread,
        maxSpread:     cfg.MaxSpread,
        volCalculator: volCalc,
    }
}

// Calculate 计算当前应使用的Spread
func (m *DynamicSpreadModel) Calculate() float64 {
    // 获取当前波动率
    volatility := m.volCalculator.Calculate()
    
    // 根据波动率调整Spread
    // spread = baseSpread + volatility * volMultiplier
    spread := m.baseSpread + volatility*m.volMultiplier
    
    // 限制在合理范围内
    if spread < m.minSpread {
        spread = m.minSpread
    }
    if spread > m.maxSpread {
        spread = m.maxSpread
    }
    
    return spread
}

// CalculateWithInventory 考虑库存的Spread计算
func (m *DynamicSpreadModel) CalculateWithInventory(inventory, maxInventory float64) float64 {
    baseSpread := m.Calculate()
    
    // 库存较大时，增加Spread鼓励平仓
    inventoryRatio := math.Abs(inventory / maxInventory)
    if inventoryRatio > 0.8 {
        // 库存超过80%时，增加20%的Spread
        baseSpread *= 1.2
    }
    
    return baseSpread
}

// Adjust 手动调整参数
func (m *DynamicSpreadModel) Adjust(factor float64) {
    m.baseSpread *= factor
    // 确保不超出范围
    if m.baseSpread < m.minSpread {
        m.baseSpread = m.minSpread
    }
    if m.baseSpread > m.maxSpread {
        m.baseSpread = m.maxSpread
    }
}
```

**集成到BasicMarketMaking**:
```go
// 修改basic_mm.go，集成动态Spread
func (s *BasicMarketMaking) GenerateQuotes(ctx Context) ([]Quote, error) {
    // 使用动态Spread替代固定Spread
    spread := s.spreadModel.CalculateWithInventory(ctx.Inventory, s.config.MaxInventory)
    halfSpread := spread * ctx.Mid / 2
    
    // 计算库存倾斜
    skew := s.calculateSkew(ctx.Inventory)
    
    // 生成报价
    buyPrice := ctx.Mid - halfSpread - skew
    sellPrice := ctx.Mid + halfSpread - skew
    
    return []Quote{
        {Side: "BUY", Price: buyPrice, Size: s.baseSize},
        {Side: "SELL", Price: sellPrice, Size: s.baseSize},
    }, nil
}
```

**测试要点**:
- [ ] Spread根据波动率正确调整
- [ ] 限制范围有效
- [ ] 库存影响正确
- [ ] 参数调整功能

**验收标准**:
- [ ] 代码实现完成
- [ ] 单元测试覆盖率 > 85%
- [ ] 回测验证Spread调整有效
- [ ] 集成到BasicMarketMaking

**预计工时**: 5小时

---

## 🚀 快速开始

```bash
# 1. 确认环境
go version  # Go 1.21+
go test ./...  # 确认所有现有测试通过

# 2. 创建集成测试目录
mkdir -p test/integration
mkdir -p test/benchmark

# 3. 从集成测试开始
# 创建 test/integration/trading_flow_test.go
# 参考本文档的测试场景设计

# 4. 运行测试
go test -v ./test/integration/...

# 5. 继续波动率和Spread模型
# 创建 internal/strategy/volatility.go
# 创建 internal/strategy/spread_model.go

# 6. 整体验证
go build ./...
go test -cover ./...
```

---

## 📚 参考资料

### 必读文档
1. `docs/REFACTOR_MASTER_PLAN.md` - 总体架构
2. `docs/REFACTOR_TODO.md` - 详细任务清单
3. `docs/HANDOFF_PHASE3_NEXT.md` - 上一阶段交接文档

### 代码参考
1. `internal/risk/monitor.go` - 监控模式的良好示例
2. `internal/strategy/basic_mm.go` - 策略设计参考
3. `infrastructure/alert/manager.go` - 模块设计参考

### 技术参考
1. Go集成测试: https://go.dev/doc/tutorial/add-a-test
2. 波动率计算: EWMA算法
3. 做市策略: 库存管理、Spread优化

---

## ✅ 验收Checklist

### 集成测试
- [ ] 至少5个集成测试场景
- [ ] Mock Gateway实现完整
- [ ] 所有测试通过
- [ ] 测试覆盖核心流程

### 波动率计算器
- [ ] 代码实现完成
- [ ] EWMA算法正确
- [ ] 单元测试 > 90%覆盖率
- [ ] 性能满足要求

### 动态Spread模型
- [ ] 代码实现完成
- [ ] 集成到BasicMarketMaking
- [ ] 单元测试 > 85%覆盖率
- [ ] 回测验证有效

### 整体验收
- [ ] 所有代码编译通过
- [ ] 所有测试通过
- [ ] 文档更新完整
- [ ] 性能指标达标

---

## 💡 开发建议

1. **优先级**: 先完成集成测试，确保现有功能稳定
2. **TDD方法**: 先写测试，再写实现
3. **小步迭代**: 每个功能完成后立即测试
4. **参考现有**: 风控和策略模块有很好的代码示例
5. **文档同步**: 完成后更新相关文档

---

## 📞 常见问题

**Q1: 集成测试需要连接真实交易所吗？**  
A: 不需要，使用Mock Gateway模拟交易所行为。参考`order/reconciler_test.go`中的MockGateway实现。

**Q2: 波动率计算使用什么算法？**  
A: 使用EWMA（指数加权移动平均）算法，既简单又有效。

**Q3: 动态Spread如何验证有效性？**  
A: 通过回测验证，对比固定Spread和动态Spread的收益和风险指标。

**Q4: 如果遇到编译错误怎么办？**  
A: 运行`go build ./...`查看详细错误，参考现有代码的import和类型定义。

---

**祝开发顺利！** 🎯

最后更新: 2025-11-23
