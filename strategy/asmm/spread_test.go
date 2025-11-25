package asmm

import (
	"testing"
)

func TestVolatilitySpreadAdjuster_GetHalfSpread(t *testing.T) {
	adjuster := NewVolatilitySpreadAdjuster(5.0, 50.0, 1.0, 1.5, 2.0)

	tests := []struct {
		name           string
		volatility     float64
		regime         MarketRegime
		expectedMin    float64
		expectedMax    float64
	}{
		{
			name:        "Calm regime with low volatility",
			volatility:  0.1,
			regime:      RegimeCalm,
			expectedMin: 5.0,
			expectedMax: 50.0,
		},
		{
			name:        "Trend regime",
			volatility:  0.5,
			regime:      RegimeTrendUp,
			expectedMin: 5.0,
			expectedMax: 50.0 * 1.5,
		},
		{
			name:        "High vol regime",
			volatility:  1.0,
			regime:      RegimeHighVol,
			expectedMin: 5.0,
			expectedMax: 50.0 * 2.0,
		},
		{
			name:        "Zero volatility",
			volatility:  0.0,
			regime:      RegimeCalm,
			expectedMin: 5.0,
			expectedMax: 5.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adjuster.GetHalfSpread(tt.volatility, tt.regime)
			if result < tt.expectedMin || result > tt.expectedMax {
				t.Errorf("GetHalfSpread() = %v, want between [%v, %v]", result, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestNewVolatilitySpreadAdjuster(t *testing.T) {
	adjuster := NewVolatilitySpreadAdjuster(5.0, 50.0, 1.0, 1.5, 2.0)
	
	if adjuster == nil {
		t.Error("NewVolatilitySpreadAdjuster() = nil, want not nil")
	}
}

func TestVolatilitySpreadAdjuster_EdgeCases(t *testing.T) {
	// Test with very high volatility
	adjuster := NewVolatilitySpreadAdjuster(5.0, 50.0, 1.0, 1.5, 2.0)
	result := adjuster.GetHalfSpread(100.0, RegimeCalm)
	if result != 50.0 {
		t.Errorf("Expected clipped max spread 50.0, got %f", result)
	}
	
	// Test with negative volatility (should be handled gracefully)
	result2 := adjuster.GetHalfSpread(-0.1, RegimeCalm)
	if result2 < 5.0 {
		t.Errorf("Expected minimum spread 5.0, got %f", result2)
	}
}