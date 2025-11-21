package gateway

import (
	"log"
	"time"

	"market-maker-go/market"
)

// BinanceWSHandler 解析 depth combined 消息，更新 orderbook 并向 MarketService 推送。
type BinanceWSHandler struct {
	Book *market.OrderBook
	Svc  *market.Service
}

func (h *BinanceWSHandler) OnDepth(symbol string, bid, ask float64) {
	if h.Svc != nil {
		h.Svc.OnDepth(symbol, bid, ask, time.Now().UTC())
	}
	if h.Book != nil {
		h.Book.ApplyDelta(map[float64]float64{bid: 1}, map[float64]float64{ask: 1})
	}
}

func (h *BinanceWSHandler) OnTrade(symbol string, price, qty float64) {
	if h.Svc != nil {
		h.Svc.OnTrade(symbol, price, qty, time.Now().UTC())
	}
}

// OnRawMessage 可供外部调用，直接传入 ws 原始消息。
func (h *BinanceWSHandler) OnRawMessage(msg []byte) {
	sym, bid, ask, err := ParseCombinedDepth(msg)
	if err != nil {
		log.Printf("parse depth msg err: %v", err)
		return
	}
	h.OnDepth(sym, bid, ask)
}
