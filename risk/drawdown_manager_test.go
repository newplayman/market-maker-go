package risk_test

import (
	"testing"
	"time"
	"github.com/stretchr/testify/assert"
	"market-maker-go/risk"
)

// mockPnLSource 模拟 PnL 数据源
type mockPnLSource struct {
	unrealized float64
}

func (m *mockPnLSource) CurrentPnL(symbol string) float64 {
	return m.unrealized
}

// mockPosition 模拟持仓
type mockPosition struct {
	net float64
}

func (m *mockPosition) NetExposure() float64 {
	return m.net
}

// TestDrawdownManager_TriggerBands 验证浮亏档位触发逻辑
func TestDrawdownManager_TriggerBands(t *testing.T) {
	pnlSrc := &mockPnLSource{unrealized: 0}
	pos := &mockPosition{net: 0.20}  // 初始净仓 0.20 ETH
	
	ddMgr := &risk.DrawdownManager{
		Bands:     []float64{5, 8, 12},          // 浮亏档位（%）
		Fractions: []float64{0.15, 0.25, 0.40}, // 减仓比例
		Mode:      "maker_first_then_taker",
		Cooldown:  2 * time.Second,
		PnL:       pnlSrc,
		Pos:       pos,
		NetMax:    0.21,
		Base:      0.009,
	}

	testCases := []struct {
		name           string
		drawdownPct    float64
		expectedQty    float64
		expectedBand   float64
		expectedMaker  bool
	}{
		{
			name:          "浮亏 3% - 未触发",
			drawdownPct:   3.0,
			expectedQty:   0,
			expectedBand:  0,
			expectedMaker: false,
		},
		{
			name:          "浮亏 5% - 触发第一档（减仓15%）",
			drawdownPct:   5.0,
			expectedQty:   0.03,  // 0.20 * 0.15 = 0.03
			expectedBand:  5.0,
			expectedMaker: true,
		},
		{
			name:          "浮亏 8% - 触发第二档（减仓25%）",
			drawdownPct:   8.0,
			expectedQty:   0.05,  // 0.20 * 0.25 = 0.05
			expectedBand:  8.0,
			expectedMaker: true,
		},
		{
			name:          "浮亏 12% - 触发第三档（减仓40%）",
			drawdownPct:   12.0,
			expectedQty:   0.08,  // 0.20 * 0.40 = 0.08
			expectedBand:  12.0,
			expectedMaker: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 重置冷却时间
			time.Sleep(2100 * time.Millisecond)
			
			qty, preferMaker, band := ddMgr.Plan("ETHUSDC", tc.drawdownPct)
			
			if tc.expectedQty == 0 {
				assert.Equal(t, 0.0, qty, "should not trigger")
			} else {
				assert.InDelta(t, tc.expectedQty, qty, 0.001, "reduce qty should match expected")
				assert.Equal(t, tc.expectedBand, band, "triggered band should match")
				assert.Equal(t, tc.expectedMaker, preferMaker, "preferMaker flag should match")
				
				t.Logf("✓ 触发档位 %.0f%%, 建议减仓 %.4f ETH (占净仓 %.1f%%)", 
					band, qty, qty/pos.NetExposure()*100)
			}
		})
	}
}

// TestDrawdownManager_Cooldown 验证冷却机制
func TestDrawdownManager_Cooldown(t *testing.T) {
	pnlSrc := &mockPnLSource{unrealized: -10}
	pos := &mockPosition{net: 0.20}
	
	ddMgr := &risk.DrawdownManager{
		Bands:     []float64{5},
		Fractions: []float64{0.15},
		Mode:      "maker_first_then_taker",
		Cooldown:  3 * time.Second,  // 3秒冷却
		PnL:       pnlSrc,
		Pos:       pos,
		NetMax:    0.21,
		Base:      0.009,
	}

	// 第一次触发
	qty1, _, band1 := ddMgr.Plan("ETHUSDC", 5.5)
	assert.Greater(t, qty1, 0.0, "first trigger should return qty")
	assert.Equal(t, 5.0, band1, "should trigger band 5")
	t.Logf("第一次触发: qty=%.4f, band=%.0f", qty1, band1)

	// 立即第二次尝试（在冷却期内）
	qty2, _, band2 := ddMgr.Plan("ETHUSDC", 6.0)
	assert.Equal(t, 0.0, qty2, "second trigger within cooldown should return 0")
	assert.Equal(t, 0.0, band2, "should not trigger any band")
	t.Logf("冷却期内再次尝试: qty=%.4f (预期0)", qty2)

	// 等待冷却结束
	t.Logf("等待冷却期结束 (3秒)...")
	time.Sleep(3100 * time.Millisecond)

	// 第三次触发（冷却期后）
	qty3, _, band3 := ddMgr.Plan("ETHUSDC", 6.0)
	assert.Greater(t, qty3, 0.0, "third trigger after cooldown should return qty")
	assert.Equal(t, 5.0, band3, "should trigger band 5 again")
	t.Logf("冷却期后再次触发: qty=%.4f, band=%.0f", qty3, band3)
}

// TestDrawdownManager_MinBaseSize 验证最小减仓量
func TestDrawdownManager_MinBaseSize(t *testing.T) {
	pnlSrc := &mockPnLSource{unrealized: -5}
	pos := &mockPosition{net: 0.01}  // 很小的净仓
	
	ddMgr := &risk.DrawdownManager{
		Bands:     []float64{5},
		Fractions: []float64{0.15},
		Mode:      "maker_first_then_taker",
		Cooldown:  1 * time.Second,
		PnL:       pnlSrc,
		Pos:       pos,
		NetMax:    0.21,
		Base:      0.009,  // 基础下单量
	}

	qty, _, band := ddMgr.Plan("ETHUSDC", 6.0)
	
	// 计算期望值: 0.01 * 0.15 = 0.0015，但应向上取整到 Base=0.009
	assert.Equal(t, 0.009, qty, "should use Base as minimum reduce qty")
	assert.Equal(t, 5.0, band, "should trigger band 5")
	t.Logf("小净仓减仓: 期望 %.4f * %.2f = %.4f, 实际使用最小值 %.4f", 
		pos.NetExposure(), 0.15, pos.NetExposure()*0.15, qty)
}
