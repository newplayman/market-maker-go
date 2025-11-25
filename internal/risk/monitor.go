package risk

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RiskState 风险状态
type RiskState int

const (
	// RiskStateNormal 正常状态
	RiskStateNormal RiskState = iota
	// RiskStateWarning 警告状态
	RiskStateWarning
	// RiskStateDanger 危险状态
	RiskStateDanger
	// RiskStateEmergency 紧急状态（熔断）
	RiskStateEmergency
)

// String 返回状态名称
func (s RiskState) String() string {
	switch s {
	case RiskStateNormal:
		return "NORMAL"
	case RiskStateWarning:
		return "WARNING"
	case RiskStateDanger:
		return "DANGER"
	case RiskStateEmergency:
		return "EMERGENCY"
	default:
		return "UNKNOWN"
	}
}

// MonitorConfig 风控监控配置
type MonitorConfig struct {
	// PnL配置
	PnLLimits PnLLimits

	// 熔断器配置
	CircuitBreakerConfig CircuitBreakerConfig

	// 监控间隔
	MonitorInterval time.Duration

	// 初始权益
	InitialEquity float64
}

// Monitor 风控监控中心
type Monitor struct {
	config MonitorConfig

	// 核心监控器
	pnlMonitor     *PnLMonitor
	circuitBreaker *CircuitBreaker

	// 风险状态
	riskState RiskState

	// 回调函数
	onRiskStateChange func(old, new RiskState)
	onEmergencyStop   func(reason string)

	// 控制
	stopChan chan struct{}
	doneChan chan struct{}

	mu sync.RWMutex
}

// NewMonitor 创建风控监控中心
func NewMonitor(config MonitorConfig) *Monitor {
	// 设置默认值
	if config.MonitorInterval <= 0 {
		config.MonitorInterval = 1 * time.Second
	}
	if config.InitialEquity <= 0 {
		config.InitialEquity = 10000.0 // 默认10000 USDC
	}

	return &Monitor{
		config:         config,
		pnlMonitor:     NewPnLMonitor(config.PnLLimits, config.InitialEquity),
		circuitBreaker: NewCircuitBreaker(config.CircuitBreakerConfig),
		riskState:      RiskStateNormal,
		stopChan:       make(chan struct{}),
		doneChan:       make(chan struct{}),
	}
}

// Start 启动风控监控
func (m *Monitor) Start(ctx context.Context) error {
	go m.monitorLoop(ctx)
	return nil
}

// Stop 停止风控监控
func (m *Monitor) Stop() error {
	close(m.stopChan)

	// 等待监控循环结束
	select {
	case <-m.doneChan:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout waiting for monitor to stop")
	}
}

// monitorLoop 监控循环
func (m *Monitor) monitorLoop(ctx context.Context) {
	defer close(m.doneChan)

	ticker := time.NewTicker(m.config.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.checkRisk()
		}
	}
}

// checkRisk 检查风险状态
func (m *Monitor) checkRisk() {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldState := m.riskState

	// 检查熔断器状态
	if m.circuitBreaker.IsOpen() {
		m.riskState = RiskStateEmergency
	} else {
		// 检查PnL限制
		if err := m.pnlMonitor.CheckLimits(); err != nil {
			// 违反限制，触发熔断
			m.circuitBreaker.ForceOpen()
			m.riskState = RiskStateEmergency

			// 触发紧急停止回调
			if m.onEmergencyStop != nil {
				go m.onEmergencyStop(err.Error())
			}
		} else if m.pnlMonitor.ShouldAlert() {
			// 需要告警但不熔断
			m.riskState = RiskStateDanger
		} else {
			// 根据回撤判断风险等级
			drawdown := m.pnlMonitor.GetDrawdown()
			if drawdown > 0.05 { // 放宽到5%回撤为警告
				m.riskState = RiskStateWarning
			} else {
				m.riskState = RiskStateNormal
			}
		}
	}

	// 如果状态变化，触发回调
	if oldState != m.riskState && m.onRiskStateChange != nil {
		go m.onRiskStateChange(oldState, m.riskState)
	}

	// 检查是否需要每日重置
	if m.pnlMonitor.ShouldCheckDailyReset() {
		m.pnlMonitor.ResetDaily()
	}
}

