package market

import (
	"math"
	"time"
)

// VolatilityCalculator calculates realized volatility based on mid prices
type VolatilityCalculator struct {
	windowSize int
	prices     []float64
	times      []time.Time
}

// NewVolatilityCalculator creates a new volatility calculator
func NewVolatilityCalculator(windowSize int) *VolatilityCalculator {
	return &VolatilityCalculator{
		windowSize: windowSize,
		prices:     make([]float64, 0, windowSize),
		times:      make([]time.Time, 0, windowSize),
	}
}

// AddPrice adds a new mid price to the calculator
func (v *VolatilityCalculator) AddPrice(mid float64, ts time.Time) {
	// Add new price and time
	v.prices = append(v.prices, mid)
	v.times = append(v.times, ts)
	
	// Keep only windowSize elements
	if len(v.prices) > v.windowSize {
		v.prices = v.prices[1:]
		v.times = v.times[1:]
	}
}

// RealizedVol calculates the realized volatility
func (v *VolatilityCalculator) RealizedVol() float64 {
	if len(v.prices) < 2 {
		return 0
	}
	
	// Calculate log returns
	logReturns := make([]float64, 0, len(v.prices)-1)
	for i := 1; i < len(v.prices); i++ {
		if v.prices[i-1] > 0 {
			logReturn := math.Log(v.prices[i] / v.prices[i-1])
			logReturns = append(logReturns, logReturn)
		}
	}
	
	if len(logReturns) < 1 {
		return 0
	}
	
	// Calculate standard deviation of log returns
	sum := 0.0
	for _, r := range logReturns {
		sum += r
	}
	mean := sum / float64(len(logReturns))
	
	sumSquaredDiff := 0.0
	for _, r := range logReturns {
		diff := r - mean
		sumSquaredDiff += diff * diff
	}
	variance := sumSquaredDiff / float64(len(logReturns))
	
	// Annualize volatility (assuming 252 trading days)
	// For simplicity, we'll just scale by sqrt of number of observations
	// In a real implementation, you might want to consider the actual time intervals
	vol := math.Sqrt(variance) * math.Sqrt(float64(len(logReturns)))
	
	return vol
}

// IsReady checks if we have enough data to calculate volatility
func (v *VolatilityCalculator) IsReady() bool {
	return len(v.prices) >= 2
}