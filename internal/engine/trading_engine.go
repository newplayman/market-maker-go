package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"market-maker-go/infrastructure/alert"
	"market-maker-go/infrastructure/logger"
	"market-maker-go/internal/risk"
	"market-maker-go/internal/strategy"
	"market-maker-go/inventory"
	"market-maker-go/market"
	"market-maker-go/order"
)

// EngineState 引擎状态
type EngineState int

const (
	// StateIdle 空闲状态
	StateIdle EngineState = iota
	// StateRunning 运行状态
	StateRunning
	// StatePaused 暂停状态
	StatePaused
	// StateStopped 停止状态
	StateStopped
)

// String 返回状态名称
func (s EngineState) String() string {
	switch s {
	case StateIdle:
		return "IDLE"
	case StateRunning:
		return "RUNNING"
	case StatePaused:
		return "PAUSED"
	case StateStopped:
		return "STOPPED"
	default:
		return "UNKNOWN"
	}
}

// Config 引擎配置
type Config struct {
	Symbol            string        // 交易对
	TickInterval      time.Duration // 策略执行间隔
	EnableRisk        bool          // 启用风控
	EnableReconcile   bool          // 启用对账
	ReconcileInterval time.Duration // 对账间隔
}

// Components 引擎依赖组件
type Components struct {
	Strategy     *strategy.BasicMarketMaking
	RiskMonitor  *risk.Monitor
	OrderManager *order.Manager
	Inventory    *inventory.Tracker
	MarketData   *market.Service
	AlertManager *alert.Manager
	Logger       *logger.Logger
	Reconciler   *order.Reconciler
}

// TradingEngine 核心交易引擎
type TradingEngine struct {
	// 配置
	config Config

	// 核心组件
	strategy    *strategy.BasicMarketMaking
	riskMonitor *risk.Monitor
	orderMgr    *order.Manager
	inventory   *inventory.Tracker
	marketData  *market.Service
	alertMgr    *alert.Manager
	logger      *logger.Logger
	reconciler  *order.Reconciler

	// 状态
	state EngineState
	mu    sync.RWMutex

	// 控制通道
	stopChan chan struct{}
	doneChan chan struct{}

	// 统计信息
	stats Statistics
}

// Statistics 引擎统计信息
type Statistics struct {
	StartTime     time.Time
	TotalTicks    int64
	TotalQuotes   int64
	TotalOrders   int64
	TotalFills    int64
	TotalErrors   int64
	LastTickTime  time.Time
	LastQuoteTime time.Time
	LastOrderTime time.Time
	mu            sync.RWMutex
}

// New 创建交易引擎
func New(cfg Config, components Components) (*TradingEngine, error) {
	// 参数验证
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if err := validateComponents(components); err != nil {
		return nil, fmt.Errorf("invalid components: %w", err)
	}

	// 设置默认值
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = 5 * time.Second
	}
	if cfg.ReconcileInterval <= 0 {
		cfg.ReconcileInterval = 30 * time.Second
	}

	engine := &TradingEngine{
		config:      cfg,
		strategy:    components.Strategy,
		riskMonitor: components.RiskMonitor,
		orderMgr:    components.OrderManager,
		inventory:   components.Inventory,
		marketData:  components.MarketData,
		alertMgr:    components.AlertManager,
		logger:      components.Logger,
		reconciler:  components.Reconciler,
		state:       StateIdle,
		stopChan:    make(chan struct{}),
		doneChan:    make(chan struct{}),
	}

	// 设置风控回调
	if cfg.EnableRisk && components.RiskMonitor != nil {
		engine.setupRiskCallbacks()
	}

	return engine, nil
}

