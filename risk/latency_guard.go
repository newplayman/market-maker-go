package risk

import "time"

// LatencyGuard 用于限制订单频率或快速重复下单。
type LatencyGuard struct {
	MinInterval time.Duration
	lastTS      time.Time
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
	if !g.lastTS.IsZero() && now.Sub(g.lastTS) < g.MinInterval {
		return ErrTooFrequent
	}
	g.lastTS = now
	return nil
}
