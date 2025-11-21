package sim

import (
	"errors"
	"time"

	"market-maker-go/inventory"
	"market-maker-go/market"
	"market-maker-go/order"
	"market-maker-go/strategy"
)

// RiskGuard 用于下单前校验（可对接风险控制）。
type RiskGuard interface {
	PreOrder(symbol string, deltaQty float64) error
}

// Runner 将行情->策略->下单串起来（简化版，不含真实 gateway/风控）。
type Runner struct {
	Symbol   string
	Engine   *strategy.Engine
	Inv      *inventory.Tracker
	OrderMgr *order.Manager
	Risk     RiskGuard
	Book     *market.OrderBook // 可选，供 VWAPGuard 使用
}

// OnTick 接收中间价，生成报价并下发双边限价单（挂单）。
func (r *Runner) OnTick(mid float64) error {
	if r.Engine == nil || r.OrderMgr == nil || r.Inv == nil {
		return errors.New("runner not initialized")
	}
	if mid <= 0 {
		return errors.New("invalid mid")
	}
	snap := strategy.MarketSnapshot{Mid: mid, Ts: time.Now()}
	quote := r.Engine.QuoteZeroInventory(snap, invWrapper{r.Inv})

	size := quote.Size
	if size <= 0 {
		return errors.New("invalid size")
	}

	// 买单
	if r.Risk != nil {
		if err := r.Risk.PreOrder(r.Symbol, size); err != nil {
			return err
		}
	}
	if _, err := r.OrderMgr.Submit(order.Order{
		Symbol:   r.Symbol,
		Side:     "BUY",
		Price:    quote.Bid,
		Quantity: size,
	}); err != nil {
		return err
	}

	// 卖单
	if r.Risk != nil {
		if err := r.Risk.PreOrder(r.Symbol, -size); err != nil {
			return err
		}
	}
	if _, err := r.OrderMgr.Submit(order.Order{
		Symbol:   r.Symbol,
		Side:     "SELL",
		Price:    quote.Ask,
		Quantity: size,
	}); err != nil {
		return err
	}
	return nil
}

type invWrapper struct {
	tr *inventory.Tracker
}

func (i invWrapper) NetExposure() float64 { return i.tr.NetExposure() }
