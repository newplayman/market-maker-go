package sim

import (
	"time"

	"market-maker-go/inventory"
	"market-maker-go/market"
	"market-maker-go/order"
	"market-maker-go/risk"
	"market-maker-go/strategy"
)

// RunnerConfig 描述 Runner 的可选参数。
type RunnerConfig struct {
	Symbol         string
	MinSpread      float64
	BaseSize       float64
	TargetPosition float64
	MaxDrift       float64
	TickSize       float64
	StepSize       float64
	MinQty         float64
	MaxQty         float64
	MinNotional    float64

	SingleMax      float64
	DailyMax       float64
	NetMax         float64
	MaxSpreadRatio float64 // VWAP spread 阈值
	MinInterval    time.Duration
	MinPnL         float64
	MaxPnL         float64
}

// BuildRunner 基于配置快速组装 Runner（使用内存组件，适合离线/仿真）。
func BuildRunner(cfg RunnerConfig) (*Runner, error) {
	engine, err := strategy.NewEngine(strategy.EngineConfig{
		MinSpread:      cfg.MinSpread,
		TargetPosition: cfg.TargetPosition,
		MaxDrift:       cfg.MaxDrift,
		BaseSize:       cfg.BaseSize,
	})
	if err != nil {
		return nil, err
	}
	tr := &inventory.Tracker{}
	ob := market.NewOrderBook()

	var pnlGuard risk.Guard
	if cfg.MinPnL != 0 || cfg.MaxPnL != 0 {
		pnlGuard = &risk.PnLGuard{
			MinPnL: cfg.MinPnL,
			MaxPnL: cfg.MaxPnL,
			Source: risk.InventoryPnL{
				Tracker: tr,
				MidFn:   ob.Mid,
			},
		}
	}
	var freqGuard risk.Guard
	if cfg.MinInterval > 0 {
		freqGuard = risk.NewLatencyGuard(cfg.MinInterval)
	}

	guard := risk.BuildGuards(
		&risk.Limits{SingleMax: cfg.SingleMax, DailyMax: cfg.DailyMax, NetMax: cfg.NetMax},
		nil,
		cfg.MaxSpreadRatio,
		ob,
		pnlGuard,
		freqGuard,
	)

	gw := &mockGateway{}
	mgr := order.NewManager(gw)
	if cfg.Symbol != "" {
		mgr.SetConstraints(map[string]order.SymbolConstraints{
			cfg.Symbol: {
				TickSize:    cfg.TickSize,
				StepSize:    cfg.StepSize,
				MinQty:      cfg.MinQty,
				MaxQty:      cfg.MaxQty,
				MinNotional: cfg.MinNotional,
			},
		})
	}

	r := &Runner{
		Symbol:   cfg.Symbol,
		Engine:   engine,
		Inv:      tr,
		OrderMgr: mgr,
		Risk:     guard,
		Book:     ob,
		Constraints: order.SymbolConstraints{
			TickSize:    cfg.TickSize,
			StepSize:    cfg.StepSize,
			MinQty:      cfg.MinQty,
			MaxQty:      cfg.MaxQty,
			MinNotional: cfg.MinNotional,
		},
	}
	return r, nil
}

// mockGateway 与 cmd/sim 中一致，用于离线模拟。
type mockGateway struct{ orders []order.Order }

func (m *mockGateway) Place(o order.Order) (string, error) {
	if o.ID == "" {
		o.ID = "sim"
	}
	m.orders = append(m.orders, o)
	return o.ID, nil
}
func (m *mockGateway) Cancel(orderID string) error { return nil }
