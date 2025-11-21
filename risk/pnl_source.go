package risk

import "market-maker-go/inventory"

// InventoryPnL 可从 inventory.Tracker 估算浮盈。
type InventoryPnL struct {
	Tracker *inventory.Tracker
	MidFn   func() float64 // 返回当前 mid 价
}

func (p InventoryPnL) CurrentPnL(symbol string) float64 {
	if p.Tracker == nil || p.MidFn == nil {
		return 0
	}
	_, pnl := p.Tracker.Valuation(p.MidFn())
	return pnl
}
