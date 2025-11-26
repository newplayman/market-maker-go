package risk

import (
	"math"
	"sync"
	"time"

	"market-maker-go/posttrade"
)

// AdaptiveRiskManager 自适应风控管理器
type AdaptiveRiskManager struct {
	mu sync.RWMutex

	// 依赖
	postTradeAnalyzer *posttrade.Analyzer

	// 配置
	config AdaptiveConfig

	// 当前自适应参数
	currentNetMax       float64
	currentBaseSize     float64
	currentMinSpreadBps float64

	// 历史指标
	recentAdverseRates []float64
	lastAdjustTime     time.Time
}

// AdaptiveConfig 自适应风控配置
type AdaptiveConfig struct {
	// 基准值
	BaseNetMax       float64
	BaseSize         float64
	BaseMinSpreadBps float64

	// 调整范围
	NetMaxMin       float64
	NetMaxMax       float64
	BaseSizeMin     float64
	BaseSizeMax     float64
	MinSpreadBpsMin float64
	MinSpreadBpsMax float64

	// 逆选阈值
	AdverseThresholdLow  float64 // 低逆选阈值
	AdverseThresholdHigh float64 // 高逆选阈值

	// 调整系数
	AdjustFactor float64 // 每次调整幅度

	// 调整间隔
	AdjustInterval time.Duration
	WindowSize     int // 滑动窗口大小
}

// DefaultAdaptiveConfig 默认配置
func DefaultAdaptiveConfig() AdaptiveConfig {
	return AdaptiveConfig{
		BaseNetMax:           3.0,
		BaseSize:             0.01,
		BaseMinSpreadBps:     6.0,
		NetMaxMin:            1.0,
		NetMaxMax:            5.0,
		BaseSizeMin:          0.005,
		BaseSizeMax:          0.02,
		MinSpreadBpsMin:      4.0,
		MinSpreadBpsMax:      15.0,
		AdverseThresholdLow:  0.4,
		AdverseThresholdHigh: 0.6,
		AdjustFactor:         0.1,
		AdjustInterval:       5 * time.Minute,
		WindowSize:           10,
	}
}

// NewAdaptiveRiskManager 创建自适应风控管理器
func NewAdaptiveRiskManager(analyzer *posttrade.Analyzer, config AdaptiveConfig) *AdaptiveRiskManager {
	return &AdaptiveRiskManager{
		postTradeAnalyzer:   analyzer,
		config:              config,
		currentNetMax:       config.BaseNetMax,
		currentBaseSize:     config.BaseSize,
		currentMinSpreadBps: config.BaseMinSpreadBps,
		recentAdverseRates:  make([]float64, 0, config.WindowSize),
		lastAdjustTime:      time.Now(),
	}
}

// Update 根据事后分析更新参数。
// 如果外部传入 forceTrend=true，将强制走“高逆选”分支（下降 netMax/baseSize、上升 spread）以应对趋势防御模式。
func (a *AdaptiveRiskManager) Update() {
	a.UpdateWithTrend(false)
}

func (a *AdaptiveRiskManager) UpdateWithTrend(forceTrend bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// 检查是否到了调整时间
	if time.Since(a.lastAdjustTime) < a.config.AdjustInterval {
		return
	}

	// 获取事后统计
	stats := a.postTradeAnalyzer.Stats()
	if stats.AnalyzedFills < 5 {
		// 样本不足，暂不调整
		return
	}

	// 记录逆选率
	a.recentAdverseRates = append(a.recentAdverseRates, stats.AdverseSelectionRate)
	if len(a.recentAdverseRates) > a.config.WindowSize {
		a.recentAdverseRates = a.recentAdverseRates[1:]
	}

	// 计算平均逆选率
	avgAdverseRate := 0.0
	for _, rate := range a.recentAdverseRates {
		avgAdverseRate += rate
	}
	avgAdverseRate /= float64(len(a.recentAdverseRates))

	// 如果强制趋势，视为高逆选分支
	if forceTrend {
		avgAdverseRate = a.config.AdverseThresholdHigh + 0.01
	}

	// 根据逆选率调整参数
	a.adjustParameters(avgAdverseRate)

	a.lastAdjustTime = time.Now()
}

// adjustParameters 根据逆选率调整参数
func (a *AdaptiveRiskManager) adjustParameters(adverseRate float64) {
	// 高逆选：变保守（减小NetMax、减小Size、增大Spread）
	if adverseRate > a.config.AdverseThresholdHigh {
		// 减小风险敞口
		a.currentNetMax *= (1 - a.config.AdjustFactor)
		a.currentNetMax = math.Max(a.currentNetMax, a.config.NetMaxMin)

		// 减小下单量
		a.currentBaseSize *= (1 - a.config.AdjustFactor)
		a.currentBaseSize = math.Max(a.currentBaseSize, a.config.BaseSizeMin)

		// 增大价差
		a.currentMinSpreadBps *= (1 + a.config.AdjustFactor)
		a.currentMinSpreadBps = math.Min(a.currentMinSpreadBps, a.config.MinSpreadBpsMax)
	} else if adverseRate < a.config.AdverseThresholdLow {
		// 低逆选：变激进（增大NetMax、增大Size、减小Spread）
		// 增大风险敞口
		a.currentNetMax *= (1 + a.config.AdjustFactor)
		a.currentNetMax = math.Min(a.currentNetMax, a.config.NetMaxMax)

		// 增大下单量
		a.currentBaseSize *= (1 + a.config.AdjustFactor)
		a.currentBaseSize = math.Min(a.currentBaseSize, a.config.BaseSizeMax)

		// 减小价差
		a.currentMinSpreadBps *= (1 - a.config.AdjustFactor)
		a.currentMinSpreadBps = math.Max(a.currentMinSpreadBps, a.config.MinSpreadBpsMin)
	}
	// 中等逆选：保持不变
}

// GetCurrentNetMax 获取当前自适应NetMax
func (a *AdaptiveRiskManager) GetCurrentNetMax() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentNetMax
}

// GetCurrentBaseSize 获取当前自适应BaseSize
func (a *AdaptiveRiskManager) GetCurrentBaseSize() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentBaseSize
}

// GetCurrentMinSpreadBps 获取当前自适应MinSpreadBps
func (a *AdaptiveRiskManager) GetCurrentMinSpreadBps() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentMinSpreadBps
}

// GetAverageAdverseRate 获取平均逆选率
func (a *AdaptiveRiskManager) GetAverageAdverseRate() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if len(a.recentAdverseRates) == 0 {
		return 0
	}

	avg := 0.0
	for _, rate := range a.recentAdverseRates {
		avg += rate
	}
	return avg / float64(len(a.recentAdverseRates))
}

// Reset 重置为基准值
func (a *AdaptiveRiskManager) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.currentNetMax = a.config.BaseNetMax
	a.currentBaseSize = a.config.BaseSize
	a.currentMinSpreadBps = a.config.BaseMinSpreadBps
	a.recentAdverseRates = make([]float64, 0, a.config.WindowSize)
	a.lastAdjustTime = time.Now()
}
