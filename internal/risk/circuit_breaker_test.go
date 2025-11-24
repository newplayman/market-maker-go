package risk

import (
	"errors"
	"testing"
	"time"
)

func TestNewCircuitBreaker(t *testing.T) {
	// 使用默认值
	cb1 := NewCircuitBreaker(CircuitBreakerConfig{})
	if cb1.threshold != 5 {
		t.Errorf("expected default threshold 5, got %d", cb1.threshold)
	}
	if cb1.timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", cb1.timeout)
	}
	if cb1.halfOpenMaxTry != 3 {
		t.Errorf("expected default halfOpenMaxTry 3, got %d", cb1.halfOpenMaxTry)
	}

	// 自定义配置
	config := CircuitBreakerConfig{
		Threshold:      10,
		Timeout:        1 * time.Minute,
		HalfOpenMaxTry: 5,
	}
	cb2 := NewCircuitBreaker(config)
	if cb2.threshold != 10 {
		t.Errorf("expected threshold 10, got %d", cb2.threshold)
	}
	if cb2.timeout != 1*time.Minute {
		t.Errorf("expected timeout 1m, got %v", cb2.timeout)
	}
	if cb2.halfOpenMaxTry != 5 {
		t.Errorf("expected halfOpenMaxTry 5, got %d", cb2.halfOpenMaxTry)
	}
}

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:      3,
		Timeout:        100 * time.Millisecond,
		HalfOpenMaxTry: 2,
	}
	cb := NewCircuitBreaker(config)

	// 初始状态应该是Closed
	if !cb.IsClosed() {
		t.Error("expected initial state to be CLOSED")
	}

	// 失败未达阈值，保持Closed
	err := errors.New("test error")
	cb.Call(func() error { return err })
	cb.Call(func() error { return err })
	if !cb.IsClosed() {
		t.Error("expected state to remain CLOSED before threshold")
	}

	// 第3次失败，应该转换到Open
	cb.Call(func() error { return err })
	if !cb.IsOpen() {
		t.Errorf("expected state to be OPEN after threshold, got %s", cb.GetState())
	}

	// Open状态下，调用应该被拒绝
	result := cb.Call(func() error { return nil })
	if result == nil {
		t.Error("expected call to be rejected when circuit is open")
	}

	// 等待超时
	time.Sleep(150 * time.Millisecond)

	// 超时后应该允许尝试（进入HalfOpen）
	cb.Call(func() error { return nil })
	if !cb.IsHalfOpen() {
		t.Errorf("expected state to be HALF_OPEN after timeout, got %s", cb.GetState())
	}

	// HalfOpen状态下成功，应该关闭
	cb.Call(func() error { return nil })
	if !cb.IsClosed() {
		t.Errorf("expected state to be CLOSED after successful half-open attempts, got %s", cb.GetState())
	}
}

func TestCircuitBreaker_HalfOpenToOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:      2,
		Timeout:        50 * time.Millisecond,
		HalfOpenMaxTry: 3,
	}
	cb := NewCircuitBreaker(config)

	// 触发熔断
	err := errors.New("test error")
	cb.Call(func() error { return err })
	cb.Call(func() error { return err })

	if !cb.IsOpen() {
		t.Error("expected circuit to be open")
	}

	// 等待超时
	time.Sleep(100 * time.Millisecond)

	// 进入HalfOpen
	cb.Call(func() error { return nil })
	if !cb.IsHalfOpen() {
		t.Error("expected circuit to be half-open")
	}

	// HalfOpen状态下失败，应该立即重新打开
	cb.Call(func() error { return err })
	if !cb.IsOpen() {
		t.Errorf("expected circuit to reopen after half-open failure, got %s", cb.GetState())
	}
}

func TestCircuitBreaker_RecordSuccess(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold: 3,
	}
	cb := NewCircuitBreaker(config)

	// 记录一些失败
	cb.RecordFailure()
	cb.RecordFailure()

	metrics := cb.GetMetrics()
	if metrics.ConsecutiveFails != 2 {
		t.Errorf("expected 2 consecutive failures, got %d", metrics.ConsecutiveFails)
	}

	// 记录成功应该重置连续失败
	cb.RecordSuccess()

	metrics = cb.GetMetrics()
	if metrics.ConsecutiveFails != 0 {
		t.Errorf("expected consecutive failures to reset, got %d", metrics.ConsecutiveFails)
	}
	if metrics.SuccessCount != 1 {
		t.Errorf("expected 1 success, got %d", metrics.SuccessCount)
	}
}

