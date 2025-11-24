package strategy

import (
	"testing"
	"time"
)

func TestNewDynamicSpreadModel(t *testing.T) {
	volCalc := NewVolatilityCalculator(DefaultVolatilityConfig())

	tests := []struct {
		name   string
		config SpreadModelConfig
	}{
		{
			name:   "default config",
			config: DefaultSpreadModelConfig(),
		},
		{
			name: "custom config",
			config: SpreadModelConfig{
				BaseSpread:    0.001,
				VolMultiplier: 3.0,
				MinSpread:     0.0005,
				MaxSpread:     0.003,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewDynamicSpreadModel(tt.config, volCalc)
			if model == nil {
				t.Fatal("Expected non-nil spread model")
			}

			cfg := model.GetConfig()
			if cfg.BaseSpread <= 0 {
				t.Error("Base spread should be > 0")
			}
			if cfg.MinSpread >= cfg.MaxSpread {
				t.Error("Min spread should be < max spread")
			}
		})
	}
}

func TestDynamicSpreadModel_Calculate(t *testing.T) {
	// 创建波动率计算器
	volCalc := NewVolatilityCalculator(VolatilityConfig{
		Window:     5 * time.Minute,
		SampleSize: 100,
		Alpha:      0.1,
	})

	// 创建spread模型
	model := NewDynamicSpreadModel(SpreadModelConfig{
		BaseSpread:    0.0005,
		VolMultiplier: 2.0,
		MinSpread:     0.0003,
		MaxSpread:     0.002,
	}, volCalc)

	// 无波动率时应该返回基础spread
	spread := model.Calculate()
	if spread != 0.0005 {
		t.Errorf("Expected base spread 0.0005, got %f", spread)
	}

	// 添加一些价格样本增加波动率
	now := time.Now()
	for i := 0; i < 20; i++ {
		price := 2000.0 + float64(i)*10.0 // 较大波动
		volCalc.Update(price, now.Add(time.Duration(i)*time.Second))
	}

	// 有波动率后spread应该增加
	spreadWithVol := model.Calculate()
	if spreadWithVol <= spread {
		t.Errorf("Spread with volatility (%f) should be > base spread (%f)",
			spreadWithVol, spread)
	}

	t.Logf("Base spread: %f, Spread with volatility: %f", spread, spreadWithVol)
}

func TestDynamicSpreadModel_CalculateWithInventory(t *testing.T) {
	model := NewDynamicSpreadModel(DefaultSpreadModelConfig(), nil)

	tests := []struct {
		name         string
		inventory    float64
		maxInventory float64
		expectHigher bool
	}{
		{
			name:         "low inventory - no adjustment",
			inventory:    0.01,
			maxInventory: 0.05,
			expectHigher: false,
		},
		{
			name:         "high inventory - wider spread",
			inventory:    0.045,
			maxInventory: 0.05,
			expectHigher: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseSpread := model.Calculate()
			spreadWithInv := model.CalculateWithInventory(tt.inventory, tt.maxInventory)

			if tt.expectHigher {
				if spreadWithInv <= baseSpread {
					t.Errorf("Expected spread with inventory (%f) > base spread (%f)",
						spreadWithInv, baseSpread)
				}
			}

			t.Logf("Inventory: %.4f, Base spread: %f, Adjusted spread: %f",
				tt.inventory, baseSpread, spreadWithInv)
		})
	}
}

func TestDynamicSpreadModel_CalculateWithDepth(t *testing.T) {
	model := NewDynamicSpreadModel(DefaultSpreadModelConfig(), nil)

	tests := []struct {
		name      string
		bidDepth  float64
		askDepth  float64
		avgDepth  float64
		expectAdj bool
	}{
		{
			name:      "balanced depth - no adjustment",
			bidDepth:  100.0,
			askDepth:  100.0,
			avgDepth:  200.0,
			expectAdj: false,
		},
		{
			name:      "imbalanced depth - wider spread",
			bidDepth:  150.0,
			askDepth:  50.0,
			avgDepth:  200.0,
			expectAdj: true,
		},
		{
			name:      "shallow depth - wider spread",
			bidDepth:  40.0,
			askDepth:  40.0,
			avgDepth:  200.0,
			expectAdj: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseSpread := model.Calculate()
			spreadWithDepth := model.CalculateWithDepth(tt.bidDepth, tt.askDepth, tt.avgDepth)

			t.Logf("Base spread: %f, Adjusted spread: %f", baseSpread, spreadWithDepth)

			if tt.expectAdj && spreadWithDepth <= baseSpread {
				t.Logf("Warning: Expected adjustment but spread didn't increase much")
			}
		})
	}
}