// Start 启动引擎
func (e *TradingEngine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.state != StateIdle && e.state != StateStopped {
		e.mu.Unlock()
		return fmt.Errorf("engine already started (state: %s)", e.state)
	}
	// 如果从 StateStopped 复启，需要重建通道
	if e.state == StateStopped {
		e.stopChan = make(chan struct{})
		e.doneChan = make(chan struct{})
	}
	e.state = StateRunning
	e.stats.StartTime = time.Now()
	e.mu.Unlock()

	e.logger.Info("Trading engine starting",
		zap.String("symbol", e.config.Symbol),
		zap.Duration("tick_interval", e.config.TickInterval),
		zap.Bool("enable_risk", e.config.EnableRisk),
		zap.Bool("enable_reconcile", e.config.EnableReconcile))

	// 启动风控监控
	if e.config.EnableRisk && e.riskMonitor != nil {
		if err := e.riskMonitor.Start(ctx); err != nil {
			return fmt.Errorf("failed to start risk monitor: %w", err)
		}
	}

	// 启动主事件循环
	go e.run(ctx)

	e.logger.Info("Trading engine started")

	return nil
}

// Stop 停止引擎
func (e *TradingEngine) Stop() error {
	e.mu.Lock()
	if e.state != StateRunning && e.state != StatePaused {
		e.mu.Unlock()
		return fmt.Errorf("engine not running (state: %s)", e.state)
	}
	// 标记为停止中，防止重复调用
	if e.state == StateStopped {
		e.mu.Unlock()
		return nil // 幂等：已停止则直接返回
	}
	e.mu.Unlock()

	e.logger.Info("Trading engine stopping...")

	// 发送停止信号（仅当通道未关闭）
	select {
	case <-e.stopChan:
		// 已关闭，跳过
	default:
		close(e.stopChan)
	}

	// 等待主循环结束
	select {
	case <-e.doneChan:
	case <-time.After(10 * time.Second):
		e.logger.Warn("Timeout waiting for engine to stop")
	}

	// 停止风控监控
	if e.config.EnableRisk && e.riskMonitor != nil {
		if err := e.riskMonitor.Stop(); err != nil {
			e.logger.Error("Failed to stop risk monitor", zap.Error(err))
		}
	}

	// 撤销所有订单
	if err := e.cancelAllOrders(); err != nil {
		e.logger.Error("Failed to cancel all orders", zap.Error(err))
	}

	e.mu.Lock()
	e.state = StateStopped
	e.mu.Unlock()

	e.logger.Info("Trading engine stopped")

	return nil
}

// Pause 暂停引擎
func (e *TradingEngine) Pause() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != StateRunning {
		return fmt.Errorf("engine not running (state: %s)", e.state)
	}

	e.state = StatePaused
	e.logger.Info("Trading engine paused")

	return nil
}

// Resume 恢复引擎
func (e *TradingEngine) Resume() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != StatePaused {
		return fmt.Errorf("engine not paused (state: %s)", e.state)
	}

	e.state = StateRunning
	e.logger.Info("Trading engine resumed")

	return nil
}

// run 主事件循环
func (e *TradingEngine) run(ctx context.Context) {
	defer close(e.doneChan)

	ticker := time.NewTicker(e.config.TickInterval)
	defer ticker.Stop()

	var reconcileTicker *time.Ticker
	if e.config.EnableReconcile && e.reconciler != nil {
		reconcileTicker = time.NewTicker(e.config.ReconcileInterval)
		defer reconcileTicker.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("Context done, stopping engine")
			return

		case <-e.stopChan:
			e.logger.Info("Stop signal received")
			return

		case <-ticker.C:
			e.onTick()

		case <-func() <-chan time.Time {
			if reconcileTicker != nil {
				return reconcileTicker.C
			}
			return nil
		}():
			e.onReconcile()
		}
	}
}

