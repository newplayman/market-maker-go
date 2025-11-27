package sim

import (
	"fmt"
	"math"
	"testing"
	"time"

	"market-maker-go/inventory"
	"market-maker-go/market"
	"market-maker-go/order"
	"market-maker-go/risk"
	"market-maker-go/strategy"
)

type stubGateway struct{ orders []order.Order }

func (s *stubGateway) Place(o order.Order) (string, error) {
	s.orders = append(s.orders, o)
	return o.ID, nil
}
func (s *stubGateway) Cancel(orderID string) error { return nil }

type stubRisk struct{ reject bool }

func (s stubRisk) PreOrder(symbol string, deltaQty float64) error {
	if s.reject {
		return fmt.Errorf("rejected %s", symbol)
	}
	return nil
}

type flakyPostOnlyGateway struct {
	failUntil int
	calls     int
	orders    []order.Order
}

func (f *flakyPostOnlyGateway) Place(o order.Order) (string, error) {
	f.calls++
	f.orders = append(f.orders, o)
	if f.calls <= f.failUntil {
		return "", fmt.Errorf("code\":-5022 post only reject")
	}
	return fmt.Sprintf("ok-%d", f.calls), nil
}

func (f *flakyPostOnlyGateway) Cancel(orderID string) error { return nil }

func TestRunnerOnTickPlacesOrders(t *testing.T) {
	engine, _ := strategy.NewEngine(strategy.EngineConfig{
		MinSpread:      0.001,
		TargetPosition: 0,
		MaxDrift:       1,
		BaseSize:       0.5,
	})
	tr := &inventory.Tracker{}
	gw := &stubGateway{}
	mgr := order.NewManager(gw)

	r := Runner{
		Symbol:   "BTCUSDT",
		Engine:   engine,
		Inv:      tr,
		OrderMgr: mgr,
	}

	if err := r.OnTick(100); err != nil {
		t.Fatalf("on tick err: %v", err)
	}
	if len(gw.orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(gw.orders))
	}
}

func TestRunnerWithRisk(t *testing.T) {
	engine, _ := strategy.NewEngine(strategy.EngineConfig{
		MinSpread:      0.001,
		TargetPosition: 0,
		MaxDrift:       1,
		BaseSize:       1,
	})
	tr := &inventory.Tracker{}
	gw := &stubGateway{}
	mgr := order.NewManager(gw)
	guard := risk.NewLimitChecker(&risk.Limits{SingleMax: 2, DailyMax: 10, NetMax: 5}, nil, nil)
	r := Runner{
		Symbol:   "BTCUSDT",
		Engine:   engine,
		Inv:      tr,
		OrderMgr: mgr,
		Risk:     guard,
	}
	if err := r.OnTick(100); err != nil {
		t.Fatalf("unexpected risk err: %v", err)
	}
	// 超过 single max
	engine2, _ := strategy.NewEngine(strategy.EngineConfig{
		MinSpread:      0.001,
		TargetPosition: 0,
		MaxDrift:       1,
		BaseSize:       3, // > SingleMax
	})
	r.Engine = engine2
	if err := r.OnTick(100); err == nil {
		t.Fatalf("expected risk rejection")
	}
}

