package market

import (
	"testing"
	"time"
)

func TestServiceMidAndStaleness(t *testing.T) {
	svc := NewService(nil)
	svc.OnDepth("BTCUSDT", 100, 101, time.Now())
	if mid := svc.Mid("BTCUSDT"); mid != 100.5 {
		t.Fatalf("unexpected mid %f", mid)
	}
	if st := svc.Staleness("BTCUSDT"); st <= 0 {
		t.Fatalf("expected positive staleness")
	}
}

func TestServiceTradePublish(t *testing.T) {
	pub := NewPublisher()
	trCh := pub.SubscribeTrade()
	svc := NewService(pub)
	svc.OnTrade("BTCUSDT", 100, 1, time.Now())
	select {
	case tr := <-trCh:
		if tr.Price != 100 || tr.Qty != 1 {
			t.Fatalf("unexpected trade %+v", tr)
		}
	default:
		t.Fatalf("expected trade published")
	}
}
