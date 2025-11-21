package inventory

// Valuation 基于当前 mid 价计算未实现盈亏。
func (t *Tracker) Valuation(mid float64) (net float64, pnl float64) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	net = t.net
	pnl = (mid - t.cost) * t.net
	return
}
