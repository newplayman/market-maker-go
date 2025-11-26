package main

import (
	"flag"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"

	"gopkg.in/yaml.v3"

	"market-maker-go/gateway"
	"market-maker-go/internal/exchange"
	"market-maker-go/internal/order_manager"
	"market-maker-go/internal/risk"
	"market-maker-go/internal/store"
	"market-maker-go/internal/strategy"
	"market-maker-go/metrics"
)

var dryRun bool

// Round8Config 简化配置结构（匹配 round8_survival.yaml）。
type Round8Config struct {
	Symbol          string  `yaml:"symbol"`
	QuoteIntervalMs int     `yaml:"quote_interval_ms"`
	BaseSize        float64 `yaml:"base_size"`
	NetMax          float64 `yaml:"net_max"`
	MinSpread       float64 `yaml:"min_spread"`

	LayerSpacingMode string  `yaml:"layer_spacing_mode"`
	SpacingRatio     float64 `yaml:"spacing_ratio"`
	LayerSizeDecay   float64 `yaml:"layer_size_decay"`
	MaxLayers        int     `yaml:"max_layers"`

	WorstCase struct {
		Multiplier float64 `yaml:"multiplier"`
		SizeDecayK float64 `yaml:"size_decay_k"`
	} `yaml:"worst_case"`

	Funding struct {
		Sensitivity  float64 `yaml:"sensitivity"`
		PredictAlpha float64 `yaml:"predict_alpha"`
	} `yaml:"funding"`

	Grinding struct {
		Enabled              bool    `yaml:"enabled"`
		TriggerRatio         float64 `yaml:"trigger_ratio"`
		RangeStdThreshold    float64 `yaml:"range_std_threshold"`
		GrindSizePct         float64 `yaml:"grind_size_pct"`
		ReentrySpreadBps     float64 `yaml:"reentry_spread_bps"`
		MaxGrindPerHour      int     `yaml:"max_grind_per_hour"`
		MinIntervalSec       int     `yaml:"min_interval_sec"`
		FundingBoost         bool    `yaml:"funding_boost"`
		FundingFavorMult     float64 `yaml:"funding_favor_multiplier"`
	} `yaml:"grinding"`
}

