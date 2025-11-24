package engine

import (
	"context"
	"testing"
	"time"

	"market-maker-go/infrastructure/alert"
	"market-maker-go/infrastructure/logger"
	"market-maker-go/internal/risk"
	"market-maker-go/internal/strategy"
	"market-maker-go/inventory"
	"market-maker-go/market"
	"market-maker-go/order"
)

// MockGateway 模拟网关
type MockGateway struct {
	placeErr  error
	cancelErr error
	orders    map[string]order.Order
}

func NewMockGateway() *MockGateway {
	return &MockGateway{
		orders: make(map[string]order.Order),
	}
}

func (m *MockGateway) Place(o order.Order) (string, error) {
	if m.placeErr != nil {
		return "", m.placeErr
	}
	m.orders[o.ID] = o
	return o.ID, nil
}

func (m *MockGateway) Cancel(orderID string) error {
	if m.cancelErr != nil {
		return m.cancelErr
	}
	delete(m.orders, orderID)
	return nil
}

// createTestComponents 创建测试组件
func createTestComponents(t *testing.T) Components {
	// 创建日志
	log, err := logger.New(logger.Config{
		Level:   "info",
		Outputs: []string{"stdout"},
		Format:  "console",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// 创建策略
	strategyConfig := strategy.Config{
		BaseSpread:   0.001,
		BaseSize:     0.01,
		MaxInventory: 0.05,
		SkewFactor:   0.3,
	}
	mmStrategy := strategy.NewBasicMarketMaking(strategyConfig)

	// 创建风控监控
	riskConfig := risk.MonitorConfig{
		PnLLimits: risk.PnLLimits{
			DailyLossLimit:   100.0,
			MaxDrawdownLimit: 0.05,
			MinPnLThreshold:  -50.0,
		},
		CircuitBreakerConfig: risk.CircuitBreakerConfig{
			Threshold: 5,
			Timeout:   30 * time.Second,
		},
		MonitorInterval: 1 * time.Second,
		InitialEquity:   10000.0,
	}
	riskMonitor := risk.NewMonitor(riskConfig)

	// 创建订单管理器
	mockGateway := NewMockGateway()
	orderMgr := order.NewManager(mockGateway)

	// 创建库存
	inv := &inventory.Tracker{}

	// 创建市场数据服务
	marketData := market.NewService(nil)

	// 创建告警管理器
	alertMgr := alert.NewManager([]alert.Channel{}, 5*time.Minute)

	return Components{
		Strategy:     mmStrategy,
		RiskMonitor:  riskMonitor,
		OrderManager: orderMgr,
		Inventory:    inv,
		MarketData:   marketData,
		AlertManager: alertMgr,
		Logger:       log,
		Reconciler:   nil,
	}
}

func TestTradingEngine_New(t *testing.T) {
	components := createTestComponents(t)
	defer components.Logger.Close()

	config := Config{
		Symbol:       "ETHUSDC",
		TickInterval: 5 * time.Second,
		EnableRisk:   true,
	}

	engine, err := New(config, components)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	if engine == nil {
		t.Fatal("Engine is nil")
	}

	if engine.GetState() != StateIdle {
		t.Errorf("Expected IDLE state, got %s", engine.GetState())
	}
}

func TestTradingEngine_StartStop(t *testing.T) {
	components := createTestComponents(t)
	defer components.Logger.Close()

	config := Config{
		Symbol:       "ETHUSDC",
		TickInterval: 100 * time.Millisecond,
		EnableRisk:   false, // 禁用风控以简化测试
	}

	engine, err := New(config, components)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	// 启动引擎
	ctx := context.Background()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}

	if engine.GetState() != StateRunning {
		t.Errorf("Expected RUNNING state, got %s", engine.GetState())
	}

	// 等待几个tick
	time.Sleep(300 * time.Millisecond)

	// 停止引擎
	if err := engine.Stop(); err != nil {
		t.Fatalf("Failed to stop engine: %v", err)
	}

	if engine.GetState() != StateStopped {
		t.Errorf("Expected STOPPED state, got %s", engine.GetState())
	}

	// 验证统计信息
	stats := engine.GetStatistics()
	if stats.TotalTicks < 2 {
		t.Errorf("Expected at least 2 ticks, got %d", stats.TotalTicks)
	}
}

func TestTradingEngine_PauseResume(t *testing.T) {
	components := createTestComponents(t)
	defer components.Logger.Close()

	config := Config{
		Symbol:       "ETHUSDC",
		TickInterval: 100 * time.Millisecond,
		EnableRisk:   false,
	}

	engine, err := New(config, components)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	ctx := context.Background()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}

	// 等待一段时间
	time.Sleep(200 * time.Millisecond)

	// 暂停
	if err := engine.Pause(); err != nil {
		t.Fatalf("Failed to pause engine: %v", err)
	}

	if engine.GetState() != StatePaused {
		t.Errorf("Expected PAUSED state, got %s", engine.GetState())
	}

	ticksBeforePause := engine.GetStatistics().TotalTicks

	// 暂停期间应该不再执行tick
	time.Sleep(300 * time.Millisecond)

	ticksAfterPause := engine.GetStatistics().TotalTicks
	if ticksAfterPause != ticksBeforePause {
		t.Errorf("Ticks should not increase when paused: before=%d, after=%d",
			ticksBeforePause, ticksAfterPause)
	}

	// 恢复
	if err := engine.Resume(); err != nil {
		t.Fatalf("Failed to resume engine: %v", err)
	}

	if engine.GetState() != StateRunning {
		t.Errorf("Expected RUNNING state, got %s", engine.GetState())
	}

	// 清理
	engine.Stop()
}

