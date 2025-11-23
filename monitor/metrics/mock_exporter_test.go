package metrics

import "testing"

func TestMockCounter(t *testing.T) {
	var c MockCounter
	c.Inc()
	c.Add(2)
	if c.Value != 3 {
		t.Fatalf("unexpected value %.1f", c.Value)
	}
}

func TestMockGauge(t *testing.T) {
	var g MockGauge
	g.Set(1.5)
	g.Set(2.1)
	if g.Value != 2.1 {
		t.Fatalf("unexpected gauge %.1f", g.Value)
	}
}

func TestMockHistogram(t *testing.T) {
	var h MockHistogram
	h.Observe(0.1)
	h.Observe(0.2)
	if len(h.Values) != 2 || h.Values[1] != 0.2 {
		t.Fatalf("unexpected histogram %+v", h.Values)
	}
}