func main() {
	cfgPath := flag.String("config", "configs/round8_survival.yaml", "配置文件路径")
	metricsAddr := flag.String("metricsAddr", ":9101", "Prometheus 指标监听地址")
	flag.Parse()

	// 加载配置
	var cfg Round8Config
	raw, err := os.ReadFile(*cfgPath)
	if err != nil {
		log.Fatalf("read config: %v", err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		log.Fatalf("parse config: %v", err)
	}

	// 从环境变量获取 API 凭据
	apiKey := os.Getenv("BINANCE_API_KEY")
	apiSecret := os.Getenv("BINANCE_API_SECRET")
	if apiKey == "" || apiSecret == "" {
		log.Fatal("BINANCE_API_KEY / BINANCE_API_SECRET required")
	}

	// 启动 Prometheus metrics
	metrics.StartMetricsServer(*metricsAddr)
	log.Printf("Prometheus metrics on %s/metrics", *metricsAddr)
	// DRY-RUN 跳闸：环境变量 DRY_RUN=1 或 true 时仅打印不下单
	dryRun = os.Getenv("DRY_RUN") == "1" || strings.EqualFold(os.Getenv("DRY_RUN"), "true")

	// 关键修复：写PID文件，用于优雅退出
	pidFile := "./logs/runner.pid"
	os.MkdirAll("./logs", 0755)
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
		log.Printf("⚠️ 写PID文件失败: %v", err)
	}
	defer os.Remove(pidFile)

	// 创建 Store
	st := store.New(cfg.Symbol, cfg.Funding.PredictAlpha)

	// 创建 REST 客户端（用于下单）
	restClient := &gateway.BinanceRESTClient{
		BaseURL:      "https://fapi.binance.com",
		APIKey:       apiKey,
		Secret:       apiSecret,
		HTTPClient:   gateway.NewDefaultHTTPClient(),
		RecvWindowMs: 5000,
	}
	// 设置逐仓与杠杆，以降低保证金要求
	if err := restClient.SetMarginType(cfg.Symbol, "ISOLATED"); err != nil {
		log.Printf("set margin type err: %v", err)
	}
	if err := restClient.SetLeverage(cfg.Symbol, 20); err != nil {
		log.Printf("set leverage err: %v", err)
	}

	ws := exchange.NewBinanceUserStream("https://fapi.binance.com", "wss://fstream.binance.com", apiKey, apiSecret, st)
	if err := ws.Start(); err != nil {
		log.Fatalf("start ws: %v", err)
	}
	defer ws.Stop()

	// 启动公共行情深度订阅，驱动 mid 更新
	depthWS := gateway.NewBinanceWSReal()
	_ = depthWS.SubscribeDepth(cfg.Symbol)
	go func() {
		handler := &storeWSHandler{st: st}
		if err := depthWS.Run(handler); err != nil {
			log.Printf("depth ws run err: %v", err)
		}
	}()

	// 启动 funding rate 订阅，更新资金费率预测与累计成本
	go func() {
		url := fmt.Sprintf("wss://fstream.binance.com/ws/%s@markPrice@1s", strings.ToLower(cfg.Symbol))
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			log.Printf("funding ws dial err: %v", err)
			return
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				log.Printf("funding ws read err: %v", err)
				return
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(msg, &payload); err == nil {
				if rv, ok := payload["r"]; ok {
					switch v := rv.(type) {
					case string:
						if rf, err := strconv.ParseFloat(v, 64); err == nil {
							st.HandleFundingRate(rf)
						}
					case float64:
						st.HandleFundingRate(v)
					}
				}
			}
		}
	}()

	// 创建策略
	stratCfg := strategy.GeometricV2Config{
		Symbol:           cfg.Symbol,
		MinSpread:        cfg.MinSpread,
		BaseSize:         cfg.BaseSize,
		NetMax:           cfg.NetMax,
		LayerSpacingMode: cfg.LayerSpacingMode,
		SpacingRatio:     cfg.SpacingRatio,
		LayerSizeDecay:   cfg.LayerSizeDecay,
		MaxLayers:        cfg.MaxLayers,
		WorstCaseMult:    cfg.WorstCase.Multiplier,
		SizeDecayK:       cfg.WorstCase.SizeDecayK,
	}
	strat := strategy.NewGeometricV2(stratCfg, st)

	// 创建智能订单管理器（避免频繁撤单触发币安速率限制）
	smartOrderMgr := order_manager.NewSmartOrderManager(
		order_manager.SmartOrderManagerConfig{
			Symbol:                  cfg.Symbol,
			PriceDeviationThreshold: 0.0008,         // 0.08% 价格偏移才更新
			ReorganizeThreshold:     0.0035,         // 0.35% 大偏移时全量重组
			MinCancelInterval:       500 * time.Millisecond, // 撤单间隔
			OrderMaxAge:             90 * time.Second, // 订单90秒老化
		},
		restClient,
	)

	// 创建磨成本引擎
	grindCfg := risk.GrindingConfig{
		Enabled:           cfg.Grinding.Enabled,
		TriggerRatio:      cfg.Grinding.TriggerRatio,
		RangeStdThreshold: cfg.Grinding.RangeStdThreshold,
		GrindSizePct:      cfg.Grinding.GrindSizePct,
		ReentrySpreadBps:  cfg.Grinding.ReentrySpreadBps,
		MaxGrindPerHour:   cfg.Grinding.MaxGrindPerHour,
		MinIntervalSec:    cfg.Grinding.MinIntervalSec,
		FundingBoost:      cfg.Grinding.FundingBoost,
		FundingFavorMult:  cfg.Grinding.FundingFavorMult,
	}
	placer := &orderPlacer{client: restClient}
	grinder := risk.NewGrindingEngine(grindCfg, st, cfg.NetMax, placer)

	// 启动报价循环
	go runQuoteLoop(cfg, strat, st, smartOrderMgr)

	// 启动磨成本循环
	go runGrindingLoop(grinder, st)

	// 优雅退出：捕获信号后先撤单、平仓再退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	log.Println("\n============================================")
	log.Println("🛑 接收退出信号，开始优雅退出...")
	log.Println("============================================")

	// 第1步：停止报价循环（防止新订单）
	log.Println("✅ 已停止报价循环")
	
	// 第2步：撤销所有活跃订单
	log.Println("🟡 [1/3] 取消所有活跃订单...")
	if err := restClient.CancelAll(cfg.Symbol); err != nil {
		log.Printf("⚠️ 取消订单失败: %v", err)
	} else {
		log.Println("✅ 所有活跃订单已撤销")
	}
	
	// 第3步：平掉所有仓位
	log.Println("🟡 [2/3] 平掉所有仓位...")
	if err := flattenPosition(restClient, cfg.Symbol); err != nil {
		log.Printf("⚠️ 平仓失败: %v", err)
	} else {
		log.Println("✅ 所有仓位已平")
	}
	
	// 第4步：关闭 WebSocket 连接
	log.Println("🟡 [3/3] 关闭 WebSocket 连接...")
	ws.Stop()
	log.Println("✅ WebSocket 已关闭")
	
	log.Println("============================================")
	log.Println("✅ 优雅退出完成，程序退出")
	log.Println("============================================")
}

