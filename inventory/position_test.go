package inventory

import "testing"

func TestTrackerUpdate(t *testing.T) {
	var tr Tracker
	tr.Update(1, 100)
	if tr.NetExposure() != 1 {
		t.Fatalf("expected net 1")
	}
	if tr.AvgCost() != 100 {
		t.Fatalf("expected cost 100 got %f", tr.AvgCost())
	}
	tr.Update(1, 110) // cost should move toward 105
	if tr.AvgCost() <= 100 || tr.AvgCost() >= 110 {
		t.Fatalf("unexpected avg cost %f", tr.AvgCost())
	}
}
