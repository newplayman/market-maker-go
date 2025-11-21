package market

import "sync"

// OrderBook 维护简单的价格->数量映射。
type OrderBook struct {
	mu   sync.RWMutex
	bids map[float64]float64 // price -> qty
	asks map[float64]float64
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
