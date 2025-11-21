package inventory

import "sync"

// Tracker 维护净仓位。
type Tracker struct {
	mu   sync.RWMutex
	net  float64
	cost float64
}

// Update 根据成交数量调整仓位。
func (t *Tracker) Update(deltaQty float64, price float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// 简化：加权平均成本
	totalValue := t.cost*t.net + price*deltaQty
	t.net += deltaQty
	if t.net != 0 {
		t.cost = totalValue / t.net
	} else {
		t.cost = 0
	}
}

func (t *Tracker) NetExposure() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.net
}

func (t *Tracker) AvgCost() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.cost
}
