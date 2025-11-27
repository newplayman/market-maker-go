package asmm

import (
	"market-maker-go/market"
	"testing"
	"time"
)

func TestFullASMMStrategy(t *testing.T) {
	// Create strategy with default config
	config := DefaultASMMConfig()
	strategy := NewASMMStrategy(config)

	// Create market snapshot
	snapshot := market.Snapshot{
		Mid:       100.0,
		Timestamp: time.Now().Unix(),
		VPIN:      0.0, // Low VPIN for normal conditions
	}

	// Generate quotes with zero inventory
	quotes := strategy.Quote(snapshot, 0.0)

	// Check that we got quotes
	if len(quotes) == 0 {
		t.Error("Expected quotes to be generated")
	}

	// Check that we have both bid and ask quotes
	bidCount := 0
	askCount := 0
	for _, quote := range quotes {
		if quote.Side == Bid {
			bidCount++
		} else if quote.Side == Ask {
			askCount++
		}
	}

	if bidCount == 0 {
		t.Error("Expected bid quotes to be generated")
	}

	if askCount == 0 {
		t.Error("Expected ask quotes to be generated")
	}

	// Test with small positive position (within limits)
	quotes2 := strategy.Quote(snapshot, 1.0)
	if len(quotes2) == 0 {
		t.Error("Expected quotes to be generated with small positive position")
	}

	// Test with small negative position (within limits)
	quotes3 := strategy.Quote(snapshot, -1.0)
	if len(quotes3) == 0 {
		t.Error("Expected quotes to be generated with small negative position")
	}

	// Test with position at soft limit (should apply reduce-only logic)
	quotes4 := strategy.Quote(snapshot, 3.0)
	if len(quotes4) == 0 {
		t.Error("Expected quotes to be generated at soft limit")
	}

	// Check that some quotes may be marked as reduce-only
	reduceOnlyCount := 0
	for _, quote := range quotes4 {
		if quote.ReduceOnly {
			reduceOnlyCount++
		}
	}
	t.Logf("At soft limit, %d quotes marked as reduce-only", reduceOnlyCount)

	// Test with position at hard limit (should still generate quotes)
	quotes5 := strategy.Quote(snapshot, 5.0)
	if len(quotes5) == 0 {
		t.Error("Expected quotes even at hard limit")
	}
	t.Logf("At hard limit, generated %d quotes", len(quotes5))
}

func TestASMMStrategyWithVaryingVolatility(t *testing.T) {
	// Create strategy with default config
	config := DefaultASMMConfig()
	strategy := NewASMMStrategy(config)

	// Add price series with low volatility
	baseTime := time.Now()
	for i := 0; i < 35; i++ { // More than the 30-sample window
		price := 100.0 + 0.01*float64(i%10) // Low volatility pattern
		snapshot := market.Snapshot{
			Mid:       price,
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute).Unix(),
		}
		_ = strategy.Quote(snapshot, 0.0)
	}

	// Add price series with high volatility
	for i := 35; i < 70; i++ {
		price := 100.0 + float64(i%20-10) // Higher volatility pattern
		snapshot := market.Snapshot{
			Mid:       price,
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute).Unix(),
		}
		_ = strategy.Quote(snapshot, 0.0)
	}

	// The strategy should internally handle the volatility differences
	// We mainly check that it doesn't crash or produce invalid quotes
}
