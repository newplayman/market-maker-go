package risk

import (
	"fmt"
	"sync"
)

// Limits holds risk exposure limits.
type Limits struct {
	SingleMax float64 // 单笔最大下单量
	DailyMax  float64 // 每日最大下单量
	NetMax    float64 // 净持仓上限
}

// PositionKeeper 跟踪仓位和成交量。
type PositionKeeper interface {
	NetExposure() float64
	AddFilled(symbol string, qty float64)
	GetDailyFilled(symbol string) float64
}

// PendingExposureProvider 暴露当前挂单尺寸（用于最坏敞口评估）
type PendingExposureProvider interface {
	PendingBuySize() float64
	PendingSellSize() float64
}

// LimitChecker implements risk guard using static limits.
type LimitChecker struct {
	limits  Limits
	pos     PositionKeeper
	pending PendingExposureProvider
	mu      sync.Mutex
}

// NewLimitChecker creates a limit checker with given limits.
func NewLimitChecker(limits *Limits, pos PositionKeeper, pending PendingExposureProvider) *LimitChecker {
	return &LimitChecker{
		limits:  *limits,
		pos:     pos,
		pending: pending,
	}
}

// PreOrder checks if an order complies with risk limits.
func (l *LimitChecker) PreOrder(symbol string, deltaQty float64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Single order limit
	if l.limits.SingleMax > 0 && abs(deltaQty) > l.limits.SingleMax {
		return fmt.Errorf("single order limit exceeded: %.4f > %.4f", abs(deltaQty), l.limits.SingleMax)
	}

	// Daily limit
	if l.limits.DailyMax > 0 && l.pos != nil {
		daily := l.pos.GetDailyFilled(symbol)
		if daily+abs(deltaQty) > l.limits.DailyMax {
			return fmt.Errorf("daily limit exceeded: %.4f + %.4f > %.4f", daily, abs(deltaQty), l.limits.DailyMax)
		}
	}

	// Net exposure limit
	if l.limits.NetMax > 0 {
		net := 0.0
		if l.pos != nil {
			net = l.pos.NetExposure()
		}
		pBuy, pSell := l.pendingExposure()
		if deltaQty > 0 {
			worstLong := net + pBuy + deltaQty
			if worstLong > l.limits.NetMax {
				return fmt.Errorf("worst-case long exposure exceeded: net=%.4f pending=%.4f new=%.4f limit=%.4f",
					net, pBuy, deltaQty, l.limits.NetMax)
			}
		} else if deltaQty < 0 {
			worstShort := net - pSell + deltaQty
			if abs(worstShort) > l.limits.NetMax {
				return fmt.Errorf("worst-case short exposure exceeded: net=%.4f pending=%.4f new=%.4f limit=%.4f",
					net, pSell, deltaQty, l.limits.NetMax)
			}
		} else {
			if net+pBuy > l.limits.NetMax || abs(net-pSell) > l.limits.NetMax {
				return fmt.Errorf("pending exposure already exceeds limit: long=%.4f short=%.4f limit=%.4f",
					net+pBuy, abs(net-pSell), l.limits.NetMax)
			}
		}
	}

	return nil
}

func (l *LimitChecker) pendingExposure() (float64, float64) {
	if l.pending == nil {
		return 0, 0
	}
	return l.pending.PendingBuySize(), l.pending.PendingSellSize()
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
