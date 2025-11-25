# Phase 2-3 工程交接文档

> **交接日期**: 2025-11-23  
> **上一阶段**: Phase 1-2 + 风控核心模块（已完成）  
> **本阶段目标**: 完成基础做市策略、告警系统、订单对账

---

## 📋 给下一个工程师的指令

```
你好！接手做市商系统重构项目的Phase 2-3后续阶段。

项目背景：
1. 这是一个Golang做市商系统重构项目
2. Phase 1-2已完成：基础设施（日志+监控+容器）和订单状态机
3. 风控核心模块已完成：PnL监控、熔断器、风控监控中心
4. 所有代码编译通过，50个单元测试全部通过

请阅读本文档了解：
- 已完成的工作内容和代码结构
- 下一步需要实现的功能
- 具体的实现指导

现在开始继续Phase 2-3工作，按优先级实现：
1. 基础做市策略（internal/strategy/basic_mm.go）- P0
2. 告警系统（infrastructure/alert/manager.go）- P1
3. 订单对账机制（order/reconciler.go）- P1
```

---

## ✅ 已完成工作总结（2025-11-23）

### 🎯 核心成果

#### 1. PnL监控器 (internal/risk/pnl_monitor.go)

**文件信息**:
- 代码: 214行
- 测试: 305行（`pnl_monitor_test.go`）
- 测试用例: 13个，全部通过
- 测试覆盖率: 100%

**核心功能**:
```go
type PnLMonitor struct {
    limits        PnLLimits
    realizedPnL   float64      // 已实现盈亏
    unrealizedPnL float64      // 未实现盈亏
    maxDrawdown   float64      // 最大回撤
    peakEquity    float64      // 权益峰值
    dailyPnL      float64      // 当日盈亏
    initialEquity float64      // 初始权益
    mu            sync.RWMutex // 并发保护
}

// 主要方法
func NewPnLMonitor(limits PnLLimits, initialEquity float64) *PnLMonitor
func (m *PnLMonitor) UpdateRealized(pnl float64)
func (m *PnLMonitor) UpdateUnrealized(unrealizedPnL float64)
func (m *PnLMonitor) CheckLimits() error
func (m *PnLMonitor) GetMetrics() PnLMetrics
func (m *PnLMonitor) ResetDaily()
```

**使用示例**:
```go
// 创建PnL监控器
limits := risk.PnLLimits{
    DailyLossLimit:   100.0,  // 日亏损限制 100 USDC
    MaxDrawdownLimit: 0.03,   // 最大回撤 3%
    MinPnLThreshold:  -50.0,  // 告警阈值 -50 USDC
}
pnlMon := risk.NewPnLMonitor(limits, 10000.0) // 初始权益10000

// 记录交易
pnlMon.UpdateRealized(50.0)  // 赚了50

// 更新未实现盈亏
pnlMon.UpdateUnrealized(30.0)

// 检查限制
if err := pnlMon.CheckLimits(); err != nil {
    // 触发风控
}

// 获取指标
metrics := pnlMon.GetMetrics()
fmt.Printf("总盈亏: %.2f, 回撤: %.4f\n", metrics.TotalPnL, metrics.MaxDrawdown)
```

#### 2. 三状态熔断器 (internal/risk/circuit_breaker.go)

**文件信息**:
- 代码: 238行
- 测试: 457行（`circuit_breaker_test.go`）
- 测试用例: 17个，全部通过
- 测试覆盖率: 100%

**核心功能**:
```go
type CircuitBreaker struct {
    state           State  // Closed/Open/HalfOpen
    failureCount    int64
    successCount    int64
    consecutiveFail int64
    threshold       int           // 失败阈值
    timeout         time.Duration // 超时时间
    mu              sync.RWMutex
}

// 状态
const (
    StateClosed   State = iota  // 正常运行
    StateOpen                    // 熔断，拒绝请求
    StateHalfOpen                // 半开，尝试恢复
)

// 主要方法
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker
func (cb *CircuitBreaker) Call(fn func() error) error
func (cb *CircuitBreaker) RecordSuccess()
func (cb *CircuitBreaker) RecordFailure()
func (cb *CircuitBreaker) GetState() State
func (cb *CircuitBreaker) Reset()
```

