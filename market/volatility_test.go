package market

import (
	"testing"
	"time"
)

func TestVolatilityCalculator_AddPrice(t *testing.T) {
	calculator := NewVolatilityCalculator(5)
	
	// Add prices
	now := time.Now()
	calculator.AddPrice(100.0, now)
	calculator.AddPrice(101.0, now.Add(time.Minute))
	calculator.AddPrice(102.0, now.Add(2*time.Minute))
	
	if len(calculator.prices) != 3 {
		t.Errorf("Expected 3 prices, got %d", len(calculator.prices))
	}
	
	if calculator.prices[0] != 100.0 {
		t.Errorf("Expected first price to be 100.0, got %f", calculator.prices[0])
	}
	
	if calculator.prices[2] != 102.0 {
		t.Errorf("Expected last price to be 102.0, got %f", calculator.prices[2])
	}
}

func TestVolatilityCalculator_RealizedVol(t *testing.T) {
	calculator := NewVolatilityCalculator(10)
	
	// Add constant prices - should result in zero volatility
	now := time.Now()
	for i := 0; i < 5; i++ {
		calculator.AddPrice(100.0, now.Add(time.Duration(i)*time.Minute))
	}
	
	vol := calculator.RealizedVol()
	if vol != 0.0 {
		t.Errorf("Expected zero volatility for constant prices, got %f", vol)
	}
	
	// Test with increasing prices
	calculator2 := NewVolatilityCalculator(10)
	for i := 0; i < 5; i++ {
		calculator2.AddPrice(100.0+float64(i), now.Add(time.Duration(i)*time.Minute))
	}
	
	vol2 := calculator2.RealizedVol()
	if vol2 < 0 {
		t.Errorf("Volatility should be non-negative, got %f", vol2)
	}
}

func TestVolatilityCalculator_WindowSize(t *testing.T) {
	calculator := NewVolatilityCalculator(3)
	
	// Add more prices than window size
	now := time.Now()
	for i := 0; i < 5; i++ {
		calculator.AddPrice(100.0+float64(i), now.Add(time.Duration(i)*time.Minute))
	}
	
	// Should only keep the last 3 prices
	if len(calculator.prices) != 3 {
		t.Errorf("Expected window size of 3, got %d", len(calculator.prices))
	}
	
	if calculator.prices[0] != 102.0 {
		t.Errorf("Expected first price in window to be 102.0, got %f", calculator.prices[0])
	}
}

func TestVolatilityCalculator_IsReady(t *testing.T) {
	calculator := NewVolatilityCalculator(5)
	
	// Not ready with less than 2 prices
	if calculator.IsReady() {
		t.Error("Should not be ready with less than 2 prices")
	}
	
	// Add one price
	now := time.Now()
	calculator.AddPrice(100.0, now)
	if calculator.IsReady() {
		t.Error("Should not be ready with only 1 price")
	}
	
	// Add second price
	calculator.AddPrice(101.0, now.Add(time.Minute))
	if !calculator.IsReady() {
		t.Error("Should be ready with 2 prices")
	}
}