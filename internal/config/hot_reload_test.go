package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// MockParameterApplier 模拟参数应用器
type MockParameterApplier struct {
	applied map[string]interface{}
}

func NewMockParameterApplier() *MockParameterApplier {
	return &MockParameterApplier{
		applied: make(map[string]interface{}),
	}
}

func (m *MockParameterApplier) ApplyParameters(params map[string]interface{}) error {
	for k, v := range params {
		m.applied[k] = v
	}
	return nil
}

func (m *MockParameterApplier) GetApplied(key string) interface{} {
	return m.applied[key]
}

func TestHotReloader_New(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// 创建临时配置文件
	if err := os.WriteFile(configPath, []byte("test: value"), 0644); err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}

	cfg := DefaultHotReloadConfig()
	reloader, err := NewHotReloader(configPath, cfg)
	if err != nil {
		t.Fatalf("Failed to create hot reloader: %v", err)
	}
	defer reloader.Stop()

	if reloader == nil {
		t.Fatal("Reloader is nil")
	}

	if reloader.configPath != configPath {
		t.Errorf("Expected config path %s, got %s", configPath, reloader.configPath)
	}
}

func TestHotReloader_RegisterValidator(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(configPath, []byte("test: value"), 0644)

	cfg := DefaultHotReloadConfig()
	reloader, _ := NewHotReloader(configPath, cfg)
	defer reloader.Stop()

	validator := &StrategyParameterValidator{}
	reloader.RegisterValidator("strategy", validator)

	// 验证注册成功
	if len(reloader.validators) != 1 {
		t.Errorf("Expected 1 validator, got %d", len(reloader.validators))
	}
}

func TestHotReloader_RegisterApplier(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(configPath, []byte("test: value"), 0644)

	cfg := DefaultHotReloadConfig()
	reloader, _ := NewHotReloader(configPath, cfg)
	defer reloader.Stop()

	applier := NewMockParameterApplier()
	reloader.RegisterApplier("strategy", applier)

	// 验证注册成功
	if len(reloader.appliers) != 1 {
		t.Errorf("Expected 1 applier, got %d", len(reloader.appliers))
	}
}

func TestHotReloader_ValidateAndApply(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(configPath, []byte("test: value"), 0644)

	cfg := DefaultHotReloadConfig()
	reloader, _ := NewHotReloader(configPath, cfg)
	defer reloader.Stop()

	// 注册验证器和应用器
	validator := &StrategyParameterValidator{}
	applier := NewMockParameterApplier()

	reloader.RegisterValidator("strategy", validator)
	reloader.RegisterApplier("strategy", applier)

	// 测试有效参数
	validParams := map[string]interface{}{
		"base_spread":   0.001,
		"base_size":     0.01,
		"max_inventory": 0.05,
		"skew_factor":   0.3,
	}

	err := reloader.ApplyParameters("strategy", validParams)
	if err != nil {
		t.Errorf("Failed to apply valid parameters: %v", err)
	}

	// 验证参数已应用
	if applier.GetApplied("base_spread") != 0.001 {
		t.Error("Parameters not applied correctly")
	}
}

func TestHotReloader_StartStop(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(configPath, []byte("test: value"), 0644)

	cfg := DefaultHotReloadConfig()
	reloader, _ := NewHotReloader(configPath, cfg)

	ctx := context.Background()

	// 启动
	err := reloader.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start reloader: %v", err)
	}

	// 等待一段时间
	time.Sleep(100 * time.Millisecond)

	// 停止
	err = reloader.Stop()
	if err != nil {
		t.Errorf("Failed to stop reloader: %v", err)
	}
}

func TestStrategyParameterValidator_Valid(t *testing.T) {
	validator := &StrategyParameterValidator{}

	testCases := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name: "Valid parameters",
			params: map[string]interface{}{
				"base_spread":   0.001,
				"base_size":     0.01,
				"max_inventory": 0.05,
				"skew_factor":   0.3,
			},
		},
		{
			name: "Minimum values",
			params: map[string]interface{}{
				"base_spread":   0.0001,
				"base_size":     0.001,
				"max_inventory": 0.001,
				"skew_factor":   0.0,
			},
		},
		{
			name: "Maximum values",
			params: map[string]interface{}{
				"base_spread":   0.099,
				"base_size":     9.9,
				"max_inventory": 99.9,
				"skew_factor":   1.0,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Validate(tc.params)
			if err != nil {
				t.Errorf("Expected valid parameters but got error: %v", err)
			}
		})
	}
}

func TestStrategyParameterValidator_Invalid(t *testing.T) {
	validator := &StrategyParameterValidator{}

	testCases := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name: "Invalid base_spread (too small)",
			params: map[string]interface{}{
				"base_spread": 0.0,
			},
		},
		{
			name: "Invalid base_spread (too large)",
			params: map[string]interface{}{
				"base_spread": 0.2,
			},
		},
		{
			name: "Invalid base_size (negative)",
			params: map[string]interface{}{
				"base_size": -0.01,
			},
		},
		{
			name: "Invalid max_inventory (too large)",
			params: map[string]interface{}{
				"max_inventory": 200.0,
			},
		},
		{
			name: "Invalid skew_factor (negative)",
			params: map[string]interface{}{
				"skew_factor": -0.1,
			},
		},
		{
			name: "Invalid skew_factor (too large)",
			params: map[string]interface{}{
				"skew_factor": 1.5,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Validate(tc.params)
			if err == nil {
				t.Error("Expected validation error but got none")
			}
		})
	}
}

func TestRiskParameterValidator_Valid(t *testing.T) {
	validator := &RiskParameterValidator{}

	validParams := map[string]interface{}{
		"daily_loss_limit":          100.0,
		"max_drawdown_limit":        0.05,
		"circuit_breaker_threshold": 5,
	}

	err := validator.Validate(validParams)
	if err != nil {
		t.Errorf("Expected valid parameters but got error: %v", err)
	}
}

func TestRiskParameterValidator_Invalid(t *testing.T) {
	validator := &RiskParameterValidator{}

	testCases := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name: "Invalid daily_loss_limit",
			params: map[string]interface{}{
				"daily_loss_limit": -100.0,
			},
		},
		{
			name: "Invalid max_drawdown_limit (too large)",
			params: map[string]interface{}{
				"max_drawdown_limit": 1.5,
			},
		},
		{
			name: "Invalid circuit_breaker_threshold (too large)",
			params: map[string]interface{}{
				"circuit_breaker_threshold": 200,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Validate(tc.params)
			if err == nil {
				t.Error("Expected validation error but got none")
			}
		})
	}
}

func TestAlertParameterValidator_Valid(t *testing.T) {
	validator := &AlertParameterValidator{}

	validParams := map[string]interface{}{
		"throttle_interval": "5m",
	}

	err := validator.Validate(validParams)
	if err != nil {
		t.Errorf("Expected valid parameters but got error: %v", err)
	}
}

func TestAlertParameterValidator_Invalid(t *testing.T) {
	validator := &AlertParameterValidator{}

	invalidParams := map[string]interface{}{
		"throttle_interval": "invalid",
	}

	err := validator.Validate(invalidParams)
	if err == nil {
		t.Error("Expected validation error but got none")
	}
}

func TestHotReloader_GetLastReloadTime(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(configPath, []byte("test: value"), 0644)

	cfg := DefaultHotReloadConfig()
	reloader, _ := NewHotReloader(configPath, cfg)
	defer reloader.Stop()

	// 初始时间应该是零值
	lastTime := reloader.GetLastReloadTime()
	if !lastTime.IsZero() {
		t.Error("Expected zero time for last reload")
	}
}
