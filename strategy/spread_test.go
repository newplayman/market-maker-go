package strategy

import "testing"

func TestCalcDynamicSpread(t *testing.T) {
	sp := CalcDynamicSpread(100, 1, 0.5, 10) // 10 bps min
	if sp <= 0 {
		t.Fatalf("spread should be >0")
	}
	// 更大波动率应扩大 spread
	spHigh := CalcDynamicSpread(100, 1, 1.0, 10)
	if spHigh <= sp {
		t.Fatalf("expected larger spread with higher vol")
	}
}
