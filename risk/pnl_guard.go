package risk

// PnLGuard 用于根据浮动盈亏阈值触发保护。
type PnLGuard struct {
	MinPnL float64 // 允许的最小浮盈（如果亏损超过该值则拒单），负数表示亏损阈值
	MaxPnL float64 // 允许的最大浮盈（预留，可用于止盈锁仓），0 表示不限制
	Source PnLSource
}

// PnLSource 提供当前浮盈。
type PnLSource interface {
	CurrentPnL(symbol string) float64
}

func (g *PnLGuard) PreOrder(symbol string, deltaQty float64) error {
	if g == nil || g.Source == nil {
		return nil
	}
	pnl := g.Source.CurrentPnL(symbol)
	if g.MinPnL != 0 && pnl < g.MinPnL {
		return ErrPnLTooLow
	}
	if g.MaxPnL > 0 && pnl > g.MaxPnL {
		return ErrPnLTooHigh
	}
	return nil
}
