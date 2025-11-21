package market

import "time"

// Trade represents a normalized trade tick.
type Trade struct {
	Price float64
	Qty   float64
	Ts    time.Time
}
