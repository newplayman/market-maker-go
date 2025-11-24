package risk

import (
	"testing"
	"time"
)

func TestNewPnLMonitor(t *testing.T) {
	limits := PnLLimits{
		DailyLossLimit:   100.0,
		MaxDrawdownLimit: 0.03,
		MinPnLThreshold:  -50.0,
	}
	initialEquity := 10000.0

	monitor := NewPnLMonitor(limits, initialEquity)

	if monitor.initialEquity != initialEquity {
		t.Errorf("expected initial equity %f, got %f", initialEquity, monitor.initialEquity)
	}
	if monitor.peakEquity != initialEquity {
		t.Errorf("expected peak equity %f, got %f", initialEquity, monitor.peakEquity)
	}
	if monitor.todayStart != initialEquity {
		t.Errorf("expected today start %f, got %f", initialEquity, monitor.todayStart)
	}
}

func TestUpdateRealized(t *testing.T) {
	limits := PnLLimits{DailyLossLimit: 100.0}
	monitor := NewPnLMonitor(limits, 10000.0)

	// 盈利50
	monitor.UpdateRealized(50.0)
	metrics := monitor.GetMetrics()
	if metrics.RealizedPnL != 50.0 {
		t.Errorf("expected realized PnL 50.0, got %f", metrics.RealizedPnL)
	}
	if metrics.DailyPnL != 50.0 {
		t.Errorf("expected daily PnL 50.0, got %f", metrics.DailyPnL)
	}

	// 再盈利30
	monitor.UpdateRealized(30.0)
	metrics = monitor.GetMetrics()
	if metrics.RealizedPnL != 80.0 {
		t.Errorf("expected realized PnL 80.0, got %f", metrics.RealizedPnL)
	}
	if metrics.DailyPnL != 80.0 {
		t.Errorf("expected daily PnL 80.0, got %f", metrics.DailyPnL)
	}

	// 亏损20
	monitor.UpdateRealized(-20.0)
	metrics = monitor.GetMetrics()
	if metrics.RealizedPnL != 60.0 {
		t.Errorf("expected realized PnL 60.0, got %f", metrics.RealizedPnL)
	}
	if metrics.DailyPnL != 60.0 {
		t.Errorf("expected daily PnL 60.0, got %f", metrics.DailyPnL)
	}
}

func TestUpdateUnrealized(t *testing.T) {
	limits := PnLLimits{}
	monitor := NewPnLMonitor(limits, 10000.0)

	// 未实现盈亏100
	monitor.UpdateUnrealized(100.0)
	metrics := monitor.GetMetrics()
	if metrics.UnrealizedPnL != 100.0 {
		t.Errorf("expected unrealized PnL 100.0, got %f", metrics.UnrealizedPnL)
	}
	if metrics.TotalPnL != 100.0 {
		t.Errorf("expected total PnL 100.0, got %f", metrics.TotalPnL)
	}

	// 未实现盈亏变为-50
	monitor.UpdateUnrealized(-50.0)
	metrics = monitor.GetMetrics()
	if metrics.UnrealizedPnL != -50.0 {
		t.Errorf("expected unrealized PnL -50.0, got %f", metrics.UnrealizedPnL)
	}
}

func TestDrawdownCalculation(t *testing.T) {
	limits := PnLLimits{}
	monitor := NewPnLMonitor(limits, 10000.0)

	// 初始状态，无回撤
	if monitor.GetDrawdown() != 0 {
		t.Errorf("expected initial drawdown 0, got %f", monitor.GetDrawdown())
	}

	// 盈利200，权益峰值更新
	monitor.UpdateRealized(200.0)
	if monitor.peakEquity != 10200.0 {
		t.Errorf("expected peak equity 10200.0, got %f", monitor.peakEquity)
	}

	// 亏损150，产生回撤
	monitor.UpdateRealized(-150.0)
	// 当前权益: 10000 + 200 - 150 = 10050
	// 回撤: (10200 - 10050) / 10200 = 0.0147
	currentEquity := monitor.GetTotalEquity()
	if currentEquity != 10050.0 {
		t.Errorf("expected current equity 10050.0, got %f", currentEquity)
	}

	expectedDrawdown := (10200.0 - 10050.0) / 10200.0
	drawdown := monitor.GetDrawdown()
	if drawdown < expectedDrawdown-0.0001 || drawdown > expectedDrawdown+0.0001 {
		t.Errorf("expected drawdown ~%f, got %f", expectedDrawdown, drawdown)
	}

	// 继续亏损，回撤增大
	monitor.UpdateRealized(-100.0)
	// 当前权益: 9950
	// 回撤: (10200 - 9950) / 10200 = 0.0245
	newDrawdown := monitor.GetDrawdown()
	if newDrawdown <= drawdown {
		t.Errorf("expected drawdown to increase, got %f vs %f", newDrawdown, drawdown)
	}
}

