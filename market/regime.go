package market

import "math"

// MarketRegime represents different market conditions
type MarketRegime int

const (
	RegimeCalm MarketRegime = iota
	RegimeTrendUp
	RegimeTrendDown
	RegimeHighVol
)

// RegimeDetector detects market regime based on volatility and imbalance
type RegimeDetector struct {
	volThresholdLow   float64
	volThresholdHigh  float64
	imbalanceThreshold float64
	priceDeviationThreshold float64
	shortMA           []float64
	longMA            []float64
	shortWindow       int
	longWindow        int
}

// NewRegimeDetector creates a new regime detector
func NewRegimeDetector(
	volThresholdLow float64,
	volThresholdHigh float64,
	imbalanceThreshold float64,
	priceDeviationThreshold float64,
	shortWindow int,
	longWindow int) *RegimeDetector {
	
	return &RegimeDetector{
		volThresholdLow:   volThresholdLow,
		volThresholdHigh:  volThresholdHigh,
		imbalanceThreshold: imbalanceThreshold,
		priceDeviationThreshold: priceDeviationThreshold,
		shortMA:           make([]float64, 0, shortWindow),
		longMA:            make([]float64, 0, longWindow),
		shortWindow:       shortWindow,
		longWindow:        longWindow,
	}
}

// AddPrice adds a new price for moving average calculation
func (r *RegimeDetector) AddPrice(price float64) {
	// Update short MA
	r.shortMA = append(r.shortMA, price)
	if len(r.shortMA) > r.shortWindow {
		r.shortMA = r.shortMA[1:]
	}
	
	// Update long MA
	r.longMA = append(r.longMA, price)
	if len(r.longMA) > r.longWindow {
		r.longMA = r.longMA[1:]
	}
}

// DetectRegime detects the current market regime
func (r *RegimeDetector) DetectRegime(volatility float64, imbalance float64, midPrice float64) MarketRegime {
	// High volatility regime
	if volatility > r.volThresholdHigh {
		return RegimeHighVol
	}
	
	// Calculate price deviation from long-term average
	if len(r.longMA) >= r.longWindow && len(r.shortMA) >= r.shortWindow {
		longAvg := 0.0
		for _, p := range r.longMA {
			longAvg += p
		}
		longAvg /= float64(len(r.longMA))
		
		shortAvg := 0.0
		for _, p := range r.shortMA {
			shortAvg += p
		}
		shortAvg /= float64(len(r.shortMA))
		
		deviation := math.Abs(shortAvg - longAvg) / longAvg
		
		// Trend regimes
		if deviation > r.priceDeviationThreshold {
			if shortAvg > longAvg {
				return RegimeTrendUp
			}
			return RegimeTrendDown
		}
	}
	
	// Calm regime by default
	return RegimeCalm
}

// IsTrendRegime checks if the current regime is a trend regime
func (r *RegimeDetector) IsTrendRegime(regime MarketRegime) bool {
	return regime == RegimeTrendUp || regime == RegimeTrendDown
}

// IsHighVolatilityRegime checks if the current regime is high volatility
func (r *RegimeDetector) IsHighVolatilityRegime(regime MarketRegime) bool {
	return regime == RegimeHighVol
}