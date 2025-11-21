package market

// Publisher 一个轻量事件分发器。
type Publisher struct {
	depthSubs []chan Depth
	tradeSubs []chan Trade
}

func NewPublisher() *Publisher {
	return &Publisher{
		depthSubs: make([]chan Depth, 0),
		tradeSubs: make([]chan Trade, 0),
	}
}

func (p *Publisher) SubscribeDepth() <-chan Depth {
	ch := make(chan Depth, 1)
	p.depthSubs = append(p.depthSubs, ch)
	return ch
}

func (p *Publisher) SubscribeTrade() <-chan Trade {
	ch := make(chan Trade, 1)
	p.tradeSubs = append(p.tradeSubs, ch)
	return ch
}

func (p *Publisher) PublishDepth(d Depth) {
	for _, ch := range p.depthSubs {
		select {
		case ch <- d:
		default:
		}
	}
}

func (p *Publisher) PublishTrade(t Trade) {
	for _, ch := range p.tradeSubs {
		select {
		case ch <- t:
		default:
		}
	}
}