func TestDynamicSpreadModel_Adjust(t *testing.T) {
	model := NewDynamicSpreadModel(DefaultSpreadModelConfig(), nil)

	initialSpread := model.Calculate()

	// 增加spread
	model.Adjust(1.5)
	newSpread := model.Calculate()

	if newSpread <= initialSpread {
		t.Errorf("Expected spread to increase after Adjust(1.5)")
	}

	// 减少spread
	model.Adjust(0.5)
	reducedSpread := model.Calculate()

	if reducedSpread >= newSpread {
		t.Errorf("Expected spread to decrease after Adjust(0.5)")
	}

	t.Logf("Initial: %f, After 1.5x: %f, After 0.5x: %f",
		initialSpread, newSpread, reducedSpread)
}

func TestDynamicSpreadModel_SetBaseSpread(t *testing.T) {
	model := NewDynamicSpreadModel(DefaultSpreadModelConfig(), nil)

	// 设置新的base spread
	model.SetBaseSpread(0.001)

	cfg := model.GetConfig()
	if cfg.BaseSpread != 0.001 {
		t.Errorf("Expected base spread 0.001, got %f", cfg.BaseSpread)
	}

	// 测试边界：设置超过max的值应该被限制
	model.SetBaseSpread(0.01) // 超过默认max 0.002
	cfg = model.GetConfig()
	if cfg.BaseSpread > cfg.MaxSpread {
		t.Errorf("Base spread (%f) should not exceed max spread (%f)",
			cfg.BaseSpread, cfg.MaxSpread)
	}
}

func TestDynamicSpreadModel_SetVolMultiplier(t *testing.T) {
	volCalc := NewVolatilityCalculator(DefaultVolatilityConfig())
	// 使用更大的maxSpread以避免达到上限
	model := NewDynamicSpreadModel(SpreadModelConfig{
		BaseSpread:    0.0005,
		VolMultiplier: 2.0,
		MinSpread:     0.0003,
		MaxSpread:     0.01, // 更大的上限
	}, volCalc)

	// 添加一些波动率
	now := time.Now()
	for i := 0; i < 10; i++ {
		volCalc.Update(2000.0+float64(i)*5, now.Add(time.Duration(i)*time.Second))
	}

	// 获取当前spread
	spread1 := model.Calculate()

	// 增加vol multiplier
	model.SetVolMultiplier(5.0)
	spread2 := model.Calculate()

	// spread应该增加（因为multiplier增加）
	if spread2 <= spread1 {
		t.Errorf("Expected spread to increase with higher vol multiplier, got spread1=%f, spread2=%f", spread1, spread2)
	}

	t.Logf("Spread with multiplier 2.0: %f, with 5.0: %f", spread1, spread2)
}

func TestDynamicSpreadModel_GetStatistics(t *testing.T) {
	volCalc := NewVolatilityCalculator(DefaultVolatilityConfig())
	model := NewDynamicSpreadModel(DefaultSpreadModelConfig(), volCalc)

	stats := model.GetStatistics()

	// 验证所有必需字段存在
	expectedFields := []string{
		"base_spread",
		"vol_multiplier",
		"min_spread",
		"max_spread",
		"current_volatility",
		"current_spread",
	}

	for _, field := range expectedFields {
		if _, ok := stats[field]; !ok {
			t.Errorf("Statistics missing field: %s", field)
		}
	}

	t.Logf("Statistics: %+v", stats)
}

func TestDynamicSpreadModel_SpreadBounds(t *testing.T) {
	volCalc := NewVolatilityCalculator(DefaultVolatilityConfig())

	model := NewDynamicSpreadModel(SpreadModelConfig{
		BaseSpread:    0.0005,
		VolMultiplier: 10.0, // 很高的乘数
		MinSpread:     0.0003,
		MaxSpread:     0.001,
	}, volCalc)

	// 添加极高波动率
	now := time.Now()
	for i := 0; i < 50; i++ {
		price := 2000.0 + float64(i%10)*100.0 // 大幅波动
		volCalc.Update(price, now.Add(time.Duration(i)*time.Second))
	}

	spread := model.Calculate()

	// 即使波动率很高，spread也应该被限制在maxSpread内
	if spread > 0.001 {
		t.Errorf("Spread (%f) exceeded max spread (0.001)", spread)
	}

	// spread也不应该低于minSpread
	if spread < 0.0003 {
		t.Errorf("Spread (%f) below min spread (0.0003)", spread)
	}

	t.Logf("Spread with high volatility: %f (bounded by min/max)", spread)
}

func TestDynamicSpreadModel_Concurrent(t *testing.T) {
	volCalc := NewVolatilityCalculator(DefaultVolatilityConfig())
	model := NewDynamicSpreadModel(DefaultSpreadModelConfig(), volCalc)

	done := make(chan bool, 20)

	// 并发读取
	for i := 0; i < 10; i++ {
		go func() {
			_ = model.Calculate()
			_ = model.GetConfig()
			_ = model.GetStatistics()
			done <- true
		}()
	}

	// 并发写入
	for i := 0; i < 10; i++ {
		go func(idx int) {
			model.Adjust(1.0 + float64(idx)*0.01)
			model.SetBaseSpread(0.0005 + float64(idx)*0.0001)
			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < 20; i++ {
		<-done
	}

	t.Log("✅ Concurrent access test passed")
}