func TestCircuitBreaker_RecordFailure(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold: 3,
	}
	cb := NewCircuitBreaker(config)

	// 记录失败
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if !cb.IsOpen() {
		t.Error("expected circuit to be open after threshold failures")
	}

	metrics := cb.GetMetrics()
	if metrics.FailureCount != 3 {
		t.Errorf("expected 3 failures, got %d", metrics.FailureCount)
	}
	if metrics.ConsecutiveFails != 3 {
		t.Errorf("expected 3 consecutive failures, got %d", metrics.ConsecutiveFails)
	}
}

func TestCircuitBreaker_Call_Success(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{Threshold: 5})

	called := false
	err := cb.Call(func() error {
		called = true
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("function was not called")
	}

	metrics := cb.GetMetrics()
	if metrics.SuccessCount != 1 {
		t.Errorf("expected 1 success, got %d", metrics.SuccessCount)
	}
}

func TestCircuitBreaker_Call_Failure(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{Threshold: 5})

	testErr := errors.New("test error")
	called := false
	err := cb.Call(func() error {
		called = true
		return testErr
	})

	if err != testErr {
		t.Errorf("expected test error, got %v", err)
	}
	if !called {
		t.Error("function was not called")
	}

	metrics := cb.GetMetrics()
	if metrics.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", metrics.FailureCount)
	}
}

func TestCircuitBreaker_GetState(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{Threshold: 2})

	if state := cb.GetState(); state != StateClosed {
		t.Errorf("expected CLOSED, got %s", state)
	}

	// 触发熔断
	cb.RecordFailure()
	cb.RecordFailure()

	if state := cb.GetState(); state != StateOpen {
		t.Errorf("expected OPEN, got %s", state)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := CircuitBreakerConfig{Threshold: 2}
	cb := NewCircuitBreaker(config)

	// 创建一些状态
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()

	metrics := cb.GetMetrics()
	if metrics.FailureCount == 0 || metrics.SuccessCount == 0 {
		t.Error("expected some failures and successes")
	}

	// 重置
	cb.Reset()

	// 验证所有状态被清除
	metrics = cb.GetMetrics()
	if metrics.State != StateClosed {
		t.Errorf("expected state CLOSED after reset, got %s", metrics.State)
	}
	if metrics.FailureCount != 0 {
		t.Errorf("expected failure count 0, got %d", metrics.FailureCount)
	}
	if metrics.SuccessCount != 0 {
		t.Errorf("expected success count 0, got %d", metrics.SuccessCount)
	}
	if metrics.ConsecutiveFails != 0 {
		t.Errorf("expected consecutive fails 0, got %d", metrics.ConsecutiveFails)
	}
}

func TestCircuitBreaker_ForceOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{})

	if !cb.IsClosed() {
		t.Error("expected initial state to be CLOSED")
	}

	cb.ForceOpen()

	if !cb.IsOpen() {
		t.Error("expected state to be OPEN after ForceOpen")
	}

	// 验证请求被拒绝
	err := cb.Call(func() error { return nil })
	if err == nil {
		t.Error("expected call to be rejected")
	}
}

func TestCircuitBreaker_ForceClose(t *testing.T) {
	config := CircuitBreakerConfig{Threshold: 1}
	cb := NewCircuitBreaker(config)

	// 触发熔断
	cb.RecordFailure()

	if !cb.IsOpen() {
		t.Error("expected circuit to be open")
	}

	// 强制关闭
	cb.ForceClose()

	if !cb.IsClosed() {
		t.Error("expected state to be CLOSED after ForceClose")
	}

	// 验证请求可以通过
	called := false
	err := cb.Call(func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error after ForceClose: %v", err)
	}
	if !called {
		t.Error("function should be called after ForceClose")
	}
}

func TestCircuitBreaker_AllowRequest(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold: 2,
		Timeout:   50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Closed状态，允许请求
	if !cb.AllowRequest() {
		t.Error("expected to allow request in CLOSED state")
	}

	// 触发熔断
	cb.RecordFailure()
	cb.RecordFailure()

	// Open状态，初期不允许
	if cb.AllowRequest() {
		t.Error("expected to reject request in OPEN state before timeout")
	}

	// 等待超时
	time.Sleep(100 * time.Millisecond)

	// 超时后应该允许（会进入HalfOpen）
	if !cb.AllowRequest() {
		t.Error("expected to allow request after timeout")
	}
}

