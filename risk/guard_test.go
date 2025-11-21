package risk

import "testing"

type stubGuard struct {
	err error
}

func (s stubGuard) PreOrder(symbol string, deltaQty float64) error {
	return s.err
}

func TestMultiGuard(t *testing.T) {
	g := MultiGuard{
		Guards: []Guard{
			stubGuard{},                      // pass
			stubGuard{err: ErrSpreadTooWide}, // fail
		},
	}
	if err := g.PreOrder("BTCUSDT", 1); err == nil {
		t.Fatalf("expected error")
	}
}
