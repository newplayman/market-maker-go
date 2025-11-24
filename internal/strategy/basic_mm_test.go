package strategy

import (
	"math"
	"testing"
)

func TestNewBasicMarketMaking(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   Config
	}{
		{
			name: "valid config",
			config: Config{
				BaseSpread:   0.001,
				BaseSize:     0.1,
				MaxInventory: 1.0,
				SkewFactor:   0.5,
			},
			want: Config{
				BaseSpread:   0.001,
				BaseSize:     0.1,
				MaxInventory: 1.0,
				SkewFactor:   0.5,
				MinSpread:    0.0005,
				MaxSpread:    0.002,
			},
		},
		{
			name: "zero config uses defaults",
			config: Config{
				BaseSpread:   0,
				BaseSize:     0,
				MaxInventory: 0,
			},
			want: Config{
				BaseSpread:   0.0005,
				BaseSize:     0.01,
				MaxInventory: 0.05,
				SkewFactor:   0.3,
				MinSpread:    0.00025,
				MaxSpread:    0.001,
			},
		},
		{
			name: "invalid skew factor should default",
			config: Config{
				BaseSpread:   0.001,
				BaseSize:     0.1,
				MaxInventory: 1.0,
				SkewFactor:   1.5, // invalid (>1)
			},
			want: Config{
				BaseSpread:   0.001,
				BaseSize:     0.1,
				MaxInventory: 1.0,
				SkewFactor:   0.3, // should use default
				MinSpread:    0.0005,
				MaxSpread:    0.002,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewBasicMarketMaking(tt.config)
			got := s.GetConfig()

			if got.BaseSpread != tt.want.BaseSpread {
				t.Errorf("BaseSpread = %v, want %v", got.BaseSpread, tt.want.BaseSpread)
			}
			if got.BaseSize != tt.want.BaseSize {
				t.Errorf("BaseSize = %v, want %v", got.BaseSize, tt.want.BaseSize)
			}
			if got.MaxInventory != tt.want.MaxInventory {
				t.Errorf("MaxInventory = %v, want %v", got.MaxInventory, tt.want.MaxInventory)
			}
			if got.SkewFactor != tt.want.SkewFactor {
				t.Errorf("SkewFactor = %v, want %v", got.SkewFactor, tt.want.SkewFactor)
			}
		})
	}
}

