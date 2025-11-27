package asmm

import (
	"market-maker-go/market"
	"testing"
	"time"
)

func TestASMMStrategyCreation(t *testing.T) {
	config := DefaultASMMConfig()
	strategy := NewASMMStrategy(config)

	if strategy == nil {
		t.Error("Failed to create ASMM strategy")
	}
}

func TestASMMConfigValidation(t *testing.T) {
	config := DefaultASMMConfig()

	if !config.Validate() {
		t.Error("Default config should be valid")
	}

	// Test invalid config - negative min spread
	invalidConfig := config
	invalidConfig.MinSpreadBps = -1
	if invalidConfig.Validate() {
		t.Error("Invalid config should not be valid")
	}

	// Test invalid config - min spread > max spread
	invalidConfig2 := config
	invalidConfig2.MinSpreadBps = 10
	invalidConfig2.MaxSpreadBps = 5
	if invalidConfig2.Validate() {
		t.Error("Invalid config should not be valid")
	}

	// Test invalid config - negative max levels
	invalidConfig3 := config
	invalidConfig3.MaxLevels = -1
	if invalidConfig3.Validate() {
		t.Error("Invalid config should not be valid")
	}
}

func TestGenerateQuotesBasic(t *testing.T) {
	config := DefaultASMMConfig()
	strategy := NewASMMStrategy(config)

	snap := market.Snapshot{
		Mid:       100.0,
		Timestamp: time.Now().Unix(),
	}

	// Test quote generation with zero inventory
	quotes := strategy.GenerateQuotes(snap, 0.0)
	if len(quotes) == 0 {
		t.Error("GenerateQuotes returned no quotes")
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
		t.Error("No bid quotes generated")
	}

	if askCount == 0 {
		t.Error("No ask quotes generated")
	}

	// Verify prices are reasonable
	for _, quote := range quotes {
		if quote.Price <= 0 {
			t.Errorf("Invalid quote price: %f", quote.Price)
		}
		if quote.Size <= 0 {
			t.Errorf("Invalid quote size: %f", quote.Size)
		}
	}
}

func TestGenerateQuotesWithInventory(t *testing.T) {
	config := DefaultASMMConfig()
	config.AvoidToxic = false // 禁用toxic flow逻辑以简化测试
	strategy := NewASMMStrategy(config)

	snap := market.Snapshot{
		Mid:       100.0,
		Timestamp: time.Now().Unix(),
		VPIN:      0.0, // Low VPIN for normal conditions
	}

	// Test with positive inventory - should generate at least ask quote
	quotes1 := strategy.GenerateQuotes(snap, 2.0)
	if len(quotes1) == 0 {
		t.Error("GenerateQuotes with positive inventory returned no quotes")
	}

	// Test with negative inventory - should generate at least bid quote
	quotes2 := strategy.GenerateQuotes(snap, -2.0)
	if len(quotes2) == 0 {
		t.Error("GenerateQuotes with negative inventory returned no quotes")
	}
}

func TestQuoteMethod(t *testing.T) {
	config := DefaultASMMConfig()
	strategy := NewASMMStrategy(config)

	snap := market.Snapshot{
		Mid:       100.0,
		Timestamp: time.Now().Unix(),
	}

	quotes := strategy.Quote(snap, 0.0)
	if len(quotes) == 0 {
		t.Error("Quote returned no quotes")
	}

	// Verify basic quote properties
	for _, quote := range quotes {
		if quote.Price <= 0 {
			t.Errorf("Invalid quote price: %f", quote.Price)
		}
		if quote.Size <= 0 {
			t.Errorf("Invalid quote size: %f", quote.Size)
		}
	}
}
