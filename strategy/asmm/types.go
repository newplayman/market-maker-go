package asmm

import (
	"market-maker-go/market"
)

// Side represents order side.
type Side string

const (
	Bid Side = "bid"
	Ask Side = "ask"
)

// Quote represents a single quote.
type Quote struct {
	Price      float64
	Size       float64
	Side       Side
	ReduceOnly bool
}

// Strategy is the ASMM strategy interface.
type Strategy interface {
	GenerateQuotes(snap market.Snapshot, inventory float64) []Quote
}