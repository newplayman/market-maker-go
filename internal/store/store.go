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
type EventSink func(string, map[string]interface{})

type Store struct {
	Symbol string

	mu               sync.RWMutex
	pendingOrders    map[int64]orderEntry
	pendingBuy       float64
	pendingSell      float64
	pendingBuyCount  int
	pendingSellCount int

	position float64

	mid     float64
	bestBid float64
	bestAsk float64

	// 30分钟价格序列，用于计算标准差
	prices []pricePoint

	// 资金费率预测（EMA）
	alpha                float64
	predictedFundingRate float64
	fundingPnlAcc        float64

	// P1修复：标准差缓存
	cachedStdDev     float64
	cachedStdDevTime time.Time
	stdDevCacheTTL   time.Duration

	sink EventSink
}

type orderEntry struct {
	side       string
	openQty    float64
	updateTime int64
}

type pricePoint struct {
	p  float64
	ts time.Time
}

func New(symbol string, predictAlpha float64, sink EventSink) *Store {
	return &Store{
		Symbol:         symbol,
		pendingOrders:  make(map[int64]orderEntry),
		prices:         make([]pricePoint, 0, 900),
		alpha:          predictAlpha,
		stdDevCacheTTL: 10 * time.Second, // P1修复：10秒缓存TTL
		sink:           sink,
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

// BestBidPrice 当前最优买价
func (s *Store) BestBidPrice() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bestBid
}

// BestAskPrice 当前最优卖价
func (s *Store) BestAskPrice() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bestAsk
}

// Position 当前净仓位
func (s *Store) Position() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.position
}

// PriceStdDev30m 最近30分钟价格标准差（P1修复：带10秒缓存）
func (s *Store) PriceStdDev30m() float64 {
	s.mu.RLock()
	// 检查缓存是否有效
	now := time.Now()
	if now.Sub(s.cachedStdDevTime) < s.stdDevCacheTTL && s.cachedStdDev > 0 {
		cached := s.cachedStdDev
		s.mu.RUnlock()
		return cached
	}
	s.mu.RUnlock()

	// 缓存失效，重新计算
	s.mu.Lock()
	defer s.mu.Unlock()

	// 双重检查（避免并发重复计算）
	if now.Sub(s.cachedStdDevTime) < s.stdDevCacheTTL && s.cachedStdDev > 0 {
		return s.cachedStdDev
	}

	cut := now.Add(-30 * time.Minute)
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

	// 更新缓存
	s.cachedStdDev = sd
	s.cachedStdDevTime = now
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
	if o.Symbol != s.Symbol || o.OrderID == 0 {
		return
	}
	s.mu.Lock()
	changed := s.applyOrderUpdateLocked(o)
	if changed {
		s.recomputePendingCountsLocked()
	}
	pendingBuy := s.pendingBuy
	pendingSell := s.pendingSell
	buyCount := s.pendingBuyCount
	sellCount := s.pendingSellCount
	s.mu.Unlock()
	if changed {
		metrics.UpdateOrderMetrics(s.Symbol, buyCount, sellCount)
		metrics.UpdatePendingExposure(s.Symbol, pendingBuy, pendingSell)
		s.logEvent("order_update", map[string]interface{}{
			"symbol":       o.Symbol,
			"order_id":     o.OrderID,
			"client_id":    o.ClientOrderID,
			"status":       o.Status,
			"execution":    o.ExecutionType,
			"side":         o.Side,
			"type":         o.OrderType,
			"price":        o.Price,
			"orig_qty":     o.OrigQty,
			"last_qty":     o.LastFilledQty,
			"accum_qty":    o.AccumulatedQty,
			"realized_pnl": o.RealizedPnL,
			"event_time":   o.EventTime,
			"update_time":  o.UpdateTime,
			"pending_buy":  pendingBuy,
			"pending_sell": pendingSell,
			"pending_bids": buyCount,
			"pending_asks": sellCount,
		})
	}
}

// ReplacePendingOrders 用REST返回的活跃订单覆盖本地快照（断线重连场景）
func (s *Store) ReplacePendingOrders(orders []gateway.OrderUpdate) {
	s.mu.Lock()
	symbol := s.Symbol
	s.pendingOrders = make(map[int64]orderEntry, len(orders))
	s.pendingBuy = 0
	s.pendingSell = 0
	for _, o := range orders {
		if o.Symbol != symbol || o.OrderID == 0 {
			continue
		}
		openQty := o.OrigQty - o.AccumulatedQty
		if openQty <= 0 {
			continue
		}
		side := strings.ToUpper(o.Side)
		entry := orderEntry{
			side:       side,
			openQty:    openQty,
			updateTime: o.UpdateTime,
		}
		s.pendingOrders[o.OrderID] = entry
		if side == "BUY" {
			s.pendingBuy += openQty
		} else if side == "SELL" {
			s.pendingSell += openQty
		}
	}
	s.recomputePendingCountsLocked()
	pendingBuy := s.pendingBuy
	pendingSell := s.pendingSell
	buyCount := s.pendingBuyCount
	sellCount := s.pendingSellCount
	orderCount := len(s.pendingOrders)
	s.mu.Unlock()
	metrics.UpdateOrderMetrics(symbol, buyCount, sellCount)
	metrics.UpdatePendingExposure(symbol, pendingBuy, pendingSell)
	s.logEvent("order_snapshot", map[string]interface{}{
		"symbol":       symbol,
		"pending_buy":  pendingBuy,
		"pending_sell": pendingSell,
		"pending_bids": buyCount,
		"pending_asks": sellCount,
		"order_count":  orderCount,
	})
}

