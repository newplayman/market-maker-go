package risk

import (
	"testing"

	"market-maker-go/market"
)

func TestVWAPGuard(t *testing.T) {
	ob := market.NewOrderBook()
	ob.ApplyDelta(map[float64]float64{100: 1}, map[float64]float64{101: 1})
	g := &VWAPGuard{MaxSpreadRatio: 0.03, Book: ob} // 3%
	if err := g.PreOrder("BTCUSDT", 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 拉宽价差
	ob.ApplyDelta(map[float64]float64{100: 0, 90: 1}, map[float64]float64{101: 0, 110: 1})
	if err := g.PreOrder("BTCUSDT", 1); err == nil {
		t.Fatalf("expected spread too wide")
	}
}
