package strategy_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"market-maker-go/strategy"
)

// TestGeometricGrid_Coverage 验证几何网格在单边趋势下的价格覆盖范围
func TestGeometricGrid_Coverage(t *testing.T) {
	mid := 3000.0
	levelCount := 24
	baseSize := 0.009
	spacingRatio := 1.20
	sizeDecay := 0.90

	levels := strategy.BuildGeometricGrid(mid, levelCount, baseSize, spacingRatio, sizeDecay)

	// 验证总层数
	assert.Equal(t, levelCount*2, len(levels), "should have levelCount*2 levels")

	// 统计覆盖范围
	minPrice := mid
	maxPrice := mid
	totalBuySize := 0.0
	totalSellSize := 0.0

	for _, lv := range levels {
		if lv.Price < minPrice {
			minPrice = lv.Price
			totalBuySize += lv.Size
		}
		if lv.Price > maxPrice {
			maxPrice = lv.Price
			totalSellSize += lv.Size
		}
	}

	// 计算覆盖范围（百分比）
	downRange := (mid - minPrice) / mid * 100.0
	upRange := (maxPrice - mid) / mid * 100.0

	t.Logf("几何网格覆盖范围:")
	t.Logf("  中间价: %.2f", mid)
	t.Logf("  最低买单: %.2f (向下覆盖 %.2f%%)", minPrice, downRange)
	t.Logf("  最高卖单: %.2f (向上覆盖 %.2f%%)", maxPrice, upRange)
	t.Logf("  总买单量: %.4f ETH", totalBuySize)
	t.Logf("  总卖单量: %.4f ETH", totalSellSize)

	// 验证覆盖范围至少 1.5%（避免单边偏移 2% 后僵持）
	assert.GreaterOrEqual(t, downRange, 1.5, "downward coverage should >= 1.5%")
	assert.GreaterOrEqual(t, upRange, 1.5, "upward coverage should >= 1.5%")

	// 验证远端衰减（最远层的下单量应小于近端）
	nearSize := levels[0].Size
	farSize := levels[len(levels)-1].Size
	assert.Less(t, farSize, nearSize, "far layer size should < near layer size due to decay")

	t.Logf("  近端层下单量: %.4f ETH", nearSize)
	t.Logf("  远端层下单量: %.4f ETH", farSize)
	t.Logf("  衰减比例: %.2f%%", (1-farSize/nearSize)*100)
}

// TestGeometricGrid_LayerSpacing 验证层间距几何递增
func TestGeometricGrid_LayerSpacing(t *testing.T) {
	mid := 3000.0
	levelCount := 10
	baseSize := 0.009
	spacingRatio := 1.20
	sizeDecay := 0.90

	levels := strategy.BuildGeometricGrid(mid, levelCount, baseSize, spacingRatio, sizeDecay)

	// 提取买单价格（降序）
	buyPrices := []float64{}
	for _, lv := range levels {
		if lv.Price < mid {
			buyPrices = append(buyPrices, lv.Price)
		}
	}

	require.GreaterOrEqual(t, len(buyPrices), 3, "need at least 3 buy levels to test spacing")

	// 计算相邻层间距比例
	for i := 1; i < len(buyPrices)-1; i++ {
		spacing1 := buyPrices[i-1] - buyPrices[i]
		spacing2 := buyPrices[i] - buyPrices[i+1]
		ratio := spacing2 / spacing1

		t.Logf("Layer %d spacing: %.2f, Layer %d spacing: %.2f, Ratio: %.3f", 
			i, spacing1, i+1, spacing2, ratio)

		// 验证间距比例接近 spacingRatio（允许 ±5% 误差）
		assert.InDelta(t, spacingRatio, ratio, 0.10, 
			"spacing ratio between layer %d and %d should be close to %.2f", i, i+1, spacingRatio)
	}
}