**使用示例**:
```go
// 创建熔断器
config := risk.CircuitBreakerConfig{
    Threshold:      5,                // 5次失败触发熔断
    Timeout:        30 * time.Second, // 30秒后尝试恢复
    HalfOpenMaxTry: 3,                // 半开状态最多3次尝试
}
cb := risk.NewCircuitBreaker(config)

// 使用熔断器执行操作
err := cb.Call(func() error {
    // 你的业务逻辑
    return placeOrder()
})

// 或手动记录结果
if success {
    cb.RecordSuccess()
} else {
    cb.RecordFailure()
}

// 检查状态
if cb.IsOpen() {
    log.Println("熔断器已打开，停止交易")
}
```

#### 3. 风控监控中心 (internal/risk/monitor.go)

**文件信息**:
- 代码: 350行
- 测试: 550行（`monitor_test.go`）
- 测试用例: 20个，全部通过
- 测试覆盖率: 100%

**核心功能**:
```go
type Monitor struct {
    config         MonitorConfig
    pnlMonitor     *PnLMonitor
    circuitBreaker *CircuitBreaker
    riskState      RiskState  // Normal/Warning/Danger/Emergency
    
    // 回调
    onRiskStateChange func(old, new RiskState)
    onEmergencyStop   func(reason string)
}

// 风险状态
const (
    RiskStateNormal    RiskState = iota  // 正常
    RiskStateWarning                     // 警告（回撤>2%）
    RiskStateDanger                      // 危险（接近限制）
    RiskStateEmergency                   // 紧急（熔断）
)

// 主要方法
func NewMonitor(config MonitorConfig) *Monitor
func (m *Monitor) Start(ctx context.Context) error
func (m *Monitor) Stop() error
func (m *Monitor) CheckPreTrade(orderValue float64) error
func (m *Monitor) RecordTrade(realizedPnL float64)
func (m *Monitor) UpdateUnrealizedPnL(unrealizedPnL float64)
func (m *Monitor) TriggerEmergencyStop(reason string)
func (m *Monitor) ResumeTrading() error
func (m *Monitor) GetMonitorMetrics() MonitorMetrics
```

**使用示例**:
```go
// 创建风控监控中心
config := risk.MonitorConfig{
    PnLLimits: risk.PnLLimits{
        DailyLossLimit:   100.0,
        MaxDrawdownLimit: 0.05,
    },
    CircuitBreakerConfig: risk.CircuitBreakerConfig{
        Threshold: 5,
        Timeout:   30 * time.Second,
    },
    MonitorInterval: 1 * time.Second,
    InitialEquity:   10000.0,
}
monitor := risk.NewMonitor(config)

// 设置回调
monitor.SetRiskStateChangeCallback(func(old, new risk.RiskState) {
    log.Printf("风险状态变化: %s -> %s", old, new)
})

monitor.SetEmergencyStopCallback(func(reason string) {
    log.Printf("紧急停止: %s", reason)
    // 撤销所有订单、平仓等
})

// 启动监控
ctx := context.Background()
monitor.Start(ctx)

// 交易前检查
if err := monitor.CheckPreTrade(100.0); err != nil {
    log.Printf("风控拒绝: %v", err)
    return
}

// 记录交易
monitor.RecordTrade(50.0)  // 盈利50

// 更新未实现盈亏（定期调用）
monitor.UpdateUnrealizedPnL(calculateUnrealizedPnL())

// 获取指标
metrics := monitor.GetMonitorMetrics()
log.Printf("风险状态: %s, PnL: %.2f", metrics.RiskState, metrics.PnLMetrics.TotalPnL)

// 停止监控
monitor.Stop()
```

---

## 📁 代码结构说明

### 当前目录结构

```
market-maker-go/
├── internal/
│   ├── risk/                    # ✅ 风控模块（已完成）
│   │   ├── pnl_monitor.go       # PnL监控器
│   │   ├── pnl_monitor_test.go  # 测试
│   │   ├── circuit_breaker.go   # 熔断器
│   │   ├── circuit_breaker_test.go
│   │   ├── monitor.go           # 风控监控中心
│   │   └── monitor_test.go
│   │
│   ├── strategy/                # ⏳ 策略模块（待实现）
│   │   └── (需要创建 basic_mm.go)
│   │
│   ├── container/               # ✅ 容器（已有）
│   │   ├── container.go
│   │   └── lifecycle.go
│   │
│   └── engine/                  # ⏳ 交易引擎（待实现）
│
├── infrastructure/
│   ├── logger/                  # ✅ 日志（已有）
│   ├── monitor/                 # ✅ 监控（已有）
│   └── alert/                   # ⏳ 告警（待实现）
│       └── (需要创建 manager.go)
│
├── order/                       # ✅ 订单（部分完成）
│   ├── state_machine.go         # ✅ 状态机
│   ├── manager.go               # ✅ 订单管理
│   └── (需要创建 reconciler.go) # ⏳ 对账机制
│
├── gateway/                     # ✅ 网关（已有）
├── market/                      # ✅ 行情（已有）
├── inventory/                   # ✅ 库存（已有）
└── config/                      # ✅ 配置（已有）
```

