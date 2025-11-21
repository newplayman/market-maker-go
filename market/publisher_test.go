package market

import "testing"

func TestPublisher(t *testing.T) {
	p := NewPublisher()
	ch := p.SubscribeDepth()
	p.PublishDepth(Depth{Bid: 1, Ask: 2})
	if got := <-ch; got.Bid != 1 || got.Ask != 2 {
		t.Fatalf("unexpected depth %+v", got)
	}
}
