package market

import "time"

// Kline represents OHLC data.
type Kline struct {
	Open  float64
	High  float64
	Low   float64
	Close float64
	Ts    time.Time
}
