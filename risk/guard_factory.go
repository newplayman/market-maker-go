package risk

import "market-maker-go/market"

// BuildGuards 方便组装常用的风控组合。
func BuildGuards(limitCfg *Limits, inv Inventory, maxSpreadRatio float64, ob *market.OrderBook, pnlGuard Guard, freqGuard Guard) Guard {
	var guards []Guard
	if limitCfg != nil {
		guards = append(guards, NewLimitChecker(limitCfg, inv))
	}
	if maxSpreadRatio > 0 && ob != nil {
		guards = append(guards, &VWAPGuard{MaxSpreadRatio: maxSpreadRatio, Book: ob})
	}
	if pnlGuard != nil {
		guards = append(guards, pnlGuard)
	}
	if freqGuard != nil {
		guards = append(guards, freqGuard)
	}
	return MultiGuard{Guards: guards}
}
