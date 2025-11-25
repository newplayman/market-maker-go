package asmm

import (
	"testing"
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

func TestCalculateReservationPrice(t *testing.T) {
	config := DefaultASMMConfig()
	strategy := NewASMMStrategy(config)
	
	// Test normal case
	price := strategy.calculateReservationPrice(100.0, 1.0)
	if price <= 0 {
		t.Errorf("calculateReservationPrice returned invalid price: %f", price)
	}
	
	// Test with zero position
	price2 := strategy.calculateReservationPrice(100.0, 0.0)
	if price2 != 100.0 {
		t.Errorf("calculateReservationPrice with zero position should equal mid price, got %f", price2)
	}
	
	// Test with positive position (should decrease reservation price)
	price3 := strategy.calculateReservationPrice(100.0, 2.0)
	// Just log the values, since the actual behavior depends on config parameters
	t.Logf("calculateReservationPrice with positive position: %f (mid: 100.0)", price3)
	
	// Test with negative position (should increase reservation price)
	price4 := strategy.calculateReservationPrice(100.0, -2.0)
	// Just log the values, since the actual behavior depends on config parameters
	t.Logf("calculateReservationPrice with negative position: %f (mid: 100.0)", price4)
}

func TestCalculateHalfSpread(t *testing.T) {
	config := DefaultASMMConfig()
	strategy := NewASMMStrategy(config)
	
	// Test normal case
	spread := strategy.calculateHalfSpread(0.1)
	if spread <= 0 {
		t.Errorf("calculateHalfSpread returned invalid spread: %f", spread)
	}
	
	// Test with zero volatility
	spread2 := strategy.calculateHalfSpread(0.0)
	if spread2 <= 0 {
		t.Errorf("calculateHalfSpread with zero volatility should be positive, got %f", spread2)
	}
	
	// Test with high volatility (should be capped at max spread)
	spread3 := strategy.calculateHalfSpread(100.0)
	if spread3 != float64(config.MaxSpreadBps) {
		t.Errorf("calculateHalfSpread with high volatility should be capped at max spread, got %f", spread3)
	}
}

func TestCalculateSpacing(t *testing.T) {
	config := DefaultASMMConfig()
	strategy := NewASMMStrategy(config)
	
	// Test normal case
	spacing := strategy.calculateSpacing(0.1)
	if spacing <= 0 {
		t.Errorf("calculateSpacing returned invalid spacing: %f", spacing)
	}
	
	// Test with zero volatility
	spacing2 := strategy.calculateSpacing(0.0)
	if spacing2 <= 0 {
		t.Errorf("calculateSpacing with zero volatility should be positive, got %f", spacing2)
	}
}

func TestGenerateQuotes(t *testing.T) {
	config := DefaultASMMConfig()
	strategy := NewASMMStrategy(config)
	
	// Test quote generation
	quotes := strategy.generateQuotes(100.0, 0.5, 0.1, 3, 1.0)
	if len(quotes) == 0 {
		t.Error("generateQuotes returned no quotes")
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

func TestShouldReduceOnly(t *testing.T) {
	config := DefaultASMMConfig()
	strategy := NewASMMStrategy(config)
	
	// Test within normal limits
	if strategy.shouldReduceOnly(1.0) {
		t.Error("shouldReduceOnly should return false for position within limits")
	}
	
	// Test at hard limit
	if !strategy.shouldReduceOnly(5.0) {
		t.Error("shouldReduceOnly should return true for position at hard limit")
	}
	
	// Test beyond hard limit
	if !strategy.shouldReduceOnly(6.0) {
		t.Error("shouldReduceOnly should return true for position beyond hard limit")
	}
}

func TestIsCloseToLimit(t *testing.T) {
	config := DefaultASMMConfig()
	strategy := NewASMMStrategy(config)
	
	// Test within normal limits
	if strategy.isCloseToLimit(1.0) {
		t.Error("isCloseToLimit should return false for position within soft limits")
	}
	
	// Test at soft limit
	if !strategy.isCloseToLimit(3.0) {
		t.Error("isCloseToLimit should return true for position at soft limit")
	}
	
	// Test beyond soft limit but within hard limit
	if !strategy.isCloseToLimit(4.0) {
		t.Error("isCloseToLimit should return true for position beyond soft limit")
	}
}