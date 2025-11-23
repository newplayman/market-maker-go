package metrics

// Counter 模拟 Prometheus Counter 接口。
type Counter interface {
	Inc()
	Add(v float64)
}

// Gauge 模拟 Prometheus Gauge。
type Gauge interface {
	Set(v float64)
}

// Histogram 模拟 Prometheus Histogram。
type Histogram interface {
	Observe(v float64)
}

// MockCounter 是一个线程不安全但简单的计数器，适合单测。
type MockCounter struct {
	Value float64
}

func (c *MockCounter) Inc() {
	c.Value++
}

func (c *MockCounter) Add(v float64) {
	c.Value += v
}

// MockGauge 记录最后一次 Set 的值。
type MockGauge struct {
	Value float64
}

func (g *MockGauge) Set(v float64) {
	g.Value = v
}

// MockHistogram 记录全部 observe 值，便于断言。
type MockHistogram struct {
	Values []float64
}

func (h *MockHistogram) Observe(v float64) {
	h.Values = append(h.Values, v)
}
