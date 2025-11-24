package alert

import (
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	ch := NewMockChannel("test")
	mgr := NewManager([]Channel{ch}, 5*time.Minute)

	if mgr == nil {
		t.Fatal("manager should not be nil")
	}

	channels := mgr.GetChannels()
	if len(channels) != 1 {
		t.Errorf("expected 1 channel, got %d", len(channels))
	}
	if channels[0] != "test" {
		t.Errorf("channel name = %s, want test", channels[0])
	}
}

func TestSendAlert(t *testing.T) {
	mock := NewMockChannel("mock")
	mgr := NewManager([]Channel{mock}, 5*time.Minute)

	err := mgr.SendAlert(Alert{
		Level:   "INFO",
		Message: "test message",
		Fields:  map[string]interface{}{"key": "value"},
	})

	if err != nil {
		t.Fatalf("SendAlert failed: %v", err)
	}

	if mock.Count() != 1 {
		t.Errorf("expected 1 alert, got %d", mock.Count())
	}

	alert := mock.GetAlerts()[0]
	if alert.Level != "INFO" {
		t.Errorf("level = %s, want INFO", alert.Level)
	}
	if alert.Message != "test message" {
		t.Errorf("message = %s, want 'test message'", alert.Message)
	}
	if alert.Fields["key"] != "value" {
		t.Errorf("field key = %v, want value", alert.Fields["key"])
	}
	if alert.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
}

func TestSendAlertLevels(t *testing.T) {
	tests := []struct {
		name    string
		sendFn  func(*Manager) error
		wantLvl string
	}{
		{
			name: "SendInfo",
			sendFn: func(m *Manager) error {
				return m.SendInfo("info msg", nil)
			},
			wantLvl: "INFO",
		},
		{
			name: "SendWarning",
			sendFn: func(m *Manager) error {
				return m.SendWarning("warning msg", nil)
			},
			wantLvl: "WARNING",
		},
		{
			name: "SendError",
			sendFn: func(m *Manager) error {
				return m.SendError("error msg", nil)
			},
			wantLvl: "ERROR",
		},
		{
			name: "SendCritical",
			sendFn: func(m *Manager) error {
				return m.SendCritical("critical msg", nil)
			},
			wantLvl: "CRITICAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockChannel("mock")
			mgr := NewManager([]Channel{mock}, 5*time.Minute)

			err := tt.sendFn(mgr)
			if err != nil {
				t.Fatalf("send failed: %v", err)
			}

			if mock.Count() != 1 {
				t.Fatalf("expected 1 alert, got %d", mock.Count())
			}

			alert := mock.GetAlerts()[0]
			if alert.Level != tt.wantLvl {
				t.Errorf("level = %s, want %s", alert.Level, tt.wantLvl)
			}
		})
	}
}

func TestThrottling(t *testing.T) {
	mock := NewMockChannel("mock")
	mgr := NewManager([]Channel{mock}, 100*time.Millisecond)

	// 第一次发送应该成功
	err := mgr.SendInfo("test", nil)
	if err != nil {
		t.Fatalf("first send failed: %v", err)
	}
	if mock.Count() != 1 {
		t.Errorf("first send: expected 1 alert, got %d", mock.Count())
	}

	// 立即再次发送相同消息应该被限流
	err = mgr.SendInfo("test", nil)
	if err != nil {
		t.Fatalf("second send failed: %v", err)
	}
	if mock.Count() != 1 {
		t.Errorf("throttled send should not increase count, got %d", mock.Count())
	}

	// 等待限流时间过后
	time.Sleep(150 * time.Millisecond)

	// 再次发送应该成功
	err = mgr.SendInfo("test", nil)
	if err != nil {
		t.Fatalf("third send failed: %v", err)
	}
	if mock.Count() != 2 {
		t.Errorf("after throttle period: expected 2 alerts, got %d", mock.Count())
	}
}

