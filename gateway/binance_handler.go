package gateway

import (
	"time"

	"market-maker-go/market"
)

// MarketDataHandler 实现 WSHandler，将深度/成交推送给 market.Service。
type MarketDataHandler struct {
	Svc *market.Service
}

func (h *MarketDataHandler) OnDepth(symbol string, bid, ask float64) {
	if h.Svc != nil {
		h.Svc.OnDepth(symbol, bid, ask, time.Now().UTC())
	}
}

func (h *MarketDataHandler) OnTrade(symbol string, price, qty float64) {
	if h.Svc != nil {
		h.Svc.OnTrade(symbol, price, qty, time.Now().UTC())
	}
}
