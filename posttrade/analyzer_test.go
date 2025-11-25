package posttrade

import (
	"testing"
	"time"
)

// mockMarketSource implements MarketSource for testing
type mockMarketSource struct {
	currentMid float64
}

func (m *mockMarketSource) GetCurrentMid() float64 {
	return m.currentMid
}

func TestAnalyzer_OnFill(t *testing.T) {
	marketSource := &mockMarketSource{currentMid: 100.0}
	analyzer := NewAnalyzer(marketSource)

	// Record a buy fill
	analyzer.OnFill("order1", 99.5, "BUY")

	// Check that the fill was recorded
	stats := analyzer.Stats()
	if stats.TotalFills != 1 {
		t.Errorf("Expected 1 total fill, got %d", stats.TotalFills)
	}
}

func TestAnalyzer_Stats(t *testing.T) {
	marketSource := &mockMarketSource{currentMid: 100.0}
	analyzer := NewAnalyzer(marketSource)

	// Add some fills with price movements
	analyzer.OnFill("buyOrder", 99.0, "BUY")
	analyzer.OnFill("sellOrder", 101.0, "SELL")

	// Manually update the fill records to simulate price movements
	time.Sleep(10 * time.Millisecond) // Small delay to ensure goroutines start

	analyzer.mu.Lock()
	// Simulate price going up after buy (adverse selection)
	if record, exists := analyzer.fills["buyOrder"]; exists {
		record.PriceAfter1s = 101.0
		record.PriceAfter5s = 102.0
	}

	// Simulate price going down after sell (adverse selection)
	if record, exists := analyzer.fills["sellOrder"]; exists {
		record.PriceAfter1s = 99.0
		record.PriceAfter5s = 98.0
	}
	analyzer.mu.Unlock()

	stats := analyzer.Stats()
	
	// Both orders should show adverse selection
	if stats.AnalyzedFills != 2 {
		t.Errorf("Expected 2 analyzed fills, got %d", stats.AnalyzedFills)
	}
}

func TestAnalyzer_CleanOldRecords(t *testing.T) {
	marketSource := &mockMarketSource{currentMid: 100.0}
	analyzer := NewAnalyzer(marketSource)

	// Add an old record
	analyzer.mu.Lock()
	analyzer.fills["oldOrder"] = &FillRecord{
		FillPrice: 99.0,
		FillTime:  time.Now().Add(-2 * time.Hour), // 2 hours old
		Side:      "BUY",
	}
	analyzer.mu.Unlock()

	stats := analyzer.Stats()
	if stats.TotalFills != 1 {
		t.Errorf("Expected 1 total fill before cleanup, got %d", stats.TotalFills)
	}

	// Clean records older than 1 hour
	analyzer.CleanOldRecords(1 * time.Hour)

	stats = analyzer.Stats()
	if stats.TotalFills != 0 {
		t.Errorf("Expected 0 total fills after cleanup, got %d", stats.TotalFills)
	}
}