func TestDifferentMessagesNotThrottled(t *testing.T) {
	mock := NewMockChannel("mock")
	mgr := NewManager([]Channel{mock}, 5*time.Minute)

	// 发送不同的消息不应被限流
	mgr.SendInfo("message 1", nil)
	mgr.SendInfo("message 2", nil)
	mgr.SendWarning("message 1", nil) // 不同level

	if mock.Count() != 3 {
		t.Errorf("expected 3 alerts, got %d", mock.Count())
	}
}

func TestMultipleChannels(t *testing.T) {
	mock1 := NewMockChannel("mock1")
	mock2 := NewMockChannel("mock2")
	mgr := NewManager([]Channel{mock1, mock2}, 5*time.Minute)

	err := mgr.SendInfo("test", nil)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	if mock1.Count() != 1 {
		t.Errorf("mock1: expected 1 alert, got %d", mock1.Count())
	}
	if mock2.Count() != 1 {
		t.Errorf("mock2: expected 1 alert, got %d", mock2.Count())
	}
}

func TestChannelError(t *testing.T) {
	mock := NewMockChannel("mock")
	mock.SetShouldError(true)
	mgr := NewManager([]Channel{mock}, 5*time.Minute)

	err := mgr.SendInfo("test", nil)
	if err == nil {
		t.Error("expected error when all channels fail")
	}
}

func TestPartialChannelFailure(t *testing.T) {
	mock1 := NewMockChannel("mock1")
	mock1.SetShouldError(true)
	mock2 := NewMockChannel("mock2")

	mgr := NewManager([]Channel{mock1, mock2}, 5*time.Minute)

	err := mgr.SendInfo("test", nil)
	if err != nil {
		t.Errorf("should not return error when some channels succeed: %v", err)
	}

	if mock2.Count() != 1 {
		t.Errorf("successful channel should receive alert")
	}
}

func TestAddRemoveChannel(t *testing.T) {
	mock1 := NewMockChannel("mock1")
	mgr := NewManager([]Channel{mock1}, 5*time.Minute)

	// 添加通道
	mock2 := NewMockChannel("mock2")
	mgr.AddChannel(mock2)

	channels := mgr.GetChannels()
	if len(channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(channels))
	}

	// 发送告警到两个通道
	mgr.SendInfo("test", nil)
	if mock1.Count() != 1 || mock2.Count() != 1 {
		t.Error("both channels should receive alert")
	}

	// 移除通道
	mgr.RemoveChannel("mock1")
	channels = mgr.GetChannels()
	if len(channels) != 1 {
		t.Errorf("expected 1 channel after removal, got %d", len(channels))
	}
	if channels[0] != "mock2" {
		t.Errorf("remaining channel should be mock2, got %s", channels[0])
	}
}

func TestResetThrottle(t *testing.T) {
	mock := NewMockChannel("mock")
	mgr := NewManager([]Channel{mock}, 5*time.Minute)

	// 发送第一次
	mgr.SendInfo("test", nil)
	if mock.Count() != 1 {
		t.Fatalf("expected 1 alert, got %d", mock.Count())
	}

	// 立即再次发送应该被限流
	mgr.SendInfo("test", nil)
	if mock.Count() != 1 {
		t.Error("should be throttled")
	}

	// 重置限流器
	mgr.ResetThrottle()

	// 再次发送应该成功
	mgr.SendInfo("test", nil)
	if mock.Count() != 2 {
		t.Errorf("after reset: expected 2 alerts, got %d", mock.Count())
	}
}

func TestThrottler(t *testing.T) {
	throttle := NewThrottler(100 * time.Millisecond)

	// 第一次应该允许
	if !throttle.Allow("key1") {
		t.Error("first call should be allowed")
	}

	// 立即再次请求应该被拒绝
	if throttle.Allow("key1") {
		t.Error("second call should be throttled")
	}

	// 不同的key不应受影响
	if !throttle.Allow("key2") {
		t.Error("different key should be allowed")
	}

	// 等待限流时间过后
	time.Sleep(150 * time.Millisecond)

	// 应该再次允许
	if !throttle.Allow("key1") {
		t.Error("after interval should be allowed")
	}
}

