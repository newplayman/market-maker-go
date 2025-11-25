package market

import (
	"sort"
	"sync"
	"time"
)

// DepthSide 指定估算深度时的方向。
type DepthSide int

const (
	DepthSideBid DepthSide = iota
	DepthSideAsk
)

// OrderBook 维护简单的价格->数量映射。
type OrderBook struct {
	mu         sync.RWMutex
	bids       map[float64]float64 // price -> qty
	asks       map[float64]float64
	lastUpdate time.Time
}

func NewOrderBook() *OrderBook {
	return &OrderBook{
		bids: make(map[float64]float64),
		asks: make(map[float64]float64),
	}
}

// ApplyDelta 应用增量更新，qty 为 0 表示删除该档。
func (ob *OrderBook) ApplyDelta(bidDelta map[float64]float64, askDelta map[float64]float64) {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	for p, q := range bidDelta {
		if q == 0 {
			delete(ob.bids, p)
		} else {
			ob.bids[p] = q
		}
	}
	for p, q := range askDelta {
		if q == 0 {
			delete(ob.asks, p)
		} else {
			ob.asks[p] = q
		}
	}
	ob.lastUpdate = time.Now()
}

// SetBest 重置 orderbook，仅保留当前最好 bid/ask。
func (ob *OrderBook) SetBest(bid, ask float64) {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	ob.bids = make(map[float64]float64)
	ob.asks = make(map[float64]float64)
	if bid > 0 {
		ob.bids[bid] = 1
	}
	if ask > 0 {
		ob.asks[ask] = 1
	}
	ob.lastUpdate = time.Now()
}

// Best 返回最好买/卖价；若不存在则为 0。
func (ob *OrderBook) Best() (bestBid float64, bestAsk float64) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	for p := range ob.bids {
		if p > bestBid {
			bestBid = p
		}
	}
	bestAsk = 0
	for p := range ob.asks {
		if bestAsk == 0 || p < bestAsk {
			bestAsk = p
		}
	}
	return bestBid, bestAsk
}

// Mid 返回中间价；若缺失任一侧返回 0。
func (ob *OrderBook) Mid() float64 {
	bid, ask := ob.Best()
	if bid == 0 || ask == 0 {
		return 0
	}
	return (bid + ask) / 2
}

// LastUpdate 返回最近一次 ApplyDelta 的时间。
func (ob *OrderBook) LastUpdate() time.Time {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.lastUpdate
}

// BidPrices returns all bid prices sorted in descending order
func (ob *OrderBook) BidPrices() []float64 {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	
	prices := make([]float64, 0, len(ob.bids))
	for price := range ob.bids {
		prices = append(prices, price)
	}
	
	// Sort in descending order (highest bid first)
	sort.Sort(sort.Reverse(sort.Float64Slice(prices)))
	return prices
}

// AskPrices returns all ask prices sorted in ascending order
func (ob *OrderBook) AskPrices() []float64 {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	
	prices := make([]float64, 0, len(ob.asks))
	for price := range ob.asks {
		prices = append(prices, price)
	}
	
	// Sort in ascending order (lowest ask first)
	sort.Float64s(prices)
	return prices
}

// BidVolume returns the volume at a specific bid price
func (ob *OrderBook) BidVolume(price float64) float64 {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.bids[price]
}

// AskVolume returns the volume at a specific ask price
func (ob *OrderBook) AskVolume(price float64) float64 {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.asks[price]
}

// EstimateFillPrice 根据订单簿估算在指定方向成交 qty 所需触及的最差价位。
// 若 depth 不足以完全成交，则返回能提供的最大数量及对应价位。
func (ob *OrderBook) EstimateFillPrice(side DepthSide, qty float64) (price float64, cumulative float64) {
	if qty <= 0 {
		return 0, 0
	}
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	var (
		levels map[float64]float64
		prices []float64
	)
	if side == DepthSideAsk {
		levels = ob.asks
	} else {
		levels = ob.bids
	}
	if len(levels) == 0 {
		return 0, 0
	}
	prices = make([]float64, 0, len(levels))
	for p := range levels {
		prices = append(prices, p)
	}
	if side == DepthSideAsk {
		sort.Float64s(prices)
	} else {
		sort.Sort(sort.Reverse(sort.Float64Slice(prices)))
	}
	cumulative = 0
	for _, p := range prices {
		qtyAt := levels[p]
		if qtyAt <= 0 {
			continue
		}
		cumulative += qtyAt
		price = p
		if cumulative >= qty {
			break
		}
	}
	return price, cumulative
}
