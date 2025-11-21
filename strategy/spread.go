package strategy

import "math"

// CalcDynamicSpread 基于盘口宽度与波动率计算目标 spread（绝对价格）。
// width：盘口宽度，如 bestAsk-bestBid，vol：波动率估计。
func CalcDynamicSpread(mid, width, vol float64, minBps float64) float64 {
	// 基础：最小价差
	spread := mid * (minBps / 10000.0)
	// 盘口宽度占 50%
	spread += width * 0.5
	// 波动率项占 50%，上限 1% mid
	spread += mid * 0.005 * math.Min(vol, 1)
	// 防止为 0
	if spread <= 0 {
		return mid * 0.0005
	}
	return spread
}
