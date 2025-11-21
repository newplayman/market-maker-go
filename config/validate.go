package config

// ValidateConfig is kept for backward compatibility; delegates to Validate.
func ValidateConfig(cfg AppConfig) error {
	return Validate(cfg)
}
