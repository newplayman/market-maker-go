package strategy

import (
	"math"
	"sync"
)

// DynamicSpreadModel 动态Spread模型
type DynamicSpreadModel struct {
	baseSpread    float64 // 基础Spread（如0.0005）
	volMultiplier float64 // 波动率乘数（如2.0）
	minSpread     float64 // 最小Spread（如0.0003）
	maxSpread     float64 // 最大Spread（如0.002）

	volCalculator *VolatilityCalculator

	mu sync.RWMutex
}

// SpreadModelConfig 配置
type SpreadModelConfig struct {
	BaseSpread    float64 // 基础Spread
	VolMultiplier float64 // 波动率乘数
	MinSpread     float64 // 最小Spread
	MaxSpread     float64 // 最大Spread
}

// DefaultSpreadModelConfig 返回默认配置
func DefaultSpreadModelConfig() SpreadModelConfig {
	return SpreadModelConfig{
		BaseSpread:    0.0005, // 0.05%
		VolMultiplier: 2.0,
		MinSpread:     0.0003, // 0.03%
		MaxSpread:     0.002,  // 0.2%
	}
}

// NewDynamicSpreadModel 创建动态Spread模型
func NewDynamicSpreadModel(
	cfg SpreadModelConfig,
	volCalc *VolatilityCalculator,
) *DynamicSpreadModel {
	// 参数验证
	if cfg.BaseSpread <= 0 {
		cfg.BaseSpread = 0.0005
	}
	if cfg.VolMultiplier < 0 {
		cfg.VolMultiplier = 2.0
	}
	if cfg.MinSpread <= 0 {
		cfg.MinSpread = cfg.BaseSpread * 0.5
	}
	if cfg.MaxSpread <= 0 {
		cfg.MaxSpread = cfg.BaseSpread * 4.0
	}

	// 确保min < max
	if cfg.MinSpread >= cfg.MaxSpread {
		cfg.MaxSpread = cfg.MinSpread * 2
	}

	return &DynamicSpreadModel{
		baseSpread:    cfg.BaseSpread,
		volMultiplier: cfg.VolMultiplier,
		minSpread:     cfg.MinSpread,
		maxSpread:     cfg.MaxSpread,
		volCalculator: volCalc,
	}
}

// Calculate 计算当前应使用的Spread
func (m *DynamicSpreadModel) Calculate() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 获取当前波动率
	volatility := 0.0
	if m.volCalculator != nil {
		volatility = m.volCalculator.Calculate()
	}

	// 根据波动率调整Spread
	// spread = baseSpread + volatility * volMultiplier
	spread := m.baseSpread + volatility*m.volMultiplier

	// 限制在合理范围内
	spread = m.clampSpread(spread)

	return spread
}

// CalculateWithInventory 考虑库存的Spread计算
func (m *DynamicSpreadModel) CalculateWithInventory(inventory, maxInventory float64) float64 {
	baseSpread := m.Calculate()

	if maxInventory <= 0 {
		return baseSpread
	}

	// 库存比率
	inventoryRatio := math.Abs(inventory / maxInventory)

	// 库存较大时，增加Spread鼓励平仓
	if inventoryRatio > 0.8 {
		// 库存超过80%时，增加Spread
		factor := 1.0 + (inventoryRatio-0.8)*0.5 // 最多增加10%
		baseSpread *= factor
	}

	// 再次限制范围
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clampSpread(baseSpread)
}

// CalculateWithDepth 考虑盘口深度的Spread计算
func (m *DynamicSpreadModel) CalculateWithDepth(bidDepth, askDepth, avgDepth float64) float64 {
	baseSpread := m.Calculate()

	if avgDepth <= 0 {
		return baseSpread
	}

	// 计算深度失衡
	totalDepth := bidDepth + askDepth
	if totalDepth <= 0 {
		return baseSpread
	}

	imbalance := math.Abs(bidDepth-askDepth) / totalDepth

	// 深度失衡时，增加Spread以降低风险
	if imbalance > 0.3 {
		factor := 1.0 + imbalance*0.2 // 最多增加20%
		baseSpread *= factor
	}

	// 深度较浅时，增加Spread
	if totalDepth < avgDepth*0.5 {
		baseSpread *= 1.1
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clampSpread(baseSpread)
}

// Adjust 手动调整参数
func (m *DynamicSpreadModel) Adjust(factor float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.baseSpread *= factor

	// 确保不超出范围
	m.baseSpread = m.clampSpread(m.baseSpread)
}

// SetBaseSpread 设置基础Spread
func (m *DynamicSpreadModel) SetBaseSpread(spread float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if spread > 0 {
		m.baseSpread = m.clampSpread(spread)
	}
}

// SetVolMultiplier 设置波动率乘数
func (m *DynamicSpreadModel) SetVolMultiplier(multiplier float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if multiplier >= 0 {
		m.volMultiplier = multiplier
	}
}

// GetConfig 获取当前配置
func (m *DynamicSpreadModel) GetConfig() SpreadModelConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return SpreadModelConfig{
		BaseSpread:    m.baseSpread,
		VolMultiplier: m.volMultiplier,
		MinSpread:     m.minSpread,
		MaxSpread:     m.maxSpread,
	}
}

// GetStatistics 获取统计信息
func (m *DynamicSpreadModel) GetStatistics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	volatility := 0.0
	if m.volCalculator != nil {
		volatility = m.volCalculator.Calculate()
	}

	currentSpread := m.baseSpread + volatility*m.volMultiplier
	currentSpread = m.clampSpread(currentSpread)

	return map[string]interface{}{
		"base_spread":        m.baseSpread,
		"vol_multiplier":     m.volMultiplier,
		"min_spread":         m.minSpread,
		"max_spread":         m.maxSpread,
		"current_volatility": volatility,
		"current_spread":     currentSpread,
	}
}

// clampSpread 限制spread在min和max之间（需要持有锁）
func (m *DynamicSpreadModel) clampSpread(spread float64) float64 {
	if spread < m.minSpread {
		return m.minSpread
	}
	if spread > m.maxSpread {
		return m.maxSpread
	}
	return spread
}
