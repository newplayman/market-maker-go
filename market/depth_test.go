package market

import "testing"

func TestDepthUpdate(t *testing.T) {
	var d Depth
	d.Update(100, 101)
	if d.Bid != 100 || d.Ask != 101 {
		t.Fatalf("unexpected depth: %+v", d)
	}
	// partial update keeps previous
	d.Update(0, 102)
	if d.Ask != 102 || d.Bid != 100 {
		t.Fatalf("partial update failed: %+v", d)
	}
}