func TestCircuitBreaker_GetMetrics(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{Threshold: 5})

	// 记录一些成功和失败
	cb.RecordSuccess()
	cb.RecordFailure()
	cb.RecordSuccess()

	metrics := cb.GetMetrics()

	if metrics.SuccessCount != 2 {
		t.Errorf("expected 2 successes, got %d", metrics.SuccessCount)
	}
	if metrics.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", metrics.FailureCount)
	}
	if metrics.State != StateClosed {
		t.Errorf("expected state CLOSED, got %s", metrics.State)
	}

	// ConsecutiveFails应该被重置（因为有成功）
	if metrics.ConsecutiveFails != 0 {
		t.Errorf("expected 0 consecutive fails, got %d", metrics.ConsecutiveFails)
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold: 100,
	}
	cb := NewCircuitBreaker(config)

	done := make(chan bool)
	operations := 50

	// 并发成功记录
	go func() {
		for i := 0; i < operations; i++ {
			cb.RecordSuccess()
		}
		done <- true
	}()

	// 并发失败记录
	go func() {
		for i := 0; i < operations; i++ {
			cb.RecordFailure()
		}
		done <- true
	}()

	// 并发读取状态
	go func() {
		for i := 0; i < operations; i++ {
			_ = cb.GetState()
			_ = cb.GetMetrics()
			_ = cb.AllowRequest()
		}
		done <- true
	}()

	// 等待完成
	for i := 0; i < 3; i++ {
		<-done
	}

	// 验证最终计数
	metrics := cb.GetMetrics()
	if metrics.SuccessCount != int64(operations) {
		t.Errorf("expected %d successes, got %d", operations, metrics.SuccessCount)
	}
	if metrics.FailureCount != int64(operations) {
		t.Errorf("expected %d failures, got %d", operations, metrics.FailureCount)
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "CLOSED"},
		{StateOpen, "OPEN"},
		{StateHalfOpen, "HALF_OPEN"},
		{State(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("State(%d).String() = %s, want %s", tt.state, got, tt.expected)
		}
	}
}

func TestCircuitBreaker_HalfOpenMaxAttempts(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:      2,
		Timeout:        50 * time.Millisecond,
		HalfOpenMaxTry: 2,
	}
	cb := NewCircuitBreaker(config)

	// 触发熔断
	cb.RecordFailure()
	cb.RecordFailure()

	// 等待超时
	time.Sleep(100 * time.Millisecond)

	// HalfOpen状态，执行最大尝试次数的成功操作
	err1 := cb.Call(func() error { return nil })
	err2 := cb.Call(func() error { return nil })

	if err1 != nil || err2 != nil {
		t.Error("expected calls to succeed in half-open state")
	}

	// 应该转换回Closed
	if !cb.IsClosed() {
		t.Errorf("expected state to be CLOSED after max successful attempts, got %s", cb.GetState())
	}
}

func TestCircuitBreaker_CompleteFlow(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:      3,
		Timeout:        100 * time.Millisecond,
		HalfOpenMaxTry: 2,
	}
	cb := NewCircuitBreaker(config)

	// 阶段1: Closed状态，正常工作
	for i := 0; i < 2; i++ {
		if err := cb.Call(func() error { return nil }); err != nil {
			t.Fatalf("unexpected error in closed state: %v", err)
		}
	}

	// 阶段2: 连续失败，触发熔断
	testErr := errors.New("test error")
	for i := 0; i < 3; i++ {
		cb.Call(func() error { return testErr })
	}
	if !cb.IsOpen() {
		t.Fatal("expected circuit to be open")
	}

	// 阶段3: Open状态，请求被拒绝
	if err := cb.Call(func() error { return nil }); err == nil {
		t.Fatal("expected call to be rejected in open state")
	}

	// 阶段4: 等待超时，进入HalfOpen
	time.Sleep(150 * time.Millisecond)

	// 阶段5: HalfOpen状态，成功恢复
	if err := cb.Call(func() error { return nil }); err != nil {
		t.Fatalf("unexpected error entering half-open: %v", err)
	}
	if !cb.IsHalfOpen() {
		t.Fatal("expected circuit to be half-open")
	}

	// 再次成功，应该关闭
	if err := cb.Call(func() error { return nil }); err != nil {
		t.Fatalf("unexpected error in half-open: %v", err)
	}
	if !cb.IsClosed() {
		t.Fatalf("expected circuit to be closed, got %s", cb.GetState())
	}

	// 阶段6: 验证完全恢复
	if err := cb.Call(func() error { return nil }); err != nil {
		t.Fatalf("unexpected error after recovery: %v", err)
	}
}
