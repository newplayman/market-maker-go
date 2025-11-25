package risk

import (
	"testing"
)

func TestAdaptiveRiskManager_EffectiveNetMax(t *testing.T) {
	// Create a basic limit checker
	limits := &Limits{
		SingleMax: 10,
		DailyMax:  100,
		NetMax:    50,
	}
	
	// Mock position keeper
	posKeeper := &mockPositionKeeper{}
	
	baseChecker := NewLimitChecker(limits, posKeeper)
	
	// Create adaptive risk manager
	adaptiveConfig := AdaptiveRiskConfig{
		BaseNetMax: 50,
		MinNetMax:  10,
	}
	
	adaptiveManager := NewAdaptiveRiskManager(baseChecker, adaptiveConfig)
	
	// Test normal conditions
	normalNetMax := adaptiveManager.EffectiveNetMax(0.01, false, 0.1) // 1% volatility, not toxic, low adverse selection
	if normalNetMax != 50 {
		t.Errorf("Expected normal net max to be 50, got %f", normalNetMax)
	}
	
	// Test high volatility conditions
	highVolNetMax := adaptiveManager.EffectiveNetMax(0.03, false, 0.1) // 3% volatility, not toxic, low adverse selection
	expectedHighVol := 50 * 0.7
	if highVolNetMax != expectedHighVol {
		t.Errorf("Expected high vol net max to be %f, got %f", expectedHighVol, highVolNetMax)
	}
	
	// Test toxic conditions
	toxicNetMax := adaptiveManager.EffectiveNetMax(0.01, true, 0.1) // 1% volatility, toxic, low adverse selection
	expectedToxic := 50 * 0.5
	if toxicNetMax != expectedToxic {
		t.Errorf("Expected toxic net max to be %f, got %f", expectedToxic, toxicNetMax)
	}
	
	// Test high adverse selection conditions
	highAdverseNetMax := adaptiveManager.EffectiveNetMax(0.01, false, 0.7) // 1% volatility, not toxic, high adverse selection
	expectedHighAdverse := 50 * 0.5
	if highAdverseNetMax != expectedHighAdverse {
		t.Errorf("Expected high adverse selection net max to be %f, got %f", expectedHighAdverse, highAdverseNetMax)
	}
	
	// Test combination of high volatility + toxic + high adverse selection
	allBadNetMax := adaptiveManager.EffectiveNetMax(0.03, true, 0.7) // 3% volatility, toxic, high adverse selection
	expectedAllBad := 50 * 0.7 * 0.5 * 0.5
	if allBadNetMax != expectedAllBad {
		t.Errorf("Expected combined bad conditions net max to be %f, got %f", expectedAllBad, allBadNetMax)
	}
	
	// Test minimum limit enforcement
	minNetMax := adaptiveManager.EffectiveNetMax(0.1, true, 0.9) // Very high volatility, toxic and adverse selection
	if minNetMax != 10 { // Should be clamped to MinNetMax
		t.Errorf("Expected min net max to be 10, got %f", minNetMax)
	}
}

func TestAdaptiveRiskManager_UpdateLimits(t *testing.T) {
	// Create a basic limit checker
	limits := &Limits{
		SingleMax: 10,
		DailyMax:  100,
		NetMax:    50,
	}
	
	// Mock position keeper
	posKeeper := &mockPositionKeeper{}
	
	baseChecker := NewLimitChecker(limits, posKeeper)
	
	// Create adaptive risk manager
	adaptiveConfig := AdaptiveRiskConfig{
		BaseNetMax: 50,
		MinNetMax:  10,
	}
	
	adaptiveManager := NewAdaptiveRiskManager(baseChecker, adaptiveConfig)
	
	// Check initial limit
	if baseChecker.limits.NetMax != 50 {
		t.Errorf("Expected initial net max to be 50, got %f", baseChecker.limits.NetMax)
	}
	
	// Update with high volatility, toxic conditions and high adverse selection
	adaptiveManager.UpdateLimits(0.03, true, 0.7)
	
	// Check updated limit
	expectedNetMax := 50 * 0.7 * 0.5 * 0.5
	if baseChecker.limits.NetMax != expectedNetMax {
		t.Errorf("Expected updated net max to be %f, got %f", expectedNetMax, baseChecker.limits.NetMax)
	}
}