func (s *Store) applyOrderUpdateLocked(o gateway.OrderUpdate) bool {
	entry, ok := s.pendingOrders[o.OrderID]
	if ok && o.UpdateTime > 0 && entry.updateTime >= o.UpdateTime {
		return false
	}
	prevSide := entry.side
	prevQty := entry.openQty

	side := strings.ToUpper(o.Side)
	openQty := o.OrigQty - o.AccumulatedQty
	if openQty < 0 {
		openQty = 0
	}

	if openQty == 0 {
		delete(s.pendingOrders, o.OrderID)
	} else {
		s.pendingOrders[o.OrderID] = orderEntry{
			side:       side,
			openQty:    openQty,
			updateTime: o.UpdateTime,
		}
	}

	switch prevSide {
	case "BUY":
		s.pendingBuy -= prevQty
	case "SELL":
		s.pendingSell -= prevQty
	}
	if openQty > 0 {
		switch side {
		case "BUY":
			s.pendingBuy += openQty
		case "SELL":
			s.pendingSell += openQty
		}
	}

	if s.pendingBuy < 0 {
		s.pendingBuy = 0
	}
	if s.pendingSell < 0 {
		s.pendingSell = 0
	}
	return prevQty != openQty || prevSide != side
}

func (s *Store) recomputePendingCountsLocked() {
	buyCount := 0
	sellCount := 0
	for _, entry := range s.pendingOrders {
		if entry.openQty <= 0 {
			continue
		}
		switch entry.side {
		case "BUY":
			buyCount++
		case "SELL":
			sellCount++
		}
	}
	s.pendingBuyCount = buyCount
	s.pendingSellCount = sellCount
}

// HandlePositionUpdate 仓位事件处理。
func (s *Store) HandlePositionUpdate(a gateway.AccountUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range a.Positions {
		if p.Symbol == s.Symbol {
			s.position = p.PositionAmt
			metrics.UpdatePositionMetrics(s.Symbol, s.position, p.EntryPrice, p.PnL, 0)
			s.logEvent("position_update", map[string]interface{}{
				"symbol":      p.Symbol,
				"position":    p.PositionAmt,
				"entry_price": p.EntryPrice,
				"pnl":         p.PnL,
				"reason":      strings.ToUpper(a.Reason),
			})
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
	s.bestBid = bid
	s.bestAsk = ask
	s.prices = append(s.prices, pricePoint{p: mid, ts: ts})
	// 修剪过长序列（保留最近 ~60 分钟的数据）
	if len(s.prices) > 3600 {
		s.prices = s.prices[len(s.prices)-3600:]
	}
	s.mu.Unlock()
	metrics.UpdateMarketData(s.Symbol, mid, bid, ask)
}

// HandleFundingRate 更新资金费率预测与累计盈亏
func (s *Store) HandleFundingRate(rate float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// EMA 更新预测费率
	if s.predictedFundingRate == 0 {
		s.predictedFundingRate = rate
	} else {
		s.predictedFundingRate = s.predictedFundingRate*(1-s.alpha) + rate*s.alpha
	}

	// 计算本次资金费盈亏 (负费率 = 多头赚钱)
	// PnL = -Position * Rate * Price
	// 注意：这里简化计算，直接用当前仓位和mid估算
	pnl := -s.position * rate * s.mid
	s.fundingPnlAcc += pnl

	metrics.FundingRate.Set(rate)
	metrics.PredictedFundingRate.Set(s.predictedFundingRate)
	metrics.FundingPnlAccum.Set(s.fundingPnlAcc)

	s.logEvent("funding_update", map[string]interface{}{
		"symbol":         s.Symbol,
		"rate":           rate,
		"predicted_rate": s.predictedFundingRate,
		"accum_pnl":      s.fundingPnlAcc,
	})
}

func (s *Store) logEvent(event string, fields map[string]interface{}) {
	if s == nil || s.sink == nil {
		return
	}
	s.sink(event, fields)
}
