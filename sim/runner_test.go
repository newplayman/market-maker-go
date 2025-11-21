package sim

import (
	"fmt"
	"testing"

	"market-maker-go/inventory"
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
	guard := risk.NewLimitChecker(&risk.Limits{SingleMax: 2, DailyMax: 10, NetMax: 5}, nil)
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
