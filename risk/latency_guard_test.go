package risk

import (
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) Now() time.Time { return f.t }

func TestLatencyGuard(t *testing.T) {
	fc := &fakeClock{t: time.Unix(0, 0)}
	guard := &LatencyGuard{
		MinInterval: 100 * time.Millisecond,
		clock:       fc,
	}
	if err := guard.PreOrder("BTC", 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := guard.PreOrder("BTC", -1); err != nil {
		t.Fatalf("sell should be allowed immediately: %v", err)
	}
	if err := guard.PreOrder("BTC", 1); err == nil {
		t.Fatalf("expected too frequent on repeated buy")
	}
	fc.t = fc.t.Add(200 * time.Millisecond)
	if err := guard.PreOrder("BTC", 1); err != nil {
		t.Fatalf("expected pass after interval")
	}
}
