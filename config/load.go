package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AppConfig holds the main runtime configuration.
type AppConfig struct {
	Env       string          `yaml:"env"`
	Risk      RiskConfig      `yaml:"risk"`
	Gateway   GatewayConfig   `yaml:"gateway"`
	Inventory InventoryConfig `yaml:"inventory"`
}

type RiskConfig struct {
	MaxOrderValueUSDT float64 `yaml:"maxOrderValueUSDT"`
	MaxNetExposure    float64 `yaml:"maxNetExposure"`
}

type GatewayConfig struct {
	APIKey    string `yaml:"apiKey"`
	APISecret string `yaml:"apiSecret"`
	BaseURL   string `yaml:"baseURL"`
}

type InventoryConfig struct {
	TargetPosition float64 `yaml:"targetPosition"`
	MaxDrift       float64 `yaml:"maxDrift"`
}

// Load reads YAML config from path and applies basic validation.
func Load(path string) (AppConfig, error) {
	var cfg AppConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parse yaml: %w", err)
	}
	if err := Validate(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// LoadWithEnvOverrides loads config then overrides sensitive fields from env vars if present.
func LoadWithEnvOverrides(path string) (AppConfig, error) {
	cfg, err := Load(path)
	if err != nil {
		return cfg, err
	}
	if v := os.Getenv("MM_GATEWAY_API_KEY"); v != "" {
		cfg.Gateway.APIKey = v
	}
	if v := os.Getenv("MM_GATEWAY_API_SECRET"); v != "" {
		cfg.Gateway.APISecret = v
	}
	return cfg, Validate(cfg)
}

// Validate ensures required fields are present.
func Validate(cfg AppConfig) error {
	if cfg.Env == "" {
		return errors.New("env is required")
	}
	if cfg.Risk.MaxOrderValueUSDT <= 0 {
		return errors.New("risk.maxOrderValueUSDT must be > 0")
	}
	if cfg.Gateway.APIKey == "" || cfg.Gateway.APISecret == "" {
		return errors.New("gateway.apiKey/apiSecret is required (or env overrides)")
	}
	return nil
}
