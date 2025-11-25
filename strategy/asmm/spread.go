package asmm

import (
	"math"
)

// MarketRegime represents the market condition.
type MarketRegime int

const (
	RegimeCalm MarketRegime = iota
	RegimeTrendUp
	RegimeTrendDown
	RegimeHighVol
)

// VolatilitySpreadAdjuster adjusts spread based on volatility and market regime.
type VolatilitySpreadAdjuster struct {
	minSpreadBps float64
	maxSpreadBps float64
	volK         float64
	trendMultiplier  float64
	highVolMultiplier float64
}

// NewVolatilitySpreadAdjuster creates a new spread adjuster.
func NewVolatilitySpreadAdjuster(minSpreadBps, maxSpreadBps, volK, trendMultiplier, highVolMultiplier float64) *VolatilitySpreadAdjuster {
	return &VolatilitySpreadAdjuster{
		minSpreadBps:      minSpreadBps,
		maxSpreadBps:      maxSpreadBps,
		volK:              volK,
		trendMultiplier:   trendMultiplier,
		highVolMultiplier: highVolMultiplier,
	}
}

// GetHalfSpread calculates the half spread based on volatility and regime.
func (v *VolatilitySpreadAdjuster) GetHalfSpread(volatility float64, regime MarketRegime) float64 {
	// Base spread calculation: minSpread + volK * volatility
	baseSpread := v.minSpreadBps + v.volK*volatility
	
	// Clip to [minSpread, maxSpread]
	halfSpreadBps := math.Max(v.minSpreadBps, math.Min(v.maxSpreadBps, baseSpread))

	// Adjust based on market regime
	switch regime {
	case RegimeTrendUp, RegimeTrendDown:
		halfSpreadBps *= v.trendMultiplier
	case RegimeHighVol:
		halfSpreadBps *= v.highVolMultiplier
	}

	return halfSpreadBps
}