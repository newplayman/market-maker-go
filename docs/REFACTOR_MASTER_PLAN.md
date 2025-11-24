# 做市商系统重构总纲（v3.0）

> **制定日期**: 2025-11-23  
> **架构师**: 专业做市商系统架构师  
> **项目代号**: MM-Rebuild  
> **目标**: 构建生产级做市商系统

---

## 第一部分：现有代码评估与复用决策

### 1.1 可复用代码（保留）

#### ✅ 高质量模块（80%以上可用）

**gateway包** - REST/WebSocket客户端
```go
✓ gateway/binance_rest_client.go    // REST客户端，质量好
✓ gateway/binance_ws_*.go           // WebSocket处理，可用
✓ gateway/binance_signature.go      // 签名算法，正确
✓ gateway/limiter.go                // 限流器，实现合理
```
**决策**: 保留，小幅优化

**inventory包** - 库存管理
```go
✓ inventory/position.go             // 仓位跟踪，逻辑清晰
✓ inventory/valuation.go            // 估值计算，可用
```
**决策**: 保留，增加对账功能

**market包** - 行情服务
```go
✓ market/orderbook.go               // 订单簿维护，基本可用
✓ market/depth.go                   // 深度数据，可用
```
**决策**: 保留，增强功能

**config包** - 配置管理
```go
✓ config/load.go                    // 配置加载，可用
✓ config/validate.go                // 参数验证，可用
```
**决策**: 保留，增加热更新

#### ⚠️ 需要重构的模块（30-50%可用）

**strategy包** - 策略引擎
```go
⚠ strategy/engine.go                // 逻辑过简，需要重写
⚠ strategy/spread.go                // 需要增强
⚠ strategy/grid.go                  // 需要实现动态网格
```
**决策**: 保留框架，重写核心逻辑

**risk包** - 风控系统
```go
⚠ risk/guard.go                    // 接口设计OK，需要增加实现
⚠ risk/limit.go                    // 基础限制可用，需要增强
⚠ risk/circuit.go                  // 熔断器，需要完善
```
**决策**: 保留接口，增加实时监控和告警

**order包** - 订单管理
```go
⚠ order/manager.go                 // 需要增加状态机
⚠ order/state.go                   // 状态定义可用，需要增强
```
**决策**: 保留基础，增加状态机和对账

#### ❌ 需要废弃的代码

```go
❌ main.go                          // 空文件，删除
❌ cmd/backtest/                    // 未实现，删除
❌ cmd/sim/                         // 简单模拟，重新设计
```

### 1.2 代码复用率评估

```
总代码量: 约15,000行
可直接复用: 6,000行 (40%)
需要重构: 4,500行 (30%)
需要新增: 4,500行 (30%)

预估节省: 40%开发时间（约1.5-2个月）
```

---

## 第二部分：系统目标定义（SMART原则）

### 2.1 核心目标

**阶段一目标（MVP - 2个月）**:
```yaml
功能目标:
  ✓ 在Binance USDC永续合约市场稳定做市
  ✓ 支持单一交易对（ETHUSDC或BTCUSDC）
  ✓ 基础做市策略（固定spread + 库存控制）
  ✓ 完整风控系统（止损、熔断、实时监控）
  ✓ 可观测性（日志、监控、告警）

性能目标:
  ✓ 订单响应延迟 < 50ms（P95）
  ✓ 策略决策延迟 < 10ms（P95）
  ✓ 系统可用性 > 99.5%（7*24小时）
  
风控目标:
  ✓ 日内最大亏损 < 账户权益的1%
  ✓ 单笔订单 < 账户权益的0.5%
  ✓ 最大持仓 < 账户权益的2%
  
收益目标（参考）:
  ✓ 日收益率 > 0.05%（年化18%+）
  ✓ 夏普比率 > 1.0
  ✓ 最大回撤 < 3%
```

**阶段二目标（优化 - 1个月）**:
```yaml
功能增强:
  ✓ 动态spread调整（基于波动率）
  ✓ 多层网格挂单
  ✓ 盘口深度感知
  ✓ 简单信号系统

性能提升:
  ✓ 订单响应延迟 < 20ms（P95）
  ✓ 策略决策延迟 < 5ms（P95）

收益提升:
  ✓ 日收益率 > 0.1%（年化36%+）
  ✓ 夏普比率 > 1.5
```

**阶段三目标（扩展 - 1-2个月）**:
```yaml
扩展能力:
  ✓ 支持多交易对
  ✓ 跨交易所套利
  ✓ 高级策略（可选）
```

