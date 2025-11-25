package metrics

import (
	"testing"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMarketMetrics(t *testing.T) {
	// Reset metrics to initial state
	VPINCurrent.Set(0)
	VolatilityRegime.Set(0)
	InventoryNet.Set(0)

	// Update metrics
	UpdateMarketMetrics(0.5, 2, 3.0)

	// Check metrics
	if testutil.ToFloat64(VPINCurrent) != 0.5 {
		t.Errorf("Expected VPINCurrent to be 0.5, got %f", testutil.ToFloat64(VPINCurrent))
	}

	if testutil.ToFloat64(VolatilityRegime) != 2 {
		t.Errorf("Expected VolatilityRegime to be 2, got %f", testutil.ToFloat64(VolatilityRegime))
	}

	if testutil.ToFloat64(InventoryNet) != 3.0 {
		t.Errorf("Expected InventoryNet to be 3.0, got %f", testutil.ToFloat64(InventoryNet))
	}
}

func TestStrategyMetrics(t *testing.T) {
	// Reset metrics to initial state
	ReservationPrice.Set(0)
	InventorySkewBps.Set(0)
	AdaptiveNetMax.Set(0)

	// Update metrics
	UpdateStrategyMetrics(100.5, 10.0, 5.0)

	// Check metrics
	if testutil.ToFloat64(ReservationPrice) != 100.5 {
		t.Errorf("Expected ReservationPrice to be 100.5, got %f", testutil.ToFloat64(ReservationPrice))
	}

	if testutil.ToFloat64(InventorySkewBps) != 10.0 {
		t.Errorf("Expected InventorySkewBps to be 10.0, got %f", testutil.ToFloat64(InventorySkewBps))
	}

	if testutil.ToFloat64(AdaptiveNetMax) != 5.0 {
		t.Errorf("Expected AdaptiveNetMax to be 5.0, got %f", testutil.ToFloat64(AdaptiveNetMax))
	}
}

func TestPostTradeMetrics(t *testing.T) {
	// Reset metrics to initial state
	AdverseSelectionRate.Set(0)

	// Update metrics
	UpdatePostTradeMetrics(0.3)

	// Check metrics
	if testutil.ToFloat64(AdverseSelectionRate) != 0.3 {
		t.Errorf("Expected AdverseSelectionRate to be 0.3, got %f", testutil.ToFloat64(AdverseSelectionRate))
	}
}

func TestIncrementFunctions(t *testing.T) {
	// Reset counters to initial state
	StrategyQuotesGenerated.Reset()
	StrategyOrdersPlaced.Reset()
	StrategyFills.Reset()

	// Increment counters
	IncrementQuotesGenerated("bid")
	IncrementQuotesGenerated("ask")
	IncrementOrdersPlaced("bid")
	IncrementFills("ask")

	// Check counters
	expectedQuotesBid := 1.0
	actualQuotesBid := testutil.ToFloat64(StrategyQuotesGenerated.WithLabelValues("bid"))
	if actualQuotesBid != expectedQuotesBid {
		t.Errorf("Expected StrategyQuotesGenerated[bid] to be %f, got %f", expectedQuotesBid, actualQuotesBid)
	}

	expectedQuotesAsk := 1.0
	actualQuotesAsk := testutil.ToFloat64(StrategyQuotesGenerated.WithLabelValues("ask"))
	if actualQuotesAsk != expectedQuotesAsk {
		t.Errorf("Expected StrategyQuotesGenerated[ask] to be %f, got %f", expectedQuotesAsk, actualQuotesAsk)
	}

	expectedOrdersBid := 1.0
	actualOrdersBid := testutil.ToFloat64(StrategyOrdersPlaced.WithLabelValues("bid"))
	if actualOrdersBid != expectedOrdersBid {
		t.Errorf("Expected StrategyOrdersPlaced[bid] to be %f, got %f", expectedOrdersBid, actualOrdersBid)
	}

	expectedFillsAsk := 1.0
	actualFillsAsk := testutil.ToFloat64(StrategyFills.WithLabelValues("ask"))
	if actualFillsAsk != expectedFillsAsk {
		t.Errorf("Expected StrategyFills[ask] to be %f, got %f", expectedFillsAsk, actualFillsAsk)
	}
}