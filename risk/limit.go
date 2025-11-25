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

// LimitChecker implements risk guard using static limits.
type LimitChecker struct {
	limits Limits
	pos    PositionKeeper
	mu     sync.Mutex
}

// NewLimitChecker creates a limit checker with given limits.
func NewLimitChecker(limits *Limits, pos PositionKeeper) *LimitChecker {
	return &LimitChecker{
		limits: *limits,
		pos:    pos,
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
	if l.limits.DailyMax > 0 {
		daily := l.pos.GetDailyFilled(symbol)
		if daily+abs(deltaQty) > l.limits.DailyMax {
			return fmt.Errorf("daily limit exceeded: %.4f + %.4f > %.4f", daily, abs(deltaQty), l.limits.DailyMax)
		}
	}

	// Net exposure limit
	if l.limits.NetMax > 0 {
		net := l.pos.NetExposure()
		newNet := net + deltaQty
		if abs(newNet) > l.limits.NetMax {
			return fmt.Errorf("net exposure limit exceeded: |%.4f + %.4f| = %.4f > %.4f", net, deltaQty, abs(newNet), l.limits.NetMax)
		}
	}

	return nil
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