func TestCheckLimits_DailyLoss(t *testing.T) {
	limits := PnLLimits{
		DailyLossLimit: 100.0,
	}
	monitor := NewPnLMonitor(limits, 10000.0)

	// 亏损50，未超限
	monitor.UpdateRealized(-50.0)
	if err := monitor.CheckLimits(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// 再亏损60，总共亏损110，超限
	monitor.UpdateRealized(-60.0)
	if err := monitor.CheckLimits(); err == nil {
		t.Error("expected daily loss limit error, got nil")
	}
}

func TestCheckLimits_MaxDrawdown(t *testing.T) {
	limits := PnLLimits{
		MaxDrawdownLimit: 0.05, // 5%回撤限制
	}
	monitor := NewPnLMonitor(limits, 10000.0)

	// 先盈利1000，权益峰值11000
	monitor.UpdateRealized(1000.0)

	// 亏损400，回撤 400/11000 = 3.6%，未超限
	monitor.UpdateRealized(-400.0)
	if err := monitor.CheckLimits(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// 再亏损200，总亏损600，回撤 600/11000 = 5.45%，超限
	monitor.UpdateRealized(-200.0)
	if err := monitor.CheckLimits(); err == nil {
		t.Error("expected max drawdown limit error, got nil")
	}
}

func TestShouldAlert(t *testing.T) {
	limits := PnLLimits{
		MinPnLThreshold: -50.0,
	}
	monitor := NewPnLMonitor(limits, 10000.0)

	// 盈利状态，不告警
	monitor.UpdateRealized(10.0)
	if monitor.ShouldAlert() {
		t.Error("should not alert when PnL is positive")
	}

	// 亏损30，未达阈值
	monitor.UpdateRealized(-40.0) // 总PnL = -30
	if monitor.ShouldAlert() {
		t.Error("should not alert when PnL above threshold")
	}

	// 再亏损30，总PnL = -60，超过阈值
	monitor.UpdateRealized(-30.0)
	if !monitor.ShouldAlert() {
		t.Error("should alert when PnL below threshold")
	}
}

func TestResetDaily(t *testing.T) {
	limits := PnLLimits{}
	monitor := NewPnLMonitor(limits, 10000.0)

	// 交易一天，实现盈亏100
	monitor.UpdateRealized(100.0)
	monitor.UpdateUnrealized(50.0)

	if monitor.GetDailyPnL() != 100.0 {
		t.Errorf("expected daily PnL 100.0, got %f", monitor.GetDailyPnL())
	}

	// 每日重置
	monitor.ResetDaily()

	// 日内盈亏应该清零
	if monitor.GetDailyPnL() != 0 {
		t.Errorf("expected daily PnL to reset to 0, got %f", monitor.GetDailyPnL())
	}

	// 但总盈亏不变
	if monitor.realizedPnL != 100.0 {
		t.Errorf("expected realized PnL to remain 100.0, got %f", monitor.realizedPnL)
	}
}

func TestReset(t *testing.T) {
	limits := PnLLimits{}
	monitor := NewPnLMonitor(limits, 10000.0)

	// 交易后有盈亏
	monitor.UpdateRealized(200.0)
	monitor.UpdateUnrealized(50.0)
	monitor.UpdateRealized(-50.0) // 创建一些回撤

	// 完全重置
	newEquity := 15000.0
	monitor.Reset(newEquity)

	metrics := monitor.GetMetrics()
	if metrics.RealizedPnL != 0 {
		t.Errorf("expected realized PnL to reset to 0, got %f", metrics.RealizedPnL)
	}
	if metrics.UnrealizedPnL != 0 {
		t.Errorf("expected unrealized PnL to reset to 0, got %f", metrics.UnrealizedPnL)
	}
	if metrics.MaxDrawdown != 0 {
		t.Errorf("expected max drawdown to reset to 0, got %f", metrics.MaxDrawdown)
	}
	if metrics.DailyPnL != 0 {
		t.Errorf("expected daily PnL to reset to 0, got %f", metrics.DailyPnL)
	}
	if monitor.initialEquity != newEquity {
		t.Errorf("expected initial equity %f, got %f", newEquity, monitor.initialEquity)
	}
	if monitor.peakEquity != newEquity {
		t.Errorf("expected peak equity %f, got %f", newEquity, monitor.peakEquity)
	}
}

func TestGetTotalEquity(t *testing.T) {
	limits := PnLLimits{}
	monitor := NewPnLMonitor(limits, 10000.0)

	// 初始权益
	if equity := monitor.GetTotalEquity(); equity != 10000.0 {
		t.Errorf("expected total equity 10000.0, got %f", equity)
	}

	// 有盈亏后
	monitor.UpdateRealized(150.0)
	monitor.UpdateUnrealized(50.0)

	expectedEquity := 10000.0 + 150.0 + 50.0
	if equity := monitor.GetTotalEquity(); equity != expectedEquity {
		t.Errorf("expected total equity %f, got %f", expectedEquity, equity)
	}
}

func TestConcurrentAccess(t *testing.T) {
	limits := PnLLimits{
		DailyLossLimit:   1000.0,
		MaxDrawdownLimit: 0.1,
	}
	monitor := NewPnLMonitor(limits, 10000.0)

	// 并发读写测试
	done := make(chan bool)
	updates := 100

	// 并发更新已实现盈亏
	go func() {
		for i := 0; i < updates; i++ {
			monitor.UpdateRealized(1.0)
		}
		done <- true
	}()

	// 并发更新未实现盈亏
	go func() {
		for i := 0; i < updates; i++ {
			monitor.UpdateUnrealized(float64(i))
		}
		done <- true
	}()

	// 并发读取
	go func() {
		for i := 0; i < updates; i++ {
			_ = monitor.GetMetrics()
			_ = monitor.CheckLimits()
			_ = monitor.GetTotalEquity()
		}
		done <- true
	}()

	// 等待完成
	for i := 0; i < 3; i++ {
		<-done
	}

	// 验证最终状态
	metrics := monitor.GetMetrics()
	if metrics.RealizedPnL != float64(updates) {
		t.Errorf("expected realized PnL %f, got %f", float64(updates), metrics.RealizedPnL)
	}
}

func TestShouldCheckDailyReset(t *testing.T) {
	limits := PnLLimits{}
	monitor := NewPnLMonitor(limits, 10000.0)

	// 刚创建，不需要重置
	if monitor.ShouldCheckDailyReset() {
		t.Error("should not need reset immediately after creation")
	}

	// 模拟跨天（修改lastResetTime）
	yesterday := time.Now().AddDate(0, 0, -1)
	monitor.mu.Lock()
	monitor.lastResetTime = yesterday
	monitor.mu.Unlock()

	// 现在应该需要重置
	if !monitor.ShouldCheckDailyReset() {
		t.Error("should need reset after day change")
	}
}

func TestPnLMetrics(t *testing.T) {
	limits := PnLLimits{}
	monitor := NewPnLMonitor(limits, 10000.0)

	// 设置一些数据
	monitor.UpdateRealized(100.0)
	monitor.UpdateUnrealized(50.0)

	metrics := monitor.GetMetrics()

	// 验证所有字段
	if metrics.RealizedPnL != 100.0 {
		t.Errorf("expected realized PnL 100.0, got %f", metrics.RealizedPnL)
	}
	if metrics.UnrealizedPnL != 50.0 {
		t.Errorf("expected unrealized PnL 50.0, got %f", metrics.UnrealizedPnL)
	}
	if metrics.TotalPnL != 150.0 {
		t.Errorf("expected total PnL 150.0, got %f", metrics.TotalPnL)
	}
	if metrics.PeakEquity != 10150.0 {
		t.Errorf("expected peak equity 10150.0, got %f", metrics.PeakEquity)
	}
	if metrics.DailyPnL != 100.0 {
		t.Errorf("expected daily PnL 100.0, got %f", metrics.DailyPnL)
	}
	if metrics.LastUpdate.IsZero() {
		t.Error("expected LastUpdate to be set")
	}
}
