package risk

import (
	"testing"
	"time"
)

func TestCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(0.01, 0.02)
	now := time.Now()
	// stable prices
	for i := 0; i < 5; i++ {
		if trip, _ := cb.OnTick(Tick{Price: 100, Ts: now.Add(time.Duration(i) * 10 * time.Second)}); trip {
			t.Fatalf("did not expect trip")
		}
	}
	// jump 2% within 1m triggers
	trip, span := cb.OnTick(Tick{Price: 102, Ts: now.Add(30 * time.Second)})
	if !trip || span != "1m" {
		t.Fatalf("expected 1m trip")
	}
}