### 测试覆盖情况

```
✅ internal/risk/pnl_monitor.go      - 100% (13 tests)
✅ internal/risk/circuit_breaker.go  - 100% (17 tests)
✅ internal/risk/monitor.go          - 100% (20 tests)
✅ order/state_machine.go            - >90%
✅ internal/container/               - >85%
✅ infrastructure/logger/            - >80%
✅ infrastructure/monitor/           - >80%
```

---

## 🎯 下一步工作清单

### 优先级 P0-Critical（必须完成）

#### 任务1: 基础做市策略 (internal/strategy/basic_mm.go)

**目标**: 实现简单的对称做市策略

**需要创建的文件**:
- `internal/strategy/basic_mm.go`
- `internal/strategy/basic_mm_test.go`

**参考设计**:
```go
package strategy

// Quote 报价
type Quote struct {
    Side  string  // "BUY" or "SELL"
    Price float64
    Size  float64
}

// Context 策略上下文
type Context struct {
    Symbol       string
    Mid          float64  // 中间价
    Inventory    float64  // 当前仓位
    MaxInventory float64  // 最大仓位
}

// BasicMarketMaking 基础做市策略
type BasicMarketMaking struct {
    baseSpread   float64  // 基础价差（如0.0005 = 0.05%）
    baseSize     float64  // 基础数量
    maxInventory float64  // 最大库存
    skewFactor   float64  // 倾斜因子
}

// Config 配置
type Config struct {
    BaseSpread   float64
    BaseSize     float64
    MaxInventory float64
    SkewFactor   float64
}

// NewBasicMarketMaking 创建策略
func NewBasicMarketMaking(config Config) *BasicMarketMaking {
    return &BasicMarketMaking{
        baseSpread:   config.BaseSpread,
        baseSize:     config.BaseSize,
        maxInventory: config.MaxInventory,
        skewFactor:   config.SkewFactor,
    }
}

// GenerateQuotes 生成报价
func (s *BasicMarketMaking) GenerateQuotes(ctx Context) ([]Quote, error) {
    // 1. 计算基础spread
    halfSpread := s.baseSpread * ctx.Mid / 2
    
    // 2. 计算库存倾斜
    inventoryRatio := ctx.Inventory / s.maxInventory  // -1 到 1
    skew := inventoryRatio * s.skewFactor * halfSpread
    
    // 3. 生成买卖报价
    buyPrice := ctx.Mid - halfSpread - skew
    sellPrice := ctx.Mid + halfSpread - skew
    
    return []Quote{
        {Side: "BUY", Price: buyPrice, Size: s.baseSize},
        {Side: "SELL", Price: sellPrice, Size: s.baseSize},
    }, nil
}

// OnFill 成交回调
func (s *BasicMarketMaking) OnFill(side string, price, size float64) {
    // 可选：根据成交调整策略参数
}
```

**测试要点**:
- [ ] 中间价计算正确
- [ ] Spread应用正确
- [ ] 库存倾斜逻辑正确
- [ ] 边界条件处理（零库存、满库存）
- [ ] 并发安全（如果需要）

**验收标准**:
- [ ] 代码编译通过
- [ ] 单元测试覆盖率 > 85%
- [ ] 能生成合法的买卖报价
- [ ] 库存倾斜符合预期

**预计工时**: 4-6小时

---

### 优先级 P1-High（重要）

#### 任务2: 告警系统 (infrastructure/alert/manager.go)

**目标**: 实现多渠道告警管理

**需要创建的文件**:
- `infrastructure/alert/manager.go`
- `infrastructure/alert/manager_test.go`
- `infrastructure/alert/channels.go` (可选)

