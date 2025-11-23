package risk

import "time"

// LatencyGuard 用于限制订单频率或快速重复下单。
type LatencyGuard struct {
	MinInterval time.Duration
	lastBuyTS   time.Time
	lastSellTS  time.Time
	clock       Clock
}

func NewLatencyGuard(minInterval time.Duration) *LatencyGuard {
	return &LatencyGuard{
		MinInterval: minInterval,
		clock:       NowUTC,
	}
}

func (g *LatencyGuard) PreOrder(symbol string, deltaQty float64) error {
	if g == nil || g.MinInterval <= 0 {
		return nil
	}
	now := g.clock.Now()
	target := &g.lastBuyTS
	if deltaQty < 0 {
		target = &g.lastSellTS
	}
	if !target.IsZero() && now.Sub(*target) < g.MinInterval {
		return ErrTooFrequent
	}
	*target = now
	return nil
}
