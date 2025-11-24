package strategy

import (
	"container/ring"
	"math"
	"sync"
	"time"
)

// PriceSample 价格样本
type PriceSample struct {
	Price     float64
	Timestamp time.Time
}

// VolatilityCalculator 波动率计算器
type VolatilityCalculator struct {
	window   time.Duration // 计算窗口（如5分钟）
	samples  *ring.Ring    // 价格样本环形缓冲区
	alpha    float64       // EWMA平滑系数
	variance float64       // 当前方差
	mu       sync.RWMutex
}

// VolatilityConfig 波动率计算器配置
type VolatilityConfig struct {
	Window     time.Duration // 计算窗口
	SampleSize int           // 样本数量
	Alpha      float64       // EWMA系数（如0.1）
}

// DefaultVolatilityConfig 返回默认配置
func DefaultVolatilityConfig() VolatilityConfig {
	return VolatilityConfig{
		Window:     5 * time.Minute,
		SampleSize: 100,
		Alpha:      0.1,
	}
}

// NewVolatilityCalculator 创建波动率计算器
func NewVolatilityCalculator(cfg VolatilityConfig) *VolatilityCalculator {
	if cfg.SampleSize <= 0 {
		cfg.SampleSize = 100
	}
	if cfg.Alpha <= 0 || cfg.Alpha > 1 {
		cfg.Alpha = 0.1
	}
	if cfg.Window <= 0 {
		cfg.Window = 5 * time.Minute
	}

	return &VolatilityCalculator{
		window:   cfg.Window,
		samples:  ring.New(cfg.SampleSize),
		alpha:    cfg.Alpha,
		variance: 0,
	}
}

// Update 更新价格样本
func (v *VolatilityCalculator) Update(price float64, timestamp time.Time) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// 添加样本到环形缓冲区
	v.samples.Value = PriceSample{
		Price:     price,
		Timestamp: timestamp,
	}
	v.samples = v.samples.Next()

	// 更新EWMA方差
	v.updateVariance()
}

// Calculate 计算当前波动率（标准差）
func (v *VolatilityCalculator) Calculate() float64 {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.variance <= 0 {
		return 0
	}
	return math.Sqrt(v.variance)
}

// GetAnnualized 获取年化波动率
func (v *VolatilityCalculator) GetAnnualized() float64 {
	vol := v.Calculate()
	if vol == 0 {
		return 0
	}

	// 假设一天有1440分钟
	periodsPerDay := 1440.0 / v.window.Minutes()
	periodsPerYear := periodsPerDay * 365
	return vol * math.Sqrt(periodsPerYear)
}

// GetVariance 获取当前方差
func (v *VolatilityCalculator) GetVariance() float64 {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.variance
}

// Reset 重置计算器
func (v *VolatilityCalculator) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.variance = 0
	// 清空环形缓冲区
	v.samples.Do(func(val interface{}) {
		val = nil
	})
}

// updateVariance 使用EWMA更新方差（需要持有锁）
func (v *VolatilityCalculator) updateVariance() {
	// 收集有效样本
	var returns []float64
	var prevPrice float64
	var count int

	v.samples.Do(func(val interface{}) {
		if val == nil {
			return
		}
		sample, ok := val.(PriceSample)
		if !ok {
			return
		}

		// 检查样本是否在时间窗口内
		now := time.Now()
		if now.Sub(sample.Timestamp) > v.window {
			return
		}

		if prevPrice > 0 {
			// 计算对数收益率
			ret := math.Log(sample.Price / prevPrice)
			returns = append(returns, ret)
		}
		prevPrice = sample.Price
		count++
	})

	// 至少需要2个样本才能计算波动率
	if len(returns) < 1 {
		return
	}

	// 计算最新收益率的平方
	latestReturn := returns[len(returns)-1]
	squaredReturn := latestReturn * latestReturn

	// EWMA更新方差
	if v.variance == 0 {
		// 首次初始化，使用样本方差
		if len(returns) >= 2 {
			mean := 0.0
			for _, r := range returns {
				mean += r
			}
			mean /= float64(len(returns))

			variance := 0.0
			for _, r := range returns {
				diff := r - mean
				variance += diff * diff
			}
			v.variance = variance / float64(len(returns)-1)
		} else {
			v.variance = squaredReturn
		}
	} else {
		// 使用EWMA更新
		v.variance = v.alpha*squaredReturn + (1-v.alpha)*v.variance
	}
}

// GetSampleCount 获取有效样本数量
func (v *VolatilityCalculator) GetSampleCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()

	count := 0
	now := time.Now()
	v.samples.Do(func(val interface{}) {
		if val == nil {
			return
		}
		sample, ok := val.(PriceSample)
		if !ok {
			return
		}
		if now.Sub(sample.Timestamp) <= v.window {
			count++
		}
	})
	return count
}

// GetStatistics 获取统计信息
func (v *VolatilityCalculator) GetStatistics() map[string]interface{} {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return map[string]interface{}{
		"volatility":            v.Calculate(),
		"annualized_volatility": v.GetAnnualized(),
		"variance":              v.variance,
		"sample_count":          v.GetSampleCount(),
		"window_minutes":        v.window.Minutes(),
		"alpha":                 v.alpha,
	}
}
