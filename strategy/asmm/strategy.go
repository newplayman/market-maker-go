package asmm

import (
	"math"
	"time"
	"market-maker-go/market"
	"market-maker-go/metrics"
)

// ASMMStrategy implements the ASMM strategy.
type ASMMStrategy struct {
	cfg                  ASMMConfig
	volatilityCalculator *market.VolatilityCalculator
	regimeDetector       *market.RegimeDetector
	spreadAdjuster       *VolatilitySpreadAdjuster
}

// NewASMMStrategy creates a new instance of ASMMStrategy.
func NewASMMStrategy(cfg ASMMConfig) *ASMMStrategy {
	return &ASMMStrategy{
		cfg: cfg,
		volatilityCalculator: market.NewVolatilityCalculator(30), // 30-sample window
		regimeDetector: market.NewRegimeDetector(0.01, 0.05, 0.2, 0.01, 5, 20),
		spreadAdjuster: NewVolatilitySpreadAdjuster(
			cfg.MinSpreadBps, 
			cfg.MaxSpreadBps, 
			cfg.VolK, // Volatility multiplier
			cfg.TrendSpreadMultiplier,
			cfg.HighVolSpreadMultiplier,
		),
	}
}

// GenerateQuotes generates quotes based on the ASMM strategy.
func (s *ASMMStrategy) GenerateQuotes(snap market.Snapshot, inventory float64) []Quote {
	// Add price to volatility calculator
	s.volatilityCalculator.AddPrice(snap.Mid, time.Unix(snap.Timestamp, 0))
	
	// Calculate volatility
	vol := s.volatilityCalculator.RealizedVol()
	
	// Add price to regime detector
	s.regimeDetector.AddPrice(snap.Mid)
	
	// Detect market regime
	regime := s.regimeDetector.DetectRegime(vol, 0.0, snap.Mid) // TODO: add imbalance
	
	// Adjust spread based on volatility and regime
	spreadBps := s.spreadAdjuster.GetHalfSpread(vol, RegimeCalm) // Default to calm regime
	
	// Calculate reservation price (mid price adjusted for inventory)
	reservationPrice := snap.Mid - s.cfg.InvSkewK*(inventory-s.cfg.TargetPosition)*snap.Mid
	
	// Calculate skew factor based on inventory
	skewFactor := 1.0 + math.Tanh(s.cfg.InvSkewK*(inventory-s.cfg.TargetPosition)/s.cfg.InvSoftLimit)
	
	// Calculate bid/ask prices
	bidPrice := reservationPrice * (1 - skewFactor*spreadBps/2/10000)
	askPrice := reservationPrice * (1 + skewFactor*spreadBps/2/10000)
	
	// Adjust size based on volatility
	size := s.cfg.BaseSize * math.Exp(-s.cfg.SizeVolK*vol)
	
	// Generate quotes
	var quotes []Quote
	if bidPrice > 0 && size > 0 {
		quotes = append(quotes, Quote{
			Price: bidPrice,
			Size:  size,
			Side:  Bid,
		})
	}
	
	if askPrice > 0 && size > 0 {
		quotes = append(quotes, Quote{
			Price: askPrice,
			Size:  size,
			Side:  Ask,
		})
	}
	
	// Update metrics
	metrics.UpdateStrategyMetrics(reservationPrice, skewFactor*10000, 0) // TODO: add adaptive net max
	metrics.VolatilityRegime.Set(float64(regime))
	
	return quotes
}