// CheckPreTrade 交易前风控检查
func (m *Monitor) CheckPreTrade(orderValue float64) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 检查熔断器状态
	if m.circuitBreaker.IsOpen() {
		return fmt.Errorf("circuit breaker is open, trading disabled")
	}

	// 检查风险状态
	if m.riskState == RiskStateEmergency {
		return fmt.Errorf("risk state is EMERGENCY, trading disabled")
	}

	// 检查PnL限制
	if err := m.pnlMonitor.CheckLimits(); err != nil {
		return fmt.Errorf("PnL limit check failed: %w", err)
	}

	return nil
}

// RecordTrade 记录交易（更新PnL）
func (m *Monitor) RecordTrade(realizedPnL float64) {
	m.pnlMonitor.UpdateRealized(realizedPnL)

	// 如果是盈利，记录成功
	if realizedPnL >= 0 {
		m.circuitBreaker.RecordSuccess()
	}
}

// UpdateUnrealizedPnL 更新未实现盈亏
func (m *Monitor) UpdateUnrealizedPnL(unrealizedPnL float64) {
	m.pnlMonitor.UpdateUnrealized(unrealizedPnL)
}

// RecordFailure 记录失败事件
func (m *Monitor) RecordFailure() {
	m.circuitBreaker.RecordFailure()
}

// RecordSuccess 记录成功事件
func (m *Monitor) RecordSuccess() {
	m.circuitBreaker.RecordSuccess()
}

// GetRiskState 获取当前风险状态
func (m *Monitor) GetRiskState() RiskState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.riskState
}

// GetPnLMetrics 获取PnL指标
func (m *Monitor) GetPnLMetrics() PnLMetrics {
	return m.pnlMonitor.GetMetrics()
}

// GetCircuitBreakerMetrics 获取熔断器指标
func (m *Monitor) GetCircuitBreakerMetrics() CircuitBreakerMetrics {
	return m.circuitBreaker.GetMetrics()
}

// GetMonitorMetrics 获取完整监控指标
func (m *Monitor) GetMonitorMetrics() MonitorMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return MonitorMetrics{
		RiskState:             m.riskState,
		PnLMetrics:            m.pnlMonitor.GetMetrics(),
		CircuitBreakerMetrics: m.circuitBreaker.GetMetrics(),
		Timestamp:             time.Now(),
	}
}

// MonitorMetrics 监控指标
type MonitorMetrics struct {
	RiskState             RiskState
	PnLMetrics            PnLMetrics
	CircuitBreakerMetrics CircuitBreakerMetrics
	Timestamp             time.Time
}

// TriggerEmergencyStop 触发紧急停止
func (m *Monitor) TriggerEmergencyStop(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 强制打开熔断器
	m.circuitBreaker.ForceOpen()
	m.riskState = RiskStateEmergency

	// 触发回调
	if m.onEmergencyStop != nil {
		go m.onEmergencyStop(reason)
	}
}

// ResumeTrading 恢复交易（谨慎使用）
func (m *Monitor) ResumeTrading() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查PnL是否还在限制内
	if err := m.pnlMonitor.CheckLimits(); err != nil {
		return fmt.Errorf("cannot resume: PnL limits still violated: %w", err)
	}

	// 关闭熔断器
	m.circuitBreaker.ForceClose()
	m.riskState = RiskStateNormal

	return nil
}

// Reset 重置监控器（谨慎使用）
func (m *Monitor) Reset(newInitialEquity float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pnlMonitor.Reset(newInitialEquity)
	m.circuitBreaker.Reset()
	m.riskState = RiskStateNormal
}

// SetRiskStateChangeCallback 设置风险状态变化回调
func (m *Monitor) SetRiskStateChangeCallback(callback func(old, new RiskState)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onRiskStateChange = callback
}

// SetEmergencyStopCallback 设置紧急停止回调
func (m *Monitor) SetEmergencyStopCallback(callback func(reason string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onEmergencyStop = callback
}

// IsTrading 判断是否允许交易
func (m *Monitor) IsTrading() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.riskState != RiskStateEmergency && !m.circuitBreaker.IsOpen()
}

// GetTotalEquity 获取当前总权益
func (m *Monitor) GetTotalEquity() float64 {
	return m.pnlMonitor.GetTotalEquity()
}

// GetDailyPnL 获取当日盈亏
func (m *Monitor) GetDailyPnL() float64 {
	return m.pnlMonitor.GetDailyPnL()
}

// GetDrawdown 获取当前回撤
func (m *Monitor) GetDrawdown() float64 {
	return m.pnlMonitor.GetDrawdown()
}

// ForceCheckRisk 强制执行一次风险检查
func (m *Monitor) ForceCheckRisk() {
	m.checkRisk()
}