### 2.2 非功能性目标

**可靠性**:
- 故障自动恢复时间 < 30秒
- 数据不丢失（订单、成交、仓位）
- 状态一致性 > 99.99%

**可维护性**:
- 代码测试覆盖率 > 80%
- 核心模块测试覆盖率 > 95%
- 文档与代码同步更新

**可观测性**:
- 所有关键操作有日志
- 所有业务指标可监控
- 异常情况实时告警

**可扩展性**:
- 支持新交易所接入（< 1周）
- 支持新策略添加（< 3天）
- 配置变更不需重启

---

## 第三部分：系统架构设计

### 3.1 总体架构

```
┌─────────────────────────────────────────────────────────┐
│                   Application Layer                      │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │  Main    │  │  Config  │  │Lifecycle │              │
│  │ Runner   │  │ Loader   │  │ Manager  │              │
│  └──────────┘  └──────────┘  └──────────┘              │
└─────────────────────────────────────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────────┐
│                    Domain Layer                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │ Trading  │  │   Risk   │  │  Order   │              │
│  │ Engine   │  │ Monitor  │  │ Manager  │              │
│  └──────────┘  └──────────┘  └──────────┘              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │Strategy  │  │Inventory │  │ Market   │              │
│  │ Service  │  │ Tracker  │  │  Data    │              │
│  └──────────┘  └──────────┘  └──────────┘              │
└─────────────────────────────────────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────────┐
│               Infrastructure Layer                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │Exchange  │  │ Monitor  │  │  Logger  │              │
│  │ Gateway  │  │ Service  │  │ Service  │              │
│  └──────────┘  └──────────┘  └──────────┘              │
└─────────────────────────────────────────────────────────┘
```

### 3.2 核心模块设计

#### 3.2.1 Trading Engine（交易引擎）

**职责**: 策略编排与执行控制

```go
type TradingEngine struct {
    strategy    Strategy
    market      MarketData
    inventory   Inventory
    risk        RiskMonitor
    orderMgr    OrderManager
    
    state       EngineState
    eventBus    *EventBus
}

// 核心方法
func (e *TradingEngine) Start(ctx context.Context) error
func (e *TradingEngine) Stop() error
func (e *TradingEngine) OnMarketUpdate(update MarketUpdate)
func (e *TradingEngine) OnOrderUpdate(update OrderUpdate)
func (e *TradingEngine) OnRiskAlert(alert RiskAlert)
```

#### 3.2.2 Strategy Service（策略服务）

**职责**: 生成交易信号和报价

```go
type StrategyService struct {
    config      StrategyConfig
    calculator  *QuoteCalculator
    signals     []SignalProvider
}

// 核心接口
type Strategy interface {
    GenerateQuotes(ctx Context) ([]Quote, error)
    OnFill(fill Fill)
    UpdateParameters(params Parameters)
}

// 实现
type BasicMarketMaking struct {
    spreadModel  SpreadModel
    sizeModel    SizeModel
    skewModel    SkewModel
}
```

#### 3.2.3 Risk Monitor（风控监控）

**职责**: 实时风险监控和熔断

```go
type RiskMonitor struct {
    // 监控器
    positionMonitor   *PositionMonitor
    pnlMonitor        *PnLMonitor
    behaviorMonitor   *BehaviorMonitor
    marketMonitor     *MarketMonitor
    
    // 熔断器
    circuitBreaker    *CircuitBreaker
    
    // 告警
    alertManager      *AlertManager
}

// 核心方法
func (r *RiskMonitor) CheckPreTrade(order Order) error
func (r *RiskMonitor) Monitor(ctx context.Context)
func (r *RiskMonitor) GetRiskState() RiskState
func (r *RiskMonitor) TriggerEmergencyStop() error
```

#### 3.2.4 Order Manager（订单管理）

**职责**: 订单生命周期管理

```go
type OrderManager struct {
    gateway     ExchangeGateway
    stateStore  *OrderStateStore
    reconciler  *Reconciler
    
    eventChan   chan OrderEvent
}

// 状态机
type OrderStateMachine struct {
    transitions map[StateTransition]bool
}

// 核心方法
func (m *OrderManager) PlaceOrder(order Order) (string, error)
func (m *OrderManager) CancelOrder(id string) error
func (m *OrderManager) GetState(id string) (OrderState, error)
func (m *OrderManager) Reconcile() error
```

### 3.3 数据流设计