func TestThrottlerReset(t *testing.T) {
	throttle := NewThrottler(5 * time.Minute)

	throttle.Allow("key1")
	if throttle.Allow("key1") {
		t.Error("should be throttled")
	}

	// 重置特定key
	throttle.Reset("key1")
	if !throttle.Allow("key1") {
		t.Error("after reset should be allowed")
	}
}

func TestThrottlerClear(t *testing.T) {
	throttle := NewThrottler(5 * time.Minute)

	throttle.Allow("key1")
	throttle.Allow("key2")

	// 清空所有
	throttle.Clear()

	if !throttle.Allow("key1") {
		t.Error("key1 should be allowed after clear")
	}
	if !throttle.Allow("key2") {
		t.Error("key2 should be allowed after clear")
	}
}

func TestLogChannel(t *testing.T) {
	ch := NewLogChannel("test", nil)

	if ch.Name() != "test" {
		t.Errorf("name = %s, want test", ch.Name())
	}

	err := ch.Send(Alert{
		Level:   "INFO",
		Message: "test message",
		Fields:  map[string]interface{}{"key": "value"},
	})

	if err != nil {
		t.Errorf("Send failed: %v", err)
	}
}

func TestConsoleChannel(t *testing.T) {
	ch := NewConsoleChannel("console")

	if ch.Name() != "console" {
		t.Errorf("name = %s, want console", ch.Name())
	}

	// 测试不同级别的告警
	levels := []string{"INFO", "WARNING", "ERROR", "CRITICAL"}
	for _, level := range levels {
		err := ch.Send(Alert{
			Level:     level,
			Message:   "test " + level,
			Timestamp: time.Now(),
			Fields:    map[string]interface{}{"test": "value"},
		})
		if err != nil {
			t.Errorf("Send %s failed: %v", level, err)
		}
	}
}

func TestMockChannel(t *testing.T) {
	mock := NewMockChannel("mock")

	if mock.Name() != "mock" {
		t.Errorf("name = %s, want mock", mock.Name())
	}
	if mock.Count() != 0 {
		t.Errorf("initial count = %d, want 0", mock.Count())
	}

	// 发送告警
	alert := Alert{Level: "INFO", Message: "test"}
	err := mock.Send(alert)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if mock.Count() != 1 {
		t.Errorf("count = %d, want 1", mock.Count())
	}

	alerts := mock.GetAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Message != "test" {
		t.Errorf("message = %s, want test", alerts[0].Message)
	}

	// 测试错误模式
	mock.SetShouldError(true)
	err = mock.Send(alert)
	if err == nil {
		t.Error("expected error when shouldErr is true")
	}

	// 清空
	mock.Clear()
	if mock.Count() != 0 {
		t.Errorf("count after clear = %d, want 0", mock.Count())
	}
}

func TestConcurrentAlerts(t *testing.T) {
	mock := NewMockChannel("mock")
	mgr := NewManager([]Channel{mock}, 100*time.Millisecond)

	// 并发发送告警
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			mgr.SendInfo("test", map[string]interface{}{"id": id})
			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 由于限流，只有第一个应该通过
	if mock.Count() != 1 {
		t.Errorf("concurrent sends with same message should be throttled, got %d alerts", mock.Count())
	}
}

// 基准测试
func BenchmarkSendAlert(b *testing.B) {
	mock := NewMockChannel("mock")
	mgr := NewManager([]Channel{mock}, 5*time.Minute)

	alert := Alert{
		Level:   "INFO",
		Message: "benchmark",
		Fields:  map[string]interface{}{"key": "value"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr.SendAlert(alert)
	}
}

func BenchmarkThrottler(b *testing.B) {
	throttle := NewThrottler(5 * time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		throttle.Allow("test_key")
	}
}
