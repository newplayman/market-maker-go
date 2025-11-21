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
	return nil
}

// ErrInvalid 用于参数验证错误。
type ErrInvalid string

func (e ErrInvalid) Error() string { return string(e) }