```
行情数据流:
Exchange WS → OrderBook → MarketData → TradingEngine → Strategy

订单数据流:
Strategy → OrderManager → Gateway → Exchange
Exchange → Gateway → OrderManager → TradingEngine → Inventory

风控数据流:
TradingEngine → RiskMonitor → AlertManager → 告警通道
```

---

## 第四部分：详细实施计划

### 4.1 Phase 1: 基础框架（2周）

**目标**: 搭建可运行的基础框架

#### Week 1: 核心基础设施

**Day 1-2: 项目结构重组**
```bash
market-maker-go/
├── cmd/
│   └── trader/              # 新主程序入口
│       └── main.go
├── internal/                # 私有业务逻辑
│   ├── engine/             # 交易引擎
│   ├── strategy/           # 策略服务
│   ├── risk/               # 风控监控
│   └── order/              # 订单管理
├── pkg/                    # 可复用公共库
│   ├── gateway/            # 交易所网关（复用）
│   ├── market/             # 行情数据（复用）
│   ├── inventory/          # 库存管理（复用）
│   └── config/             # 配置管理（复用）
├── infrastructure/         # 基础设施
│   ├── monitor/            # 监控服务
│   ├── logger/             # 日志服务
│   └── alert/              # 告警服务
├── test/                   # 测试
│   ├── integration/
│   └── benchmark/
└── docs/                   # 文档
```

**Day 3-4: 依赖注入容器**
```go
// internal/container/container.go
type Container struct {
    cfg            *config.Config
    logger         *logger.Logger
    monitor        *monitor.Monitor
    gateway        gateway.Exchange
    marketData     *market.Service
    inventory      *inventory.Tracker
    orderManager   *order.Manager
    riskMonitor    *risk.Monitor
    strategy       strategy.Strategy
    tradingEngine  *engine.TradingEngine
}

func NewContainer(cfg *config.Config) (*Container, error)
func (c *Container) Build() error
func (c *Container) Start(ctx context.Context) error
func (c *Container) Stop() error
```

**Day 5: 基础监控和日志**
```go
// infrastructure/logger/logger.go
type Logger struct {
    *zap.Logger
    level      zapcore.Level
    outputs    []string
    errorFile  string
}

// infrastructure/monitor/monitor.go
type Monitor struct {
    registry   *prometheus.Registry
    collectors []prometheus.Collector
}
```

#### Week 2: 核心业务模块

**Day 1-2: 订单状态机**
```go
// internal/order/state_machine.go
type StateMachine struct {
    transitions map[string]map[OrderStatus]OrderStatus
}

func NewStateMachine() *StateMachine
func (sm *StateMachine) ValidateTransition(from, to OrderStatus) error
func (sm *StateMachine) AllowedTransitions(current OrderStatus) []OrderStatus
```

**Day 3-4: 风控基础设施**
```go
// internal/risk/monitor.go
type Monitor struct {
    positionLimit  *PositionLimit
    pnlLimit       *PnLLimit
    circuitBreaker *CircuitBreaker
    alertMgr       *AlertManager
}

// internal/risk/circuit_breaker.go
type CircuitBreaker struct {
    state          State  // Closed, Open, HalfOpen
    failureCount   int64
    threshold      int
    timeout        time.Duration
}
```

**Day 5: 简单策略实现**
```go
// internal/strategy/basic_mm.go
type BasicMarketMaking struct {
    config     Config
    spread     float64
    baseSize   float64
}

func (s *BasicMarketMaking) GenerateQuotes(ctx Context) ([]Quote, error) {
    mid := ctx.Market.Mid()
    inventory := ctx.Inventory.NetExposure()
    
    // 计算spread
    spread := s.calculateSpread(ctx)
    
    // 库存倾斜
    skew := s.calculateSkew(inventory)
    
    return []Quote{
        {Side: "BUY", Price: mid - spread/2 - skew, Size: s.baseSize},
        {Side: "SELL", Price: mid + spread/2 - skew, Size: s.baseSize},
    }, nil
}
```

### 4.2 Phase 2: 功能完善（3周）

#### Week 3: 订单管理完善

**Day 1-2: 订单对账机制**
```go
// internal/order/reconciler.go
type Reconciler struct {
    gateway     gateway.Exchange
    stateStore  *StateStore
    interval    time.Duration
}

func (r *Reconciler) FullReconciliation() error
func (r *Reconciler) IncrementalSync() error
func (r *Reconciler) ResolveConflict(local, remote *Order) error
```

