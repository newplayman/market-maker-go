package market

import "testing"

func TestTradeStruct(t *testing.T) {
	tr := Trade{Price: 10, Qty: 2}
	if tr.Price <= 0 {
		t.Fatalf("unexpected trade %+v", tr)
	}
}
