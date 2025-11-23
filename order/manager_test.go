package order

import "testing"

type mockGateway struct {
	placed    []Order
	canceled  []string
	errPlace  error
	errCancel error
}

func (m *mockGateway) Place(o Order) (string, error) {
	m.placed = append(m.placed, o)
	return o.ID, m.errPlace
}

func (m *mockGateway) Cancel(id string) error {
	m.canceled = append(m.canceled, id)
	return m.errCancel
}

func TestManagerSubmitAndCancel(t *testing.T) {
	gw := &mockGateway{}
	m := NewManager(gw)
	o := Order{Symbol: "BTCUSDT", Side: "BUY", Price: 100, Quantity: 1}
	sent, err := m.Submit(o)
	if err != nil {
		t.Fatalf("submit err: %v", err)
	}
	if sent.Status != StatusAck {
		t.Fatalf("expected ACK status, got %s", sent.Status)
	}
	if err := m.Cancel(sent.ID); err != nil {
		t.Fatalf("cancel err: %v", err)
	}
}

func TestManagerConstraint(t *testing.T) {
	gw := &mockGateway{}
	m := NewManager(gw)
	m.SetConstraints(map[string]SymbolConstraints{
		"ETHUSDC": {
			TickSize: 0.01,
			StepSize: 0.001,
			MinQty:   0.001,
		},
	})
	if _, err := m.Submit(Order{Symbol: "ETHUSDC", Price: 100.01, Quantity: 0.002}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, err := m.Submit(Order{Symbol: "ETHUSDC", Price: 100.015, Quantity: 0.002}); err == nil {
		t.Fatalf("expected ticksize error")
	}
}
