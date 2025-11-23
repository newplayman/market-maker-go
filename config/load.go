package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AppConfig holds the main runtime configuration.
type AppConfig struct {
	Env       string                  `yaml:"env"`
	Risk      RiskConfig              `yaml:"risk"`
	Gateway   GatewayConfig           `yaml:"gateway"`
	Inventory InventoryConfig         `yaml:"inventory"`
	Symbols   map[string]SymbolConfig `yaml:"symbols"`
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

// SymbolConfig 保存交易对的精度/名义限制（来自 exchangeInfo）。
type SymbolConfig struct {
	TickSize    float64        `yaml:"tickSize"`
	StepSize    float64        `yaml:"stepSize"`
	MinQty      float64        `yaml:"minQty"`
	MaxQty      float64        `yaml:"maxQty"`
	MinNotional float64        `yaml:"minNotional"`
	Strategy    StrategyParams `yaml:"strategy"`
	Risk        SymbolRisk     `yaml:"risk"`
}

type StrategyParams struct {
	MinSpread        float64 `yaml:"minSpread"`        // 最小绝对价差（若配合 mid 使用则视为基准 spread）
	BaseSize         float64 `yaml:"baseSize"`         // 标准下单数量，静态/动态腿均基于该数拆分
	TargetPosition   float64 `yaml:"targetPosition"`   // 策略期望的目标仓位
	MaxDrift         float64 `yaml:"maxDrift"`         // 允许偏离目标仓位的最大比例
	QuoteIntervalMs  int     `yaml:"quoteIntervalMs"`  // 报价基础周期（毫秒）
	TakeProfitPct    float64 `yaml:"takeProfitPct"`    // 浮盈达到该比例时触发锁盈/减仓
	StaticFraction   float64 `yaml:"staticFraction"`   // 静态挂单占 baseSize 的比例
	StaticTicks      int     `yaml:"staticTicks"`      // 静态挂单可容忍的偏差（tick 数）
	StaticRestMs     int     `yaml:"staticRestMs"`     // 静态挂单最小休眠时间（毫秒）
	DynamicRestMs    int     `yaml:"dynamicRestMs"`    // 动态挂单最小休眠时间（毫秒）
	DynamicRestTicks int     `yaml:"dynamicRestTicks"` // 动态挂单需要替换时的最小价差（tick）
}

type SymbolRisk struct {
	SingleMax                  float64 `yaml:"singleMax"`
	DailyMax                   float64 `yaml:"dailyMax"`
	NetMax                     float64 `yaml:"netMax"`
	LatencyMs                  int     `yaml:"latencyMs"`
	PnLMin                     float64 `yaml:"pnlMin"`
	PnLMax                     float64 `yaml:"pnlMax"`
	ReduceOnlyThreshold        float64 `yaml:"reduceOnlyThreshold"`
	ReduceOnlyMaxSlippagePct   float64 `yaml:"reduceOnlyMaxSlippagePct"`
	ReduceOnlyMarketTriggerPct float64 `yaml:"reduceOnlyMarketTriggerPct"`
	StopLoss                   float64 `yaml:"stopLoss"`
	HaltSeconds                int     `yaml:"haltSeconds"`
	ShockPct                   float64 `yaml:"shockPct"`
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
	if len(cfg.Symbols) == 0 {
		return errors.New("symbols config is required")
	}
	for sym, sc := range cfg.Symbols {
		if sc.TickSize <= 0 {
			return fmt.Errorf("symbol %s tickSize must be > 0", sym)
		}
		if sc.StepSize <= 0 {
			return fmt.Errorf("symbol %s stepSize must be > 0", sym)
		}
		if sc.MinQty < 0 || sc.MaxQty < 0 {
			return fmt.Errorf("symbol %s qty bounds must be >= 0", sym)
		}
		if sc.Strategy.MinSpread <= 0 {
			return fmt.Errorf("symbol %s strategy.minSpread must be > 0", sym)
		}
		if sc.Strategy.BaseSize <= 0 {
			return fmt.Errorf("symbol %s strategy.baseSize must be > 0", sym)
		}
		if sc.Strategy.QuoteIntervalMs < 0 {
			return fmt.Errorf("symbol %s strategy.quoteIntervalMs must be >= 0", sym)
		}
		if sc.Strategy.StaticRestMs < 0 {
			return fmt.Errorf("symbol %s strategy.staticRestMs must be >= 0", sym)
		}
		if sc.Strategy.DynamicRestMs < 0 {
			return fmt.Errorf("symbol %s strategy.dynamicRestMs must be >= 0", sym)
		}
		if sc.Strategy.DynamicRestTicks < 0 {
			return fmt.Errorf("symbol %s strategy.dynamicRestTicks must be >= 0", sym)
		}
		if sc.Risk.SingleMax < 0 || sc.Risk.DailyMax < 0 || sc.Risk.NetMax < 0 {
			return fmt.Errorf("symbol %s risk limits must be >= 0", sym)
		}
		if sc.Risk.LatencyMs < 0 {
			return fmt.Errorf("symbol %s risk.latencyMs must be >= 0", sym)
		}
		if sc.Risk.ReduceOnlyThreshold < 0 {
			return fmt.Errorf("symbol %s risk.reduceOnlyThreshold must be >= 0", sym)
		}
		if sc.Risk.ReduceOnlyMaxSlippagePct < 0 {
			return fmt.Errorf("symbol %s risk.reduceOnlyMaxSlippagePct must be >= 0", sym)
		}
		if sc.Risk.ReduceOnlyMarketTriggerPct < 0 {
			return fmt.Errorf("symbol %s risk.reduceOnlyMarketTriggerPct must be >= 0", sym)
		}
		if sc.Risk.HaltSeconds < 0 {
			return fmt.Errorf("symbol %s risk.haltSeconds must be >= 0", sym)
		}
		if sc.Risk.ShockPct < 0 {
			return fmt.Errorf("symbol %s risk.shockPct must be >= 0", sym)
		}
	}
	return nil
}
