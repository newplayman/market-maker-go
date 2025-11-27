package risk

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewMonitor(t *testing.T) {
	config := MonitorConfig{
		PnLLimits: PnLLimits{
			DailyLossLimit:   100.0,
			MaxDrawdownLimit: 0.10,
		},
		CircuitBreakerConfig: CircuitBreakerConfig{
			Threshold: 5,
			Timeout:   30 * time.Second,
		},
		MonitorInterval: 100 * time.Millisecond,
		InitialEquity:   10000.0,
	}

	monitor := NewMonitor(config)

	if monitor == nil {
		t.Fatal("expected monitor to be created")
	}
	if monitor.riskState != RiskStateNormal {
		t.Errorf("expected initial state NORMAL, got %s", monitor.riskState)
	}
	if monitor.pnlMonitor == nil {
		t.Error("expected PnL monitor to be initialized")
	}
	if monitor.circuitBreaker == nil {
		t.Error("expected circuit breaker to be initialized")
	}
}

func TestMonitor_DefaultValues(t *testing.T) {
	// 测试默认值
	config := MonitorConfig{}
	monitor := NewMonitor(config)

	if monitor.config.MonitorInterval != 1*time.Second {
		t.Errorf("expected default interval 1s, got %v", monitor.config.MonitorInterval)
	}
	if monitor.config.InitialEquity != 10000.0 {
		t.Errorf("expected default equity 10000, got %f", monitor.config.InitialEquity)
	}
}

func TestMonitor_StartStop(t *testing.T) {
	config := MonitorConfig{
		MonitorInterval: 10 * time.Millisecond,
	}
	monitor := NewMonitor(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动监控
	err := monitor.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start monitor: %v", err)
	}

	// 等待一小段时间确保监控循环运行
	time.Sleep(50 * time.Millisecond)

	// 停止监控
	err = monitor.Stop()
	if err != nil {
		t.Errorf("failed to stop monitor: %v", err)
	}
}

func TestMonitor_CheckPreTrade(t *testing.T) {
	config := MonitorConfig{
		PnLLimits: PnLLimits{
			DailyLossLimit:   100.0,
			MaxDrawdownLimit: 0.05,
		},
		InitialEquity: 10000.0,
	}
	monitor := NewMonitor(config)

	// 正常状态，应该允许交易
	err := monitor.CheckPreTrade(100.0)
	if err != nil {
		t.Errorf("unexpected error in normal state: %v", err)
	}

	// 触发紧急停止
	monitor.TriggerEmergencyStop("test")

	// 紧急状态，应该拒绝交易
	err = monitor.CheckPreTrade(100.0)
	if err == nil {
		t.Error("expected error in emergency state")
	}
}

func TestMonitor_RecordTrade(t *testing.T) {
	config := MonitorConfig{
		InitialEquity: 10000.0,
	}
	monitor := NewMonitor(config)

	// 记录盈利交易
	monitor.RecordTrade(50.0)

	metrics := monitor.GetPnLMetrics()
	if metrics.RealizedPnL != 50.0 {
		t.Errorf("expected realized PnL 50.0, got %f", metrics.RealizedPnL)
	}

	// 验证熔断器也记录了成功
	cbMetrics := monitor.GetCircuitBreakerMetrics()
	if cbMetrics.SuccessCount != 1 {
		t.Errorf("expected 1 success in circuit breaker, got %d", cbMetrics.SuccessCount)
	}

	// 记录亏损交易（不算成功）
	monitor.RecordTrade(-30.0)
	metrics = monitor.GetPnLMetrics()
	if metrics.RealizedPnL != 20.0 {
		t.Errorf("expected realized PnL 20.0, got %f", metrics.RealizedPnL)
	}
}

func TestMonitor_UpdateUnrealizedPnL(t *testing.T) {
	config := MonitorConfig{
		InitialEquity: 10000.0,
	}
	monitor := NewMonitor(config)

	monitor.UpdateUnrealizedPnL(100.0)

	metrics := monitor.GetPnLMetrics()
	if metrics.UnrealizedPnL != 100.0 {
		t.Errorf("expected unrealized PnL 100.0, got %f", metrics.UnrealizedPnL)
	}
	if metrics.TotalPnL != 100.0 {
		t.Errorf("expected total PnL 100.0, got %f", metrics.TotalPnL)
	}
}

