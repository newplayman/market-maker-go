package asmm

import (
	"market-maker-go/market"
	"market-maker-go/metrics"
	"market-maker-go/risk"
	"math"
	"time"
)

// ASMMStrategy implements the ASMM strategy.
type ASMMStrategy struct {
	cfg                  ASMMConfig
	volatilityCalculator *market.VolatilityCalculator
	regimeDetector       *market.RegimeDetector
	spreadAdjuster       *VolatilitySpreadAdjuster
	adaptiveRisk         *risk.AdaptiveRiskManager // 自适应风控
}

// NewASMMStrategy creates a new instance of ASMMStrategy.
func NewASMMStrategy(cfg ASMMConfig) *ASMMStrategy {
	return &ASMMStrategy{
		cfg:                  cfg,
		volatilityCalculator: market.NewVolatilityCalculator(30), // 30-sample window
		regimeDetector:       market.NewRegimeDetector(0.01, 0.05, 0.2, 0.01, 5, 20),
		spreadAdjuster: NewVolatilitySpreadAdjuster(
			cfg.MinSpreadBps,
			cfg.MaxSpreadBps,
			cfg.VolK, // Volatility multiplier
			cfg.TrendSpreadMultiplier,
			cfg.HighVolSpreadMultiplier,
		),
	}
}

// SetAdaptiveRisk 设置自适应风控管理器
func (s *ASMMStrategy) SetAdaptiveRisk(ar *risk.AdaptiveRiskManager) {
	s.adaptiveRisk = ar
}

// GenerateQuotes generates quotes based on the ASMM strategy.
func (s *ASMMStrategy) GenerateQuotes(snap market.Snapshot, inventory float64) []Quote {
	// 获取自适应参数（如果启用）
	baseSize := s.cfg.BaseSize
	minSpreadBps := s.cfg.MinSpreadBps
	invSoftLimit := s.cfg.InvSoftLimit
	if s.adaptiveRisk != nil {
		baseSize = s.adaptiveRisk.GetCurrentBaseSize()
		minSpreadBps = s.adaptiveRisk.GetCurrentMinSpreadBps()
		invSoftLimit = s.adaptiveRisk.GetCurrentNetMax()
	}

	// Add price to volatility calculator
	s.volatilityCalculator.AddPrice(snap.Mid, time.Unix(snap.Timestamp, 0))

	// Calculate volatility
	vol := s.volatilityCalculator.RealizedVol()

	// Add price to regime detector
	s.regimeDetector.AddPrice(snap.Mid)

	// Detect market regime
	regime := s.regimeDetector.DetectRegime(vol, snap.Imbalance, snap.Mid)

	// Adjust spread based on volatility and regime
	spreadBps := s.spreadAdjuster.GetHalfSpread(vol, MarketRegime(regime))
	// 应用自适应最小价差
	if spreadBps < minSpreadBps {
		spreadBps = minSpreadBps
	}

	// Apply VPIN toxic flow adjustment
	if s.cfg.AvoidToxic && snap.VPIN > s.cfg.VPINToxicThreshold {
		spreadBps *= s.cfg.ToxicSpreadMultiplier
	}

	// Calculate reservation price (mid price adjusted for inventory)
	reservationPrice := snap.Mid - s.cfg.InvSkewK*(inventory-s.cfg.TargetPosition)*snap.Mid

	// Calculate skew factor based on inventory
	skewFactor := 1.0 + math.Tanh(s.cfg.InvSkewK*(inventory-s.cfg.TargetPosition)/s.cfg.InvSoftLimit)

	// Calculate bid/ask prices
	bidPrice := reservationPrice * (1 - skewFactor*spreadBps/2/10000)
	askPrice := reservationPrice * (1 + skewFactor*spreadBps/2/10000)

	// Adjust size based on volatility
	size := baseSize * math.Exp(-s.cfg.SizeVolK*vol)

	// Generate quotes
	var quotes []Quote
	if bidPrice > 0 && size > 0 {
		// Apply toxic flow reduce-only logic
		reduceOnly := false
		if s.cfg.AvoidToxic && s.cfg.ToxicReduceOnly && snap.VPIN > s.cfg.VPINToxicThreshold {
			if inventory < 0 {
				reduceOnly = true
			} else if inventory > 0 {
				// Long position, skip bid to avoid increasing
			} else {
				reduceOnly = false
			}
		}
		// Only add bid if not suppressed by toxic logic
		if inventory <= 0 || !s.cfg.ToxicReduceOnly || snap.VPIN <= s.cfg.VPINToxicThreshold {
			quotes = append(quotes, Quote{
				Price:      bidPrice,
				Size:       size,
				Side:       Bid,
				ReduceOnly: reduceOnly,
			})
		}
	}

	if askPrice > 0 && size > 0 {
		reduceOnly := false
		if s.cfg.AvoidToxic && s.cfg.ToxicReduceOnly && snap.VPIN > s.cfg.VPINToxicThreshold {
			if inventory > 0 {
				reduceOnly = true
			} else if inventory < 0 {
				// Short position, skip ask to avoid increasing
			} else {
				reduceOnly = false
			}
		}
		// Only add ask if not suppressed by toxic logic
		if inventory >= 0 || !s.cfg.ToxicReduceOnly || snap.VPIN <= s.cfg.VPINToxicThreshold {
			quotes = append(quotes, Quote{
				Price:      askPrice,
				Size:       size,
				Side:       Ask,
				ReduceOnly: reduceOnly,
			})
		}
	}

	// Update metrics
	adaptiveNetMax := invSoftLimit
	if s.adaptiveRisk != nil {
		adaptiveNetMax = s.adaptiveRisk.GetCurrentNetMax()
	}
	metrics.UpdateStrategyMetrics(reservationPrice, skewFactor*10000, adaptiveNetMax)
	metrics.VolatilityRegime.Set(float64(regime))
	metrics.VPINCurrent.Set(snap.VPIN)
	if s.adaptiveRisk != nil {
		metrics.AdverseSelectionRate.Set(s.adaptiveRisk.GetAverageAdverseRate())
	}
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
	regime := s.regimeDetector.DetectRegime(volatility, marketSnapshot.Imbalance, marketSnapshot.Mid)

	// Adjust spread based on volatility and regime
	spreadBps := s.spreadAdjuster.GetHalfSpread(volatility, MarketRegime(regime))

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
	metrics.UpdateMarketMetrics(0, int(regime), inventory)                                // VPIN set to 0 as placeholder
	metrics.UpdateStrategyMetrics(reservationPrice, inventorySkewBps, s.cfg.InvHardLimit) // AdaptiveNetMax placeholder
	metrics.UpdatePostTradeMetrics(0)                                                     // AdverseSelectionRate placeholder

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
