package risk

import (
	"fmt"
	"sync"
	"time"
)

// State 熔断器状态
type State int

const (
	// StateClosed 关闭状态 - 正常运行
	StateClosed State = iota
	// StateOpen 打开状态 - 熔断，拒绝所有请求
	StateOpen
	// StateHalfOpen 半开状态 - 尝试恢复
	StateHalfOpen
)

// String 返回状态名称
func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	// 配置
	threshold      int           // 失败次数阈值
	timeout        time.Duration // 打开状态持续时间
	halfOpenMaxTry int           // 半开状态最大尝试次数

	// 状态
	state           State
	failureCount    int64
	successCount    int64
	consecutiveFail int64 // 连续失败次数
	lastFailTime    time.Time
	openTime        time.Time // 进入打开状态的时间

	mu sync.RWMutex
}

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	Threshold      int           // 触发熔断的失败次数阈值
	Timeout        time.Duration // 熔断后等待时间
	HalfOpenMaxTry int           // 半开状态最大尝试次数（默认3次）
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.Threshold <= 0 {
		config.Threshold = 5
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.HalfOpenMaxTry <= 0 {
		config.HalfOpenMaxTry = 3
	}

	return &CircuitBreaker{
		threshold:      config.Threshold,
		timeout:        config.Timeout,
		halfOpenMaxTry: config.HalfOpenMaxTry,
		state:          StateClosed,
	}
}

// Call 执行操作，通过熔断器
func (cb *CircuitBreaker) Call(fn func() error) error {
	// 检查是否允许执行
	if err := cb.beforeCall(); err != nil {
		return err
	}

	// 执行操作
	err := fn()

	// 记录结果
	cb.afterCall(err)

	return err
}

// beforeCall 调用前检查
func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		// 关闭状态，允许执行
		return nil

	case StateOpen:
		// 检查是否可以转换到半开状态
		if time.Since(cb.openTime) >= cb.timeout {
			cb.state = StateHalfOpen
			cb.successCount = 0
			cb.failureCount = 0
			return nil
		}
		// 仍在熔断期间
		return fmt.Errorf("circuit breaker is open, wait for %v", cb.timeout-time.Since(cb.openTime))

	case StateHalfOpen:
		// 半开状态，允许有限次尝试
		totalAttempts := cb.successCount + cb.failureCount
		if totalAttempts >= int64(cb.halfOpenMaxTry) {
			// 已达到最大尝试次数，需要根据结果决定状态
			if cb.failureCount > 0 {
				// 有失败，重新打开
				cb.state = StateOpen
				cb.openTime = time.Now()
				return fmt.Errorf("circuit breaker half-open attempts exceeded with failures")
			}
			// 全部成功，关闭熔断器
			cb.state = StateClosed
			cb.consecutiveFail = 0
			cb.failureCount = 0
			cb.successCount = 0
		}
		return nil

	default:
		return fmt.Errorf("unknown circuit breaker state: %d", cb.state)
	}
}

// afterCall 调用后记录结果
func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}
}

// onFailure 记录失败
func (cb *CircuitBreaker) onFailure() {
	cb.failureCount++
	cb.consecutiveFail++
	cb.lastFailTime = time.Now()

	switch cb.state {
	case StateClosed:
		// 检查是否达到失败阈值
		if cb.consecutiveFail >= int64(cb.threshold) {
			cb.state = StateOpen
			cb.openTime = time.Now()
		}

	case StateHalfOpen:
		// 半开状态下失败，立即重新打开
		cb.state = StateOpen
		cb.openTime = time.Now()
		cb.successCount = 0
		cb.failureCount = 0
	}
}

// onSuccess 记录成功
func (cb *CircuitBreaker) onSuccess() {
	cb.successCount++
	cb.consecutiveFail = 0 // 重置连续失败计数

	switch cb.state {
	case StateHalfOpen:
		// 半开状态下连续成功，可能关闭熔断器
		if cb.successCount >= int64(cb.halfOpenMaxTry) {
			cb.state = StateClosed
			cb.failureCount = 0
			cb.successCount = 0
		}
	}
}

// RecordSuccess 手动记录成功（不执行函数）
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.onSuccess()
}

// RecordFailure 手动记录失败（不执行函数）
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.onFailure()
}

// GetState 获取当前状态
func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// IsOpen 判断是否处于打开状态
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state == StateOpen
}

// IsClosed 判断是否处于关闭状态
func (cb *CircuitBreaker) IsClosed() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state == StateClosed
}

// IsHalfOpen 判断是否处于半开状态
func (cb *CircuitBreaker) IsHalfOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state == StateHalfOpen
}

// GetMetrics 获取熔断器指标
func (cb *CircuitBreaker) GetMetrics() CircuitBreakerMetrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerMetrics{
		State:            cb.state,
		FailureCount:     cb.failureCount,
		SuccessCount:     cb.successCount,
		ConsecutiveFails: cb.consecutiveFail,
		LastFailTime:     cb.lastFailTime,
		OpenTime:         cb.openTime,
	}
}

// CircuitBreakerMetrics 熔断器指标
type CircuitBreakerMetrics struct {
	State            State
	FailureCount     int64
	SuccessCount     int64
	ConsecutiveFails int64
	LastFailTime     time.Time
	OpenTime         time.Time
}

// Reset 重置熔断器（谨慎使用）
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.consecutiveFail = 0
	cb.lastFailTime = time.Time{}
	cb.openTime = time.Time{}
}

// ForceOpen 强制打开熔断器
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateOpen
	cb.openTime = time.Now()
}

// ForceClose 强制关闭熔断器
func (cb *CircuitBreaker) ForceClose() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.consecutiveFail = 0
}

// AllowRequest 判断是否允许请求通过
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		// 检查是否应该进入半开状态
		return time.Since(cb.openTime) >= cb.timeout
	case StateHalfOpen:
		// 半开状态，检查尝试次数
		return (cb.successCount + cb.failureCount) < int64(cb.halfOpenMaxTry)
	default:
		return false
	}
}
