package asmm

// ASMMConfig holds config for ASMM strategy.
type ASMMConfig struct {
	QuoteIntervalMs int     `json:"quoteIntervalMs"`
	MinSpreadBps    float64 `json:"minSpreadBps"`
	MaxSpreadBps    float64 `json:"maxSpreadBps"`
	MinSpacingBps   float64 `json:"minSpacingBps"`
	MaxLevels       int     `json:"maxLevels"`
	BaseSize        float64 `json:"baseSize"`
	SizeVolK        float64 `json:"sizeVolK"`

	TargetPosition float64 `json:"targetPosition"`
	InvSoftLimit   float64 `json:"invSoftLimit"`
	InvHardLimit   float64 `json:"invHardLimit"`
	InvSkewK       float64 `json:"invSkewK"`

	VolK                    float64 `json:"volK"`
	TrendSpreadMultiplier   float64 `json:"trendSpreadMultiplier"`
	HighVolSpreadMultiplier float64 `json:"highVolSpreadMultiplier"`
	AvoidToxic              bool    `json:"avoidToxic"`
	// VPIN/Toxic flow settings
	VPINToxicThreshold    float64 `json:"vpinToxicThreshold"`
	ToxicSpreadMultiplier float64 `json:"toxicSpreadMultiplier"`
	ToxicReduceOnly       bool    `json:"toxicReduceOnly"`
}

// DefaultASMMConfig returns a default config.
func DefaultASMMConfig() ASMMConfig {
	return ASMMConfig{
		QuoteIntervalMs: 150,
		MinSpreadBps:    6,
		MaxSpreadBps:    40,
		MinSpacingBps:   4,
		MaxLevels:       3,
		BaseSize:        0.01,
		SizeVolK:        0.5,

		TargetPosition: 0,
		InvSoftLimit:   3,
		InvHardLimit:   5,
		InvSkewK:       1.5,

		VolK:                    0.8,
		TrendSpreadMultiplier:   1.5,
		HighVolSpreadMultiplier: 2.0,
		AvoidToxic:              true,
		VPINToxicThreshold:      0.4,
		ToxicSpreadMultiplier:   2.5,
		ToxicReduceOnly:         true,
	}
}

// Validate checks if the ASMMConfig is valid.
func (c *ASMMConfig) Validate() bool {
	if c.QuoteIntervalMs <= 0 {
		return false
	}
	if c.MinSpreadBps <= 0 || c.MaxSpreadBps <= 0 || c.MinSpreadBps > c.MaxSpreadBps {
		return false
	}
	if c.MinSpacingBps <= 0 {
		return false
	}
	if c.MaxLevels <= 0 {
		return false
	}
	if c.BaseSize <= 0 {
		return false
	}
	if c.InvSoftLimit <= 0 || c.InvHardLimit <= 0 || c.InvSoftLimit > c.InvHardLimit {
		return false
	}
	if c.InvSkewK < 0 {
		return false
	}
	if c.VolK < 0 {
		return false
	}
	if c.TrendSpreadMultiplier <= 0 {
		return false
	}
	if c.HighVolSpreadMultiplier <= 0 {
		return false
	}
	return true
}