func TestTradingEngine_WithRiskMonitor(t *testing.T) {
	components := createTestComponents(t)
	defer components.Logger.Close()

	config := Config{
		Symbol:       "ETHUSDC",
		TickInterval: 100 * time.Millisecond,
		EnableRisk:   true,
	}

	engine, err := New(config, components)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	ctx := context.Background()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}

	// 等待风控监控启动
	time.Sleep(200 * time.Millisecond)

	// 验证风控监控正在运行
	if !engine.riskMonitor.IsTrading() {
		t.Error("Risk monitor should allow trading initially")
	}

	// 获取风控指标
	metrics := engine.GetRiskMetrics()
	if metrics.RiskState != risk.RiskStateNormal {
		t.Errorf("Expected NORMAL risk state, got %s", metrics.RiskState)
	}

	// 清理
	engine.Stop()
}

func TestTradingEngine_Statistics(t *testing.T) {
	components := createTestComponents(t)
	defer components.Logger.Close()

	// 设置市场数据
	components.MarketData.OnDepth("ETHUSDC", 1999.0, 2001.0, time.Now())

	config := Config{
		Symbol:       "ETHUSDC",
		TickInterval: 100 * time.Millisecond,
		EnableRisk:   false,
	}

	engine, err := New(config, components)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	ctx := context.Background()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}

	// 等待几个tick
	time.Sleep(500 * time.Millisecond)

	engine.Stop()

	// 验证统计信息
	stats := engine.GetStatistics()

	if stats.TotalTicks == 0 {
		t.Error("Expected some ticks")
	}

	if stats.TotalQuotes == 0 {
		t.Error("Expected some quotes")
	}

	if stats.StartTime.IsZero() {
		t.Error("Start time should be set")
	}

	t.Logf("Statistics: ticks=%d, quotes=%d, orders=%d, errors=%d",
		stats.TotalTicks, stats.TotalQuotes, stats.TotalOrders, stats.TotalErrors)
}

func TestTradingEngine_InvalidConfig(t *testing.T) {
	components := createTestComponents(t)
	defer components.Logger.Close()

	testCases := []struct {
		name   string
		config Config
	}{
		{
			name: "Empty symbol",
			config: Config{
				Symbol:       "",
				TickInterval: 5 * time.Second,
			},
		},
		{
			name: "Negative tick interval",
			config: Config{
				Symbol:       "ETHUSDC",
				TickInterval: -1 * time.Second,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.config, components)
			if err == nil {
				t.Error("Expected error for invalid config")
			}
		})
	}
}

func TestTradingEngine_InvalidComponents(t *testing.T) {
	config := Config{
		Symbol:       "ETHUSDC",
		TickInterval: 5 * time.Second,
	}

	testCases := []struct {
		name       string
		components Components
	}{
		{
			name: "Missing strategy",
			components: Components{
				Strategy: nil,
			},
		},
		{
			name: "Missing order manager",
			components: Components{
				Strategy:     strategy.NewBasicMarketMaking(strategy.Config{}),
				OrderManager: nil,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(config, tc.components)
			if err == nil {
				t.Error("Expected error for invalid components")
			}
		})
	}
}

func TestTradingEngine_GetInventory(t *testing.T) {
	components := createTestComponents(t)
	defer components.Logger.Close()

	config := Config{
		Symbol:       "ETHUSDC",
		TickInterval: 5 * time.Second,
	}

	engine, err := New(config, components)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	// 初始库存应该为0
	if inventory := engine.GetInventory(); inventory != 0 {
		t.Errorf("Expected 0 inventory, got %f", inventory)
	}

	// 更新库存
	components.Inventory.Update(0.05, 2000.0)

	// 验证库存
	if inventory := engine.GetInventory(); inventory != 0.05 {
		t.Errorf("Expected 0.05 inventory, got %f", inventory)
	}
}

func TestTradingEngine_StateTransitions(t *testing.T) {
	components := createTestComponents(t)
	defer components.Logger.Close()

	config := Config{
		Symbol:       "ETHUSDC",
		TickInterval: 5 * time.Second,
	}

	engine, err := New(config, components)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	// 初始状态
	if engine.GetState() != StateIdle {
		t.Errorf("Expected IDLE state, got %s", engine.GetState())
	}

	// 不能停止未启动的引擎
	if err := engine.Stop(); err == nil {
		t.Error("Should not be able to stop idle engine")
	}

	// 不能暂停未启动的引擎
	if err := engine.Pause(); err == nil {
		t.Error("Should not be able to pause idle engine")
	}

	// 启动
	ctx := context.Background()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}

	// 不能重复启动
	if err := engine.Start(ctx); err == nil {
		t.Error("Should not be able to start running engine")
	}

	// 暂停
	if err := engine.Pause(); err != nil {
		t.Fatalf("Failed to pause: %v", err)
	}

	// 不能重复暂停
	if err := engine.Pause(); err == nil {
		t.Error("Should not be able to pause paused engine")
	}

	// 恢复
	if err := engine.Resume(); err != nil {
		t.Fatalf("Failed to resume: %v", err)
	}

	// 不能恢复运行中的引擎
	if err := engine.Resume(); err == nil {
		t.Error("Should not be able to resume running engine")
	}

	// 停止
	if err := engine.Stop(); err != nil {
		t.Fatalf("Failed to stop: %v", err)
	}

	// 验证最终状态
	if engine.GetState() != StateStopped {
		t.Errorf("Expected STOPPED state, got %s", engine.GetState())
	}
}