// onTick 定时执行策略
func (e *TradingEngine) onTick() {
	e.mu.RLock()
	state := e.state
	e.mu.RUnlock()

	// 如果暂停，跳过
	if state == StatePaused {
		return
	}

	e.stats.mu.Lock()
	e.stats.TotalTicks++
	e.stats.LastTickTime = time.Now()
	e.stats.mu.Unlock()

	// 1. 检查风控状态
	if e.config.EnableRisk && e.riskMonitor != nil {
		if !e.riskMonitor.IsTrading() {
			e.logger.Warn("Trading disabled by risk monitor",
				zap.String("risk_state", e.riskMonitor.GetRiskState().String()))
			return
		}
	}

	// 2. 获取市场数据
	mid, err := e.getCurrentMid()
	if err != nil {
		e.logger.Error("Failed to get market mid price", zap.Error(err))
		e.recordError()
		return
	}

	// 3. 获取当前库存
	currentInventory := e.inventory.NetExposure()

	// 4. 生成报价
	ctx := strategy.Context{
		Symbol:       e.config.Symbol,
		Mid:          mid,
		Inventory:    currentInventory,
		MaxInventory: e.strategy.GetConfig().MaxInventory,
	}

	quotes, err := e.strategy.GenerateQuotes(ctx)
	if err != nil {
		e.logger.Error("Failed to generate quotes",
			zap.Error(err),
			zap.Float64("mid", mid),
			zap.Float64("inventory", currentInventory))
		e.recordError()
		return
	}

	e.stats.mu.Lock()
	e.stats.TotalQuotes++
	e.stats.LastQuoteTime = time.Now()
	e.stats.mu.Unlock()

	e.logger.Debug("Generated quotes",
		zap.Int("count", len(quotes)),
		zap.Float64("mid", mid),
		zap.Float64("inventory", currentInventory))

	// 5. 先撤销旧订单
	if err := e.cancelAllOrders(); err != nil {
		e.logger.Error("Failed to cancel old orders", zap.Error(err))
		// 继续执行，不中断
	}

	// 6. 下新订单
	for _, quote := range quotes {
		if err := e.placeOrder(quote); err != nil {
			e.logger.Error("Failed to place order",
				zap.String("side", quote.Side),
				zap.Float64("price", quote.Price),
				zap.Float64("size", quote.Size),
				zap.Error(err))
			e.recordError()
			continue
		}
	}
}

// onReconcile 执行订单对账
func (e *TradingEngine) onReconcile() {
	if e.reconciler == nil {
		return
	}

	e.logger.Debug("Starting order reconciliation")

	if err := e.reconciler.Reconcile(); err != nil {
		e.logger.Error("Order reconciliation failed", zap.Error(err))
		e.recordError()

		// 发送告警
		if e.alertMgr != nil {
			e.alertMgr.SendAlert(alert.Alert{
				Level:     "ERROR",
				Message:   fmt.Sprintf("订单对账失败: %v", err),
				Timestamp: time.Now(),
			})
		}
	} else {
		e.logger.Debug("Order reconciliation completed")
	}
}

// placeOrder 下单
func (e *TradingEngine) placeOrder(quote strategy.Quote) error {
	// 风控预检查
	if e.config.EnableRisk && e.riskMonitor != nil {
		orderValue := quote.Price * quote.Size
		if err := e.riskMonitor.CheckPreTrade(orderValue); err != nil {
			return fmt.Errorf("pre-trade risk check failed: %w", err)
		}
	}

	// 在提交前按净仓上限收敛下单量
	// 仅当本次动作会扩大绝对净仓时，按剩余容量收敛 size
	{
		net := e.inventory.NetExposure()
		maxInv := e.strategy.GetConfig().MaxInventory
		if maxInv > 0 {
			var delta float64
			if quote.Side == "BUY" {
				delta = quote.Size
			} else {
				delta = -quote.Size
			}
			curAbs := net
			if curAbs < 0 { curAbs = -curAbs }
			new := net + delta
			newAbs := new
			if newAbs < 0 { newAbs = -newAbs }
			if newAbs > curAbs {
				remaining := maxInv - curAbs
				if remaining <= 0 {
					return fmt.Errorf("pre-trade capacity exhausted: |%.4f| >= %.4f", curAbs, maxInv)
				}
				if remaining < quote.Size {
					quote.Size = remaining
				}
			}
		}
	}
	// 下单
	newOrder, err := e.orderMgr.Submit(order.Order{
		Symbol:   e.config.Symbol,
		Side:     quote.Side,
		Type:     "LIMIT",
		Price:    quote.Price,
		Quantity: quote.Size,
	})
	if err != nil {
		// 记录失败
		if e.riskMonitor != nil {
			e.riskMonitor.RecordFailure()
		}
		return err
	}

	// 记录成功
	if e.riskMonitor != nil {
		e.riskMonitor.RecordSuccess()
	}

	e.stats.mu.Lock()
	e.stats.TotalOrders++
	e.stats.LastOrderTime = time.Now()
	e.stats.mu.Unlock()

	e.logger.Debug("Order placed",
		zap.String("order_id", newOrder.ID),
		zap.String("side", quote.Side),
		zap.Float64("price", quote.Price),
		zap.Float64("size", quote.Size))

	return nil
}

