package posttrade

import (
	"sync"
	"time"
)

// FillRecord represents a record of a filled order
type FillRecord struct {
	FillPrice      float64
	FillTime       time.Time
	Side           string
	PriceAfter1s   float64
	PriceAfter5s   float64
	PriceAfter1sTs time.Time
	PriceAfter5sTs time.Time
}

// Stats contains statistics computed by the analyzer
type Stats struct {
	AdverseSelectionRate float64
	AvgPnL1s            float64
	AvgPnL5s            float64
	TotalFills          int
	AnalyzedFills       int
}

// Analyzer analyzes post-trade performance and adverse selection
type Analyzer struct {
	fills        map[string]*FillRecord
	mu           sync.RWMutex
	marketSource MarketSource
}

// MarketSource is an interface for getting market data
type MarketSource interface {
	GetCurrentMid() float64
}

// NewAnalyzer creates a new post-trade analyzer
func NewAnalyzer(marketSource MarketSource) *Analyzer {
	return &Analyzer{
		fills:        make(map[string]*FillRecord),
		marketSource: marketSource,
	}
}

// OnFill records a filled order
func (a *Analyzer) OnFill(orderID string, price float64, side string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	record := &FillRecord{
		FillPrice: price,
		FillTime:  time.Now(),
		Side:      side,
	}
	a.fills[orderID] = record

	// Start tracking price movement after 1s and 5s
	go a.trackPriceMovement(orderID)
}

// trackPriceMovement tracks price movement after order fill
func (a *Analyzer) trackPriceMovement(orderID string) {
	// Wait 1 second
	time.Sleep(1 * time.Second)
	
	a.mu.Lock()
	record, exists := a.fills[orderID]
	a.mu.Unlock()
	
	if exists {
		a.mu.Lock()
		record.PriceAfter1s = a.marketSource.GetCurrentMid()
		record.PriceAfter1sTs = time.Now()
		a.mu.Unlock()
	}

	// Wait additional 4 seconds (5 seconds total)
	time.Sleep(4 * time.Second)
	
	a.mu.Lock()
	record, exists = a.fills[orderID]
	a.mu.Unlock()
	
	if exists {
		a.mu.Lock()
		record.PriceAfter5s = a.marketSource.GetCurrentMid()
		record.PriceAfter5sTs = time.Now()
		a.mu.Unlock()
	}
}

// Stats computes and returns statistics
func (a *Analyzer) Stats() Stats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	stats := Stats{
		TotalFills: len(a.fills),
	}

	if len(a.fills) == 0 {
		return stats
	}

	var adverseCount, analyzedCount int
	var totalPnL1s, totalPnL5s float64

	for _, record := range a.fills {
		// Check if we have price data after 1s and 5s
		if record.PriceAfter1s == 0 || record.PriceAfter5s == 0 {
			continue
		}

		analyzedCount++

		// Calculate PnL based on order side
		var pnl1s, pnl5s float64
		if record.Side == "BUY" {
			// For buy orders, positive PnL means price went up (adverse selection)
			pnl1s = (record.PriceAfter1s - record.FillPrice) / record.FillPrice
			pnl5s = (record.PriceAfter5s - record.FillPrice) / record.FillPrice
		} else {
			// For sell orders, positive PnL means price went down (adverse selection)
			pnl1s = (record.FillPrice - record.PriceAfter1s) / record.FillPrice
			pnl5s = (record.FillPrice - record.PriceAfter5s) / record.FillPrice
		}

		totalPnL1s += pnl1s
		totalPnL5s += pnl5s

		// Count adverse selection (positive PnL indicates adverse selection)
		if pnl1s > 0 {
			adverseCount++
		}
	}

	stats.AnalyzedFills = analyzedCount
	if analyzedCount > 0 {
		stats.AdverseSelectionRate = float64(adverseCount) / float64(analyzedCount)
		stats.AvgPnL1s = totalPnL1s / float64(analyzedCount)
		stats.AvgPnL5s = totalPnL5s / float64(analyzedCount)
	}

	return stats
}

// CleanOldRecords removes old records to prevent memory leaks
func (a *Analyzer) CleanOldRecords(maxAge time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for id, record := range a.fills {
		if now.Sub(record.FillTime) > maxAge {
			delete(a.fills, id)
		}
	}
}