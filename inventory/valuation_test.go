package inventory

import "testing"

func TestValuation(t *testing.T) {
	var tr Tracker
	tr.Update(1, 100)
	_, pnl := tr.Valuation(110)
	if pnl <= 0 {
		t.Fatalf("expected positive pnl")
	}
}
