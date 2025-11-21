package risk

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrSingleExceed  = errors.New("single order exceed")
	ErrDailyExceed   = errors.New("daily volume exceed")
	ErrNetExceed     = errors.New("net exposure exceed")
	ErrSpreadTooWide = errors.New("spread too wide")
	ErrTooFrequent   = errors.New("too frequent order")
	ErrPnLTooLow     = errors.New("pnl too low")
	ErrPnLTooHigh    = errors.New("pnl too high")
)

// Limits 配置。
type Limits struct {
	SingleMax float64
	DailyMax  float64
	NetMax    float64
}

// Inventory 提供净仓位。
type Inventory interface {
	NetExposure(symbol string) float64
}

// LimitChecker 维护日累计成交量与净敞口校验。
type LimitChecker struct {
	cfg      *Limits
	inv      Inventory
	dayVol   map[string]float64
	dayReset time.Time
	clock    Clock
}

func NewLimitChecker(cfg *Limits, inv Inventory) *LimitChecker {
	return &LimitChecker{
		cfg:      cfg,
		inv:      inv,
		dayVol:   make(map[string]float64),
		dayReset: NowUTC.Now(),
		clock:    NowUTC,
	}
}

// PreOrder 校验下单前约束；deltaQty 为本次下单数量（正买负卖），以基准计价。
func (lc *LimitChecker) PreOrder(symbol string, deltaQty float64) error {
	if lc.cfg == nil {
		return errors.New("limits not configured")
	}
	now := lc.clock.Now()
	if now.Sub(lc.dayReset) > 24*time.Hour {
		lc.dayVol = make(map[string]float64)
		lc.dayReset = now
	}

	absQty := abs(deltaQty)
	if lc.cfg.SingleMax > 0 && absQty > lc.cfg.SingleMax {
		return fmt.Errorf("%w: %.2f > single %.2f", ErrSingleExceed, absQty, lc.cfg.SingleMax)
	}
	// 日累计
	lc.dayVol[symbol] += absQty
	if lc.cfg.DailyMax > 0 && lc.dayVol[symbol] > lc.cfg.DailyMax {
		return fmt.Errorf("%w: %.2f > daily %.2f", ErrDailyExceed, lc.dayVol[symbol], lc.cfg.DailyMax)
	}
	// 净敞口
	if lc.inv != nil && lc.cfg.NetMax > 0 {
		net := lc.inv.NetExposure(symbol) + deltaQty
		if abs(net) > lc.cfg.NetMax {
			return fmt.Errorf("%w: %.2f > net %.2f", ErrNetExceed, net, lc.cfg.NetMax)
		}
	}
	return nil
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
