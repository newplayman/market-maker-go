package order

import "testing"

func TestBookSetGetList(t *testing.T) {
	b := NewBook()
	o := Order{ID: "1", Symbol: "BTCUSDT", Status: StatusNew}
	b.Set(o)
	got, ok := b.Get("1")
	if !ok || got.Symbol != "BTCUSDT" {
		t.Fatalf("get failed: %+v %v", got, ok)
	}
	list := b.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 order, got %d", len(list))
	}
}
