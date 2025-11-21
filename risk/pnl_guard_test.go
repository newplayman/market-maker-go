package risk

import "testing"

type stubPnL struct{ v float64 }

func (s stubPnL) CurrentPnL(symbol string) float64 { return s.v }

func TestPnLGuard(t *testing.T) {
	g := &PnLGuard{MinPnL: -10, MaxPnL: 100, Source: stubPnL{v: -5}}
	if err := g.PreOrder("BTC", 1); err != nil {
		t.Fatalf("expected allowed, got %v", err)
	}
	g.Source = stubPnL{v: -20}
	if err := g.PreOrder("BTC", 1); err == nil {
		t.Fatalf("expected pnl too low")
	}
	g.Source = stubPnL{v: 200}
	if err := g.PreOrder("BTC", 1); err == nil {
		t.Fatalf("expected pnl too high")
	}
}
