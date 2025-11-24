package benchmark

import (
	"context"
	"runtime"
	"testing"
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

// MockGateway 模拟网关
type MockGateway struct {
	orders map[string]order.Order
}

func NewMockGateway() *MockGateway {
	return &MockGateway{
		orders: make(map[string]order.Order),
	}
}

func (m *MockGateway) Place(o order.Order) (string, error) {
	m.orders[o.ID] = o
	return o.ID, nil
}

func (m *MockGateway) Cancel(orderID string) error {
	delete(m.orders, orderID)
	return nil
}

// createBenchmarkEngine 创建用于基准测试的引擎
func createBenchmarkEngine(b *testing.B) *engine.TradingEngine {
	// 创建日志（使用最小输出）
	log, err := logger.New(logger.Config{
		Level:   "error", // 只记录错误，减少基准测试开销
		Outputs: []string{"stdout"},
		Format:  "console",
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}

	// 创建组件
	components := engine.Components{
		Strategy: strategy.NewBasicMarketMaking(strategy.Config{
			BaseSpread:   0.001,
			BaseSize:     0.01,
			MaxInventory: 0.05,
			SkewFactor:   0.3,
		}),
		RiskMonitor: risk.NewMonitor(risk.MonitorConfig{
			PnLLimits: risk.PnLLimits{
				DailyLossLimit:   100.0,
				MaxDrawdownLimit: 0.05,
			},
			MonitorInterval: 10 * time.Second, // 减少监控频率
			InitialEquity:   10000.0,
		}),
		OrderManager: order.NewManager(NewMockGateway()),
		Inventory:    &inventory.Tracker{},
		MarketData:   market.NewService(nil),
		AlertManager: alert.NewManager(nil, 5*time.Minute),
		Logger:       log,
	}

	// 设置市场数据
	components.MarketData.OnDepth("ETHUSDC", 1999.0, 2001.0, time.Now())

	// 创建引擎
	eng, err := engine.New(engine.Config{
		Symbol:       "ETHUSDC",
		TickInterval: 100 * time.Millisecond,
		EnableRisk:   false, // 禁用风控减少开销
	}, components)
	if err != nil {
		b.Fatalf("Failed to create engine: %v", err)
	}

	return eng
}

// BenchmarkEngineCreation 基准测试引擎创建
func BenchmarkEngineCreation(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = createBenchmarkEngine(b)
	}
}

// BenchmarkEngineStartStop 基准测试引擎启动停止
func BenchmarkEngineStartStop(b *testing.B) {
	b.ReportAllocs()

	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		eng := createBenchmarkEngine(b)

		b.StartTimer()
		_ = eng.Start(ctx)
		_ = eng.Stop()
		b.StopTimer()
	}
}

// BenchmarkEngineOnTick 基准测试单次tick执行
func BenchmarkEngineOnTick(b *testing.B) {
	eng := createBenchmarkEngine(b)

	// 不真正启动引擎，直接测试onTick方法的性能
	// 注意：这需要访问私有方法，这里我们通过运行引擎然后测量统计来间接评估

	ctx := context.Background()
	_ = eng.Start(ctx)
	defer eng.Stop()

	// 等待几个tick
	time.Sleep(500 * time.Millisecond)

	initialTicks := eng.GetStatistics().TotalTicks

	b.ResetTimer()
	b.ReportAllocs()

	start := time.Now()
	// 等待执行N个tick
	targetTicks := int64(b.N)
	for {
		stats := eng.GetStatistics()
		if stats.TotalTicks-initialTicks >= targetTicks {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	elapsed := time.Since(start)

	b.StopTimer()

	// 报告每个tick的平均时间
	b.ReportMetric(float64(elapsed.Nanoseconds())/float64(b.N), "ns/tick")
}

// BenchmarkEngineGetStatistics 基准测试获取统计信息
func BenchmarkEngineGetStatistics(b *testing.B) {
	eng := createBenchmarkEngine(b)

	ctx := context.Background()
	_ = eng.Start(ctx)
	defer eng.Stop()

	// 等待一些tick执行
	time.Sleep(200 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = eng.GetStatistics()
	}
}

// BenchmarkEngineGetRiskMetrics 基准测试获取风控指标
func BenchmarkEngineGetRiskMetrics(b *testing.B) {
	// 创建带风控的引擎
	log, _ := logger.New(logger.Config{
		Level:   "error",
		Outputs: []string{"stdout"},
	})

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
		OrderManager: order.NewManager(NewMockGateway()),
		Inventory:    &inventory.Tracker{},
		MarketData:   market.NewService(nil),
		AlertManager: alert.NewManager(nil, 5*time.Minute),
		Logger:       log,
	}

	components.MarketData.OnDepth("ETHUSDC", 1999.0, 2001.0, time.Now())

	eng, _ := engine.New(engine.Config{
		Symbol:       "ETHUSDC",
		TickInterval: 100 * time.Millisecond,
		EnableRisk:   true,
	}, components)

	ctx := context.Background()
	_ = eng.Start(ctx)
	defer eng.Stop()

	time.Sleep(200 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = eng.GetRiskMetrics()
	}
}

// BenchmarkEnginePauseResume 基准测试暂停恢复
func BenchmarkEnginePauseResume(b *testing.B) {
	eng := createBenchmarkEngine(b)

	ctx := context.Background()
	_ = eng.Start(ctx)
	defer eng.Stop()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = eng.Pause()
		_ = eng.Resume()
	}
}

// BenchmarkEngineStateQueries 基准测试状态查询
func BenchmarkEngineStateQueries(b *testing.B) {
	eng := createBenchmarkEngine(b)

	ctx := context.Background()
	_ = eng.Start(ctx)
	defer eng.Stop()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = eng.GetState()
		_ = eng.GetInventory()
	}
}

// BenchmarkConcurrentEngineAccess 基准测试并发访问引擎
func BenchmarkConcurrentEngineAccess(b *testing.B) {
	eng := createBenchmarkEngine(b)

	ctx := context.Background()
	_ = eng.Start(ctx)
	defer eng.Stop()

	time.Sleep(200 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = eng.GetStatistics()
			_ = eng.GetState()
			_ = eng.GetInventory()
		}
	})
}

// BenchmarkEngineMemoryFootprint 基准测试引擎内存占用
func BenchmarkEngineMemoryFootprint(b *testing.B) {
	b.ReportAllocs()

	engines := make([]*engine.TradingEngine, b.N)

	for i := 0; i < b.N; i++ {
		engines[i] = createBenchmarkEngine(b)
	}

	// 报告内存使用
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	b.ReportMetric(float64(m.Alloc)/float64(b.N), "bytes/engine")
}
