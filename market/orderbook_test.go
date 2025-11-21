package market

import "testing"

func TestOrderBookApplyAndMid(t *testing.T) {
	ob := NewOrderBook()
	ob.ApplyDelta(map[float64]float64{100: 1, 99.5: 2}, map[float64]float64{101: 1.5, 102: 3})
	bid, ask := ob.Best()
	if bid != 100 || ask != 101 {
		t.Fatalf("unexpected best bid/ask: %f/%f", bid, ask)
	}
	if mid := ob.Mid(); mid != 100.5 {
		t.Fatalf("unexpected mid %f", mid)
	}
	// 删除一档
	ob.ApplyDelta(map[float64]float64{100: 0}, map[float64]float64{})
	bid, _ = ob.Best()
	if bid != 99.5 {
		t.Fatalf("expected best bid 99.5 got %f", bid)
	}
}
