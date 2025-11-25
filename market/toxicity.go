package market

import (
	"math"
	"sync"
)

// VPINCalculator calculates Volume-Synchronized Probability of Informed Trading
type VPINCalculator struct {
	mu             sync.RWMutex
	buckets        []VolumeBucket
	bucketSize     float64 // target volume per bucket
	currentBucket  VolumeBucket
	maxBuckets     int
	vpin           float64
	toxicThreshold float64
}

// VolumeBucket represents a volume bucket for VPIN calculation
type VolumeBucket struct {
	BuyVolume   float64
	SellVolume  float64
	TotalVolume float64
}

// NewVPINCalculator creates a new VPIN calculator
func NewVPINCalculator(bucketSize float64, maxBuckets int, toxicThreshold float64) *VPINCalculator {
	return &VPINCalculator{
		buckets:        make([]VolumeBucket, 0, maxBuckets),
		bucketSize:     bucketSize,
		maxBuckets:     maxBuckets,
		toxicThreshold: toxicThreshold,
	}
}

// AddTrade adds a trade to the VPIN calculation
func (v *VPINCalculator) AddTrade(price, volume float64, isBuy bool) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Add to current bucket
	if isBuy {
		v.currentBucket.BuyVolume += volume
	} else {
		v.currentBucket.SellVolume += volume
	}
	v.currentBucket.TotalVolume += volume

	// Check if bucket is full
	if v.currentBucket.TotalVolume >= v.bucketSize {
		// Save current bucket
		v.buckets = append(v.buckets, v.currentBucket)
		if len(v.buckets) > v.maxBuckets {
			v.buckets = v.buckets[1:]
		}

		// Reset current bucket
		v.currentBucket = VolumeBucket{}

		// Recalculate VPIN
		v.calculateVPIN()
	}
}

// calculateVPIN calculates VPIN from buckets
func (v *VPINCalculator) calculateVPIN() {
	if len(v.buckets) == 0 {
		v.vpin = 0
		return
	}

	totalImbalance := 0.0
	totalVolume := 0.0

	for _, bucket := range v.buckets {
		imbalance := math.Abs(bucket.BuyVolume - bucket.SellVolume)
		totalImbalance += imbalance
		totalVolume += bucket.TotalVolume
	}

	if totalVolume > 0 {
		v.vpin = totalImbalance / totalVolume
	} else {
		v.vpin = 0
	}
}

// GetVPIN returns the current VPIN value
func (v *VPINCalculator) GetVPIN() float64 {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.vpin
}

// IsToxic checks if the current VPIN indicates toxic flow
func (v *VPINCalculator) IsToxic() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.vpin > v.toxicThreshold
}

// IsReady checks if enough buckets have been collected
func (v *VPINCalculator) IsReady() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.buckets) >= v.maxBuckets/2
}

// Reset clears all buckets and resets VPIN
func (v *VPINCalculator) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.buckets = make([]VolumeBucket, 0, v.maxBuckets)
	v.currentBucket = VolumeBucket{}
	v.vpin = 0
}
