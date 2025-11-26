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
	Type                       string  `yaml:"type"`                     // 策略类型 ("grid" or "asmm")
	MinSpread                  float64 `yaml:"minSpread"`                // 最小绝对价差（若配合 mid 使用则视为基准 spread）
	FeeBuffer                  float64 `yaml:"feeBuffer"`                // 附加价差，用于覆盖手续费
	BaseSize                   float64 `yaml:"baseSize"`                 // 标准下单数量，静态/动态腿均基于该数拆分
	TargetPosition             float64 `yaml:"targetPosition"`           // 策略期望的目标仓位
	MaxDrift                   float64 `yaml:"maxDrift"`                 // 允许偏离目标仓位的最大比例
	QuoteIntervalMs            int     `yaml:"quoteIntervalMs"`          // 报价基础周期（毫秒）
	TakeProfitPct              float64 `yaml:"takeProfitPct"`            // 浮盈达到该比例时触发锁盈/减仓
	StaticFraction             float64 `yaml:"staticFraction"`           // 静态挂单占 baseSize 的比例
	StaticTicks                int     `yaml:"staticTicks"`              // 静态挂单可容忍的偏差（tick 数）
	StaticRestMs               int     `yaml:"staticRestMs"`             // 静态挂单最小休眠时间（毫秒）
	DynamicRestMs              int     `yaml:"dynamicRestMs"`            // 动态挂单最小休眠时间（毫秒）
	DynamicRestTicks           int     `yaml:"dynamicRestTicks"`         // 动态挂单需要替换时的最小价差（tick）
	InventoryPressureThreshold float64 `yaml:"inventoryPressureThreshold"`
	InventoryPressureStrength  float64 `yaml:"inventoryPressureStrength"`
	InventoryPressureExponent  float64 `yaml:"inventoryPressureExponent"`
	MomentumThreshold          float64 `yaml:"momentumThreshold"`
	MomentumAlpha              float64 `yaml:"momentumAlpha"`
	EnableMultiLayer           bool    `yaml:"enableMultiLayer"`         // 是否启用多层持仓
	LayerCount                 int     `yaml:"layerCount"`               // 层数（2-3）
	LayerSpacing               float64 `yaml:"layerSpacing"`             // 层间距（百分比）
	// 几何网格参数
	LayerSpacingMode           string  `yaml:"layerSpacingMode"`         // "linear" or "geometric"
	SpacingRatio               float64 `yaml:"spacingRatio"`             // 几何模式下层间距比例（如 1.20）
	LayerSizeDecay             float64 `yaml:"layerSizeDecay"`           // 远端下单量衰减系数（如 0.90）
	MaxLayers                  int     `yaml:"maxLayers"`                // 最大层数（默认 24）
	
	// ASMM策略专用参数
	MinSpreadBps               float64 `yaml:"minSpreadBps"`             // 最小价差（基点）
	MaxSpreadBps               float64 `yaml:"maxSpreadBps"`             // 最大价差（基点）
	MinSpacingBps              float64 `yaml:"minSpacingBps"`            // 最小档位间距（基点）
	MaxLevels                  int     `yaml:"maxLevels"`                // 最大档位数
	SizeVolK                   float64 `yaml:"sizeVolK"`                 // 价格波动系数
	InvSoftLimit               float64 `yaml:"invSoftLimit"`             // 软性仓位限制
	InvHardLimit               float64 `yaml:"invHardLimit"`             // 硬性仓位限制
	InvSkewK                   float64 `yaml:"invSkewK"`                 // 仓位偏移系数
	VolK                       float64 `yaml:"volK"`                     // 波动率系数
	TrendSpreadMultiplier      float64 `yaml:"trendSpreadMultiplier"`    // 趋势市场价差乘数
	HighVolSpreadMultiplier    float64 `yaml:"highVolSpreadMultiplier"`  // 高波动市场价差乘数
	AvoidToxic                 bool    `yaml:"avoidToxic"`               // 是否避免有毒订单流
}

type SymbolRisk struct {
	SingleMax                  float64   `yaml:"singleMax"`
	DailyMax                   float64   `yaml:"dailyMax"`
	NetMax                     float64   `yaml:"netMax"`
	LatencyMs                  int       `yaml:"latencyMs"`
	PnLMin                     float64   `yaml:"pnlMin"`
	PnLMax                     float64   `yaml:"pnlMax"`
	ReduceOnlyThreshold        float64   `yaml:"reduceOnlyThreshold"`
	ReduceOnlyMaxSlippagePct   float64   `yaml:"reduceOnlyMaxSlippagePct"`
	ReduceOnlyMarketTriggerPct float64   `yaml:"reduceOnlyMarketTriggerPct"`
	StopLoss                   float64   `yaml:"stopLoss"`
	HaltSeconds                int       `yaml:"haltSeconds"`
	ShockPct                   float64   `yaml:"shockPct"`
	// 浮亏分层减仓
	DrawdownBands           []float64 `yaml:"drawdownBands"`
	ReduceFractions         []float64 `yaml:"reduceFractions"`
	ReduceMode              string    `yaml:"reduceMode"`
	ReduceCooldownSeconds   int       `yaml:"reduceCooldownSeconds"`
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
	var cfg AppConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parse yaml: %w", err)
	}
	// 先覆盖环境变量
	if v := os.Getenv("MM_GATEWAY_API_KEY"); v != "" {
		cfg.Gateway.APIKey = v
	}
	if v := os.Getenv("MM_GATEWAY_API_SECRET"); v != "" {
		cfg.Gateway.APISecret = v
	}
	// 然后再验证
	if err := Validate(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
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
		if sc.Strategy.InventoryPressureThreshold < 0 || sc.Strategy.InventoryPressureStrength < 0 || sc.Strategy.InventoryPressureExponent < 0 {
			return fmt.Errorf("symbol %s strategy.inventoryPressure* must be >= 0", sym)
		}
		if sc.Strategy.MomentumThreshold < 0 || sc.Strategy.MomentumAlpha < 0 {
			return fmt.Errorf("symbol %s strategy.momentum* must be >= 0", sym)
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
