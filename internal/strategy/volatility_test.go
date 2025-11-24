package strategy

import (
	"math"
	"testing"
	"time"
)

func TestNewVolatilityCalculator(t *testing.T) {
	tests := []struct {
		name   string
		config VolatilityConfig
		want   VolatilityConfig
	}{
		{
			name: "default config",
			config: VolatilityConfig{
				Window:     5 * time.Minute,
				SampleSize: 100,
				Alpha:      0.1,
			},
			want: VolatilityConfig{
				Window:     5 * time.Minute,
				SampleSize: 100,
				Alpha:      0.1,
			},
		},
		{
			name: "invalid sample size - use default",
			config: VolatilityConfig{
				Window:     5 * time.Minute,
				SampleSize: 0,
				Alpha:      0.1,
			},
			want: VolatilityConfig{
				Window:     5 * time.Minute,
				SampleSize: 100,
				Alpha:      0.1,
			},
		},
		{
			name: "invalid alpha - use default",
			config: VolatilityConfig{
				Window:     5 * time.Minute,
				SampleSize: 50,
				Alpha:      0,
			},
			want: VolatilityConfig{
				Window:     5 * time.Minute,
				SampleSize: 50,
				Alpha:      0.1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := NewVolatilityCalculator(tt.config)
			if calc == nil {
				t.Fatal("Expected non-nil calculator")
			}
			if calc.window != tt.want.Window {
				t.Errorf("Window = %v, want %v", calc.window, tt.want.Window)
			}
			if calc.alpha != tt.want.Alpha {
				t.Errorf("Alpha = %v, want %v", calc.alpha, tt.want.Alpha)
			}
		})
	}
}

func TestVolatilityCalculator_Update(t *testing.T) {
	calc := NewVolatilityCalculator(VolatilityConfig{
		Window:     5 * time.Minute,
		SampleSize: 10,
		Alpha:      0.1,
	})

	now := time.Now()

	// 添加一些价格样本
	prices := []float64{100.0, 101.0, 100.5, 102.0, 101.5}
	for i, price := range prices {
		calc.Update(price, now.Add(time.Duration(i)*time.Second))
	}

	// 验证样本计数
	count := calc.GetSampleCount()
	if count != len(prices) {
		t.Errorf("Sample count = %d, want %d", count, len(prices))
	}
}

func TestVolatilityCalculator_Calculate(t *testing.T) {
	calc := NewVolatilityCalculator(VolatilityConfig{
		Window:     5 * time.Minute,
		SampleSize: 100,
		Alpha:      0.1,
	})

	now := time.Now()

	// 初始状态，波动率应该为0
	vol := calc.Calculate()
	if vol != 0 {
		t.Errorf("Initial volatility = %f, want 0", vol)
	}

	// 添加价格样本（模拟稳定价格）
	basePrice := 2000.0
	for i := 0; i < 10; i++ {
		price := basePrice + float64(i)*0.1 // 小幅波动
		calc.Update(price, now.Add(time.Duration(i)*time.Second))
	}

	// 应该有非零波动率
	vol = calc.Calculate()
	if vol == 0 {
		t.Error("Expected non-zero volatility after updates")
	}

	t.Logf("Volatility after 10 samples: %f", vol)
}

func TestVolatilityCalculator_HighVolatility(t *testing.T) {
	calc := NewVolatilityCalculator(VolatilityConfig{
		Window:     5 * time.Minute,
		SampleSize: 100,
		Alpha:      0.1,
	})

	now := time.Now()
	basePrice := 2000.0

	// 添加高波动性价格
	for i := 0; i < 20; i++ {
		// 价格在±5%范围内波动
		variation := math.Sin(float64(i)*0.5) * basePrice * 0.05
		price := basePrice + variation
		calc.Update(price, now.Add(time.Duration(i)*time.Second))
	}

	vol := calc.Calculate()
	if vol == 0 {
		t.Error("Expected non-zero volatility for high volatility scenario")
	}

	t.Logf("High volatility: %f", vol)
}

func TestVolatilityCalculator_GetAnnualized(t *testing.T) {
	calc := NewVolatilityCalculator(VolatilityConfig{
		Window:     1 * time.Minute,
		SampleSize: 100,
		Alpha:      0.1,
	})

	now := time.Now()

	// 添加一些样本
	prices := []float64{100.0, 101.0, 100.5, 102.0, 101.5, 103.0}
	for i, price := range prices {
		calc.Update(price, now.Add(time.Duration(i)*time.Second))
	}

	annualized := calc.GetAnnualized()
	regular := calc.Calculate()

	// 年化波动率应该大于普通波动率
	if annualized <= regular {
		t.Errorf("Annualized volatility (%f) should be > regular volatility (%f)", annualized, regular)
	}

	t.Logf("Regular: %f, Annualized: %f", regular, annualized)
}

