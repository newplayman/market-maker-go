package market

import "testing"

func TestKlineStruct(t *testing.T) {
	k := Kline{Open: 1, High: 2, Low: 0.5, Close: 1.5}
	if k.High < k.Open {
		t.Fatalf("unexpected kline values: %+v", k)
	}
}