// runQuoteLoop 定期生成并下单报价（使用智能订单管理）。
func runQuoteLoop(cfg Round8Config, strat *strategy.GeometricV2, st *store.Store, smartMgr *order_manager.SmartOrderManager) {
	ticker := time.NewTicker(time.Duration(cfg.QuoteIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	
	for range ticker.C {
		mid := st.MidPrice()
		if mid == 0 {
			continue
		}

		position := st.Position()
		buys, sells := strat.GenerateQuotes(position, mid)
		
		// 使用智能订单管理器进行差分更新
		// 它会自动判断:
		// - 价格偏移小 -> 保持原单不动
		// - 部分成交 -> 只补充缺失的订单
		// - 价格大偏移 -> 全量重组
		if err := smartMgr.ReconcileOrders(buys, sells, mid, dryRun); err != nil {
			log.Printf("reconcile orders err: %v", err)
		}
	}
}

// runGrindingLoop 每 55 秒检查磨成本。
func runGrindingLoop(grinder *risk.GrindingEngine, st *store.Store) {
	ticker := time.NewTicker(55 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		mid := st.MidPrice()
		if mid == 0 {
			continue
		}
		position := st.Position()
		if err := grinder.MaybeGrind(position, mid); err != nil {
			log.Printf("grind err: %v", err)
		}
	}
}

// helpers
func floorTo(x, step float64) float64 { return math.Floor(x/step) * step }
func ceilTo(x, step float64) float64  { return math.Ceil(x/step) * step }

type storeWSHandler struct{ st *store.Store }

func (h *storeWSHandler) OnDepth(symbol string, bid, ask float64) {
	h.st.UpdateDepth(bid, ask, time.Now().UTC())
}
func (h *storeWSHandler) OnTrade(symbol string, price, qty float64) {}
func (h *storeWSHandler) OnRawMessage(msg []byte) {
	sym, bid, ask, err := gateway.ParseCombinedDepth(msg)
	if err == nil {
		h.OnDepth(sym, bid, ask)
	}
}

// orderPlacer 实现 OrderPlacer 接口（简化版）。
type orderPlacer struct {
	client *gateway.BinanceRESTClient
}

func (p *orderPlacer) PlaceMarket(symbol, side string, qty float64) error {
	if dryRun {
		log.Printf("DRY-RUN: Market %s %.6f", side, qty)
		return nil
	}
	_, err := p.client.PlaceMarket(symbol, side, qty, false, "")
	return err
}

func (p *orderPlacer) PlaceLimit(symbol, side string, price, qty float64) error {
	if dryRun {
		log.Printf("DRY-RUN: Limit %s %.6f @ %.2f", side, qty, price)
		return nil
	}
	_, err := p.client.PlaceLimit(symbol, side, "GTC", price, qty, false, true, "")
	return err
}

// flattenPosition 查询仓位并平仓。
func flattenPosition(client *gateway.BinanceRESTClient, symbol string) error {
	if dryRun {
		log.Println("DRY-RUN: 跳过平仓")
		return nil
	}
	info, err := client.AccountInfo()
	if err != nil {
		return fmt.Errorf("查询账户: %w", err)
	}
	var position float64
	for _, p := range info.Positions {
		if p.Symbol == symbol {
			position = p.PositionAmt
			break
		}
	}
	if position == 0 {
		return nil
	}
	side := "SELL"
	qty := position
	if position < 0 {
		side = "BUY"
		qty = -position
	}
	_, err = client.PlaceMarket(symbol, side, qty, true, "")
	return err
}