// cancelAllOrders 撤销所有活跃订单
func (e *TradingEngine) cancelAllOrders() error {
	activeOrders := e.orderMgr.GetActiveOrders()

	var errors []error
	for _, ord := range activeOrders {
		if err := e.orderMgr.Cancel(ord.ID); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to cancel %d orders", len(errors))
	}

	return nil
}

// getCurrentMid 获取当前中间价
func (e *TradingEngine) getCurrentMid() (float64, error) {
	if e.marketData == nil {
		return 0, errors.New("market data service not available")
	}

	// 从市场数据服务获取最新价格
	mid := e.marketData.Mid(e.config.Symbol)
	if mid <= 0 {
		return 0, fmt.Errorf("invalid mid price: %f", mid)
	}

	return mid, nil
}

// recordError 记录错误
func (e *TradingEngine) recordError() {
	e.stats.mu.Lock()
	e.stats.TotalErrors++
	e.stats.mu.Unlock()
}

// setupRiskCallbacks 设置风控回调
func (e *TradingEngine) setupRiskCallbacks() {
	// 风险状态变化回调
	e.riskMonitor.SetRiskStateChangeCallback(func(old, new risk.RiskState) {
		e.logger.Warn("Risk state changed",
			zap.String("old", old.String()),
			zap.String("new", new.String()))

		// 发送告警
		if e.alertMgr != nil {
			level := "WARNING"
			if new == risk.RiskStateEmergency {
				level = "CRITICAL"
			} else if new == risk.RiskStateDanger {
				level = "ERROR"
			}

			e.alertMgr.SendAlert(alert.Alert{
				Level:     level,
				Message:   fmt.Sprintf("风险状态从 %s 变为 %s", old.String(), new.String()),
				Timestamp: time.Now(),
			})
		}
	})

	// 紧急停止回调
	e.riskMonitor.SetEmergencyStopCallback(func(reason string) {
		e.logger.Error("Emergency stop triggered", zap.String("reason", reason))

		// 暂停引擎
		if err := e.Pause(); err != nil {
			e.logger.Error("Failed to pause engine", zap.Error(err))
		}

		// 撤销所有订单
		if err := e.cancelAllOrders(); err != nil {
			e.logger.Error("Failed to cancel orders during emergency stop", zap.Error(err))
		}

		// 发送紧急告警
		if e.alertMgr != nil {
			e.alertMgr.SendAlert(alert.Alert{
				Level:     "CRITICAL",
				Message:   fmt.Sprintf("系统触发紧急停止: %s", reason),
				Timestamp: time.Now(),
			})
		}
	})
}

// GetState 获取引擎状态
func (e *TradingEngine) GetState() EngineState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state
}

// GetStatistics 获取统计信息
func (e *TradingEngine) GetStatistics() Statistics {
	e.stats.mu.RLock()
	defer e.stats.mu.RUnlock()
	return e.stats
}

// GetRiskMetrics 获取风控指标
func (e *TradingEngine) GetRiskMetrics() risk.MonitorMetrics {
	if e.riskMonitor == nil {
		return risk.MonitorMetrics{}
	}
	return e.riskMonitor.GetMonitorMetrics()
}

// GetInventory 获取当前库存
func (e *TradingEngine) GetInventory() float64 {
	return e.inventory.NetExposure()
}

// validateConfig 验证配置
func validateConfig(cfg Config) error {
	if cfg.Symbol == "" {
		return errors.New("symbol is required")
	}
	if cfg.TickInterval < 0 {
		return errors.New("tick_interval must be >= 0")
	}
	return nil
}

// validateComponents 验证组件
func validateComponents(comp Components) error {
	if comp.Strategy == nil {
		return errors.New("strategy is required")
	}
	if comp.OrderManager == nil {
		return errors.New("order_manager is required")
	}
	if comp.Inventory == nil {
		return errors.New("inventory is required")
	}
	if comp.Logger == nil {
		return errors.New("logger is required")
	}
	return nil
}
