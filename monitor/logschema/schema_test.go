package logschema

import "testing"

func TestValidate(t *testing.T) {
	err := Validate("strategy_adjust", map[string]interface{}{
		"symbol":      "ETHUSDC",
		"mid":         2700.0,
		"spread":      1.2,
		"spreadRatio": 0.0005,
		"intervalMs":  int64(300),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = Validate("strategy_adjust", map[string]interface{}{
		"symbol": "ETHUSDC",
	})
	if err == nil {
		t.Fatalf("expected error for missing fields")
	}
}

func TestKnownEvents(t *testing.T) {
	names := Known()
	if len(names) == 0 {
		t.Fatalf("expected non-empty schema list")
	}
	found := false
	for _, n := range names {
		if n == "risk_event" {
			found = true
		}
	}
	if !found {
		t.Fatalf("risk_event not found in schemas")
	}
}
