# Phase 4 快速开始指南

> 如何使用新创建的 TradingEngine

---

## 🚀 5 分钟快速开始

### 1. 最小可运行示例

```go
package main

import (
    "context"
    "log"
    "time"
    
    "market-maker-go/infrastructure/alert"
    "market-maker-go/infrastructure/logger"
    "market-maker-go/internal/engine"
    "market-maker-go/internal/risk"
    "market-maker-go/internal/strategy"
    "market-maker-go/inventory"
    "market-maker-go/market"
    "market-maker-go/order"
)

func main() {
    // 1. 创建日志
    log := logger.New(logger.Config{
        Level:   "info",
        Outputs: []string{"stdout"},
    })
    defer log.Close()
    
    // 2. 创建网关（这里使用模拟网关）
    gateway := &MockGateway{}
    
    // 3. 创建组件
    components := engine.Components{
        Strategy: strategy.NewBasicMarketMaking(strategy.Config{
            BaseSpread:   0.001,
            BaseSize:     0.01,
            MaxInventory: 0.05,
        }),
        RiskMonitor: risk.NewMonitor(risk.MonitorConfig{
            PnLLimits: risk.PnLLimits{
                DailyLossLimit:   100.0,
                MaxDrawdownLimit: 0.05,
            },
            InitialEquity: 10000.0,
        }),
        OrderManager: order.NewManager(gateway),
        Inventory:    &inventory.Tracker{},
        MarketData:   market.NewService(nil),
        AlertManager: alert.NewManager(nil, 5*time.Minute),
        Logger:       log,
    }
    
    // 4. 创建引擎
    engine, err := engine.New(engine.Config{
        Symbol:       "ETHUSDC",
        TickInterval: 5 * time.Second,
        EnableRisk:   true,
    }, components)
    if err != nil {
        log.Fatal(err)
    }
    
    // 5. 启动
    ctx := context.Background()
    if err := engine.Start(ctx); err != nil {
        log.Fatal(err)
    }
    
    // 6. 运行
    log.Info("Trading engine is running...")
    
    // 等待信号...
    select {}
}
```

---

## 📁 项目文件结构

```
market-maker-go/
├── internal/
│   └── engine/                      # ✅ 新增
│       ├── trading_engine.go        # 核心引擎
│       └── trading_engine_test.go   # 单元测试
├── docs/
│   ├── PHASE4_PROGRESS.md          # ✅ 进度报告
│   └── PHASE4_QUICK_START.md       # ✅ 快速开始
└── test/
    ├── benchmark/                   # 待创建 - P1
    └── backtest/                    # 待创建 - P1
```

---

## 🎯 核心 API

### 创建引擎

```go
engine, err := engine.New(config, components)
```

### 生命周期管理

```go
// 启动
engine.Start(ctx)

// 暂停
engine.Pause()

// 恢复
engine.Resume()

// 停止
engine.Stop()
```

### 获取状态信息

```go
// 引擎状态
state := engine.GetState()

// 统计信息
stats := engine.GetStatistics()

// 风控指标
metrics := engine.GetRiskMetrics()

// 当前库存
inventory := engine.GetInventory()
```

---

## 🔧 配置说明

### Engine Config

```go
type Config struct {
    Symbol            string        // 交易对，如 "ETHUSDC"
    TickInterval      time.Duration // 策略执行间隔，如 5*time.Second
    EnableRisk        bool          // 是否启用风控
    EnableReconcile   bool          // 是否启用对账
    ReconcileInterval time.Duration // 对账间隔，如 30*time.Second
}
```

### Components

```go
type Components struct {
    Strategy     *strategy.BasicMarketMaking  // 必需
    RiskMonitor  *risk.Monitor                // 可选（EnableRisk=true时必需）
    OrderManager *order.Manager               // 必需
    Inventory    *inventory.Tracker           // 必需
    MarketData   *market.Service              // 可选
    AlertManager *alert.Manager               // 可选
    Logger       *logger.Logger               // 必需
    Reconciler   *order.Reconciler            // 可选（EnableReconcile=true时必需）
}
```

---

## 📊 监控指标

### Statistics 统计信息

```go
stats := engine.GetStatistics()
fmt.Printf("Ticks: %d\n", stats.TotalTicks)
fmt.Printf("Quotes: %d\n", stats.TotalQuotes)
fmt.Printf("Orders: %d\n", stats.TotalOrders)
fmt.Printf("Errors: %d\n", stats.TotalErrors)
```

### RiskMetrics 风控指标

```go
metrics := engine.GetRiskMetrics()
fmt.Printf("Risk State: %s\n", metrics.RiskState)
fmt.Printf("Daily PnL: %.2f\n", metrics.PnLMetrics.DailyPnL)
fmt.Printf("Drawdown: %.4f\n", metrics.PnLMetrics.MaxDrawdown)
```

---

## 🧪 运行测试

```bash
# 运行所有引擎测试
go test ./internal/engine/... -v

# 运行特定测试
go test ./internal/engine/... -run TestTradingEngine_StartStop -v

# 查看测试覆盖率
go test ./internal/engine/... -cover
```

---

## 🐛 常见问题

### Q: 启动后立即出现 "invalid mid price" 错误？

**A**: 这是正常的。需要先设置市场数据：

```go
components.MarketData.OnDepth("ETHUSDC", 1999.0, 2001.0, time.Now())
```

### Q: 如何优雅关闭引擎？

**A**: 使用 defer 或信号处理：

```go
// 方式1: defer
defer engine.Stop()

// 方式2: 信号处理
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
<-sigChan
engine.Stop()
```

### Q: 如何动态调整策略参数？

**A**: 暂停引擎，更新参数，然后恢复：

```go
engine.Pause()
strategy.UpdateParameters(map[string]interface{}{
    "base_spread": 0.002,
})
engine.Resume()
```

---

## 📚 下一步学习

1. **阅读设计文档**: `docs/PHASE4_PROGRESS.md`
2. **查看测试示例**: `internal/engine/trading_engine_test.go`
3. **理解风控集成**: `internal/risk/monitor.go`
4. **学习策略开发**: `internal/strategy/basic_mm.go`

---

## 🎯 待实现功能 (Phase 4 P1/P2)

- [ ] 性能基准测试
- [ ] 回测框架
- [ ] 配置热更新
- [ ] Prometheus 指标导出
- [ ] HTTP 管理接口

---

**最后更新**: 2025-11-23
