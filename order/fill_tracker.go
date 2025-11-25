package order

import (
	"sync"
	"time"
)

// FillEvent 成交事件
type FillEvent struct {
	OrderID   string
	Side      string
	Price     float64
	Quantity  float64
	Timestamp time.Time
}

// FillTracker 跟踪成交历史，用于高频撤单抑制
type FillTracker struct {
	mu sync.RWMutex

	// 近期成交记录（滑动窗口）
	recentFills []FillEvent
	maxHistory  int           // 最大历史记录数
	windowSize  time.Duration // 时间窗口

	// 统计信息
	totalFills     int
	recentFillRate float64 // 每分钟成交次数
}

// NewFillTracker 创建成交跟踪器
func NewFillTracker(maxHistory int, windowSize time.Duration) *FillTracker {
	if maxHistory <= 0 {
		maxHistory = 100
	}
	if windowSize <= 0 {
		windowSize = 5 * time.Minute
	}

	return &FillTracker{
		recentFills: make([]FillEvent, 0, maxHistory),
		maxHistory:  maxHistory,
		windowSize:  windowSize,
	}
}

// RecordFill 记录成交
func (f *FillTracker) RecordFill(orderID, side string, price, quantity float64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	event := FillEvent{
		OrderID:   orderID,
		Side:      side,
		Price:     price,
		Quantity:  quantity,
		Timestamp: time.Now(),
	}

	f.recentFills = append(f.recentFills, event)
	f.totalFills++

	// 清理过期记录
	f.cleanOldFillsUnsafe()

	// 更新成交率
	f.updateFillRateUnsafe()
}

// cleanOldFillsUnsafe 清理超出窗口的成交记录（非线程安全）
func (f *FillTracker) cleanOldFillsUnsafe() {
	now := time.Now()
	cutoff := now.Add(-f.windowSize)

	// 找到第一个在窗口内的记录
	validStart := 0
	for i, fill := range f.recentFills {
		if fill.Timestamp.After(cutoff) {
			validStart = i
			break
		}
	}

	// 移除过期记录
	if validStart > 0 {
		f.recentFills = f.recentFills[validStart:]
	}

	// 限制最大历史数
	if len(f.recentFills) > f.maxHistory {
		f.recentFills = f.recentFills[len(f.recentFills)-f.maxHistory:]
	}
}

// updateFillRateUnsafe 更新成交率（非线程安全）
func (f *FillTracker) updateFillRateUnsafe() {
	if len(f.recentFills) == 0 {
		f.recentFillRate = 0
		return
	}

	now := time.Now()
	windowStart := now.Add(-f.windowSize)

	// 计算窗口内的成交次数
	count := 0
	for _, fill := range f.recentFills {
		if fill.Timestamp.After(windowStart) {
			count++
		}
	}

	// 计算每分钟成交率
	windowMinutes := f.windowSize.Minutes()
	if windowMinutes > 0 {
		f.recentFillRate = float64(count) / windowMinutes
	}
}

// GetRecentFillRate 获取近期成交率（每分钟成交次数）
func (f *FillTracker) GetRecentFillRate() float64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.recentFillRate
}

// GetRecentFills 获取近期成交记录（只读副本）
func (f *FillTracker) GetRecentFills(duration time.Duration) []FillEvent {
	f.mu.RLock()
	defer f.mu.RUnlock()

	cutoff := time.Now().Add(-duration)
	var result []FillEvent

	for _, fill := range f.recentFills {
		if fill.Timestamp.After(cutoff) {
			result = append(result, fill)
		}
	}

	return result
}

// GetTotalFills 获取总成交次数
func (f *FillTracker) GetTotalFills() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.totalFills
}

// ShouldSuppressCancel 判断是否应抑制撤单（高频成交时避免频繁撤单）
func (f *FillTracker) ShouldSuppressCancel(fillRateThreshold float64, recentFillsThreshold int, checkDuration time.Duration) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// 检查成交率是否超过阈值
	if f.recentFillRate > fillRateThreshold {
		return true
	}

	// 检查近期成交次数
	if checkDuration <= 0 {
		checkDuration = 1 * time.Minute
	}

	cutoff := time.Now().Add(-checkDuration)
	recentCount := 0
	for _, fill := range f.recentFills {
		if fill.Timestamp.After(cutoff) {
			recentCount++
		}
	}

	return recentCount >= recentFillsThreshold
}

// Reset 重置跟踪器
func (f *FillTracker) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.recentFills = make([]FillEvent, 0, f.maxHistory)
	f.totalFills = 0
	f.recentFillRate = 0
}

// GetStats 获取统计信息
func (f *FillTracker) GetStats() FillTrackerStats {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return FillTrackerStats{
		TotalFills:     f.totalFills,
		RecentFills:    len(f.recentFills),
		RecentFillRate: f.recentFillRate,
	}
}

// FillTrackerStats 成交跟踪器统计
type FillTrackerStats struct {
	TotalFills     int
	RecentFills    int
	RecentFillRate float64
}
