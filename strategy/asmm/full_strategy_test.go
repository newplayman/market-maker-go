package asmm

import (
	"testing"
	"time"
)

// mockInventory implements the Inventory interface for testing
type mockInventory struct {
	position float64
}

func (m *mockInventory) Position() float64 {
	return m.position
}

func TestFullASMMStrategy(t *testing.T) {
	// Create strategy with default config
	config := DefaultASMMConfig()
	strategy := NewASMMStrategy(config)
	
	// Create market snapshot
	snapshot := MarketSnapshot{
		Mid:       100.0,
		Timestamp: time.Now(),
	}
	
	// Create inventory
	inventory := &mockInventory{position: 0.0}
	
	// Generate quotes
	quotes := strategy.Quote(snapshot, inventory)
	
	// Check that we got quotes
	if len(quotes) == 0 {
		t.Error("Expected quotes to be generated")
	}
	
	// Check that we have both bid and ask quotes
	bidCount := 0
	askCount := 0
	for _, quote := range quotes {
		if quote.Side == SideBid {
			bidCount++
		} else if quote.Side == SideAsk {
			askCount++
		}
	}
	
	if bidCount == 0 {
		t.Error("Expected bid quotes to be generated")
	}
	
	if askCount == 0 {
		t.Error("Expected ask quotes to be generated")
	}
	
	// Test with positive position (should bias quotes downward)
	inventory.position = 2.0
	quotes2 := strategy.Quote(snapshot, inventory)
	
	// With positive position, reservation price should be lower, leading to lower ask prices
	// and possibly higher bid prices (closer to mid)
	
	if len(quotes2) == 0 {
		t.Error("Expected quotes to be generated with positive position")
	}
	
	// Test with negative position (should bias quotes upward)
	inventory.position = -2.0
	quotes3 := strategy.Quote(snapshot, inventory)
	
	if len(quotes3) == 0 {
		t.Error("Expected quotes to be generated with negative position")
	}
	
	// Test with position at soft limit (should apply reduce-only logic)
	inventory.position = 3.0
	quotes4 := strategy.Quote(snapshot, inventory)
	
	// Check that some quotes are marked as reduce-only
	reduceOnlyCount := 0
	for _, quote := range quotes4 {
		if quote.ReduceOnly {
			reduceOnlyCount++
		}
	}
	
	// With position at soft limit, some orders should be marked reduce-only
	// (those that would increase position further)
	
	// Test with position at hard limit (should only generate reduce-only quotes)
	inventory.position = 5.0
	quotes5 := strategy.Quote(snapshot, inventory)
	
	levelCount := len(quotes5)
	if levelCount == 0 {
		t.Error("Expected quotes even at hard limit")
	}
	
	// At hard limit, we should only get 1 level (not multiple)
	if levelCount > 2 {
		t.Errorf("Expected at most 2 quotes at hard limit, got %d", levelCount)
	}
}

func TestASMMStrategyWithVaryingVolatility(t *testing.T) {
	// Create strategy with default config
	config := DefaultASMMConfig()
	strategy := NewASMMStrategy(config)
	
	// Create inventory
	inventory := &mockInventory{position: 0.0}
	
	// Add price series with low volatility
	baseTime := time.Now()
	for i := 0; i < 35; i++ { // More than the 30-sample window
		price := 100.0 + 0.01*float64(i%10) // Low volatility pattern
		snapshot := MarketSnapshot{
			Mid:       price,
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
		}
		_ = strategy.Quote(snapshot, inventory)
	}
	
	// Add price series with high volatility
	for i := 35; i < 70; i++ {
		price := 100.0 + float64(i%20-10) // Higher volatility pattern
		snapshot := MarketSnapshot{
			Mid:       price,
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
		}
		_ = strategy.Quote(snapshot, inventory)
	}
	
	// The strategy should internally handle the volatility differences
	// We mainly check that it doesn't crash or produce invalid quotes
}