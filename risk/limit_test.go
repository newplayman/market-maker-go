package risk

import "testing"

type stubInv struct{ net float64 }

func (s stubInv) NetExposure(symbol string) float64 { return s.net }

func TestLimitChecker(t *testing.T) {
	cfg := &Limits{
		SingleMax: 100,
		DailyMax:  200,
		NetMax:    150,
	}
	lc := NewLimitChecker(cfg, stubInv{net: 0})

	if err := lc.PreOrder("BTCUSDT", 50); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if err := lc.PreOrder("BTCUSDT", 120); err == nil {
		t.Fatalf("expected single exceed")
	}

	lc.dayVol["BTCUSDT"] = 190
	if err := lc.PreOrder("BTCUSDT", 20); err == nil {
		t.Fatalf("expected daily exceed")
	}

	lc.inv = stubInv{net: 150}
	if err := lc.PreOrder("BTCUSDT", 10); err == nil {
		t.Fatalf("expected net exceed")
	}
}
