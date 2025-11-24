package risk

import (
	"fmt"
	"sync"
	"time"
)

// PnLLimits 定义PnL相关的风控限制
type PnLLimits struct {
	DailyLossLimit   float64 // 日内最大亏损限制（USDC）
	MaxDrawdownLimit float64 // 最大回撤限制（比例，如0.03表示3%）
	MinPnLThreshold  float64 // 最小PnL阈值（触发告警但不熔断）
}

// PnLMetrics PnL指标数据
type PnLMetrics struct {
	RealizedPnL   float64   // 已实现盈亏
	UnrealizedPnL float64   // 未实现盈亏
	TotalPnL      float64   // 总盈亏
	MaxDrawdown   float64   // 最大回撤
	PeakEquity    float64   // 权益峰值
	DailyPnL      float64   // 当日盈亏
	LastUpdate    time.Time // 最后更新时间
}

// PnLMonitor PnL监控器
type PnLMonitor struct {
	// 配置
	limits PnLLimits

	// 状态数据
	realizedPnL   float64   // 已实现盈亏
	unrealizedPnL float64   // 未实现盈亏
	maxDrawdown   float64   // 最大回撤
	peakEquity    float64   // 权益峰值
	dailyPnL      float64   // 当日盈亏
	lastResetTime time.Time // 上次重置时间（用于每日重置）

	// 历史记录
	initialEquity float64 // 初始权益
	todayStart    float64 // 今日开始权益

	mu sync.RWMutex
}

// NewPnLMonitor 创建PnL监控器
func NewPnLMonitor(limits PnLLimits, initialEquity float64) *PnLMonitor {
	now := time.Now()
	return &PnLMonitor{
		limits:        limits,
		initialEquity: initialEquity,
		peakEquity:    initialEquity,
		todayStart:    initialEquity,
		lastResetTime: now,
	}
}

// UpdateRealized 更新已实现盈亏（当订单成交时调用）
func (m *PnLMonitor) UpdateRealized(pnl float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.realizedPnL += pnl
	m.dailyPnL += pnl
	m.updateDrawdown()
}

// UpdateUnrealized 更新未实现盈亏（根据当前持仓和市场价格计算）
func (m *PnLMonitor) UpdateUnrealized(unrealizedPnL float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.unrealizedPnL = unrealizedPnL
	m.updateDrawdown()
}

// updateDrawdown 更新回撤指标（内部方法，调用前需要持有锁）
func (m *PnLMonitor) updateDrawdown() {
	totalEquity := m.initialEquity + m.realizedPnL + m.unrealizedPnL

	// 更新权益峰值
	if totalEquity > m.peakEquity {
		m.peakEquity = totalEquity
	}

	// 计算当前回撤
	if m.peakEquity > 0 {
		currentDrawdown := (m.peakEquity - totalEquity) / m.peakEquity
		if currentDrawdown > m.maxDrawdown {
			m.maxDrawdown = currentDrawdown
		}
	}
}

// CheckLimits 检查是否违反风控限制
func (m *PnLMonitor) CheckLimits() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 检查日内亏损限制
	if m.limits.DailyLossLimit > 0 && m.dailyPnL < -m.limits.DailyLossLimit {
		return fmt.Errorf("daily loss limit exceeded: %.2f < -%.2f",
			m.dailyPnL, m.limits.DailyLossLimit)
	}

	// 检查最大回撤限制
	if m.limits.MaxDrawdownLimit > 0 && m.maxDrawdown > m.limits.MaxDrawdownLimit {
		return fmt.Errorf("max drawdown limit exceeded: %.4f > %.4f",
			m.maxDrawdown, m.limits.MaxDrawdownLimit)
	}

	return nil
}

// ShouldAlert 判断是否应该发送告警（但不熔断）
func (m *PnLMonitor) ShouldAlert() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 如果PnL低于阈值，发送告警
	// MinPnLThreshold通常是负数（如-50表示亏损50时告警）
	if m.limits.MinPnLThreshold != 0 {
		totalPnL := m.realizedPnL + m.unrealizedPnL
		if totalPnL < m.limits.MinPnLThreshold {
			return true
		}
	}

	return false
}

// GetMetrics 获取当前PnL指标
func (m *PnLMonitor) GetMetrics() PnLMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return PnLMetrics{
		RealizedPnL:   m.realizedPnL,
		UnrealizedPnL: m.unrealizedPnL,
		TotalPnL:      m.realizedPnL + m.unrealizedPnL,
		MaxDrawdown:   m.maxDrawdown,
		PeakEquity:    m.peakEquity,
		DailyPnL:      m.dailyPnL,
		LastUpdate:    time.Now(),
	}
}

// ResetDaily 每日重置（通常在UTC 00:00调用）
func (m *PnLMonitor) ResetDaily() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 重置日内统计
	m.dailyPnL = 0
	m.todayStart = m.initialEquity + m.realizedPnL + m.unrealizedPnL
	m.lastResetTime = time.Now()
}

// Reset 完全重置监控器（谨慎使用）
func (m *PnLMonitor) Reset(newInitialEquity float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.realizedPnL = 0
	m.unrealizedPnL = 0
	m.maxDrawdown = 0
	m.dailyPnL = 0
	m.initialEquity = newInitialEquity
	m.peakEquity = newInitialEquity
	m.todayStart = newInitialEquity
	m.lastResetTime = time.Now()
}

// GetTotalEquity 获取当前总权益
func (m *PnLMonitor) GetTotalEquity() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.initialEquity + m.realizedPnL + m.unrealizedPnL
}

// GetDailyPnL 获取当日盈亏
func (m *PnLMonitor) GetDailyPnL() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.dailyPnL
}

// GetDrawdown 获取当前回撤比例
func (m *PnLMonitor) GetDrawdown() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.maxDrawdown
}

// ShouldCheckDailyReset 检查是否需要进行每日重置
func (m *PnLMonitor) ShouldCheckDailyReset() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	// 如果跨天了（UTC时间），返回true
	return now.Day() != m.lastResetTime.Day() || now.Month() != m.lastResetTime.Month()
}
