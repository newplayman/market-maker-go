package sim

import (
	"testing"
	"time"
)

func TestBuildRunner(t *testing.T) {
	r, err := BuildRunner(RunnerConfig{
		Symbol:         "BTCUSDT",
		MinSpread:      0.001,
		BaseSize:       1,
		TargetPosition: 0,
		MaxDrift:       1,
		SingleMax:      5,
		DailyMax:       50,
		NetMax:         10,
		MaxSpreadRatio: 0.05,
		MinInterval:    200 * time.Millisecond,
		MinPnL:         -10,
		MaxPnL:         100,
	})
	if err != nil {
		t.Fatalf("build runner err: %v", err)
	}
	if r.Engine == nil || r.Risk == nil || r.Book == nil {
		t.Fatalf("runner components not initialized")
	}
}
