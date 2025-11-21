package strategy

import (
	"testing"
	"time"
)

type fakeInv struct{ net float64 }

func (f fakeInv) NetExposure() float64 { return f.net }

func TestNewEngine_Invalid(t *testing.T) {
	_, err := NewEngine(EngineConfig{})
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func TestQuoteZeroInventory_ShiftsWithDrift(t *testing.T) {
	engine, err := NewEngine(EngineConfig{
		MinSpread:      0.001, // 10 bps
		TargetPosition: 0,
		MaxDrift:       1,
		BaseSize:       0.1,
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	shot := MarketSnapshot{Mid: 100, Ts: time.Now()}

	qNeutral := engine.QuoteZeroInventory(shot, fakeInv{net: 0})
	if qNeutral.Ask-qNeutral.Bid <= 0 {
		t.Fatalf("spread should be positive")
	}

	// 多头过多，期望降价（bid/ask 下移）
	qLong := engine.QuoteZeroInventory(shot, fakeInv{net: 2})
	if qLong.Bid >= qNeutral.Bid {
		t.Fatalf("expected bid lower when long drift")
	}

	// 空头过多，期望抬价
	qShort := engine.QuoteZeroInventory(shot, fakeInv{net: -2})
	if qShort.Bid <= qNeutral.Bid {
		t.Fatalf("expected bid higher when short drift")
	}
}

func TestBacktestUpdate(t *testing.T) {
	engine, _ := NewEngine(EngineConfig{
		MinSpread:      0.001,
		TargetPosition: 0,
		MaxDrift:       1,
		BaseSize:       0.1,
	})
	snaps := []MarketSnapshot{
		{Mid: 100}, {Mid: 101}, {Mid: 99},
	}
	invs := []float64{0, 1, -1}
	quotes := engine.BacktestUpdate(snaps, invs)
	if len(quotes) != len(snaps) {
		t.Fatalf("expected quotes len %d got %d", len(snaps), len(quotes))
	}
	if quotes[0].Bid == 0 || quotes[2].Ask == 0 {
		t.Fatalf("unexpected quotes: %+v", quotes)
	}
}
