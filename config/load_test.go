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
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Env != "dev" || cfg.Gateway.APIKey != "foo" {
		t.Fatalf("unexpected cfg values: %+v", cfg)
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