func TestRunnerAlignsToConstraints(t *testing.T) {
	engine, _ := strategy.NewEngine(strategy.EngineConfig{
		MinSpread:      0.0004,
		TargetPosition: 0,
		MaxDrift:       1,
		BaseSize:       0.0063,
	})
	tr := &inventory.Tracker{}
	gw := &stubGateway{}
	mgr := order.NewManager(gw)
	constraints := order.SymbolConstraints{
		TickSize:    0.1,
		StepSize:    0.001,
		MinQty:      0.005,
		MaxQty:      10,
		MinNotional: 20,
	}
	mgr.SetConstraints(map[string]order.SymbolConstraints{
		"ETHUSDC": constraints,
	})
	r := Runner{
		Symbol:      "ETHUSDC",
		Engine:      engine,
		Inv:         tr,
		OrderMgr:    mgr,
		Constraints: constraints,
	}
	if err := r.OnTick(2050.123); err != nil {
		t.Fatalf("runner on tick err: %v", err)
	}
	if len(gw.orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(gw.orders))
	}
	buy := gw.orders[0]
	sell := gw.orders[1]
	if !isAligned(buy.Price, constraints.TickSize) {
		t.Fatalf("bid not aligned: %.8f", buy.Price)
	}
	if !isAligned(sell.Price, constraints.TickSize) {
		t.Fatalf("ask not aligned: %.8f", sell.Price)
	}
	if sell.Price-buy.Price < constraints.TickSize-1e-8 {
		t.Fatalf("spread too small after snapping: bid=%.4f ask=%.4f", buy.Price, sell.Price)
	}
	if buy.Quantity < constraints.MinQty-1e-9 {
		t.Fatalf("qty < minQty: %.8f", buy.Quantity)
	}
	if buy.Price*buy.Quantity < constraints.MinNotional-1e-6 {
		t.Fatalf("buy notional < minNotional: %.4f", buy.Price*buy.Quantity)
	}
	if sell.Price*sell.Quantity < constraints.MinNotional-1e-6 {
		t.Fatalf("sell notional < minNotional: %.4f", sell.Price*sell.Quantity)
	}
}

func isAligned(val, step float64) bool {
	if step <= 0 {
		return true
	}
	ratio := val / step
	return math.Abs(ratio-math.Round(ratio)) < 1e-9
}

func TestRunnerReduceOnlyState(t *testing.T) {
	engine, _ := strategy.NewEngine(strategy.EngineConfig{
		MinSpread:      0.001,
		TargetPosition: 0,
		MaxDrift:       1,
		BaseSize:       0.5,
	})
	tr := &inventory.Tracker{}
	tr.SetExposure(2, 100)
	gw := &stubGateway{}
	mgr := order.NewManager(gw)
	r := Runner{
		Symbol:              "ETHUSDC",
		Engine:              engine,
		Inv:                 tr,
		OrderMgr:            mgr,
		ReduceOnlyThreshold: 1,
		NetMax:              5,
		Constraints:         order.SymbolConstraints{},
		HaltDuration:        time.Second,
		StopLoss:            -1000, // disable
		ShockThreshold:      0,
		BaseSpread:          0.001,
		BaseInterval:        300 * time.Millisecond,
		TakeProfitPct:       0,
		lastQuoteTime:       time.Time{},
		onRiskStateChange:   nil,
		onStrategyAdjust:    nil,
		riskState:           RiskStateNormal,
	}
	var state RiskState
	r.SetRiskStateListener(func(s RiskState, reason string) {
		state = s
	})
	if err := r.OnTick(120); err != nil {
		t.Fatalf("on tick err: %v", err)
	}
	if len(gw.orders) != 1 {
		t.Fatalf("expected only one order, got %d", len(gw.orders))
	}
	if gw.orders[0].Side != "SELL" {
		t.Fatalf("expected sell order, got %s", gw.orders[0].Side)
	}
	if state != RiskStateReduceOnly {
		t.Fatalf("expected reduce only state, got %s", state.String())
	}
}

