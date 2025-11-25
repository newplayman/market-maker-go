# Phase 4-5 工程交接文档

> **交接日期**: 2025-11-23  
> **上一阶段**: Phase 2-3后续（集成测试+波动率+动态Spread）已完成  
> **本阶段目标**: 交易引擎集成、性能优化、回测验证

---

## 📋 Phase 4 工作内容

### 当前已完成模块

**Phase 1-3 完成：**
- ✅ 基础设施（日志+监控+容器）
- ✅ 订单状态机
- ✅ 风控核心（PnL监控、熔断器、监控中心）
- ✅ 基础做市策略
- ✅ 告警系统
- ✅ 订单对账机制

**Phase 3-4 完成：**
- ✅ 集成测试套件（5个测试场景）
- ✅ 波动率计算器（EWMA算法）
- ✅ 动态Spread模型

**测试状态：**
- 117个单元测试 + 5个集成测试，全部通过 ✅
- 核心模块覆盖率 > 90%

---

## 🎯 Phase 4 实施计划

### P0任务：交易引擎集成

**目标：** 将所有模块整合到统一的TradingEngine中

**需要创建文件：**

1. **internal/engine/trading_engine.go** (优先级：P0)
```go
// TradingEngine 核心交易引擎
// 职责：
// - 策略编排与执行控制
// - 模块协调（策略、风控、订单、库存）
// - 事件驱动循环
// - 生命周期管理
```

2. **internal/engine/trading_engine_test.go**
```go
// 测试场景：
// - 引擎启动/停止
// - 策略执行流程
// - 风控触发
// - 异常恢复
```

3. **cmd/trader/main.go** (新的主程序入口)
```go
// 替代当前的cmd/runner
// 使用TradingEngine作为核心
```

**实现步骤：**

```
Step 1: 创建TradingEngine基础结构
  - 定义接口和数据结构
  - 实现依赖注入
  - 实现生命周期管理

Step 2: 集成现有模块
  - 集成BasicMarketMaking策略
  - 集成RiskMonitor
  - 集成OrderManager
  - 集成Inventory
  - 集成AlertManager

Step 3: 实现事件循环
  - 定时触发策略
  - 处理订单回报
  - 处理风控告警
  - 处理异常情况

Step 4: 单元测试
  - 测试引擎启动/停止
  - 测试策略执行
  - 测试风控集成
  - 测试异常处理
```

**预计工时：** 8-12小时

---

### P1任务：性能基准测试

**目标：** 建立性能基准和优化指标

**需要创建文件：**

1. **test/benchmark/strategy_benchmark_test.go**
```go
// 基准测试：
// - 策略生成报价性能
// - 波动率计算性能
// - Spread计算性能
```

2. **test/benchmark/engine_benchmark_test.go**
```go
// 基准测试：
// - 引擎事件循环吞吐量
// - 订单处理延迟
// - 内存使用情况
```

**性能目标：**
```yaml
策略决策延迟: < 5ms (P95)
订单响应延迟: < 50ms (P95)
CPU使用率: < 50%
内存占用: < 500MB
吞吐量: > 100 ticks/s
```

**预计工时：** 4-6小时

---

### P1任务：简单回测框架

**目标：** 验证策略有效性

**需要创建文件：**

1. **test/backtest/backtest_engine.go**
```go
// 简单回测引擎
// - 加载历史价格数据
// - 模拟订单撮合
// - 计算收益指标
```

2. **test/backtest/backtest_test.go**
```go
// 回测测试：
// - 固定spread策略回测
// - 动态spread策略回测
// - 收益指标计算
```

**回测指标：**
```yaml
收益率: 日/周/月收益率
夏普比率: > 1.0
最大回撤: < 5%
胜率: > 50%
成交率: > 30%
```

**预计工时：** 6-8小时

---

### P2任务：配置热更新

**目标：** 支持运行时参数调整

**需要创建文件：**

1. **internal/config/hot_reload.go**
```go
// 配置热更新
// - 监听配置文件变化
// - 验证新配置
// - 平滑切换参数
```

**支持热更新的参数：**
- 策略参数（spread、size等）
- 风控参数（限额、熔断阈值）
- 告警参数（通道、频率）

**预计工时：** 3-4小时

---

## 📋 详细实施指导

### Task 1: 交易引擎核心 (P0)

**文件：internal/engine/trading_engine.go**

