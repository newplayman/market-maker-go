package risk

import "errors"

var (
	ErrTooFrequent  = errors.New("order too frequent")
	ErrPnLTooLow    = errors.New("pnl too low")
	ErrPnLTooHigh   = errors.New("pnl too high")
	ErrSpreadTooWide = errors.New("spread too wide")
)