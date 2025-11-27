package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"market-maker-go/gateway"
	"market-maker-go/internal/order_manager"
	"market-maker-go/internal/store"
	"market-maker-go/internal/strategy"

	"gopkg.in/yaml.v3"
)

// TestConfig 测试配置
type TestConfig struct {
	Symbol string `yaml:"symbol"`
}

func main() {
	cfgPath := flag.String("config", "configs/round8_survival.yaml", "配置文件路径")
	durationSec := flag.Int("duration", 60, "测试时长（秒）")
	flag.Parse()

	// 加载配置
	var cfg TestConfig
	raw, err := os.ReadFile(*cfgPath)
	if err != nil {
		log.Fatalf("read config: %v", err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		log.Fatalf("parse config: %v", err)
	}

	apiKey := os.Getenv("BINANCE_API_KEY")
	apiSecret := os.Getenv("BINANCE_API_SECRET")
	if apiKey == "" || apiSecret == "" {
		log.Fatal("需要设置 BINANCE_API_KEY 和 BINANCE_API_SECRET")
	}

	// 创建客户端
	client := &gateway.BinanceRESTClient{
		BaseURL:      "https://fapi.binance.com",
		APIKey:       apiKey,
		Secret:       apiSecret,
		HTTPClient:   gateway.NewDefaultHTTPClient(),
		RecvWindowMs: 5000,
	}

	// 创建智能订单管理器
	smartMgr := order_manager.NewSmartOrderManager(
		order_manager.SmartOrderManagerConfig{
			Symbol:                  cfg.Symbol,
			PriceDeviationThreshold: 0.0008,
			ReorganizeThreshold:     0.0035,
			MinCancelInterval:       500 * time.Millisecond,
			OrderMaxAge:             90 * time.Second,
		},
		client,
	)

	// 创建虚拟 store 和策略（用于生成测试订单）
	st := store.New(cfg.Symbol, 0.25, nil)
	strat := strategy.NewGeometricV2(strategy.GeometricV2Config{
		Symbol:           cfg.Symbol,
		MinSpread:        0.0005,
		BaseSize:         0.01,
		NetMax:           0.2,
		LayerSpacingMode: "geometric",
		SpacingRatio:     1.15,
		LayerSizeDecay:   0.95,
		MaxLayers:        5, // 减少层数用于测试
		WorstCaseMult:    1.15,
		SizeDecayK:       3.8,
	}, st)

	fmt.Printf("🔸 智能订单管理器测试\n")
	fmt.Printf("   配置: 价格偏移阈值=0.08%%, 重组阈值=0.35%%\n")
	fmt.Printf("   测试时长: %d 秒\n\n", *durationSec)

	// 获取初始深度
	bid, ask, err := client.GetBestBidAsk(cfg.Symbol, 5)
	if err != nil {
		log.Fatalf("获取深度失败: %v", err)
	}
	mid := (bid + ask) / 2.0
	st.UpdateDepth(bid, ask, time.Now())

	fmt.Printf("初始中值价: %.2f\n\n", mid)

	// 运行测试
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	endTime := time.Now().Add(time.Duration(*durationSec) * time.Second)
	iteration := 0

	for time.Now().Before(endTime) {
		<-ticker.C
		iteration++

		// 模拟价格小幅波动（+/- 0.05%）
		midOffset := (float64(iteration%10) - 5) * mid * 0.0001
		currentMid := mid + midOffset

		// 生成订单
		position := 0.0 // 无仓位测试
		buys, sells := strat.GenerateQuotes(position, currentMid)

		// 调用智能订单管理器
		if err := smartMgr.ReconcileOrders(buys, sells, currentMid, false); err != nil {
			log.Printf("⚠️  ReconcileOrders 失败: %v", err)
		}

		// 打印统计
		stats := smartMgr.GetStatistics()
		statsJSON, _ := json.MarshalIndent(stats, "", "  ")
		fmt.Printf("[迭代 %d] mid=%.2f (偏移=%.4f%%)\n", iteration, currentMid, midOffset/mid*100)
		fmt.Printf("%s\n\n", statsJSON)
	}

	// 最终清理
	fmt.Println("🔸 测试结束，清理所有订单...")
	if err := client.CancelAll(cfg.Symbol); err != nil {
		log.Printf("清理失败: %v", err)
	} else {
		fmt.Println("✅ 清理完成")
	}

	// 最终统计
	finalStats := smartMgr.GetStatistics()
	fmt.Println("\n📊 最终统计:")
	fmt.Printf("   总撤单次数: %v\n", finalStats["total_cancels"])
	fmt.Printf("   活跃买单数: %v\n", finalStats["active_buy_orders"])
	fmt.Printf("   活跃卖单数: %v\n", finalStats["active_sell_orders"])
}