**Day 3-4: 订单超时处理**
```go
// internal/order/timeout_handler.go
type TimeoutHandler struct {
    pendingOrders sync.Map
    timeout       time.Duration
    checkInterval time.Duration
}

func (h *TimeoutHandler) Watch(ctx context.Context)
func (h *TimeoutHandler) HandleTimeout(order *Order)
```

**Day 5: 订单重试机制**
```go
// internal/order/retry_policy.go
type RetryPolicy struct {
    maxAttempts int
    backoff     BackoffStrategy
}

func (p *RetryPolicy) ShouldRetry(err error, attempt int) bool
func (p *RetryPolicy) WaitDuration(attempt int) time.Duration
```

#### Week 4: 风控系统完善

**Day 1-2: 实时PnL监控**
```go
// internal/risk/pnl_monitor.go
type PnLMonitor struct {
    inventory    *inventory.Tracker
    marketData   *market.Service
    
    realized     float64
    unrealized   float64
    maxDD        float64
    
    limits       PnLLimits
}

func (m *PnLMonitor) Update()
func (m *PnLMonitor) CheckLimits() error
func (m *PnLMonitor) GetMetrics() PnLMetrics
```

**Day 3-4: 行为风控**
```go
// internal/risk/behavior_monitor.go
type BehaviorMonitor struct {
    orderRate     *RateTracker
    cancelRate    *RateTracker
    fillRate      *RateTracker
    
    limits        BehaviorLimits
}

func (m *BehaviorMonitor) RecordOrder()
func (m *BehaviorMonitor) RecordCancel()
func (m *BehaviorMonitor) CheckRates() error
```

**Day 5: 告警系统**
```go
// infrastructure/alert/manager.go
type AlertManager struct {
    channels   []AlertChannel
    rules      []AlertRule
    throttle   *Throttler
}

type AlertChannel interface {
    Send(alert Alert) error
}

// 实现邮件、企业微信、钉钉等告警
type EmailChannel struct { ... }
type DingTalkChannel struct { ... }
```

#### Week 5: 策略增强

**Day 1-2: 波动率计算**
```go
// internal/strategy/volatility.go
type VolatilityCalculator struct {
    window     time.Duration
    samples    *ring.Buffer
}

func (c *VolatilityCalculator) Calculate() float64
func (c *VolatilityCalculator) Update(price float64)
```

**Day 3-4: 动态spread调整**
```go
// internal/strategy/spread_model.go
type DynamicSpreadModel struct {
    baseSpread    float64
    volMultiplier float64
    minSpread     float64
    maxSpread     float64
}

func (m *DynamicSpreadModel) Calculate(volatility float64) float64
```

**Day 5: 库存倾斜算法**
```go
// internal/strategy/skew_model.go
type SkewModel struct {
    maxInventory float64
    sensitivity  float64
}

func (m *SkewModel) Calculate(inventory float64) float64
```

### 4.3 Phase 3: 测试与优化（2周）

#### Week 6: 测试体系

**Day 1-2: 单元测试**
```bash
# 目标覆盖率
- 核心模块: 95%+
- 业务模块: 80%+
- 基础设施: 70%+
```

**Day 3-4: 集成测试**
```go
// test/integration/trading_flow_test.go
func TestFullTradingFlow(t *testing.T) {
    // 测试完整交易流程
}

func TestOrderReconciliation(t *testing.T) {
    // 测试订单对账
}

func TestRiskCircuitBreaker(t *testing.T) {
    // 测试熔断器
}
```

**Day 5: 压力测试**
```go
// test/benchmark/order_throughput_test.go
func BenchmarkOrderPlacement(b *testing.B)
func BenchmarkStrategyGeneration(b *testing.B)
```

#### Week 7: 性能优化

**Day 1-2: Profiling分析**
```bash
# CPU profiling
go test -cpuprofile=cpu.prof
pprof cpu.prof

# Memory profiling
go test -memprofile=mem.prof
pprof mem.prof
```

**Day 3-4: 优化实施**
- 对象池化
- 减少内存分配
- 优化锁使用
- 减少JSON序列化

**Day 5: 基准测试验证**
```bash
# 性能指标验证
- 订单延迟 < 50ms (P95)
- 策略延迟 < 10ms (P95)
- 内存占用 < 500MB
- CPU使用 < 50%
```

### 4.4 Phase 4: 部署准备（1周）

#### Week 8: 生产环境准备

**Day 1-2: 监控Dashboard**
```yaml
# Grafana配置
Dashboards:
  - 交易总览
  - 性能指标
  - 风控指标
  - 系统健康

Alerts:
  - 高亏损告警
  - 系统异常告警
  - 性能下降告警
```

