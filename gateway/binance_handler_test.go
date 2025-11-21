package gateway

import (
	"testing"

	"market-maker-go/market"
)

func TestMarketDataHandler(t *testing.T) {
	pub := market.NewPublisher()
	depthCh := pub.SubscribeDepth()
	tradeCh := pub.SubscribeTrade()
	h := &MarketDataHandler{Svc: market.NewService(pub)}
	ws := &BinanceWSStub{}
	_ = ws.SubscribeDepth("BTCUSDT")
	if err := ws.Run(h); err != nil {
		t.Fatalf("ws run err: %v", err)
	}
	<-depthCh
	<-tradeCh
}
