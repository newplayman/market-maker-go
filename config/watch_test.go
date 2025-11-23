package config

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWatcherSkipsOnStatError(t *testing.T) {
	orig := readFileInfo
	defer func() { readFileInfo = orig }()
	readFileInfo = func(string) (interface{ ModTime() time.Time }, error) {
		return nil, errors.New("boom")
	}
	w := Watcher{Path: "noop", Interval: 10 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	if err := w.Start(ctx, nil); err == nil {
		t.Fatalf("expected context cancellation")
	}
}

func TestWatcherTriggersOnChange(t *testing.T) {
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
      latencyMs: 0
`)

	now := time.Now()
	orig := readFileInfo
	defer func() { readFileInfo = orig }()
	tick := 0
	readFileInfo = func(string) (interface{ ModTime() time.Time }, error) {
		tick++
		if tick == 1 {
			return fakeInfo{mod: now}, nil
		}
		return fakeInfo{mod: now.Add(time.Second)}, nil
	}

	w := Watcher{Path: path, Interval: 5 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan struct{}, 1)
	go func() {
		_ = w.Start(ctx, func(AppConfig) { ch <- struct{}{} })
	}()
	select {
	case <-ch:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected update callback")
	}
}

type fakeInfo struct{ mod time.Time }

func (f fakeInfo) ModTime() time.Time { return f.mod }