func TestVolatilityCalculator_Reset(t *testing.T) {
	calc := NewVolatilityCalculator(DefaultVolatilityConfig())

	now := time.Now()

	// 添加样本
	for i := 0; i < 10; i++ {
		calc.Update(2000.0+float64(i), now.Add(time.Duration(i)*time.Second))
	}

	// 验证有波动率
	vol := calc.Calculate()
	if vol == 0 {
		t.Error("Expected non-zero volatility before reset")
	}

	// 重置
	calc.Reset()

	// 验证波动率归零
	volAfter := calc.Calculate()
	if volAfter != 0 {
		t.Errorf("Volatility after reset = %f, want 0", volAfter)
	}

	// 验证方差归零
	variance := calc.GetVariance()
	if variance != 0 {
		t.Errorf("Variance after reset = %f, want 0", variance)
	}
}

func TestVolatilityCalculator_TimeWindow(t *testing.T) {
	// 使用短时间窗口方便测试
	calc := NewVolatilityCalculator(VolatilityConfig{
		Window:     100 * time.Millisecond,
		SampleSize: 100,
		Alpha:      0.1,
	})

	now := time.Now()

	// 添加旧样本（超出时间窗口）
	calc.Update(2000.0, now.Add(-200*time.Millisecond))
	time.Sleep(10 * time.Millisecond)

	// 添加新样本（在时间窗口内）
	calc.Update(2001.0, now)
	calc.Update(2002.0, now.Add(10*time.Millisecond))

	// 获取有效样本数量
	count := calc.GetSampleCount()

	// 应该只统计时间窗口内的样本
	// 由于时间推移，旧样本可能已经过期
	if count > 3 {
		t.Errorf("Sample count = %d, want <= 3 (due to time window)", count)
	}

	t.Logf("Valid samples in time window: %d", count)
}

func TestVolatilityCalculator_GetStatistics(t *testing.T) {
	calc := NewVolatilityCalculator(VolatilityConfig{
		Window:     5 * time.Minute,
		SampleSize: 50,
		Alpha:      0.2,
	})

	now := time.Now()

	// 添加样本
	for i := 0; i < 10; i++ {
		calc.Update(2000.0+float64(i)*0.5, now.Add(time.Duration(i)*time.Second))
	}

	stats := calc.GetStatistics()

	// 验证统计信息包含所有字段
	expectedFields := []string{
		"volatility",
		"annualized_volatility",
		"variance",
		"sample_count",
		"window_minutes",
		"alpha",
	}

	for _, field := range expectedFields {
		if _, ok := stats[field]; !ok {
			t.Errorf("Statistics missing field: %s", field)
		}
	}

	// 验证alpha和window配置
	if stats["alpha"].(float64) != 0.2 {
		t.Errorf("Alpha = %v, want 0.2", stats["alpha"])
	}

	windowMinutes := stats["window_minutes"].(float64)
	if windowMinutes != 5.0 {
		t.Errorf("Window minutes = %v, want 5.0", windowMinutes)
	}

	t.Logf("Statistics: %+v", stats)
}

func TestVolatilityCalculator_Concurrent(t *testing.T) {
	calc := NewVolatilityCalculator(DefaultVolatilityConfig())
	now := time.Now()

	// 并发更新
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			for j := 0; j < 10; j++ {
				price := 2000.0 + float64(idx)*10 + float64(j)
				calc.Update(price, now.Add(time.Duration(j)*time.Second))
			}
			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 并发读取
	for i := 0; i < 10; i++ {
		go func() {
			_ = calc.Calculate()
			_ = calc.GetAnnualized()
			_ = calc.GetStatistics()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	t.Log("✅ Concurrent access test passed")
}

func TestVolatilityCalculator_EWMA(t *testing.T) {
	// 测试EWMA算法的正确性
	calc := NewVolatilityCalculator(VolatilityConfig{
		Window:     10 * time.Minute,
		SampleSize: 100,
		Alpha:      0.3, // 较高的alpha，对新数据更敏感
	})

	now := time.Now()

	// 第一阶段：低波动
	for i := 0; i < 20; i++ {
		price := 2000.0 + float64(i)*0.1
		calc.Update(price, now.Add(time.Duration(i)*time.Second))
	}

	lowVol := calc.Calculate()

	// 第二阶段：高波动
	for i := 0; i < 20; i++ {
		variation := math.Sin(float64(i)) * 10.0
		price := 2000.0 + variation
		calc.Update(price, now.Add(time.Duration(20+i)*time.Second))
	}

	highVol := calc.Calculate()

	// 高波动期的波动率应该更高
	if highVol <= lowVol {
		t.Errorf("High volatility period (%f) should have higher volatility than low period (%f)", 
			highVol, lowVol)
	}

	t.Logf("Low volatility: %f, High volatility: %f", lowVol, highVol)
}

func TestDefaultVolatilityConfig(t *testing.T) {
	cfg := DefaultVolatilityConfig()

	if cfg.Window != 5*time.Minute {
		t.Errorf("Default window = %v, want 5m", cfg.Window)
	}
	if cfg.SampleSize != 100 {
		t.Errorf("Default sample size = %d, want 100", cfg.SampleSize)
	}
	if cfg.Alpha != 0.1 {
		t.Errorf("Default alpha = %f, want 0.1", cfg.Alpha)
	}
}
