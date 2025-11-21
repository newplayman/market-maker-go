package market

// Depth 保存简单的 bid/ask 价格。
type Depth struct {
	Bid float64
	Ask float64
}

// Update 使用增量更新 bid/ask。
func (d *Depth) Update(bid, ask float64) {
	if bid > 0 {
		d.Bid = bid
	}
	if ask > 0 {
		d.Ask = ask
	}
}
