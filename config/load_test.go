package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoad(t *testing.T) {
	path := writeTempConfig(t, `
env: dev
risk:
  maxOrderValueUSDT: 1000
  maxNetExposure: 0.5
gateway:
  apiKey: foo
  apiSecret: bar
  baseURL: https://api.test
inventory:
  targetPosition: 0.1
  maxDrift: 0.01
symbols:
  ETHUSDC:
    tickSize: 0.01
    stepSize: 0.001
    minQty: 0.001
    maxQty: 10
    minNotional: 5
    strategy:
      minSpread: 0.0008
      baseSize: 0.01
      targetPosition: 0
      maxDrift: 1
      quoteIntervalMs: 1000
    risk:
      singleMax: 1
      dailyMax: 10
      netMax: 5
      latencyMs: 500
      pnlMin: -5
      pnlMax: 10
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Env != "dev" || cfg.Gateway.APIKey != "foo" {
		t.Fatalf("unexpected cfg values: %+v", cfg)
	}
	if cfg.Symbols["ETHUSDC"].TickSize != 0.01 {
		t.Fatalf("symbol config not parsed: %+v", cfg.Symbols)
	}
}

func TestLoadWithEnvOverrides(t *testing.T) {
	path := writeTempConfig(t, `
env: prod
risk:
  maxOrderValueUSDT: 10
  maxNetExposure: 0.1
gateway:
  apiKey: foo
  apiSecret: bar
  baseURL: https://api.test
inventory:
  targetPosition: 0.1
  maxDrift: 0.01
symbols:
  ETHUSDC:
    tickSize: 0.01
    stepSize: 0.001
    minQty: 0.001
    maxQty: 1
    minNotional: 5
    strategy:
      minSpread: 0.0008
      baseSize: 0.01
      targetPosition: 0
      maxDrift: 1
      quoteIntervalMs: 1000
    risk:
      singleMax: 1
      dailyMax: 1
      netMax: 1
      latencyMs: 500
`)
	t.Setenv("MM_GATEWAY_API_KEY", "env-key")
	t.Setenv("MM_GATEWAY_API_SECRET", "env-secret")
	cfg, err := LoadWithEnvOverrides(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Gateway.APIKey != "env-key" || cfg.Gateway.APISecret != "env-secret" {
		t.Fatalf("env overrides not applied: %+v", cfg.Gateway)
	}
}

func TestValidate(t *testing.T) {
	err := Validate(AppConfig{})
	if err == nil {
		t.Fatalf("expected error for empty config")
	}
}