**参考设计**:
```go
package alert

import (
    "fmt"
    "sync"
    "time"
)

// Alert 告警
type Alert struct {
    Level     string                 // "INFO", "WARNING", "ERROR", "CRITICAL"
    Message   string
    Timestamp time.Time
    Fields    map[string]interface{}
}

// Channel 告警通道接口
type Channel interface {
    Send(alert Alert) error
}

// Manager 告警管理器
type Manager struct {
    channels []Channel
    throttle *Throttler  // 限流
    mu       sync.RWMutex
}

// Throttler 告警限流器
type Throttler struct {
    lastSent map[string]time.Time
    interval time.Duration
    mu       sync.RWMutex
}

func NewManager(channels []Channel) *Manager {
    return &Manager{
        channels: channels,
        throttle: &Throttler{
            lastSent: make(map[string]time.Time),
            interval: 5 * time.Minute,  // 同一告警5分钟最多一次
        },
    }
}

func (m *Manager) SendAlert(alert Alert) error {
    // 检查限流
    key := fmt.Sprintf("%s:%s", alert.Level, alert.Message)
    if !m.throttle.Allow(key) {
        return nil  // 被限流，静默忽略
    }
    
    // 发送到所有通道
    var lastErr error
    for _, ch := range m.channels {
        if err := ch.Send(alert); err != nil {
            lastErr = err
        }
    }
    return lastErr
}

// 实现几个基本的告警通道

// LogChannel 日志告警
type LogChannel struct {
    logger Logger
}

func (c *LogChannel) Send(alert Alert) error {
    c.logger.Log(alert.Level, alert.Message, alert.Fields)
    return nil
}

// EmailChannel 邮件告警（可选，先用日志代替）
type EmailChannel struct {
    // SMTP配置
}

// WebhookChannel Webhook告警（企业微信/钉钉）
type WebhookChannel struct {
    url string
}
```

**测试要点**:
- [ ] 告警发送正确
- [ ] 限流机制有效
- [ ] 多通道并发发送
- [ ] 错误处理

**验收标准**:
- [ ] 至少实现一个告警通道（日志）
- [ ] 限流机制工作正常
- [ ] 单元测试覆盖率 > 80%

**预计工时**: 3-4小时

---

#### 任务3: 订单对账机制 (order/reconciler.go)

**目标**: 实现订单状态对账，保证本地与交易所状态一致

**需要创建的文件**:
- `order/reconciler.go`
- `order/reconciler_test.go`

**参考设计**:
```go
package order

import (
    "context"
    "time"
)

// Reconciler 订单对账器
type Reconciler struct {
    gateway    ExchangeGateway  // 交易所接口
    manager    *Manager         // 本地订单管理
    interval   time.Duration    // 对账间隔
    stopChan   chan struct{}
}

// ExchangeGateway 交易所接口（需要从gateway包获取）
type ExchangeGateway interface {
    GetOrder(orderID string) (*Order, error)
    GetOpenOrders(symbol string) ([]*Order, error)
}

func NewReconciler(gateway ExchangeGateway, manager *Manager, interval time.Duration) *Reconciler {
    return &Reconciler{
        gateway:  gateway,
        manager:  manager,
        interval: interval,
        stopChan: make(chan struct{}),
    }
}

// Start 启动对账
func (r *Reconciler) Start(ctx context.Context) error {
    ticker := time.NewTicker(r.interval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-r.stopChan:
            return nil
        case <-ticker.C:
            if err := r.Reconcile(); err != nil {
                // 记录错误但继续
            }
        }
    }
}

// Stop 停止对账
func (r *Reconciler) Stop() error {
    close(r.stopChan)
    return nil
}

// Reconcile 执行一次对账
func (r *Reconciler) Reconcile() error {
    // 1. 获取本地所有活跃订单
    localOrders := r.manager.GetActiveOrders()
    
    // 2. 从交易所获取订单状态
    for _, localOrder := range localOrders {
        remoteOrder, err := r.gateway.GetOrder(localOrder.ID)
        if err != nil {
            continue  // 记录错误
        }
        
        // 3. 比较并解决冲突
        if err := r.resolveConflict(localOrder, remoteOrder); err != nil {
            // 记录错误
        }
    }
    
    return nil
}

// resolveConflict 解决状态冲突
func (r *Reconciler) resolveConflict(local, remote *Order) error {
    // 以交易所状态为准
    if local.Status != remote.Status {
        return r.manager.UpdateStatus(local.ID, remote.Status)
    }
    
    // 检查成交数量
    if local.FilledQty != remote.FilledQty {
        local.FilledQty = remote.FilledQty
        return r.manager.Update(local)
    }
    
    return nil
}
```

**测试要点**:
- [ ] 状态同步正确
- [ ] 冲突解决逻辑
- [ ] 定期对账机制
- [ ] 错误处理

