package config

// ValidateParams 额外验证非空/非零的关键参数。
func ValidateParams(cfg AppConfig) error {
	if cfg.Risk.MaxOrderValueUSDT <= 0 {
		return ErrInvalid("risk.maxOrderValueUSDT must be > 0")
	}
	if cfg.Risk.MaxNetExposure <= 0 {
		return ErrInvalid("risk.maxNetExposure must be > 0")
	}
	if cfg.Gateway.APIKey == "" || cfg.Gateway.APISecret == "" {
		return ErrInvalid("gateway.apiKey/apiSecret is required")
	}
	if cfg.Inventory.MaxDrift <= 0 {
		return ErrInvalid("inventory.maxDrift must be > 0")
	}
	for sym, sc := range cfg.Symbols {
		if sc.TickSize <= 0 {
			return ErrInvalid("symbol " + sym + " tickSize must be > 0")
		}
		if sc.StepSize <= 0 {
			return ErrInvalid("symbol " + sym + " stepSize must be > 0")
		}
		if sc.MinQty < 0 || sc.MaxQty < 0 {
			return ErrInvalid("symbol " + sym + " qty bounds must be >= 0")
		}
		if sc.Strategy.MinSpread <= 0 || sc.Strategy.BaseSize <= 0 {
			return ErrInvalid("symbol " + sym + " strategy.minSpread/baseSize must be > 0")
		}
		if sc.Strategy.FeeBuffer < 0 {
			return ErrInvalid("symbol " + sym + " strategy.feeBuffer must be >= 0")
		}
		if sc.Strategy.QuoteIntervalMs < 0 {
			return ErrInvalid("symbol " + sym + " strategy.quoteIntervalMs must be >= 0")
		}
		if sc.Strategy.StaticFraction < 0 || sc.Strategy.StaticFraction > 1 {
			return ErrInvalid("symbol " + sym + " strategy.staticFraction must be between 0 and 1")
		}
		if sc.Strategy.StaticTicks < 0 {
			return ErrInvalid("symbol " + sym + " strategy.staticTicks must be >= 0")
		}
		if sc.Strategy.InventoryPressureThreshold < 0 || sc.Strategy.InventoryPressureStrength < 0 || sc.Strategy.InventoryPressureExponent < 0 {
			return ErrInvalid("symbol " + sym + " strategy.inventoryPressure* must be >= 0")
		}
		if sc.Risk.SingleMax < 0 || sc.Risk.DailyMax < 0 || sc.Risk.NetMax < 0 {
			return ErrInvalid("symbol " + sym + " risk limits must be >= 0")
		}
		if sc.Risk.LatencyMs < 0 {
			return ErrInvalid("symbol " + sym + " risk.latencyMs must be >= 0")
		}
		if sc.Risk.ReduceOnlyThreshold < 0 || sc.Risk.ReduceOnlyMaxSlippagePct < 0 || sc.Risk.ReduceOnlyMarketTriggerPct < 0 {
			return ErrInvalid("symbol " + sym + " reduceOnly params must be >= 0")
		}
	}
	return nil
}

// ErrInvalid 用于参数验证错误。
type ErrInvalid string

func (e ErrInvalid) Error() string { return string(e) }
