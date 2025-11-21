package gateway

import "testing"

func TestParseCombinedDepth(t *testing.T) {
	raw := []byte(`{
		"stream":"btcusdt@depth20@100ms",
		"data":{
		  "s":"BTCUSDT",
		  "b":[["100.1","1.2"],["100.0","2"]],
		  "a":[["100.2","1.1"],["100.3","2.2"]]
		}
	}`)
	sym, bid, ask, err := ParseCombinedDepth(raw)
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if sym != "BTCUSDT" || bid != 100.1 || ask != 100.2 {
		t.Fatalf("unexpected parse result: %s %.3f %.3f", sym, bid, ask)
	}
}