// Quote generates bid/ask quotes based on market snapshot and inventory position.
func (s *ASMMStrategy) Quote(marketSnapshot market.Snapshot, inventory float64) []Quote {
	// Update market data
	s.volatilityCalculator.AddPrice(marketSnapshot.Mid, time.Unix(marketSnapshot.Timestamp, 0))
	s.regimeDetector.AddPrice(marketSnapshot.Mid)
	
	// Calculate volatility
	var volatility = 0.0
	if s.volatilityCalculator.IsReady() {
		volatility = s.volatilityCalculator.RealizedVol()
	}
	
	// Detect market regime
	regime := s.regimeDetector.DetectRegime(volatility, 0.0, marketSnapshot.Mid)
	
	// Adjust spread based on volatility and regime
	spreadBps := s.spreadAdjuster.GetHalfSpread(volatility, RegimeCalm) // Default to calm regime
	
	// Calculate reservation price (mid price adjusted for inventory)
	reservationPrice := s.calculateReservationPrice(marketSnapshot.Mid, inventory)
	
	// Calculate inventory skew in basis points
	inventorySkewBps := s.calculateInventorySkewBps(inventory)
	
	// Calculate bid/ask prices
	bidPrice := reservationPrice * (1 - spreadBps/2/10000)
	askPrice := reservationPrice * (1 + spreadBps/2/10000)
	
	// Adjust size based on volatility
	size := s.cfg.BaseSize * math.Exp(-s.cfg.SizeVolK*volatility)
	
	// Generate quotes
	var quotes []Quote
	if bidPrice > 0 && size > 0 {
		quotes = append(quotes, Quote{
			Price: bidPrice,
			Size:  size,
			Side:  Bid,
		})
	}
	
	if askPrice > 0 && size > 0 {
		quotes = append(quotes, Quote{
			Price: askPrice,
			Size:  size,
			Side:  Ask,
		})
	}
	
	// Reduce-only logic for inventory limits
	if s.isCloseToLimit(inventory) || s.shouldReduceOnly(inventory) {
		for i := range quotes {
			// Mark orders as reduce-only if they increase position beyond limits
			if (inventory >= s.cfg.InvSoftLimit && quotes[i].Side == Bid) ||
			   (inventory <= -s.cfg.InvSoftLimit && quotes[i].Side == Ask) {
				// Don't mark as reduce-only - these would reduce position
				continue
			} else if (inventory > 0 && quotes[i].Side == Bid) ||
			         (inventory < 0 && quotes[i].Side == Ask) {
				// These would increase position beyond limits - mark as reduce-only
				quotes[i].ReduceOnly = true
			}
		}
	}
	
	// Update metrics
	metrics.UpdateMarketMetrics(0, int(regime), inventory) // VPIN set to 0 as placeholder
	metrics.UpdateStrategyMetrics(reservationPrice, inventorySkewBps, s.cfg.InvHardLimit) // AdaptiveNetMax placeholder
	metrics.UpdatePostTradeMetrics(0) // AdverseSelectionRate placeholder
	
	// Count generated quotes
	for _, quote := range quotes {
		if quote.Side == Bid {
			metrics.IncrementQuotesGenerated("bid")
		} else {
			metrics.IncrementQuotesGenerated("ask")
		}
	}
	
	return quotes
}

// calculateReservationPrice calculates the reservation price based on mid price and inventory skew.
func (s *ASMMStrategy) calculateReservationPrice(mid float64, position float64) float64 {
	// Calculate inventory ratio, clamped between -1 and 1
	ratio := position / s.cfg.InvSoftLimit
	if ratio > 1.0 {
		ratio = 1.0
	}
	if ratio < -1.0 {
		ratio = -1.0
	}
	
	// Reservation price adjustment factor
	adjustment := s.cfg.InvSkewK * ratio * mid
	
	return mid - adjustment
}

// calculateInventorySkewBps calculates the inventory skew in basis points.
func (s *ASMMStrategy) calculateInventorySkewBps(position float64) float64 {
	// Calculate inventory ratio, clamped between -1 and 1
	ratio := position / s.cfg.InvSoftLimit
	if ratio > 1.0 {
		ratio = 1.0
	}
	if ratio < -1.0 {
		ratio = -1.0
	}
	
	// Convert to basis points (0.01 = 1 basis point)
	return s.cfg.InvSkewK * ratio * 10000
}

// isCloseToLimit checks if the current position is close to the inventory limit.
func (s *ASMMStrategy) isCloseToLimit(position float64) bool {
	return math.Abs(position) >= s.cfg.InvSoftLimit*0.8
}

// shouldReduceOnly determines if we should only place reduce-only orders.
func (s *ASMMStrategy) shouldReduceOnly(position float64) bool {
	return math.Abs(position) >= s.cfg.InvSoftLimit
}