func TestSubmitOrderWithFallbackKeepsMakerAfterRetries(t *testing.T) {
	gw := &flakyPostOnlyGateway{failUntil: 2}
	mgr := order.NewManager(gw)
	r := Runner{
		OrderMgr:    mgr,
		Constraints: order.SymbolConstraints{TickSize: 0.1},
	}
	ord := order.Order{
		Symbol:   "ETHUSDC",
		Side:     "BUY",
		Price:    100,
		Quantity: 0.01,
		PostOnly: true,
	}
	res, usedPostOnly, err := r.submitOrderWithFallback("BUY", ord, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !usedPostOnly {
		t.Fatalf("expected maker order, got taker")
	}
	if res == nil {
		t.Fatalf("nil response order")
	}
	expectedPrice := ord.Price - float64(gw.failUntil)*r.Constraints.TickSize
	if math.Abs(res.Price-expectedPrice) > 1e-9 {
		t.Fatalf("expected price %.2f, got %.2f", expectedPrice, res.Price)
	}
	if len(gw.orders) != gw.failUntil+1 {
		t.Fatalf("expected %d attempts, got %d", gw.failUntil+1, len(gw.orders))
	}
}

func TestSubmitOrderWithFallbackEventuallyConvertsToTaker(t *testing.T) {
	gw := &flakyPostOnlyGateway{failUntil: 4}
	mgr := order.NewManager(gw)
	r := Runner{
		OrderMgr:    mgr,
		Constraints: order.SymbolConstraints{TickSize: 0.1},
	}
	ord := order.Order{
		Symbol:   "ETHUSDC",
		Side:     "BUY",
		Price:    100,
		Quantity: 0.01,
		PostOnly: true,
	}
	res, usedPostOnly, err := r.submitOrderWithFallback("BUY", ord, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usedPostOnly {
		t.Fatalf("expected taker fallback")
	}
	if res == nil || res.PostOnly {
		t.Fatalf("expected fallback order to be non-post-only")
	}
	if len(gw.orders) != gw.failUntil+1 {
		t.Fatalf("expected %d attempts, got %d", gw.failUntil+1, len(gw.orders))
	}
}

func TestSubmitOrderWithFallbackStopsWithoutTakerOutsideReduceOnly(t *testing.T) {
	gw := &flakyPostOnlyGateway{failUntil: 5}
	mgr := order.NewManager(gw)
	r := Runner{
		OrderMgr:    mgr,
		Constraints: order.SymbolConstraints{TickSize: 0.1},
	}
	ord := order.Order{
		Symbol:   "ETHUSDC",
		Side:     "BUY",
		Price:    100,
		Quantity: 0.01,
		PostOnly: true,
	}
	res, usedPostOnly, err := r.submitOrderWithFallback("BUY", ord, false)
	if err == nil {
		t.Fatalf("expected error when taker fallback disabled")
	}
	if res != nil {
		t.Fatalf("expected nil result when submission fails")
	}
	if !usedPostOnly {
		t.Fatalf("should remain post-only when fallback disabled")
	}
	if len(gw.orders) != 4 {
		t.Fatalf("expected 4 attempts (max retries), got %d", len(gw.orders))
	}
}

func TestRunnerStopLossTriggersHalt(t *testing.T) {
	engine, _ := strategy.NewEngine(strategy.EngineConfig{
		MinSpread:      0.001,
		TargetPosition: 0,
		MaxDrift:       1,
		BaseSize:       0.5,
	})
	tr := &inventory.Tracker{}
	tr.SetExposure(1, 110)
	gw := &stubGateway{}
	mgr := order.NewManager(gw)
	r := Runner{
		Symbol:       "ETHUSDC",
		Engine:       engine,
		Inv:          tr,
		OrderMgr:     mgr,
		StopLoss:     -1,
		HaltDuration: time.Second,
		BaseSpread:   0.001,
		BaseInterval: time.Second,
		NetMax:       5,
		Constraints:  order.SymbolConstraints{},
		riskState:    RiskStateNormal,
	}
	var state RiskState
	r.SetRiskStateListener(func(s RiskState, reason string) {
		state = s
	})
	if err := r.OnTick(100); err == nil {
		t.Fatalf("expected stop loss error")
	}
	if len(gw.orders) != 0 {
		t.Fatalf("expected no orders when halted")
	}
	if state != RiskStateHalted {
		t.Fatalf("expected halted state, got %s", state.String())
	}
	if r.ReadyForNext(100) {
		t.Fatalf("should not be ready while halt active")
	}
}

func TestRunnerStrategyAdjustCallback(t *testing.T) {
	engine, _ := strategy.NewEngine(strategy.EngineConfig{
		MinSpread:      0.001,
		TargetPosition: 0,
		MaxDrift:       1,
		BaseSize:       0.5,
	})
	tr := &inventory.Tracker{}
	gw := &stubGateway{}
	mgr := order.NewManager(gw)
	r := Runner{
		Symbol:       "ETHUSDC",
		Engine:       engine,
		Inv:          tr,
		OrderMgr:     mgr,
		BaseSpread:   0.001,
		BaseInterval: 400 * time.Millisecond,
		NetMax:       5,
	}
	var info StrategyAdjustInfo
	r.SetStrategyAdjustListener(func(si StrategyAdjustInfo) {
		info = si
	})
	if err := r.OnTick(2000); err != nil {
		t.Fatalf("on tick err: %v", err)
	}
	if info.Spread <= 0 {
		t.Fatalf("expected positive spread, got %.6f", info.Spread)
	}
	if info.SpreadRatio <= 0 {
		t.Fatalf("expected positive spread ratio, got %.6f", info.SpreadRatio)
	}
	if info.Interval <= 0 {
		t.Fatalf("expected interval > 0")
	}
	if info.Mid != 2000 {
		t.Fatalf("unexpected mid %.2f", info.Mid)
	}
	if info.ReduceOnly {
		t.Fatalf("should not be reduce only at zero inventory")
	}
	if len(gw.orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(gw.orders))
	}
}

func TestReduceOnlyKeepsOrderWhenProfitable(t *testing.T) {
	engine, _ := strategy.NewEngine(strategy.EngineConfig{
		MinSpread:      0.0008,
		TargetPosition: 0,
		MaxDrift:       1,
		BaseSize:       0.01,
	})
	tr := &inventory.Tracker{}
	tr.SetExposure(0.02, 2000)
	gw := &stubGateway{}
	mgr := order.NewManager(gw)
	book := market.NewOrderBook()
	book.ApplyDelta(map[float64]float64{2049.5: 5}, map[float64]float64{2050.5: 5})
	r := Runner{
		Symbol:                "ETHUSDC",
		Engine:                engine,
		Inv:                   tr,
		OrderMgr:              mgr,
		Book:                  book,
		Constraints:           order.SymbolConstraints{TickSize: 0.1, StepSize: 0.001, MinQty: 0.001},
		BaseSpread:            0.0008,
		BaseInterval:          200 * time.Millisecond,
		NetMax:                5,
		ReduceOnlyThreshold:   0.005,
		ReduceOnlyMaxSlippage: 0.002,
	}
	if err := r.OnTick(2050); err != nil {
		t.Fatalf("reduce-only on tick err: %v", err)
	}
	if len(gw.orders) != 1 {
		t.Fatalf("expected 1 order in reduce-only, got %d", len(gw.orders))
	}
	if err := r.OnTick(2051); err != nil {
		t.Fatalf("second tick err: %v", err)
	}
	if len(gw.orders) == 0 {
		t.Fatalf("expected reduce-only order to exist")
	}
	if len(gw.orders) > 2 {
		t.Fatalf("expected <= 2 orders after drift, got %d", len(gw.orders))
	}
}

func TestPlanReduceOnlyUsesBestOpposite(t *testing.T) {
	r := Runner{
		Book:                  market.NewOrderBook(),
		ReduceOnlyMaxSlippage: 0.01,
	}
	r.Book.ApplyDelta(map[float64]float64{99: 5, 98.5: 3}, map[float64]float64{101: 4, 101.5: 6})
	plan := r.planReduceOnlyPrice(true, 100, 98, 1)
	if plan.price < 101 {
		t.Fatalf("expected plan price >= best ask, got %.2f", plan.price)
	}
	planSell := r.planReduceOnlyPrice(false, 100, 102, 1)
	if planSell.price > 99 {
		t.Fatalf("expected plan price <= best bid, got %.2f", planSell.price)
	}
}