```go
package engine

import (
    "context"
    "sync"
    "time"
    
    "market-maker-go/infrastructure/alert"
    "market-maker-go/infrastructure/logger"
    "market-maker-go/internal/risk"
    "market-maker-go/internal/strategy"
    "market-maker-go/inventory"
    "market-maker-go/market"
    "market-maker-go/order"
)

// TradingEngine 核心交易引擎
type TradingEngine struct {
    // 核心组件
    strategy    *strategy.BasicMarketMaking
    riskMonitor *risk.Monitor
    orderMgr    *order.Manager
    inventory   *inventory.Tracker
    alertMgr    *alert.Manager
    logger      *logger.Logger
    
    // 配置
    config      Config
    
    // 状态
    state       EngineState
    mu          sync.RWMutex
    
    // 控制
    stopChan    chan struct{}
    doneChan    chan struct{}
}

type EngineState int

const (
    StateIdle EngineState = iota
    StateRunning
    StatePaused
    StateStopped
)

type Config struct {
    Symbol          string
    TickInterval    time.Duration  // 策略执行间隔
    EnableRisk      bool
    EnableReconcile bool
}

// New 创建交易引擎
func New(cfg Config, components Components) *TradingEngine {
    return &TradingEngine{
        strategy:    components.Strategy,
        riskMonitor: components.RiskMonitor,
        orderMgr:    components.OrderManager,
        inventory:   components.Inventory,
        alertMgr:    components.AlertManager,
        logger:      components.Logger,
        config:      cfg,
        state:       StateIdle,
        stopChan:    make(chan struct{}),
        doneChan:    make(chan struct{}),
    }
}

// Start 启动引擎
func (e *TradingEngine) Start(ctx context.Context) error {
    e.mu.Lock()
    if e.state != StateIdle {
        e.mu.Unlock()
        return errors.New("engine already started")
    }
    e.state = StateRunning
    e.mu.Unlock()
    
    e.logger.Info("Trading engine started")
    
    go e.run(ctx)
    
    return nil
}

// Stop 停止引擎
func (e *TradingEngine) Stop() error {
    e.mu.Lock()
    if e.state != StateRunning {
        e.mu.Unlock()
        return errors.New("engine not running")
    }
    e.mu.Unlock()
    
    close(e.stopChan)
    <-e.doneChan
    
    e.mu.Lock()
    e.state = StateStopped
    e.mu.Unlock()
    
    e.logger.Info("Trading engine stopped")
    
    return nil
}

// run 主事件循环
func (e *TradingEngine) run(ctx context.Context) {
    defer close(e.doneChan)
    
    ticker := time.NewTicker(e.config.TickInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-e.stopChan:
            return
        case <-ticker.C:
            e.onTick()
        }
    }
}

// onTick 定时执行
func (e *TradingEngine) onTick() {
    // 1. 检查风控状态
    if e.config.EnableRisk {
        riskState := e.riskMonitor.GetRiskState()
        if riskState.Level >= risk.LevelHigh {
            e.logger.Warn("High risk detected, skipping tick",
                zap.String("level", riskState.Level.String()))
            return
        }
    }
    
    // 2. 获取当前状态
    inventory := e.inventory.NetExposure()
    
    // 3. 生成报价
    ctx := strategy.Context{
        Symbol:       e.config.Symbol,
        Mid:          2000.0, // TODO: 从市场数据获取
        Inventory:    inventory,
        MaxInventory: e.strategy.GetConfig().MaxInventory,
    }
    
    quotes, err := e.strategy.GenerateQuotes(ctx)
    if err != nil {
        e.logger.Error("Failed to generate quotes", zap.Error(err))
        return
    }
    
    // 4. 风控检查
    // TODO: 实现风控检查
    
    // 5. 下单
    for _, quote := range quotes {
        _, err := e.orderMgr.Submit(order.Order{
            Symbol:   e.config.Symbol,
            Side:     quote.Side,
            Type:     "LIMIT",
            Price:    quote.Price,
            Quantity: quote.Size,
        })
        if err != nil {
            e.logger.Error("Failed to submit order",
                zap.String("side", quote.Side),
                zap.Error(err))
        }
    }
}

// GetState 获取引擎状态
func (e *TradingEngine) GetState() EngineState {
    e.mu.RLock()
    defer e.mu.RUnlock()
    return e.state
}

type Components struct {
    Strategy     *strategy.BasicMarketMaking
    RiskMonitor  *risk.Monitor
    OrderManager *order.Manager
    Inventory    *inventory.Tracker
    AlertManager *alert.Manager
    Logger       *logger.Logger
}
```

**使用示例：**

```go
// 创建所有组件
components := engine.Components{
    Strategy:     strategy.NewBasicMarketMaking(strategyConfig),
    RiskMonitor:  risk.NewMonitor(riskConfig),
    OrderManager: order.NewManager(gateway),
    Inventory:    &inventory.Tracker{},
    AlertManager: alert.NewManager(alertChannels),
    Logger:       logger,
}

// 创建引擎
engineConfig := engine.Config{
    Symbol:       "ETHUSDC",
    TickInterval: 5 * time.Second,
    EnableRisk:   true,
}

eng := engine.New(engineConfig, components)

// 启动
ctx := context.Background()
eng.Start(ctx)

// 运行...

// 停止
eng.Stop()
```

---

## 🚀 快速开始

```bash
# 1. 创建引擎目录
mkdir -p internal/engine
mkdir -p cmd/trader
mkdir -p test/backtest
mkdir -p test/benchmark

# 2. 开始实施
# 按优先级顺序实施：
# P0: TradingEngine
# P1: 性能基准测试
# P1: 回测框架
# P2: 配置热更新

# 3. 运行测试
go test ./internal/engine/... -v
go test ./test/benchmark/... -bench=.
go test ./test/backtest/... -v
```

---

## ✅ 验收标准

### TradingEngine
- [ ] 引擎可以正常启动/停止
- [ ] 策略定时执行
- [ ] 风控集成正常工作
- [ ] 订单管理集成正常
- [ ] 单元测试覆盖率 > 80%

### 性能基准
- [ ] 策略延迟 < 5ms (P95)
- [ ] 订单延迟 < 50ms (P95)
- [ ] CPU < 50%
- [ ] 内存 < 500MB

### 回测验证
- [ ] 可以加载历史数据
- [ ] 可以计算收益指标
- [ ] 策略参数可调整
- [ ] 回测结果可复现

---

**祝Phase 4开发顺利！** 🎯

最后更新: 2025-11-23
