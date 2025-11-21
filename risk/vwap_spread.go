package risk

import "market-maker-go/market"

// VWAPGuard 基于 VWAP 价差做风控：若当前价差超过阈值则拒单。
type VWAPGuard struct {
	MaxSpreadRatio float64 // max allowed spread ratio (bid-ask)/mid
	Book           *market.OrderBook
}

func (g *VWAPGuard) PreOrder(symbol string, deltaQty float64) error {
	if g == nil || g.Book == nil || g.MaxSpreadRatio <= 0 {
		return nil
	}
	bid, ask := g.Book.Best()
	if bid == 0 || ask == 0 {
		return nil
	}
	mid := (bid + ask) / 2
	spread := (ask - bid) / mid
	if spread > g.MaxSpreadRatio {
		return ErrSpreadTooWide
	}
	return nil
}