func TestGenerateQuotes(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		ctx     Context
		wantErr bool
		check   func(t *testing.T, quotes []Quote)
	}{
		{
			name: "zero inventory symmetric quotes",
			config: Config{
				BaseSpread:   0.001, // 0.1%
				BaseSize:     0.1,
				MaxInventory: 1.0,
				SkewFactor:   0.3,
			},
			ctx: Context{
				Symbol:       "ETHUSDC",
				Mid:          2000.0,
				Inventory:    0.0, // 零库存
				MaxInventory: 1.0,
			},
			wantErr: false,
			check: func(t *testing.T, quotes []Quote) {
				if len(quotes) != 2 {
					t.Fatalf("expected 2 quotes, got %d", len(quotes))
				}

				buy := quotes[0]
				sell := quotes[1]

				// 零库存时应该对称
				expectedMid := 2000.0
				expectedHalfSpread := 0.001 * 2000.0 / 2.0 // 1.0

				if buy.Side != "BUY" {
					t.Errorf("first quote should be BUY, got %s", buy.Side)
				}
				if sell.Side != "SELL" {
					t.Errorf("second quote should be SELL, got %s", sell.Side)
				}

				// 检查价格在合理范围内（允许一定误差因为有roundPrice）
				if math.Abs(buy.Price-(expectedMid-expectedHalfSpread)) > 0.1 {
					t.Errorf("buy price = %v, expected around %v", buy.Price, expectedMid-expectedHalfSpread)
				}
				if math.Abs(sell.Price-(expectedMid+expectedHalfSpread)) > 0.1 {
					t.Errorf("sell price = %v, expected around %v", sell.Price, expectedMid+expectedHalfSpread)
				}

				// 检查数量
				if buy.Size != 0.1 {
					t.Errorf("buy size = %v, want 0.1", buy.Size)
				}
				if sell.Size != 0.1 {
					t.Errorf("sell size = %v, want 0.1", sell.Size)
				}
			},
		},
		{
			name: "positive inventory skews quotes down",
			config: Config{
				BaseSpread:   0.001,
				BaseSize:     0.1,
				MaxInventory: 1.0,
				SkewFactor:   0.5,
			},
			ctx: Context{
				Symbol:       "ETHUSDC",
				Mid:          2000.0,
				Inventory:    0.5, // 50% 多头
				MaxInventory: 1.0,
			},
			wantErr: false,
			check: func(t *testing.T, quotes []Quote) {
				if len(quotes) != 2 {
					t.Fatalf("expected 2 quotes, got %d", len(quotes))
				}

				buy := quotes[0]
				sell := quotes[1]

				// 持有多头时，应该降低价格促进卖出
				// skew = 0.5 * 0.5 * (0.001 * 2000) = 0.5
				// buy = 2000 - 1 - 0.5 = 1998.5
				// sell = 2000 + 1 - 0.5 = 2000.5

				// 买价应该更低
				if buy.Price >= 1999.0 {
					t.Errorf("buy price should be lower due to positive inventory, got %v", buy.Price)
				}
				// 卖价应该也更低
				if sell.Price >= 2001.0 {
					t.Errorf("sell price should be lower due to positive inventory, got %v", sell.Price)
				}
				// spread仍然应该存在
				if sell.Price <= buy.Price {
					t.Errorf("sell price (%v) should be higher than buy price (%v)", sell.Price, buy.Price)
				}
			},
		},
		{
			name: "negative inventory skews quotes up",
			config: Config{
				BaseSpread:   0.001,
				BaseSize:     0.1,
				MaxInventory: 1.0,
				SkewFactor:   0.5,
			},
			ctx: Context{
				Symbol:       "ETHUSDC",
				Mid:          2000.0,
				Inventory:    -0.5, // 50% 空头
				MaxInventory: 1.0,
			},
			wantErr: false,
			check: func(t *testing.T, quotes []Quote) {
				if len(quotes) != 2 {
					t.Fatalf("expected 2 quotes, got %d", len(quotes))
				}

				buy := quotes[0]
				sell := quotes[1]

				// 持有空头时，应该提高价格促进买入
				// skew = -0.5 * 0.5 * (0.001 * 2000) = -0.5
				// buy = 2000 - 1 - (-0.5) = 1999.5
				// sell = 2000 + 1 - (-0.5) = 2001.5

				// 买价应该更高
				if buy.Price <= 1999.0 {
					t.Errorf("buy price should be higher due to negative inventory, got %v", buy.Price)
				}
				// 卖价应该也更高
				if sell.Price <= 2001.0 {
					t.Errorf("sell price should be higher due to negative inventory, got %v", sell.Price)
				}
			},
		},
		{
			name: "high inventory reduces buy size",
			config: Config{
				BaseSpread:   0.001,
				BaseSize:     0.1,
				MaxInventory: 1.0,
				SkewFactor:   0.3,
			},
			ctx: Context{
				Symbol:       "ETHUSDC",
				Mid:          2000.0,
				Inventory:    0.9, // 90% 满仓
				MaxInventory: 1.0,
			},
			wantErr: false,
			check: func(t *testing.T, quotes []Quote) {
				if len(quotes) != 2 {
					t.Fatalf("expected 2 quotes, got %d", len(quotes))
				}

				buy := quotes[0]
				sell := quotes[1]

				// 高库存时，买单数量应该减少
				if buy.Size >= 0.1 {
					t.Errorf("buy size should be reduced, got %v", buy.Size)
				}
				// 卖单数量应该保持
				if sell.Size != 0.1 {
					t.Errorf("sell size should remain 0.1, got %v", sell.Size)
				}
			},
		},
		{
			name: "invalid mid price",
			config: Config{
				BaseSpread:   0.001,
				BaseSize:     0.1,
				MaxInventory: 1.0,
			},
			ctx: Context{
				Symbol:    "ETHUSDC",
				Mid:       0.0, // invalid
				Inventory: 0.0,
			},
			wantErr: true,
		},
		{
			name: "negative mid price",
			config: Config{
				BaseSpread:   0.001,
				BaseSize:     0.1,
				MaxInventory: 1.0,
			},
			ctx: Context{
				Symbol:    "ETHUSDC",
				Mid:       -100.0, // invalid
				Inventory: 0.0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewBasicMarketMaking(tt.config)
			quotes, err := s.GenerateQuotes(tt.ctx)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.check != nil {
				tt.check(t, quotes)
			}
		})
	}
}

func TestOnFill(t *testing.T) {
	s := NewBasicMarketMaking(Config{
		BaseSpread:   0.001,
		BaseSize:     0.1,
		MaxInventory: 1.0,
	})

	// 初始状态
	stats := s.GetStatistics()
	if stats["total_buy_fills"].(int) != 0 {
		t.Errorf("initial buy fills should be 0")
	}

	// 记录买入成交
	s.OnFill(Fill{Side: "BUY", Price: 2000.0, Size: 0.1})
	stats = s.GetStatistics()
	if stats["total_buy_fills"].(int) != 1 {
		t.Errorf("buy fills = %v, want 1", stats["total_buy_fills"])
	}
	if stats["total_volume"].(float64) != 0.1 {
		t.Errorf("total volume = %v, want 0.1", stats["total_volume"])
	}

	// 记录卖出成交
	s.OnFill(Fill{Side: "SELL", Price: 2001.0, Size: 0.2})
	stats = s.GetStatistics()
	if stats["total_sell_fills"].(int) != 1 {
		t.Errorf("sell fills = %v, want 1", stats["total_sell_fills"])
	}
	if math.Abs(stats["total_volume"].(float64)-0.3) > 0.0001 {
		t.Errorf("total volume = %v, want 0.3", stats["total_volume"])
	}

	// 多次成交
	s.OnFill(Fill{Side: "BUY", Price: 2000.0, Size: 0.1})
	s.OnFill(Fill{Side: "BUY", Price: 2000.0, Size: 0.1})
	stats = s.GetStatistics()
	if stats["total_buy_fills"].(int) != 3 {
		t.Errorf("buy fills = %v, want 3", stats["total_buy_fills"])
	}
}

