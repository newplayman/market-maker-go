package order

import "testing"

func TestSymbolConstraintsValidate(t *testing.T) {
	c := SymbolConstraints{
		TickSize:    0.01,
		StepSize:    0.001,
		MinQty:      0.001,
		MaxQty:      10,
		MinNotional: 5,
	}
	if err := c.Validate(100.01, 0.1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := c.Validate(100.015, 0.002); err == nil {
		t.Fatalf("expected tick size error")
	}
	if err := c.Validate(100.01, 0.0005); err == nil {
		t.Fatalf("expected qty error")
	}
	if err := c.Validate(100.01, 0.0006); err == nil {
		t.Fatalf("expected min qty error")
	}
	if err := c.Validate(100.01, 11); err == nil {
		t.Fatalf("expected max qty error")
	}
	if err := c.Validate(10, 0.2); err == nil {
		t.Fatalf("expected notional error")
	}
}
