package benchmark

import (
	"testing"
	"time"

	"market-maker-go/internal/strategy"
)

// BenchmarkStrategyGenerateQuotes 基准测试策略生成报价性能
func BenchmarkStrategyGenerateQuotes(b *testing.B) {
	config := strategy.Config{
		BaseSpread:   0.001,
		BaseSize:     0.01,
		MaxInventory: 0.05,
		SkewFactor:   0.3,
	}
	mmStrategy := strategy.NewBasicMarketMaking(config)

	ctx := strategy.Context{
		Symbol:       "ETHUSDC",
		Mid:          2000.0,
		Inventory:    0.0,
		MaxInventory: 0.05,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mmStrategy.GenerateQuotes(ctx)
	}
}

// BenchmarkStrategyGenerateQuotes_WithInventory 带库存倾斜的基准测试
func BenchmarkStrategyGenerateQuotes_WithInventory(b *testing.B) {
	config := strategy.Config{
		BaseSpread:   0.001,
		BaseSize:     0.01,
		MaxInventory: 0.05,
		SkewFactor:   0.3,
	}
	mmStrategy := strategy.NewBasicMarketMaking(config)

	testCases := []struct {
		name      string
		inventory float64
	}{
		{"NoInventory", 0.0},
		{"SmallLong", 0.01},
		{"LargeLong", 0.04},
		{"SmallShort", -0.01},
		{"LargeShort", -0.04},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			ctx := strategy.Context{
				Symbol:       "ETHUSDC",
				Mid:          2000.0,
				Inventory:    tc.inventory,
				MaxInventory: 0.05,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = mmStrategy.GenerateQuotes(ctx)
			}
		})
	}
}

// BenchmarkVolatilityCalculation 波动率计算基准测试
func BenchmarkVolatilityCalculation(b *testing.B) {
	calc := strategy.NewVolatilityCalculator(strategy.VolatilityConfig{
		SampleSize: 20,
		Alpha:      0.1,
	})

	// 预填充一些数据
	now := time.Now()
	for i := 0; i < 20; i++ {
		calc.Update(2000.0+float64(i)*0.5, now.Add(time.Duration(i)*time.Second))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calc.Update(2000.0+float64(i)*0.1, now.Add(time.Duration(20+i)*time.Second))
		_ = calc.Calculate()
	}
}

// BenchmarkSpreadModelCalculation Spread模型计算基准测试
func BenchmarkSpreadModelCalculation(b *testing.B) {
	volCalc := strategy.NewVolatilityCalculator(strategy.VolatilityConfig{
		SampleSize: 20,
		Alpha:      0.1,
	})

	model := strategy.NewDynamicSpreadModel(strategy.SpreadModelConfig{
		BaseSpread:    0.001,
		VolMultiplier: 2.0,
		MinSpread:     0.0005,
		MaxSpread:     0.005,
	}, volCalc)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = model.Calculate()
	}
}

// BenchmarkSpreadModelWithInventory Spread模型带库存计算基准测试
func BenchmarkSpreadModelWithInventory(b *testing.B) {
	volCalc := strategy.NewVolatilityCalculator(strategy.VolatilityConfig{
		SampleSize: 20,
		Alpha:      0.1,
	})

	model := strategy.NewDynamicSpreadModel(strategy.SpreadModelConfig{
		BaseSpread:    0.001,
		VolMultiplier: 2.0,
		MinSpread:     0.0005,
		MaxSpread:     0.005,
	}, volCalc)

	testCases := []struct {
		name      string
		inventory float64
	}{
		{"NoInventory", 0.0},
		{"SmallInventory", 0.02},
		{"LargeInventory", 0.045},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = model.CalculateWithInventory(tc.inventory, 0.05)
			}
		})
	}
}

// BenchmarkStrategyUpdateParameters 参数更新基准测试
func BenchmarkStrategyUpdateParameters(b *testing.B) {
	config := strategy.Config{
		BaseSpread:   0.001,
		BaseSize:     0.01,
		MaxInventory: 0.05,
		SkewFactor:   0.3,
	}
	mmStrategy := strategy.NewBasicMarketMaking(config)

	params := map[string]interface{}{
		"base_spread": 0.002,
		"base_size":   0.02,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mmStrategy.UpdateParameters(params)
	}
}

// BenchmarkStrategyOnFill 成交回调基准测试
func BenchmarkStrategyOnFill(b *testing.B) {
	config := strategy.Config{
		BaseSpread:   0.001,
		BaseSize:     0.01,
		MaxInventory: 0.05,
		SkewFactor:   0.3,
	}
	mmStrategy := strategy.NewBasicMarketMaking(config)

	fill := strategy.Fill{
		Side:  "BUY",
		Price: 2000.0,
		Size:  0.01,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mmStrategy.OnFill(fill)
	}
}

// BenchmarkConcurrentQuoteGeneration 并发生成报价基准测试
func BenchmarkConcurrentQuoteGeneration(b *testing.B) {
	config := strategy.Config{
		BaseSpread:   0.001,
		BaseSize:     0.01,
		MaxInventory: 0.05,
		SkewFactor:   0.3,
	}
	mmStrategy := strategy.NewBasicMarketMaking(config)

	ctx := strategy.Context{
		Symbol:       "ETHUSDC",
		Mid:          2000.0,
		Inventory:    0.0,
		MaxInventory: 0.05,
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = mmStrategy.GenerateQuotes(ctx)
		}
	})
}

// BenchmarkStrategyGetStatistics 获取统计信息基准测试
func BenchmarkStrategyGetStatistics(b *testing.B) {
	config := strategy.Config{
		BaseSpread:   0.001,
		BaseSize:     0.01,
		MaxInventory: 0.05,
		SkewFactor:   0.3,
	}
	mmStrategy := strategy.NewBasicMarketMaking(config)

	// 模拟一些成交
	for i := 0; i < 100; i++ {
		mmStrategy.OnFill(strategy.Fill{
			Side:  "BUY",
			Price: 2000.0,
			Size:  0.01,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mmStrategy.GetStatistics()
	}
}
