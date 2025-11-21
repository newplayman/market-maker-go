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
