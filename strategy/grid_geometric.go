package strategy

// BuildGeometricGrid 生成几何加宽间距的网格，并支持远端下单量衰减。
// mid: 当前中间价
// levelCount: 每侧层数（总层数约为 2*levelCount）
// baseSize: 基础下单量（近端）
// spacingRatio: 间距几何比例（例如 1.20 表示后续层间距逐层乘以 1.20）
// sizeDecay: 远端下单量衰减系数（例如 0.90 表示每层乘以 0.90）
func BuildGeometricGrid(mid float64, levelCount int, baseSize, spacingRatio, sizeDecay float64) []GridLevel {
	if levelCount < 2 { levelCount = 2 }
	if baseSize <= 0 { baseSize = 1 }
	if spacingRatio <= 1.0 { spacingRatio = 1.10 }
	if sizeDecay <= 0 || sizeDecay >= 1 { sizeDecay = 0.95 }

	levels := make([]GridLevel, 0, levelCount*2)
	// 初始间距：按 mid 的 0.05% 做基准，再由 spacingRatio 扩大
	baseStep := mid * 0.0005
	step := baseStep
	size := baseSize
	for i := 1; i <= levelCount; i++ {
		// 远端衰减
		if i > 1 {
			step *= spacingRatio
			size *= sizeDecay
		}
		levels = append(levels, GridLevel{ Price: mid - step, Size: size })
		levels = append(levels, GridLevel{ Price: mid + step, Size: size })
	}
	return levels
}
