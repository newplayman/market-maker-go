package alert

import (
	"fmt"
	"sync"
	"time"
)

// Alert 告警信息
type Alert struct {
	Level     string                 // "INFO", "WARNING", "ERROR", "CRITICAL"
	Message   string                 // 告警消息
	Timestamp time.Time              // 告警时间
	Fields    map[string]interface{} // 附加字段
}

// Channel 告警通道接口
type Channel interface {
	Send(alert Alert) error
	Name() string
}

// Manager 告警管理器
type Manager struct {
	channels []Channel
	throttle *Throttler
	mu       sync.RWMutex
}

// Throttler 告警限流器
type Throttler struct {
	lastSent map[string]time.Time
	interval time.Duration
	mu       sync.RWMutex
}

// NewThrottler 创建限流器
func NewThrottler(interval time.Duration) *Throttler {
	return &Throttler{
		lastSent: make(map[string]time.Time),
		interval: interval,
	}
}

// Allow 检查是否允许发送（限流）
func (t *Throttler) Allow(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	lastTime, exists := t.lastSent[key]

	if !exists || now.Sub(lastTime) >= t.interval {
		t.lastSent[key] = now
		return true
	}

	return false
}

// Reset 重置限流器
func (t *Throttler) Reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.lastSent, key)
}

// Clear 清空所有限流记录
func (t *Throttler) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastSent = make(map[string]time.Time)
}

// NewManager 创建告警管理器
func NewManager(channels []Channel, throttleInterval time.Duration) *Manager {
	return &Manager{
		channels: channels,
		throttle: NewThrottler(throttleInterval),
	}
}

// SendAlert 发送告警
func (m *Manager) SendAlert(alert Alert) error {
	// 设置时间戳
	if alert.Timestamp.IsZero() {
		alert.Timestamp = time.Now()
	}

	// 构建限流key
	key := fmt.Sprintf("%s:%s", alert.Level, alert.Message)

	// 检查限流
	if !m.throttle.Allow(key) {
		return nil // 被限流，静默忽略
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// 发送到所有通道
	var lastErr error
	successCount := 0

	for _, ch := range m.channels {
		if err := ch.Send(alert); err != nil {
			lastErr = fmt.Errorf("channel %s failed: %w", ch.Name(), err)
		} else {
			successCount++
		}
	}

	// 如果所有通道都失败，返回最后一个错误
	if successCount == 0 && lastErr != nil {
		return lastErr
	}

	return nil
}

// SendInfo 发送INFO级别告警
func (m *Manager) SendInfo(message string, fields map[string]interface{}) error {
	return m.SendAlert(Alert{
		Level:   "INFO",
		Message: message,
		Fields:  fields,
	})
}

// SendWarning 发送WARNING级别告警
func (m *Manager) SendWarning(message string, fields map[string]interface{}) error {
	return m.SendAlert(Alert{
		Level:   "WARNING",
		Message: message,
		Fields:  fields,
	})
}

// SendError 发送ERROR级别告警
func (m *Manager) SendError(message string, fields map[string]interface{}) error {
	return m.SendAlert(Alert{
		Level:   "ERROR",
		Message: message,
		Fields:  fields,
	})
}

// SendCritical 发送CRITICAL级别告警
func (m *Manager) SendCritical(message string, fields map[string]interface{}) error {
	return m.SendAlert(Alert{
		Level:   "CRITICAL",
		Message: message,
		Fields:  fields,
	})
}

// AddChannel 添加告警通道
func (m *Manager) AddChannel(ch Channel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels = append(m.channels, ch)
}

// RemoveChannel 移除告警通道
func (m *Manager) RemoveChannel(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	filtered := make([]Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		if ch.Name() != name {
			filtered = append(filtered, ch)
		}
	}
	m.channels = filtered
}

// GetChannels 获取所有通道
func (m *Manager) GetChannels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.channels))
	for _, ch := range m.channels {
		names = append(names, ch.Name())
	}
	return names
}

// ResetThrottle 重置限流器
func (m *Manager) ResetThrottle() {
	m.throttle.Clear()
}