**验收标准**:
- [ ] 能检测状态不一致
- [ ] 能修复不一致状态
- [ ] 单元测试覆盖率 > 85%

**预计工时**: 4-5小时

---

## 🔧 集成到Container

完成上述模块后，需要集成到依赖注入容器：

```go
// internal/container/container.go

type Container struct {
    // 现有字段
    cfg          *config.AppConfig
    logger       *logger.Logger
    monitor      *monitor.Monitor
    // ...
    
    // 新增字段
    riskMonitor  *risk.Monitor      // ✅ 已有
    strategy     *strategy.BasicMarketMaking  // ⏳ 待添加
    alertManager *alert.Manager     // ⏳ 待添加
    reconciler   *order.Reconciler  // ⏳ 待添加
}

func (c *Container) Build() error {
    // 1. 构建基础设施
    // ...现有代码
    
    // 2. 构建风控监控（已有）
    c.riskMonitor = risk.NewMonitor(riskConfig)
    c.riskMonitor.SetEmergencyStopCallback(func(reason string) {
        c.logger.LogRisk("emergency_stop", map[string]interface{}{
            "reason": reason,
        })
        // 撤销所有订单等
    })
    
    // 3. 构建告警管理器（待添加）
    c.alertManager = alert.NewManager([]alert.Channel{
        &alert.LogChannel{Logger: c.logger},
    })
    
    // 4. 构建策略（待添加）
    c.strategy = strategy.NewBasicMarketMaking(strategyConfig)
    
    // 5. 构建对账器（待添加）
    c.reconciler = order.NewReconciler(
        c.gateway,
        c.orderManager,
        30*time.Second,
    )
    
    return nil
}

func (c *Container) Start(ctx context.Context) error {
    // 启动风控监控
    if err := c.riskMonitor.Start(ctx); err != nil {
        return err
    }
    
    // 启动对账器
    if err := c.reconciler.Start(ctx); err != nil {
        return err
    }
    
    // ... 其他组件
    
    return nil
}
```

---

## 📚 参考资料

### 必读文档
1. `docs/CRITICAL_ANALYSIS.md` - 了解系统问题
2. `docs/REFACTOR_MASTER_PLAN.md` - 总体架构
3. `docs/REFACTOR_TODO.md` - 详细任务清单

### 代码参考
1. `internal/risk/monitor.go` - 监控模式的良好示例
2. `order/state_machine.go` - 状态机设计参考
3. `internal/container/lifecycle.go` - 生命周期管理参考

### 外部资源
1. 做市商策略基础: [链接待补充]
2. Go并发模式: https://go.dev/blog/pipelines

---

## ✅ 验收checklist

### 基础做市策略
- [ ] 代码实现完成
- [ ] 单元测试 > 85%覆盖率
- [ ] 能生成合法报价
- [ ] 库存倾斜正确
- [ ] 集成到Container

### 告警系统
- [ ] 代码实现完成
- [ ] 至少一个告警通道
- [ ] 限流机制工作
- [ ] 单元测试 > 80%覆盖率
- [ ] 集成到风控监控

### 订单对账
- [ ] 代码实现完成
- [ ] 能检测状态不一致
- [ ] 能修复不一致
- [ ] 单元测试 > 85%覆盖率
- [ ] 定期运行正常

### 整体验收
- [ ] 所有代码编译通过
- [ ] 所有测试通过
- [ ] 文档更新完整
- [ ] 能运行简单的交易场景

---

## 🚀 快速开始

```bash
# 1. 确认环境
go version  # 需要 Go 1.21+
go build ./...  # 确认现有代码编译通过
go test ./internal/risk/...  # 运行风控模块测试

# 2. 创建策略模块
mkdir -p internal/strategy
touch internal/strategy/basic_mm.go
touch internal/strategy/basic_mm_test.go

# 3. 开始实现
# 参考本文档"任务1"的设计

# 4. 测试
go test -v ./internal/strategy/...

# 5. 集成测试
go build ./...
```

---

## 💡 开发建议

1. **遵循TDD**: 先写测试，再写实现
2. **小步迭代**: 每个功能完成后立即测试
3. **参考现有代码**: 风控模块是很好的参考
4. **保持简单**: 先实现基础功能，再优化
5. **文档同步**: 完成后更新相关文档

---

## 📞 问题反馈

如遇到问题：
1. 检查现有代码的类似实现
2. 运行相关测试了解预期行为
3. 查看TODO文档的详细说明

---

**祝开发顺利！** 🎯

最后更新: 2025-11-23
