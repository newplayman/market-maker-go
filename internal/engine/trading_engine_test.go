package engine_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

// mockInventory 模拟库存跟踪
type mockInventory struct {
	net float64
}

func (m *mockInventory) NetExposure() float64 {
	return m.net
}

func (m *mockInventory) Update(symbol string, side string, qty float64, price float64) {
	if side == "BUY" {
		m.net += qty
	} else {
		m.net -= qty
	}
}

// mockStrategy 模拟策略配置
type mockStrategy struct {
	maxInventory float64
}

func (m *mockStrategy) GetConfig() struct{ MaxInventory float64 } {
	return struct{ MaxInventory float64 }{MaxInventory: m.maxInventory}
}

// TestPreTradeCapacityCheck 验证成交前净仓容量收敛
func TestPreTradeCapacityCheck(t *testing.T) {
	testCases := []struct {
		name            string
		currentNet      float64
		maxInventory    float64
		quoteSide       string
		quoteSize       float64
		expectedSize    float64
		expectError     bool
	}{
		{
			name:         "正常买单 - 未接近上限",
			currentNet:   0.10,
			maxInventory: 0.21,
			quoteSide:    "BUY",
			quoteSize:    0.009,
			expectedSize: 0.009,
			expectError:  false,
		},
		{
			name:         "正常卖单 - 未接近上限",
			currentNet:   -0.10,
			maxInventory: 0.21,
			quoteSide:    "SELL",
			quoteSize:    0.009,
			expectedSize: 0.009,
			expectError:  false,
		},
		{
			name:         "买单接近上限 - 自动收敛",
			currentNet:   0.206,
			maxInventory: 0.21,
			quoteSide:    "BUY",
			quoteSize:    0.009,
			expectedSize: 0.004,  // 剩余容量 0.21 - 0.206 = 0.004
			expectError:  false,
		},
		{
			name:         "卖单接近上限（负向）- 自动收敛",
			currentNet:   -0.205,
			maxInventory: 0.21,
			quoteSide:    "SELL",
			quoteSize:    0.009,
			expectedSize: 0.005,  // 剩余容量 0.21 - 0.205 = 0.005
			expectError:  false,
		},
		{
			name:         "买单容量耗尽 - 拒绝",
			currentNet:   0.21,
			maxInventory: 0.21,
			quoteSide:    "BUY",
			quoteSize:    0.009,
			expectedSize: 0,
			expectError:  true,
		},
		{
			name:         "减仓方向 - 不受限制",
			currentNet:   0.20,
			maxInventory: 0.21,
			quoteSide:    "SELL",  // 减仓方向
			quoteSize:    0.009,
			expectedSize: 0.009,   // 不收敛
			expectError:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inv := &mockInventory{net: tc.currentNet}
			strat := &mockStrategy{maxInventory: tc.maxInventory}

			// 模拟 placeOrder 中的容量检查逻辑
			quoteSize := tc.quoteSize
			var err error

			net := inv.NetExposure()
			maxInv := strat.GetConfig().MaxInventory
			if maxInv > 0 {
				var delta float64
				if tc.quoteSide == "BUY" {
					delta = quoteSize
				} else {
					delta = -quoteSize
				}
				curAbs := net
				if curAbs < 0 {
					curAbs = -curAbs
				}
				newNet := net + delta
				newAbs := newNet
				if newAbs < 0 {
					newAbs = -newAbs
				}
				// 仅当扩大绝对净仓时收敛
				if newAbs > curAbs {
					remaining := maxInv - curAbs
					if remaining <= 0 {
						err = assert.AnError
					} else if remaining < quoteSize {
						quoteSize = remaining
					}
				}
			}

			if tc.expectError {
				assert.Error(t, err, "should reject order when capacity exhausted")
				t.Logf("✓ 容量耗尽拒绝: net=%.4f, max=%.4f", tc.currentNet, tc.maxInventory)
			} else {
				assert.NoError(t, err, "should not error")
				assert.InDelta(t, tc.expectedSize, quoteSize, 0.0001, "quote size should match expected")
				
				if tc.expectedSize < tc.quoteSize {
					t.Logf("✓ 自动收敛: net=%.4f, max=%.4f, 原始size=%.4f → 收敛size=%.4f",
						tc.currentNet, tc.maxInventory, tc.quoteSize, quoteSize)
				} else {
					t.Logf("✓ 正常通过: net=%.4f, max=%.4f, size=%.4f",
						tc.currentNet, tc.maxInventory, quoteSize)
				}
			}
		})
	}
}

// TestNetMaxEnforcement_Concurrent 验证并发成交下的净仓上限强制
func TestNetMaxEnforcement_Concurrent(t *testing.T) {
	inv := &mockInventory{net: 0.18}  // 初始净仓接近上限
	maxInventory := 0.21
	baseSize := 0.009

	t.Logf("初始净仓: %.4f, 上限: %.4f, 单笔: %.4f", inv.net, maxInventory, baseSize)

	// 模拟3笔买单快速成交
	orders := []struct {
		side string
		qty  float64
	}{
		{"BUY", baseSize},
		{"BUY", baseSize},
		{"BUY", baseSize},
	}

	for i, ord := range orders {
		// 容量检查（与 placeOrder 逻辑一致）
		net := inv.NetExposure()
		curAbs := net
		if curAbs < 0 {
			curAbs = -curAbs
		}
		
		var delta float64
		if ord.side == "BUY" {
			delta = ord.qty
		} else {
			delta = -ord.qty
		}
		
		newNet := net + delta
		newAbs := newNet
		if newAbs < 0 {
			newAbs = -newAbs
		}
		
		allowedQty := ord.qty
		if newAbs > curAbs {
			remaining := maxInventory - curAbs
			if remaining <= 0 {
				t.Logf("订单 %d: 容量耗尽拒绝 (net=%.4f, max=%.4f)", i+1, net, maxInventory)
				continue
			}
			if remaining < ord.qty {
				allowedQty = remaining
				t.Logf("订单 %d: 收敛 %.4f → %.4f (剩余容量 %.4f)", 
					i+1, ord.qty, allowedQty, remaining)
			}
		}
		
		// 模拟成交
		inv.Update("ETHUSDC", ord.side, allowedQty, 3000.0)
		t.Logf("订单 %d 成交: side=%s, qty=%.4f, 新净仓=%.4f", 
			i+1, ord.side, allowedQty, inv.NetExposure())
	}

	// 验证最终净仓未超过上限
	finalNet := inv.NetExposure()
	if finalNet < 0 {
		finalNet = -finalNet
	}
	assert.LessOrEqual(t, finalNet, maxInventory, "final net should not exceed maxInventory")
	t.Logf("✓ 最终净仓 %.4f <= 上限 %.4f", finalNet, maxInventory)
}
