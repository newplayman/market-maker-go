package strategy

import "math"

// GridLevel 定义单个网格档位。
type GridLevel struct {
	Price float64
	Size  float64
}

// BuildDynamicGrid 根据 mid 价与波动率动态调整网格密度。
// vol 表示近似年化波动率（0.0~1.0），levelCount 为网格层数（双向各一半）。
func BuildDynamicGrid(mid float64, vol float64, levelCount int, baseSize float64) []GridLevel {
	if levelCount < 2 {
		levelCount = 2
	}
	if baseSize <= 0 {
		baseSize = 1
	}
	// 简化：波动率越大，网格步长越大；最小步长 0.0005*mid。
	step := mid * (0.0005 + 0.005*math.Min(vol, 1))
	levels := make([]GridLevel, 0, levelCount*2)
	for i := 1; i <= levelCount; i++ {
		levels = append(levels, GridLevel{
			Price: mid - float64(i)*step,
			Size:  baseSize,
		})
		levels = append(levels, GridLevel{
			Price: mid + float64(i)*step,
			Size:  baseSize,
		})
	}
	return levels
}
