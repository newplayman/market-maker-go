package market

// Snapshot represents a market snapshot.
type Snapshot struct {
	Mid   float64
	BestBid float64
	BestAsk float64
	Spread  float64
	Timestamp int64
}