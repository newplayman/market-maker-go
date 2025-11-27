package asmm

import (
	"market-maker-go/market"
	"testing"
	"time"
)

func TestASMMIntegration(t *testing.T) {
	// Create a new ASMM strategy with default config
	config := DefaultASMMConfig()
	strategy := NewASMMStrategy(config)

	// Create a market snapshot
	snapshot := market.Snapshot{
		Mid:       100.0,
		Timestamp: time.Now().Unix(),
	}

	// Generate quotes
	quotes := strategy.Quote(snapshot, 0.0)

	// Check that we got some quotes
	if len(quotes) == 0 {
		t.Log("No quotes generated - this might be expected during initial volatility calculation period")
	}

	// Add more market data to make volatility calculator ready
	for i := 1; i <= 35; i++ {
		newSnapshot := market.Snapshot{
			Mid:       100.0 + float64(i)*0.01, // Create some price movement
			Timestamp: time.Now().Add(time.Duration(i) * time.Second).Unix(),
		}
		quotes = strategy.Quote(newSnapshot, 0.0)
	}

	// Now we should have quotes
	if len(quotes) == 0 {
		t.Error("Still no quotes generated after providing sufficient market data")
	}

	// Verify that quotes have reasonable prices
	for _, quote := range quotes {
		if quote.Price <= 0 {
			t.Errorf("Invalid quote price: %f", quote.Price)
		}
		if quote.Size <= 0 {
			t.Errorf("Invalid quote size: %f", quote.Size)
		}
	}
}
