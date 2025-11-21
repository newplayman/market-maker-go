package main

import (
	"flag"
	"fmt"
	"math/rand"
	"time"

	"market-maker-go/inventory"
	"market-maker-go/order"
	"market-maker-go/risk"
	"market-maker-go/sim"
	"market-maker-go/strategy"
)

// 一个极简的本地模拟：随机生成 mid 价格，驱动策略与下单链路。
// 可通过命令行参数调整策略和风控；仅用于演示，不会连接真实交易所。
func main() {
	symbol := flag.String("symbol", "BTCUSDT", "trading symbol")
	ticks := flag.Int("ticks", 5, "number of random ticks to simulate")
	minSpread := flag.Float64("minSpread", 0.001, "min spread ratio (e.g. 0.001=10bps)")
	baseSize := flag.Float64("baseSize", 0.5, "base order size")
	targetPos := flag.Float64("targetPos", 0, "target position")
	maxDrift := flag.Float64("maxDrift", 1, "max drift before shifting quotes")
	singleMax := flag.Float64("singleMax", 5, "risk: single order max")
	dailyMax := flag.Float64("dailyMax", 50, "risk: daily volume max")
	netMax := flag.Float64("netMax", 10, "risk: net exposure max")
	latencyMs := flag.Int("latencyMs", 0, "risk: min interval between orders in ms (0 to disable)")
	pnlMin := flag.Float64("pnlMin", 0, "risk: min pnl threshold (skip if 0)")
	pnlMax := flag.Float64("pnlMax", 0, "risk: max pnl threshold (skip if 0)")
	flag.Parse()

	engine, _ := strategy.NewEngine(strategy.EngineConfig{
		MinSpread:      *minSpread,
		TargetPosition: *targetPos,
		MaxDrift:       *maxDrift,
		BaseSize:       *baseSize,
	})
	tr := &inventory.Tracker{}
	gw := &mockGateway{}
	mgr := order.NewManager(gw)
	base := 100.0
	var guards []risk.Guard
	guards = append(guards, risk.NewLimitChecker(&risk.Limits{SingleMax: *singleMax, DailyMax: *dailyMax, NetMax: *netMax}, nil))
	if *latencyMs > 0 {
		guards = append(guards, risk.NewLatencyGuard(time.Duration(*latencyMs)*time.Millisecond))
	}
	if *pnlMin != 0 || *pnlMax != 0 {
		guards = append(guards, &risk.PnLGuard{
			MinPnL: *pnlMin,
			MaxPnL: *pnlMax,
			Source: risk.InventoryPnL{
				Tracker: tr,
				MidFn:   func() float64 { return base },
			},
		})
	}
	riskGuard := risk.MultiGuard{Guards: guards}

	runner := sim.Runner{
		Symbol:   *symbol,
		Engine:   engine,
		Inv:      tr,
		OrderMgr: mgr,
		Risk:     riskGuard,
	}

	rand.Seed(time.Now().UnixNano())
	for i := 0; i < *ticks; i++ {
		mid := base + rand.NormFloat64() // 简单高斯扰动
		if err := runner.OnTick(mid); err != nil {
			fmt.Printf("tick %d mid=%.2f err=%v\n", i, mid, err)
		} else {
			fmt.Printf("tick %d mid=%.2f placed=%d\n", i, mid, len(gw.orders))
		}
	}
	fmt.Printf("total orders: %d\n", len(gw.orders))
}

// mockGateway 用于本地模拟下单。
type mockGateway struct{ orders []order.Order }

func (m *mockGateway) Place(o order.Order) (string, error) {
	if o.ID == "" {
		o.ID = fmt.Sprintf("sim-%d", len(m.orders)+1)
	}
	m.orders = append(m.orders, o)
	return o.ID, nil
}
func (m *mockGateway) Cancel(orderID string) error { return nil }