func TestMonitor_RiskStateTransitions(t *testing.T) {
	config := MonitorConfig{
		PnLLimits: PnLLimits{
			DailyLossLimit:   100.0,
			MaxDrawdownLimit: 0.10,
		},
		MonitorInterval: 10 * time.Millisecond,
		InitialEquity:   10000.0,
	}
	monitor := NewMonitor(config)

	// 初始状态
	if state := monitor.GetRiskState(); state != RiskStateNormal {
		t.Errorf("expected initial state NORMAL, got %s", state)
	}

	// 创造一些盈利，然后亏损产生回撤
	monitor.RecordTrade(500.0) // 权益 10500
	monitor.ForceCheckRisk()

	monitor.RecordTrade(-550.0) // 权益 9950，回撤 ~5.2%，净亏损 -50
	monitor.ForceCheckRisk()

	// 回撤超过 5% 应该进入 WARNING
	if state := monitor.GetRiskState(); state != RiskStateWarning {
		t.Errorf("expected WARNING state once drawdown >5%%, got %s", state)
	}

	// 继续亏损触发日内限制
	monitor.RecordTrade(-100.0) // 总亏损 -50 (净盈亏 +50)
	monitor.RecordTrade(-160.0) // 总亏损 -210，超过日内限制100
	monitor.ForceCheckRisk()

	// 应该进入EMERGENCY状态
	if state := monitor.GetRiskState(); state != RiskStateEmergency {
		t.Errorf("expected EMERGENCY state after limit breach, got %s", state)
	}
}

func TestMonitor_RecordFailureSuccess(t *testing.T) {
	config := MonitorConfig{
		CircuitBreakerConfig: CircuitBreakerConfig{
			Threshold: 3,
		},
	}
	monitor := NewMonitor(config)

	// 记录成功
	monitor.RecordSuccess()
	cbMetrics := monitor.GetCircuitBreakerMetrics()
	if cbMetrics.SuccessCount != 1 {
		t.Errorf("expected 1 success, got %d", cbMetrics.SuccessCount)
	}

	// 记录失败
	monitor.RecordFailure()
	monitor.RecordFailure()
	monitor.RecordFailure()

	// 应该触发熔断
	if !monitor.circuitBreaker.IsOpen() {
		t.Error("expected circuit breaker to be open after threshold failures")
	}
}

func TestMonitor_GetMonitorMetrics(t *testing.T) {
	config := MonitorConfig{
		InitialEquity: 10000.0,
	}
	monitor := NewMonitor(config)

	monitor.RecordTrade(100.0)
	monitor.UpdateUnrealizedPnL(50.0)

	metrics := monitor.GetMonitorMetrics()

	if metrics.RiskState != RiskStateNormal {
		t.Errorf("expected risk state NORMAL, got %s", metrics.RiskState)
	}
	if metrics.PnLMetrics.RealizedPnL != 100.0 {
		t.Errorf("expected realized PnL 100.0, got %f", metrics.PnLMetrics.RealizedPnL)
	}
	if metrics.PnLMetrics.UnrealizedPnL != 50.0 {
		t.Errorf("expected unrealized PnL 50.0, got %f", metrics.PnLMetrics.UnrealizedPnL)
	}
	if metrics.Timestamp.IsZero() {
		t.Error("expected timestamp to be set")
	}
}

func TestMonitor_TriggerEmergencyStop(t *testing.T) {
	config := MonitorConfig{}
	monitor := NewMonitor(config)

	// 设置回调
	var mu sync.Mutex
	emergencyStopCalled := false
	reason := ""
	monitor.SetEmergencyStopCallback(func(r string) {
		mu.Lock()
		emergencyStopCalled = true
		reason = r
		mu.Unlock()
	})

	// 触发紧急停止
	testReason := "manual emergency stop"
	monitor.TriggerEmergencyStop(testReason)

	// 等待回调执行
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if !emergencyStopCalled {
		t.Error("expected emergency stop callback to be called")
	}
	if reason != testReason {
		t.Errorf("expected reason '%s', got '%s'", testReason, reason)
	}
	mu.Unlock()

	if state := monitor.GetRiskState(); state != RiskStateEmergency {
		t.Errorf("expected EMERGENCY state, got %s", state)
	}
	if !monitor.circuitBreaker.IsOpen() {
		t.Error("expected circuit breaker to be open")
	}
}

func TestMonitor_ResumeTrading(t *testing.T) {
	config := MonitorConfig{
		PnLLimits: PnLLimits{
			DailyLossLimit: 100.0,
		},
		InitialEquity: 10000.0,
	}
	monitor := NewMonitor(config)

	// 触发紧急停止
	monitor.TriggerEmergencyStop("test")

	if monitor.IsTrading() {
		t.Error("expected trading to be disabled")
	}

	// 尝试恢复交易
	err := monitor.ResumeTrading()
	if err != nil {
		t.Errorf("failed to resume trading: %v", err)
	}

	// 验证状态
	if state := monitor.GetRiskState(); state != RiskStateNormal {
		t.Errorf("expected NORMAL state after resume, got %s", state)
	}
	if !monitor.IsTrading() {
		t.Error("expected trading to be enabled after resume")
	}
}

