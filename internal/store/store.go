package store

import (
	"math"
	"strings"
	"sync"
	"time"

	"market-maker-go/gateway"
	"market-maker-go/metrics"
)

// Store 维护用户侧状态（订单、仓位、价格序列）。
// 提供五个只读方法供策略/风控调用。
type Store struct {
	Symbol string

	mu            sync.RWMutex
	pendingOrders map[int64]orderEntry
	pendingBuy    float64
	pendingSell   float64

	position float64
	mid      float64

	// 30分钟价格序列，用于计算标准差
	prices []pricePoint

	// 资金费率预测（EMA）
	alpha                float64
	predictedFundingRate float64
	fundingPnlAcc        float64
}

type orderEntry struct {
	side    string
	openQty float64
}

type pricePoint struct {
	p  float64
	ts time.Time
}

func New(symbol string, predictAlpha float64) *Store {
	return &Store{
		Symbol:        symbol,
		pendingOrders: make(map[int64]orderEntry),
		prices:        make([]pricePoint, 0, 900),
		alpha:         predictAlpha,
	}
}

// PendingBuySize 所有活跃买单数量之和
func (s *Store) PendingBuySize() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pendingBuy
}

// PendingSellSize 所有活跃卖单数量之和
func (s *Store) PendingSellSize() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pendingSell
}

// MidPrice 当前中值价
func (s *Store) MidPrice() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mid
}

// Position 当前净仓位
func (s *Store) Position() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.position
}

// PriceStdDev30m 最近30分钟价格标准差
func (s *Store) PriceStdDev30m() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cut := time.Now().Add(-30 * time.Minute)
	var vals []float64
	for i := len(s.prices) - 1; i >= 0; i-- {
		pt := s.prices[i]
		if pt.ts.Before(cut) {
			break
		}
		vals = append(vals, pt.p)
	}
	if len(vals) < 2 {
		return 0
	}
	// 计算标准差
	mean := 0.0
	for _, v := range vals {
		mean += v
	}
	mean /= float64(len(vals))
	var varSum float64
	for _, v := range vals {
		d := v - mean
		varSum += d * d
	}
	sd := math.Sqrt(varSum / float64(len(vals)))
	metrics.PriceStdDev30m.Set(sd)
	return sd
}

// PredictedFundingRate 当前预测的下一期资金费率（正=多头吃亏）
func (s *Store) PredictedFundingRate() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.predictedFundingRate
}

// HandleOrderUpdate 订单事件处理，维护活跃订单聚合尺寸。
func (s *Store) HandleOrderUpdate(o gateway.OrderUpdate) {
	if o.Symbol != s.Symbol {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ent := s.pendingOrders[o.OrderID]
	ent.side = strings.ToUpper(o.Side)
	// 估算未成交剩余量：用 OrigQty - AccumulatedQty（若有）
	openQty := o.OrigQty - o.AccumulatedQty
	if openQty < 0 {
		openQty = 0
	}
	previous := ent.openQty
	ent.openQty = openQty
	s.pendingOrders[o.OrderID] = ent
	// 更新聚合
	if ent.side == "BUY" {
		s.pendingBuy += (openQty - previous)
		if openQty == 0 && previous > 0 {
			s.pendingBuy -= previous
		}
	} else if ent.side == "SELL" {
		s.pendingSell += (openQty - previous)
		if openQty == 0 && previous > 0 {
			s.pendingSell -= previous
		}
	}
	if s.pendingBuy < 0 {
		s.pendingBuy = 0
	}
	if s.pendingSell < 0 {
		s.pendingSell = 0
	}
	metrics.UpdateOrderMetrics(s.Symbol, int(s.pendingBuy), int(s.pendingSell))
}

// HandlePositionUpdate 仓位事件处理。
func (s *Store) HandlePositionUpdate(a gateway.AccountUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range a.Positions {
		if p.Symbol == s.Symbol {
			s.position = p.PositionAmt
			metrics.UpdatePositionMetrics(s.Symbol, s.position, p.EntryPrice, p.PnL, 0)
			break
		}
	}
}

// UpdateDepth 更新中间价并记录价格序列。
func (s *Store) UpdateDepth(bid, ask float64, ts time.Time) {
	mid := 0.0
	if bid > 0 && ask > 0 {
		mid = (bid + ask) / 2
	}
	s.mu.Lock()
	s.mid = mid
	s.prices = append(s.prices, pricePoint{p: mid, ts: ts})
	// 修剪过长序列（保留最近 ~60 分钟的数据）
	if len(s.prices) > 3600 {
		s.prices = s.prices[len(s.prices)-3600:]
	}
	s.mu.Unlock()
	metrics.UpdateMarketData(s.Symbol, mid, bid, ask)
}

// HandleFundingRate 外部资金费率事件回调。
func (s *Store) HandleFundingRate(rate float64) {
	s.mu.Lock()
	// EMA 预测
	if s.predictedFundingRate == 0 {
		s.predictedFundingRate = rate
	} else {
		s.predictedFundingRate = s.alpha*rate + (1-s.alpha)*s.predictedFundingRate
	}
	// 粗略计费：position * mid * rate
	delta := s.position * s.mid * rate
	s.fundingPnlAcc += delta
	pf := s.predictedFundingRate
	acc := s.fundingPnlAcc
	s.mu.Unlock()
	metrics.PredictedFundingRate.Set(pf)
	metrics.FundingPnlAccum.Set(acc)
}