func TestUpdateParameters(t *testing.T) {
	s := NewBasicMarketMaking(Config{
		BaseSpread:   0.001,
		BaseSize:     0.1,
		MaxInventory: 1.0,
		SkewFactor:   0.3,
	})

	// 更新参数
	err := s.UpdateParameters(map[string]interface{}{
		"base_spread":   0.002,
		"base_size":     0.2,
		"max_inventory": 2.0,
		"skew_factor":   0.5,
	})
	if err != nil {
		t.Fatalf("update parameters failed: %v", err)
	}

	cfg := s.GetConfig()
	if cfg.BaseSpread != 0.002 {
		t.Errorf("BaseSpread = %v, want 0.002", cfg.BaseSpread)
	}
	if cfg.BaseSize != 0.2 {
		t.Errorf("BaseSize = %v, want 0.2", cfg.BaseSize)
	}
	if cfg.MaxInventory != 2.0 {
		t.Errorf("MaxInventory = %v, want 2.0", cfg.MaxInventory)
	}
	if cfg.SkewFactor != 0.5 {
		t.Errorf("SkewFactor = %v, want 0.5", cfg.SkewFactor)
	}

	// 无效参数应该被忽略
	err = s.UpdateParameters(map[string]interface{}{
		"base_spread": -0.001, // invalid
		"skew_factor": 2.0,    // invalid
	})
	if err != nil {
		t.Fatalf("update parameters failed: %v", err)
	}

	cfg = s.GetConfig()
	// 参数应该保持不变
	if cfg.BaseSpread != 0.002 {
		t.Errorf("BaseSpread should not change with invalid value")
	}
	if cfg.SkewFactor != 0.5 {
		t.Errorf("SkewFactor should not change with invalid value")
	}
}

func TestRoundPrice(t *testing.T) {
	s := NewBasicMarketMaking(Config{})

	tests := []struct {
		name      string
		price     float64
		reference float64
		want      float64
	}{
		{"high price", 2000.567, 2000.0, 2000.6},
		{"medium price", 150.1234, 150.0, 150.12},
		{"low price", 15.12345, 15.0, 15.123},
		{"very low price", 1.123456, 1.0, 1.1235},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.roundPrice(tt.price, tt.reference)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("roundPrice(%v, %v) = %v, want %v", tt.price, tt.reference, got, tt.want)
			}
		})
	}
}

func TestRoundSize(t *testing.T) {
	s := NewBasicMarketMaking(Config{})

	tests := []struct {
		size float64
		want float64
	}{
		{0.12345, 0.123},
		{0.1, 0.1},
		{1.9999, 2.0},
		{0.0001, 0.0},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := s.roundSize(tt.size)
			if math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("roundSize(%v) = %v, want %v", tt.size, got, tt.want)
			}
		})
	}
}

func TestConcurrency(t *testing.T) {
	s := NewBasicMarketMaking(Config{
		BaseSpread:   0.001,
		BaseSize:     0.1,
		MaxInventory: 1.0,
	})

	ctx := Context{
		Symbol:       "ETHUSDC",
		Mid:          2000.0,
		Inventory:    0.0,
		MaxInventory: 1.0,
	}

	// 并发生成报价
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := s.GenerateQuotes(ctx)
			if err != nil {
				t.Errorf("concurrent GenerateQuotes failed: %v", err)
			}
			done <- true
		}()
	}

	// 并发记录成交
	for i := 0; i < 10; i++ {
		go func() {
			s.OnFill(Fill{Side: "BUY", Price: 2000.0, Size: 0.1})
			done <- true
		}()
	}

	// 等待所有goroutine完成
	for i := 0; i < 20; i++ {
		<-done
	}

	stats := s.GetStatistics()
	if stats["total_buy_fills"].(int) != 10 {
		t.Errorf("concurrent fills not recorded correctly: got %v, want 10", stats["total_buy_fills"])
	}
}

// 基准测试
func BenchmarkGenerateQuotes(b *testing.B) {
	s := NewBasicMarketMaking(Config{
		BaseSpread:   0.001,
		BaseSize:     0.1,
		MaxInventory: 1.0,
		SkewFactor:   0.3,
	})

	ctx := Context{
		Symbol:       "ETHUSDC",
		Mid:          2000.0,
		Inventory:    0.5,
		MaxInventory: 1.0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.GenerateQuotes(ctx)
	}
}
