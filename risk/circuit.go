package risk

import "time"

// Tick 依赖 minimal 行情信息。
type Tick struct {
	Price float64
	Ts    time.Time
}

// CircuitBreaker 基于近期波动率触发熔断。
type CircuitBreaker struct {
	// 阈值：1m、5m 相对涨跌幅
	OneMinuteThresh  float64
	FiveMinuteThresh float64
	window1m         []Tick
	window5m         []Tick
}

func NewCircuitBreaker(one, five float64) *CircuitBreaker {
	return &CircuitBreaker{
		OneMinuteThresh:  one,
		FiveMinuteThresh: five,
		window1m:         make([]Tick, 0, 128),
		window5m:         make([]Tick, 0, 512),
	}
}

// OnTick 返回 (是否触发, 触发窗口 "1m"/"5m"/"")
func (c *CircuitBreaker) OnTick(t Tick) (bool, string) {
	c.window1m = append(c.window1m, t)
	c.window5m = append(c.window5m, t)
	c.trim(&c.window1m, t.Ts.Add(-1*time.Minute))
	c.trim(&c.window5m, t.Ts.Add(-5*time.Minute))

	if trip := c.check(c.window1m, c.OneMinuteThresh); trip {
		return true, "1m"
	}
	if trip := c.check(c.window5m, c.FiveMinuteThresh); trip {
		return true, "5m"
	}
	return false, ""
}

func (c *CircuitBreaker) trim(buf *[]Tick, cutoff time.Time) {
	i := 0
	for ; i < len(*buf); i++ {
		if (*buf)[i].Ts.After(cutoff) {
			break
		}
	}
	if i > 0 {
		*buf = (*buf)[i:]
	}
}

func (c *CircuitBreaker) check(buf []Tick, thresh float64) bool {
	if thresh <= 0 || len(buf) == 0 {
		return false
	}
	first := buf[0].Price
	last := buf[len(buf)-1].Price
	if first == 0 {
		return false
	}
	change := (last - first) / first
	if change > thresh || change < -thresh {
		return true
	}
	return false
}