func TestMonitor_ResumeTrading_WithViolation(t *testing.T) {
	config := MonitorConfig{
		PnLLimits: PnLLimits{
			DailyLossLimit: 100.0,
		},
		InitialEquity: 10000.0,
	}
	monitor := NewMonitor(config)

	// 创建PnL违规
	monitor.RecordTrade(-150.0) // 超过日内亏损限制
	monitor.TriggerEmergencyStop("daily loss limit")

	// 尝试恢复交易应该失败
	err := monitor.ResumeTrading()
	if err == nil {
		t.Error("expected error when resuming with PnL violation")
	}

	if monitor.IsTrading() {
		t.Error("trading should still be disabled")
	}
}

func TestMonitor_Reset(t *testing.T) {
	config := MonitorConfig{
		InitialEquity: 10000.0,
	}
	monitor := NewMonitor(config)

	// 创建一些状态
	monitor.RecordTrade(100.0)
	monitor.UpdateUnrealizedPnL(50.0)
	monitor.RecordFailure()
	monitor.RecordFailure()

	// 重置
	newEquity := 15000.0
	monitor.Reset(newEquity)

	// 验证PnL被重置
	metrics := monitor.GetPnLMetrics()
	if metrics.RealizedPnL != 0 {
		t.Errorf("expected realized PnL to be 0, got %f", metrics.RealizedPnL)
	}
	if metrics.UnrealizedPnL != 0 {
		t.Errorf("expected unrealized PnL to be 0, got %f", metrics.UnrealizedPnL)
	}

	// 验证熔断器被重置
	cbMetrics := monitor.GetCircuitBreakerMetrics()
	if cbMetrics.FailureCount != 0 {
		t.Errorf("expected failure count to be 0, got %d", cbMetrics.FailureCount)
	}

	// 验证风险状态
	if state := monitor.GetRiskState(); state != RiskStateNormal {
		t.Errorf("expected NORMAL state after reset, got %s", state)
	}

	// 验证新权益
	if equity := monitor.GetTotalEquity(); equity != newEquity {
		t.Errorf("expected equity %f, got %f", newEquity, equity)
	}
}

