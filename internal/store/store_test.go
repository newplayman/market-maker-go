package store

import (
	"testing"

	"market-maker-go/gateway"
)

func TestHandleOrderUpdateDeduplicatesByTimestamp(t *testing.T) {
	st := New("ETHUSDC", 0.3, nil)

	st.HandleOrderUpdate(gateway.OrderUpdate{
		Symbol:         "ETHUSDC",
		Side:           "BUY",
		OrderID:        1,
		OrigQty:        1.5,
		AccumulatedQty: 0.2,
		UpdateTime:     1_000,
	})
	if got := st.PendingBuySize(); got < 1.29 || got > 1.31 {
		t.Fatalf("expected ~1.3 pending buy, got %.4f", got)
	}

	// Older update should be ignored
	st.HandleOrderUpdate(gateway.OrderUpdate{
		Symbol:         "ETHUSDC",
		Side:           "BUY",
		OrderID:        1,
		OrigQty:        1.5,
		AccumulatedQty: 1,
		UpdateTime:     900,
	})
	if got := st.PendingBuySize(); got < 1.29 || got > 1.31 {
		t.Fatalf("older update mutated pending size: %.4f", got)
	}

	// Newer update zeroes out the order
	st.HandleOrderUpdate(gateway.OrderUpdate{
		Symbol:         "ETHUSDC",
		Side:           "BUY",
		OrderID:        1,
		OrigQty:        1.5,
		AccumulatedQty: 1.5,
		UpdateTime:     1_100,
	})
	if got := st.PendingBuySize(); got != 0 {
		t.Fatalf("expected pending exposure cleared, got %.4f", got)
	}
}

func TestReplacePendingOrdersResetsState(t *testing.T) {
	st := New("ETHUSDC", 0.5, nil)

	initial := []gateway.OrderUpdate{
		{Symbol: "ETHUSDC", Side: "BUY", OrderID: 10, OrigQty: 2, AccumulatedQty: 0.5, UpdateTime: 100},
		{Symbol: "ETHUSDC", Side: "SELL", OrderID: 11, OrigQty: 1.7, AccumulatedQty: 0.2, UpdateTime: 120},
		{Symbol: "BTCUSDT", Side: "SELL", OrderID: 12, OrigQty: 5, AccumulatedQty: 0},
	}

	st.ReplacePendingOrders(initial)

	if got := st.PendingBuySize(); got < 1.49 || got > 1.51 {
		t.Fatalf("unexpected pending buy %.4f", got)
	}
	if got := st.PendingSellSize(); got < 1.49 || got > 1.51 {
		t.Fatalf("unexpected pending sell %.4f", got)
	}
}
