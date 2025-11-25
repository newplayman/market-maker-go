package market

import (
	"testing"
)

func TestCalculateImbalance(t *testing.T) {
	tests := []struct {
		name          string
		bidVolume     float64
		askVolume     float64
		expected      float64
	}{
		{
			name:      "Equal volumes",
			bidVolume: 100,
			askVolume: 100,
			expected:  0,
		},
		{
			name:      "More bid volume",
			bidVolume: 150,
			askVolume: 100,
			expected:  0.2,
		},
		{
			name:      "More ask volume",
			bidVolume: 100,
			askVolume: 150,
			expected:  -0.2,
		},
		{
			name:      "Zero volumes",
			bidVolume: 0,
			askVolume: 0,
			expected:  0,
		},
		{
			name:      "One zero volume",
			bidVolume: 100,
			askVolume: 0,
			expected:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateImbalance(tt.bidVolume, tt.askVolume)
			if result != tt.expected {
				t.Errorf("CalculateImbalance(%f, %f) = %f, want %f", 
					tt.bidVolume, tt.askVolume, result, tt.expected)
			}
		})
	}
}

func TestCalculateImbalanceFromOrderBook(t *testing.T) {
	// Create a simple order book
	book := NewOrderBook()
	
	// Add some bids
	book.ApplyDelta(map[float64]float64{100.0: 2, 99.9: 3, 99.8: 1}, 
	                map[float64]float64{100.1: 1, 100.2: 2, 100.3: 3})
	
	// Test with 1 level
	imbalance1 := CalculateImbalanceFromOrderBook(book, 1)
	expected1 := CalculateImbalance(2, 1)
	if imbalance1 != expected1 {
		t.Errorf("CalculateImbalanceFromOrderBook(1 level) = %f, want %f", imbalance1, expected1)
	}
	
	// Test with 2 levels
	imbalance2 := CalculateImbalanceFromOrderBook(book, 2)
	expected2 := CalculateImbalance(2+3, 1+2)
	if imbalance2 != expected2 {
		t.Errorf("CalculateImbalanceFromOrderBook(2 levels) = %f, want %f", imbalance2, expected2)
	}
	
	// Test with more levels than available
	imbalance3 := CalculateImbalanceFromOrderBook(book, 10)
	expected3 := CalculateImbalance(2+3+1, 1+2+3)
	if imbalance3 != expected3 {
		t.Errorf("CalculateImbalanceFromOrderBook(10 levels) = %f, want %f", imbalance3, expected3)
	}
	
	// Test with nil book
	imbalance4 := CalculateImbalanceFromOrderBook(nil, 1)
	if imbalance4 != 0 {
		t.Errorf("CalculateImbalanceFromOrderBook(nil) = %f, want 0", imbalance4)
	}
	
	// Test with zero levels
	imbalance5 := CalculateImbalanceFromOrderBook(book, 0)
	if imbalance5 != 0 {
		t.Errorf("CalculateImbalanceFromOrderBook(0 levels) = %f, want 0", imbalance5)
	}
}