func TestMonitor_RiskStateChangeCallback(t *testing.T) {
	config := MonitorConfig{
		MonitorInterval: 10 * time.Millisecond,
		PnLLimits: PnLLimits{
			DailyLossLimit: 100.0,
		},
		InitialEquity: 10000.0,
	}
	monitor := NewMonitor(config)

	// 设置回调
	var mu sync.Mutex
	callbackTriggered := false
	var oldState, newState RiskState
	monitor.SetRiskStateChangeCallback(func(old, new RiskState) {
		mu.Lock()
		callbackTriggered = true
		oldState = old
		newState = new
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动监控
	monitor.Start(ctx)

	// 触发状态变化
	monitor.RecordTrade(-150.0) // 触发日内亏损限制

	// 等待监控循环检测到变化
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if !callbackTriggered {
		t.Error("expected risk state change callback to be triggered")
	}
	if oldState != RiskStateNormal {
		t.Errorf("expected old state NORMAL, got %s", oldState)
	}
	if newState != RiskStateEmergency {
		t.Errorf("expected new state EMERGENCY, got %s", newState)
	}
	mu.Unlock()

	monitor.Stop()
}

func TestMonitor_IsTrading(t *testing.T) {
	config := MonitorConfig{}
	monitor := NewMonitor(config)

	// 正常状态，允许交易
	if !monitor.IsTrading() {
		t.Error("expected trading to be allowed in normal state")
	}

	// 触发紧急状态
	monitor.TriggerEmergencyStop("test")
	if monitor.IsTrading() {
		t.Error("expected trading to be disabled in emergency state")
	}

	// 恢复
	monitor.ResumeTrading()
	if !monitor.IsTrading() {
		t.Error("expected trading to be allowed after resume")
	}
}

func TestMonitor_GetEquityAndPnL(t *testing.T) {
	config := MonitorConfig{
		InitialEquity: 10000.0,
	}
	monitor := NewMonitor(config)

	// 初始状态
	if equity := monitor.GetTotalEquity(); equity != 10000.0 {
		t.Errorf("expected initial equity 10000, got %f", equity)
	}
	if daily := monitor.GetDailyPnL(); daily != 0 {
		t.Errorf("expected initial daily PnL 0, got %f", daily)
	}
	if dd := monitor.GetDrawdown(); dd != 0 {
		t.Errorf("expected initial drawdown 0, got %f", dd)
	}

	// 交易后
	monitor.RecordTrade(100.0)
	monitor.UpdateUnrealizedPnL(50.0)

	if equity := monitor.GetTotalEquity(); equity != 10150.0 {
		t.Errorf("expected total equity 10150, got %f", equity)
	}
	if daily := monitor.GetDailyPnL(); daily != 100.0 {
		t.Errorf("expected daily PnL 100, got %f", daily)
	}
}

func TestMonitor_ForceCheckRisk(t *testing.T) {
	config := MonitorConfig{
		PnLLimits: PnLLimits{
			DailyLossLimit: 50.0,
		},
		InitialEquity: 10000.0,
	}
	monitor := NewMonitor(config)

	// 创建违规但不自动检查
	monitor.RecordTrade(-60.0)

	// 状态应该还是正常（因为没有运行监控循环）
	if state := monitor.GetRiskState(); state != RiskStateNormal {
		t.Errorf("expected NORMAL state before check, got %s", state)
	}

	// 强制检查
	monitor.ForceCheckRisk()

	// 现在应该检测到违规
	if state := monitor.GetRiskState(); state != RiskStateEmergency {
		t.Errorf("expected EMERGENCY state after force check, got %s", state)
	}
}

func TestMonitor_ConcurrentAccess(t *testing.T) {
	config := MonitorConfig{
		InitialEquity: 10000.0,
	}
	monitor := NewMonitor(config)

	done := make(chan bool)
	operations := 50

	// 并发记录交易
	go func() {
		for i := 0; i < operations; i++ {
			monitor.RecordTrade(1.0)
		}
		done <- true
	}()

	// 并发更新未实现盈亏
	go func() {
		for i := 0; i < operations; i++ {
			monitor.UpdateUnrealizedPnL(float64(i))
		}
		done <- true
	}()

	// 并发读取状态
	go func() {
		for i := 0; i < operations; i++ {
			_ = monitor.GetRiskState()
			_ = monitor.GetMonitorMetrics()
			_ = monitor.IsTrading()
		}
		done <- true
	}()

	// 并发风控检查
	go func() {
		for i := 0; i < operations; i++ {
			_ = monitor.CheckPreTrade(100.0)
			monitor.ForceCheckRisk()
		}
		done <- true
	}()

	// 等待完成
	for i := 0; i < 4; i++ {
		<-done
	}

	// 验证最终状态
	metrics := monitor.GetPnLMetrics()
	if metrics.RealizedPnL != float64(operations) {
		t.Errorf("expected realized PnL %f, got %f", float64(operations), metrics.RealizedPnL)
	}
}

func TestRiskState_String(t *testing.T) {
	tests := []struct {
		state    RiskState
		expected string
	}{
		{RiskStateNormal, "NORMAL"},
		{RiskStateWarning, "WARNING"},
		{RiskStateDanger, "DANGER"},
		{RiskStateEmergency, "EMERGENCY"},
		{RiskState(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("RiskState(%d).String() = %s, want %s", tt.state, got, tt.expected)
		}
	}
}

func TestMonitor_AutoDailyReset(t *testing.T) {
	config := MonitorConfig{
		MonitorInterval: 10 * time.Millisecond,
		InitialEquity:   10000.0,
	}
	monitor := NewMonitor(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动监控
	monitor.Start(ctx)

	// 记录一些交易
	monitor.RecordTrade(100.0)

	// 模拟跨天（修改lastResetTime）
	monitor.pnlMonitor.mu.Lock()
	monitor.pnlMonitor.lastResetTime = time.Now().AddDate(0, 0, -1)
	monitor.pnlMonitor.mu.Unlock()

	// 等待监控循环检测并重置
	time.Sleep(50 * time.Millisecond)

	// 日内PnL应该被重置
	if daily := monitor.GetDailyPnL(); daily != 0 {
		t.Errorf("expected daily PnL to be reset to 0, got %f", daily)
	}

	// 但已实现PnL不应该变
	metrics := monitor.GetPnLMetrics()
	if metrics.RealizedPnL != 100.0 {
		t.Errorf("expected realized PnL to remain 100, got %f", metrics.RealizedPnL)
	}

	monitor.Stop()
}
