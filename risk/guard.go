package risk

// Guard 是通用接口，限价、VWAP 等都可实现。
type Guard interface {
	PreOrder(symbol string, deltaQty float64) error
}

// MultiGuard 顺序执行多个 Guard，只要有一个返回错误则中止。
type MultiGuard struct {
	Guards []Guard
}

func (m MultiGuard) PreOrder(symbol string, deltaQty float64) error {
	for _, g := range m.Guards {
		if g == nil {
			continue
		}
		if err := g.PreOrder(symbol, deltaQty); err != nil {
			return err
		}
	}
	return nil
}
