package risk

import "testing"

type memAlert struct{ typ, msg string }

func (m *memAlert) Send(typ, msg string) {
	m.typ = typ
	m.msg = msg
}

func TestNotifier(t *testing.T) {
	alert := &memAlert{}
	n := NewNotifier(alert)
	n.NotifyLimitExceeded("ETHUSDT", ErrSingleExceed)
	if alert.typ != "RiskLimit" {
		t.Fatalf("expected RiskLimit, got %s", alert.typ)
	}
	n.NotifyCircuitTrip("5m", 123)
	if alert.typ != "CircuitBreaker" {
		t.Fatalf("expected CircuitBreaker, got %s", alert.typ)
	}
}
