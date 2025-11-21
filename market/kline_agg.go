package market

import (
	"sync"
	"time"
)

// KlineAggregator 从成交流生成固定周期的 Kline。
type KlineAggregator struct {
	Interval time.Duration
	mu       sync.Mutex
	current  *Kline
}

func NewKlineAggregator(interval time.Duration) *KlineAggregator {
	return &KlineAggregator{Interval: interval}
}

// OnTrade 更新当前 Kline；返回新生成的 Kline（闭合的）或 nil。
func (a *KlineAggregator) OnTrade(price, qty float64, ts time.Time) *Kline {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.current == nil || ts.Sub(a.current.Ts) >= a.Interval {
		// 关闭旧 kline
		var closed *Kline
		if a.current != nil {
			closed = a.current
			// 将当前成交作为上一根的收盘，便于边界测试/简化处理
			closed.Close = price
			if price > closed.High {
				closed.High = price
			}
			if price < closed.Low {
				closed.Low = price
			}
		}
		// 开启新 kline
		a.current = &Kline{
			Open:  price,
			High:  price,
			Low:   price,
			Close: price,
			Ts:    ts,
		}
		return closed
	}

	if price > a.current.High {
		a.current.High = price
	}
	if price < a.current.Low {
		a.current.Low = price
	}
	a.current.Close = price
	return nil
}
