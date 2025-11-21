package market

import (
	"testing"
	"time"
)

func TestKlineAggregator(t *testing.T) {
	agg := NewKlineAggregator(time.Minute)
	ts := time.Unix(0, 0)
	if closed := agg.OnTrade(100, 1, ts); closed != nil {
		t.Fatalf("should not close on first trade")
	}
	agg.OnTrade(102, 1, ts.Add(10*time.Second))
	agg.OnTrade(99, 1, ts.Add(20*time.Second))
	closed := agg.OnTrade(101, 1, ts.Add(70*time.Second))
	if closed == nil {
		t.Fatalf("expected kline close")
	}
	if closed.Open != 100 || closed.High != 102 || closed.Low != 99 || closed.Close != 101 {
		t.Fatalf("unexpected kline %+v", closed)
	}
}