**Day 3: 回测验证**
```go
// 使用历史数据验证策略
- 数据时间范围: 最近3个月
- 验证指标: 收益率、夏普比率、最大回撤
- 参数调优
```

**Day 4: 灰度发布准备**
```yaml
灰度计划:
  - 小资金测试（1000 USDC）
  - 运行时长: 48小时
  - 监控重点: 订单准确性、风控有效性
```

**Day 5: 应急预案**
```markdown
# 应急预案

## 紧急停止
- 触发条件: 亏损超过阈值
- 操作步骤: 全撤单 → 平仓 → 停止策略

## 数据恢复
- 订单状态恢复
- 仓位重新对账
- 系统状态重置

## 故障处理流程
1. 故障检测
2. 告警通知
3. 自动降级
4. 人工介入
5. 问题修复
6. 系统恢复
```

---

## 第五部分：关键技术决策

### 5.1 编程语言与框架

**语言**: Go 1.21+
**理由**: 
- 高性能、低延迟
- 并发模型优秀
- 部署简单
- 现有代码基于Go

**关键依赖**:
```go
// 核心依赖
github.com/prometheus/client_golang  // 监控
go.uber.org/zap                      // 日志
github.com/spf13/viper              // 配置
golang.org/x/time/rate              // 限流

// 测试依赖
github.com/stretchr/testify         // 测试框架
github.com/golang/mock              // Mock工具
```

### 5.2 部署架构

**环境**: 新加坡VPS（接近交易所）
**配置**: 
```yaml
CPU: 4核+
内存: 8GB+
网络: 低延迟(<1ms到交易所)
存储: SSD
```

**服务组件**:
```
┌─────────────────┐
│  TradingEngine  │
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
┌───┴───┐ ┌──┴────┐
│Prometheus│ Grafana│
└───────┘ └───────┘
```

### 5.3 监控方案

**指标收集**: Prometheus
**可视化**: Grafana
**日志**: 结构化日志 + 文件存储
**告警**: 多渠道（邮件、企业微信、钉钉）

---

## 第六部分：风险管理

### 6.1 技术风险

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|
| 订单丢失 | 低 | 高 | 状态机+对账机制 |
| 状态不一致 | 中 | 高 | 定期对账+日志审计 |
| 性能不达标 | 中 | 中 | 提前压测+优化 |
| 交易所API变更 | 低 | 中 | 抽象层隔离 |

### 6.2 业务风险

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|
| 策略亏损 | 中 | 高 | 回测验证+风控限制 |
| 黑天鹅事件 | 低 | 高 | 熔断机制+紧急停止 |
| 逆向选择 | 中 | 中 | 盘口深度分析 |
| 流动性枯竭 | 低 | 中 | 市场监控+自动暂停 |

---

## 第七部分：成功标准

### 7.1 Phase 1 验收标准

```yaml
功能完整性:
  ✓ 所有核心模块实现完整
  ✓ 单元测试覆盖率 > 80%
  ✓ 集成测试通过

稳定性:
  ✓ 连续运行24小时无崩溃
  ✓ 订单状态准确率 > 99.9%
  ✓ 无资金泄漏

性能:
  ✓ 订单延迟 < 50ms (P95)
  ✓ 策略延迟 < 10ms (P95)
  ✓ CPU使用 < 50%

风控:
  ✓ 熔断机制正常工作
  ✓ 告警及时触发
  ✓ 紧急停止有效
```

### 7.2 Phase 2 验收标准

```yaml
策略有效性:
  ✓ 回测收益率 > 策略预期
  ✓ 夏普比率 > 1.0
  ✓ 最大回撤 < 3%

实盘表现:
  ✓ 小资金运行7天盈利
  ✓ 成交率 > 30%
  ✓ 无风控违规
```

---

## 附录A：代码规范

```go
// 命名规范
- 包名: 小写，单词，简短
- 接口: 名词或形容词
- 结构体: 大写开头
- 方法: 动词开头

// 注释规范
- 所有公开函数必须有注释
- 复杂逻辑必须注释
- 注释说明why而不是what

// 错误处理
- 不忽略错误
- 错误包含上下文
- 区分可恢复和不可恢复错误

// 测试规范
- 测试文件以_test.go结尾
- 测试函数以Test开头
- 使用table-driven tests
```

---

**下一步**: 查看详细的TODO清单（REFACTOR_TODO.md）
