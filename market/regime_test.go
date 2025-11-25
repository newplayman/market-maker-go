package market

import (
	"testing"
)

func TestRegimeDetector_DetectRegime(t *testing.T) {
	detector := NewRegimeDetector(0.01, 0.05, 0.2, 0.01, 5, 20)
	
	// Add prices to initialize moving averages
	for i := 0; i < 30; i++ {
		detector.AddPrice(100.0)
	}
	
	// Test calm regime
	regime := detector.DetectRegime(0.005, 0.1, 100.0)
	if regime != RegimeCalm {
		t.Errorf("Expected RegimeCalm, got %v", regime)
	}
	
	// Test high volatility regime
	regime = detector.DetectRegime(0.06, 0.1, 100.0)
	if regime != RegimeHighVol {
		t.Errorf("Expected RegimeHighVol, got %v", regime)
	}
}

func TestRegimeDetector_AddPrice(t *testing.T) {
	detector := NewRegimeDetector(0.01, 0.05, 0.2, 0.01, 5, 20)
	
	// Add prices
	for i := 0; i < 10; i++ {
		detector.AddPrice(float64(i))
	}
	
	// Check that we have the right number of prices
	if len(detector.shortMA) != 5 {
		t.Errorf("Expected shortMA length 5, got %d", len(detector.shortMA))
	}
	
	if len(detector.longMA) != 10 { // Limited by the smaller window
		t.Errorf("Expected longMA length 10, got %d", len(detector.longMA))
	}
	
	// Check that the oldest prices were dropped
	if detector.shortMA[0] != 5 { // Should be the 6th price (0-indexed)
		t.Errorf("Expected first element of shortMA to be 5, got %f", detector.shortMA[0])
	}
}

func TestRegimeDetector_HelperMethods(t *testing.T) {
	detector := NewRegimeDetector(0.01, 0.05, 0.2, 0.01, 5, 20)
	
	// Test IsTrendRegime
	if !detector.IsTrendRegime(RegimeTrendUp) {
		t.Error("RegimeTrendUp should be a trend regime")
	}
	
	if !detector.IsTrendRegime(RegimeTrendDown) {
		t.Error("RegimeTrendDown should be a trend regime")
	}
	
	if detector.IsTrendRegime(RegimeCalm) {
		t.Error("RegimeCalm should not be a trend regime")
	}
	
	if detector.IsTrendRegime(RegimeHighVol) {
		t.Error("RegimeHighVol should not be a trend regime")
	}
	
	// Test IsHighVolatilityRegime
	if !detector.IsHighVolatilityRegime(RegimeHighVol) {
		t.Error("RegimeHighVol should be a high volatility regime")
	}
	
	if detector.IsHighVolatilityRegime(RegimeCalm) {
		t.Error("RegimeCalm should not be a high volatility regime")
	}
	
	if detector.IsHighVolatilityRegime(RegimeTrendUp) {
		t.Error("RegimeTrendUp should not be a high volatility regime")
	}
}