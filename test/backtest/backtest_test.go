package backtest

import (
	"testing"
	"time"

	"market-maker-go/internal/strategy"
)

// TestBacktestEngine 回测引擎基本测试
func TestBacktestEngine(t *testing.T) {
	config := BacktestConfig{
		InitialBalance: 10000.0,
		TakerFee:       0.001,
		SlippageRate:   0.0001,
		StrategyConfig: strategy.Config{
			BaseSpread:   0.001,
			BaseSize:     0.01,
			MaxInventory: 0.05,
			SkewFactor:   0.3,
		},
	}

	engine := NewBacktestEngine(config)

	// 生成模拟数据
	priceData := generateMockPriceData(100)

	// 运行回测
	result, err := engine.Run(priceData)
	if err != nil {
		t.Fatalf("Backtest failed: %v", err)
	}

	// 验证结果
	if result == nil {
		t.Fatal("Result is nil")
	}

	if result.InitialBalance != 10000.0 {
		t.Errorf("Expected initial balance 10000, got %.2f", result.InitialBalance)
	}

	if result.TotalTrades <= 0 {
		t.Error("Expected some trades")
	}

	t.Logf("Backtest completed: %d trades, PnL: %.2f, Return: %.2f%%",
		result.TotalTrades, result.TotalPnL, result.TotalReturn*100)
}

// TestBacktestResult_PrintResult 测试结果打印
func TestBacktestResult_PrintResult(t *testing.T) {
	config := BacktestConfig{
		InitialBalance: 10000.0,
		StrategyConfig: strategy.Config{
			BaseSpread:   0.001,
			BaseSize:     0.01,
			MaxInventory: 0.05,
		},
	}

	engine := NewBacktestEngine(config)
	priceData := generateMockPriceData(50)

	result, err := engine.Run(priceData)
	if err != nil {
		t.Fatalf("Backtest failed: %v", err)
	}

	// 打印结果（测试不会失败，只是验证不会panic）
	result.PrintResult()
}

// TestBacktest_TrendingMarket 测试趋势市场
func TestBacktest_TrendingMarket(t *testing.T) {
	config := BacktestConfig{
		InitialBalance: 10000.0,
		StrategyConfig: strategy.Config{
			BaseSpread:   0.001,
			BaseSize:     0.01,
			MaxInventory: 0.05,
		},
	}

	engine := NewBacktestEngine(config)

	// 生成上涨趋势数据
	priceData := generateTrendingPriceData(100, 2000.0, 0.01) // 每天上涨1%

	result, err := engine.Run(priceData)
	if err != nil {
		t.Fatalf("Backtest failed: %v", err)
	}

	t.Logf("Trending market backtest:")
	t.Logf("  Total trades: %d", result.TotalTrades)
	t.Logf("  PnL: %.2f USDC (%.2f%%)", result.TotalPnL, result.TotalReturn*100)
	t.Logf("  Win rate: %.2f%%", result.WinRate*100)
	t.Logf("  Max drawdown: %.2f%%", result.MaxDrawdown*100)
	t.Logf("  Sharpe ratio: %.2f", result.SharpeRatio)
}

// TestBacktest_RangingMarket 测试震荡市场
func TestBacktest_RangingMarket(t *testing.T) {
	config := BacktestConfig{
		InitialBalance: 10000.0,
		StrategyConfig: strategy.Config{
			BaseSpread:   0.002, // 更大的spread适合震荡市场
			BaseSize:     0.01,
			MaxInventory: 0.05,
		},
	}

	engine := NewBacktestEngine(config)

	// 生成震荡数据
	priceData := generateRangingPriceData(100, 2000.0, 50.0) // 在1950-2050之间震荡

	result, err := engine.Run(priceData)
	if err != nil {
		t.Fatalf("Backtest failed: %v", err)
	}

	t.Logf("Ranging market backtest:")
	t.Logf("  Total trades: %d", result.TotalTrades)
	t.Logf("  PnL: %.2f USDC (%.2f%%)", result.TotalPnL, result.TotalReturn*100)
	t.Logf("  Win rate: %.2f%%", result.WinRate*100)
	t.Logf("  Max drawdown: %.2f%%", result.MaxDrawdown*100)
}

// TestBacktest_DifferentSpreads 测试不同spread参数
func TestBacktest_DifferentSpreads(t *testing.T) {
	spreads := []float64{0.0005, 0.001, 0.002, 0.005}
	priceData := generateMockPriceData(100)

	for _, spread := range spreads {
		config := BacktestConfig{
			InitialBalance: 10000.0,
			StrategyConfig: strategy.Config{
				BaseSpread:   spread,
				BaseSize:     0.01,
				MaxInventory: 0.05,
			},
		}

		engine := NewBacktestEngine(config)
		result, err := engine.Run(priceData)
		if err != nil {
			t.Fatalf("Backtest failed for spread %.4f: %v", spread, err)
		}

		t.Logf("Spread %.4f: trades=%d, PnL=%.2f, return=%.2f%%",
			spread, result.TotalTrades, result.TotalPnL, result.TotalReturn*100)
	}
}

// generateMockPriceData 生成模拟价格数据
func generateMockPriceData(count int) []PriceData {
	basePrice := 2000.0
	start := time.Now().Add(-time.Duration(count) * 24 * time.Hour)

	data := make([]PriceData, count)
	for i := 0; i < count; i++ {
		// 简单的随机游走
		volatility := 20.0
		change := (float64(i%3) - 1) * volatility

		open := basePrice
		close := basePrice + change
		high := max(open, close) + volatility*0.5
		low := min(open, close) - volatility*0.5

		data[i] = PriceData{
			Timestamp: start.Add(time.Duration(i) * 24 * time.Hour),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    1000.0,
		}

		basePrice = close
	}

	return data
}

// generateTrendingPriceData 生成趋势价格数据
func generateTrendingPriceData(count int, startPrice, dailyReturn float64) []PriceData {
	start := time.Now().Add(-time.Duration(count) * 24 * time.Hour)
	currentPrice := startPrice

	data := make([]PriceData, count)
	for i := 0; i < count; i++ {
		open := currentPrice
		close := currentPrice * (1 + dailyReturn)
		high := close * 1.005
		low := open * 0.995

		data[i] = PriceData{
			Timestamp: start.Add(time.Duration(i) * 24 * time.Hour),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    1000.0,
		}

		currentPrice = close
	}

	return data
}

// generateRangingPriceData 生成震荡价格数据
func generateRangingPriceData(count int, centerPrice, amplitude float64) []PriceData {
	start := time.Now().Add(-time.Duration(count) * 24 * time.Hour)

	data := make([]PriceData, count)
	for i := 0; i < count; i++ {
		// 简单震荡模式
		deviation := amplitude * (float64(i%5) - 2) / 2.0

		open := centerPrice + deviation
		close := centerPrice - deviation

		data[i] = PriceData{
			Timestamp: start.Add(time.Duration(i) * 24 * time.Hour),
			Open:      open,
			High:      max(open, close) + amplitude*0.1,
			Low:       min(open, close) - amplitude*0.1,
			Close:     close,
			Volume:    1000.0,
		}
	}

	return data
}

// 辅助函数
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
