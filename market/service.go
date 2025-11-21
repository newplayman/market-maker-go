package market

import (
	"sync"
	"time"
)

// Service 维护最新深度与交易，并向订阅者广播。
type Service struct {
	pub   *Publisher
	mu    sync.RWMutex
	depth map[string]Depth
	last  map[string]time.Time
}

func NewService(pub *Publisher) *Service {
	if pub == nil {
		pub = NewPublisher()
	}
	return &Service{
		pub:   pub,
		depth: make(map[string]Depth),
		last:  make(map[string]time.Time),
	}
}

// OnDepth 更新并广播。
func (s *Service) OnDepth(symbol string, bid, ask float64, ts time.Time) {
	s.mu.Lock()
	d := s.depth[symbol]
	d.Update(bid, ask)
	s.depth[symbol] = d
	s.last[symbol] = ts
	s.mu.Unlock()
	s.pub.PublishDepth(d)
}

// OnTrade 广播成交。
func (s *Service) OnTrade(symbol string, price, qty float64, ts time.Time) {
	s.pub.PublishTrade(Trade{Price: price, Qty: qty, Ts: ts})
}

// Mid 返回当前中间价；若缺失则返回 0。
func (s *Service) Mid(symbol string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.depth[symbol]
	if !ok || d.Bid == 0 || d.Ask == 0 {
		return 0
	}
	return (d.Bid + d.Ask) / 2
}

// Staleness 返回距离上次更新的时间间隔；如无数据返回正无穷。
func (s *Service) Staleness(symbol string) time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ts, ok := s.last[symbol]
	if !ok {
		return time.Hour * 24 * 365
	}
	return time.Since(ts)
}
