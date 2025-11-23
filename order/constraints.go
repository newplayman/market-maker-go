package order

import (
	"fmt"
	"math"
)

// SymbolConstraints 描述交易对的步长与名义限制。
type SymbolConstraints struct {
	TickSize    float64
	StepSize    float64
	MinQty      float64
	MaxQty      float64
	MinNotional float64
}

// Validate 检查订单价格/数量是否符合精度与最小名义。
func (c SymbolConstraints) Validate(price, qty float64) error {
	if c.TickSize > 0 && !isMultiple(price, c.TickSize) {
		return fmt.Errorf("price %.8f not aligned to tickSize %.8f", price, c.TickSize)
	}
	if c.StepSize > 0 && !isMultiple(qty, c.StepSize) {
		return fmt.Errorf("qty %.8f not aligned to stepSize %.8f", qty, c.StepSize)
	}
	if c.MinQty > 0 && qty < c.MinQty {
		return fmt.Errorf("qty %.8f < minQty %.8f", qty, c.MinQty)
	}
	if c.MaxQty > 0 && qty > c.MaxQty {
		return fmt.Errorf("qty %.8f > maxQty %.8f", qty, c.MaxQty)
	}
	if c.MinNotional > 0 && price*qty < c.MinNotional {
		return fmt.Errorf("notional %.8f < minNotional %.8f", price*qty, c.MinNotional)
	}
	return nil
}

func isMultiple(value, step float64) bool {
	if step <= 0 {
		return true
	}
	ratio := value / step
	return math.Abs(ratio-math.Round(ratio)) <= 1e-8
}
