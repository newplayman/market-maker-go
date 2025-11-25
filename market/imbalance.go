package market

// CalculateImbalance calculates the imbalance between bid and ask volumes
// Imbalance = (BidVol - AskVol) / (BidVol + AskVol)
func CalculateImbalance(bidVolumeTop float64, askVolumeTop float64) float64 {
	totalVolume := bidVolumeTop + askVolumeTop
	if totalVolume == 0 {
		return 0
	}
	return (bidVolumeTop - askVolumeTop) / totalVolume
}

// CalculateImbalanceFromOrderBook calculates imbalance using full order book data
// levels specifies how many levels to consider from the top
func CalculateImbalanceFromOrderBook(book *OrderBook, levels int) float64 {
	if book == nil || levels <= 0 {
		return 0
	}
	
	bidVolume := 0.0
	askVolume := 0.0
	
	// Get top N bid levels
	bidPrices := book.BidPrices()
	for i, price := range bidPrices {
		if i >= levels {
			break
		}
		bidVolume += book.BidVolume(price)
	}
	
	// Get top N ask levels
	askPrices := book.AskPrices()
	for i, price := range askPrices {
		if i >= levels {
			break
		}
		askVolume += book.AskVolume(price)
	}
	
	return CalculateImbalance(bidVolume, askVolume)
}