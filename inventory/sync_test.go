package inventory

import "testing"

func TestSyncSnapshot(t *testing.T) {
	tr := &Tracker{}
	tr.Update(1, 100)
	s := Sync{Tracker: tr}
	net, pnl := s.Snapshot(110)
	if net != 1 || pnl <= 0 {
		t.Fatalf("unexpected snapshot net=%f pnl=%f", net, pnl)
	}
}
