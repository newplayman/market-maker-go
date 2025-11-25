package risk

// AdaptiveRiskConfig holds configuration for adaptive risk management
type AdaptiveRiskConfig struct {
	BaseNetMax float64 // 基础净持仓上限
	MinNetMax  float64 // 最小净持仓上限
}

// AdaptiveRiskManager implements adaptive risk management based on market conditions and post-trade stats
type AdaptiveRiskManager struct {
	base      *LimitChecker
	cfg       AdaptiveRiskConfig
}

// NewAdaptiveRiskManager creates a new adaptive risk manager
func NewAdaptiveRiskManager(base *LimitChecker, cfg AdaptiveRiskConfig) *AdaptiveRiskManager {
	return &AdaptiveRiskManager{
		base: base,
		cfg:  cfg,
	}
}

// EffectiveNetMax calculates the effective net position limit based on market conditions and post-trade stats
func (a *AdaptiveRiskManager) EffectiveNetMax(volatility float64, isToxic bool, adverseSelectionRate float64) float64 {
	netMax := a.cfg.BaseNetMax

	// Reduce limits under high volatility
	if volatility > 0.02 { // 2% threshold
		netMax *= 0.7
	}
	
	// Further reduce limits under toxic conditions
	if isToxic {
		netMax *= 0.5
	}

	// Reduce limits with high adverse selection rate
	if adverseSelectionRate > 0.6 {
		netMax *= 0.5
	}

	// Ensure we don't go below minimum
	if netMax < a.cfg.MinNetMax {
		netMax = a.cfg.MinNetMax
	}
	
	return netMax
}

// UpdateLimits updates the underlying limit checker with adaptive limits
func (a *AdaptiveRiskManager) UpdateLimits(volatility float64, isToxic bool, adverseSelectionRate float64) {
	effectiveNetMax := a.EffectiveNetMax(volatility, isToxic, adverseSelectionRate)
	a.base.limits.NetMax = effectiveNetMax
}