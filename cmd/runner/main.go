package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
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
	MarginType       string  `yaml:"margin_type"`

	WorstCase struct {
		Multiplier float64 `yaml:"multiplier"`
		SizeDecayK float64 `yaml:"size_decay_k"`
	} `yaml:"worst_case"`

	Funding struct {
		Sensitivity  float64 `yaml:"sensitivity"`
		PredictAlpha float64 `yaml:"predict_alpha"`
	} `yaml:"funding"`

	Grinding struct {
		Enabled           bool    `yaml:"enabled"`
		TriggerRatio      float64 `yaml:"trigger_ratio"`
		RangeStdThreshold float64 `yaml:"range_std_threshold"`
		GrindSizePct      float64 `yaml:"grind_size_pct"`
		ReentrySpreadBps  float64 `yaml:"reentry_spread_bps"`
		MaxGrindPerHour   int     `yaml:"max_grind_per_hour"`
		MinIntervalSec    int     `yaml:"min_interval_sec"`
		FundingBoost      bool    `yaml:"funding_boost"`
		FundingFavorMult  float64 `yaml:"funding_favor_multiplier"`
	} `yaml:"grinding"`

	Risk struct {
		ReduceOnlySoftMultiplier   float64 `yaml:"reduce_only_soft_multiplier"`
		ReduceOnlyHardMultiplier   float64 `yaml:"reduce_only_hard_multiplier"`
		ReduceOnlyMarketMultiplier float64 `yaml:"reduce_only_market_multiplier"`
	} `yaml:"risk"`

	QuotePinning struct {
		Enabled                bool    `yaml:"enabled"`
		TriggerRatio           float64 `yaml:"trigger_ratio"`
		NearLayers             int     `yaml:"near_layers"`
		FarLayers              int     `yaml:"far_layers"`
		FarLayerFixedSize      float64 `yaml:"far_layer_fixed_size"`
		FarLayerMinDistancePct float64 `yaml:"far_layer_min_distance_pct"`
		FarLayerMaxDistancePct float64 `yaml:"far_layer_max_distance_pct"`
		PinToBestTick          bool    `yaml:"pin_to_best_tick"`
		PinSizeMultiplier      float64 `yaml:"pin_size_multiplier"`
	} `yaml:"quote_pinning"`
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

	// P1修复：验证配置参数
	// if err := validateConfig(&cfg); err != nil {
	// 	log.Fatalf("❌ 配置验证失败: %v", err)
	// }
	// log.Println("✅ 配置验证通过")

	if cfg.MarginType == "" {
		cfg.MarginType = "ISOLATED"
	}
	if cfg.Risk.ReduceOnlySoftMultiplier <= 0 {
		cfg.Risk.ReduceOnlySoftMultiplier = 6 // ≈6倍base size触发减仓
	}
	if cfg.Risk.ReduceOnlyHardMultiplier <= 0 {
		cfg.Risk.ReduceOnlyHardMultiplier = cfg.Risk.ReduceOnlySoftMultiplier * 1.5
	}
	if cfg.Risk.ReduceOnlyMarketMultiplier <= 0 {
		cfg.Risk.ReduceOnlyMarketMultiplier = 2
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

	eventLog, err := newEventLogger("./logs/runner_events.log")
	if err != nil {
		log.Fatalf("event logger init: %v", err)
	}
	defer eventLog.Close()
	eventSink := func(evt string, fields map[string]interface{}) {
		eventLog.Log(evt, fields)
	}
	eventLog.Log("runner_start", map[string]interface{}{
		"symbol":     cfg.Symbol,
		"marginType": strings.ToUpper(cfg.MarginType),
	})

	// 创建 Store
	st := store.New(cfg.Symbol, cfg.Funding.PredictAlpha, eventSink)

	httpClient := &http.Client{Timeout: 5 * time.Second}
	// 创建 REST 客户端（用于下单）
	restClient := &gateway.BinanceRESTClient{
		BaseURL:      "https://fapi.binance.com",
		APIKey:       apiKey,
		Secret:       apiSecret,
		HTTPClient:   httpClient,
		RecvWindowMs: 5000,
		MaxRetries:   3,
		RetryDelay:   500 * time.Millisecond,
	}
	// 设置逐仓/全仓与杠杆
	marginType := strings.ToUpper(cfg.MarginType)
	if marginType == "" {
		marginType = "ISOLATED"
	}
	if err := restClient.SetMarginType(cfg.Symbol, marginType); err != nil {
		log.Printf("set margin type err: %v", err)
	} else {
		log.Printf("margin type set to %s", marginType)
	}
	if err := restClient.SetLeverage(cfg.Symbol, 20); err != nil {
		log.Printf("set leverage err: %v", err)
	}

	ws := exchange.NewBinanceUserStream("https://fapi.binance.com", "wss://fstream.binance.com", apiKey, apiSecret, st)
	ws.SetEventSink(eventSink)

	// 注册WebSocket致命错误回调（P0级修复）
	ws.SetFatalErrorHandler(func(err error) {
		log.Printf("❌ WebSocket致命错误: %v", err)
		eventLog.Log("ws_fatal_error", map[string]interface{}{
			"error": err.Error(),
		})
		// 触发优雅退出
		p, _ := os.FindProcess(os.Getpid())
		p.Signal(syscall.SIGTERM)
	})

	if err := ws.Start(); err != nil {
		log.Fatalf("start ws: %v", err)
	}
	defer ws.Stop()

	tradeWS := exchange.NewTradeWSClient(exchange.TradeWSConfig{
		APIKey:    apiKey,
		SecretKey: apiSecret,
		OnNotify: func(method string, payload json.RawMessage) {
			eventLog.Log("trade_ws_notify", map[string]interface{}{
				"method":  method,
				"payload": string(payload),
			})
		},
		OnFallback: func(meta exchange.WSRequestMeta, reason error) {
			eventLog.Log("trade_ws_fallback", map[string]interface{}{
				"method": meta.Method,
				"reason": reason.Error(),
			})
		},
	})
	tradeWS.Start(context.Background())
	defer tradeWS.Close()

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
		QuotePinning: strategy.QuotePinningConfig{
			Enabled:                cfg.QuotePinning.Enabled,
			TriggerRatio:           cfg.QuotePinning.TriggerRatio,
			NearLayers:             cfg.QuotePinning.NearLayers,
			FarLayers:              cfg.QuotePinning.FarLayers,
			FarLayerFixedSize:      cfg.QuotePinning.FarLayerFixedSize,
			FarLayerMinDistancePct: cfg.QuotePinning.FarLayerMinDistancePct,
			FarLayerMaxDistancePct: cfg.QuotePinning.FarLayerMaxDistancePct,
			PinToBestTick:          cfg.QuotePinning.PinToBestTick,
			PinSizeMultiplier:      cfg.QuotePinning.PinSizeMultiplier,
		},
	}
	strat := strategy.NewGeometricV2(stratCfg, st)

	// 创建智能订单管理器（避免频繁撤单触发币安速率限制）
	limitClient := &wsLimitClient{
		rest:    restClient,
		tradeWS: tradeWS,
		sink:    eventSink,
	}
	smartOrderMgr := order_manager.NewSmartOrderManager(
		order_manager.SmartOrderManagerConfig{
			Symbol:                  cfg.Symbol,
			PriceDeviationThreshold: 0.0008,                 // 0.08% 价格偏移才更新
			ReorganizeThreshold:     0.0035,                 // 0.35% 大偏移时全量重组
			MinCancelInterval:       500 * time.Millisecond, // 撤单间隔
			OrderMaxAge:             90 * time.Second,       // 订单90秒老化
		},
		limitClient,
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
	placer := &orderPlacer{client: restClient, sink: eventSink, tradeWS: tradeWS}
	grinder := risk.NewGrindingEngine(grindCfg, st, cfg.NetMax, cfg.BaseSize, cfg.QuotePinning.PinSizeMultiplier, placer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup

	reduceCtrl := newReduceOnlyController(cfg, st.Symbol, placer, eventSink)

	// 启动报价循环
	wg.Add(1)
	go func() {
		defer wg.Done()
		runQuoteLoop(ctx, cfg, strat, st, smartOrderMgr, reduceCtrl)
	}()

	// 启动磨成本循环
	wg.Add(1)
	go func() {
		defer wg.Done()
		runGrindingLoop(ctx, grinder, st)
	}()

	// 事件快照
	wg.Add(1)
	go func() {
		defer wg.Done()
		runEventSnapshotLoop(ctx, st, eventLog)
	}()

	if ok, err := daemon.SdNotify(false, daemon.SdNotifyReady); err != nil {
		log.Printf("systemd notify ready failed: %v", err)
	} else if ok {
		log.Println("systemd notified READY=1")
	}

	// 优雅退出：捕获信号后先撤单、平仓再退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	log.Println("\n============================================")
	log.Println("🛑 接收退出信号，开始优雅退出...")
	log.Println("============================================")
	eventLog.Log("runner_stop_signal", map[string]interface{}{"symbol": cfg.Symbol})

	if ok, err := daemon.SdNotify(false, daemon.SdNotifyStopping); err != nil {
		log.Printf("systemd notify stopping failed: %v", err)
	} else if ok {
		log.Println("systemd notified STOPPING=1")
	}

	// 第1步：停止报价循环（防止新订单）
	cancel()
	wg.Wait()
	log.Println("✅ 已停止报价与磨成本循环")

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
func runQuoteLoop(ctx context.Context, cfg Round8Config, strat *strategy.GeometricV2, st *store.Store, smartMgr *order_manager.SmartOrderManager, reducer *reduceOnlyController) {
	ticker := time.NewTicker(time.Duration(cfg.QuoteIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("quote loop stopped")
			return
		case <-ticker.C:
		}

		mid := st.MidPrice()
		if mid == 0 {
			continue
		}

		position := st.Position()
		buys, sells := strat.GenerateQuotes(position, mid)

		// Round8防闪烁：钉子模式 (已移入 strategy.GenerateQuotes，此处无需重复调用 applyQuotePinning)
		// if cfg.QuotePinning.Enabled {
		// 	buys, sells = applyQuotePinning(cfg, position, mid, buys, sells)
		// }

		if reducer != nil {
			buys, sells = reducer.Apply(position, mid, buys, sells)
		}

		// 使用智能订单管理器进行差分更新
		if err := smartMgr.ReconcileOrders(buys, sells, mid, dryRun); err != nil {
			log.Printf("reconcile orders err: %v", err)
		}
	}
}

type reduceOnlyController struct {
	soft      float64
	hard      float64
	chunk     float64
	boost     float64
	symbol    string
	placer    *orderPlacer
	lastState int
	lastForce time.Time
	cooldown  time.Duration
	sink      store.EventSink
}

func newReduceOnlyController(cfg Round8Config, symbol string, placer *orderPlacer, sink store.EventSink) *reduceOnlyController {
	softMult := cfg.Risk.ReduceOnlySoftMultiplier
	hardMult := cfg.Risk.ReduceOnlyHardMultiplier
	marketMult := cfg.Risk.ReduceOnlyMarketMultiplier
	if softMult <= 0 {
		softMult = 4
	}
	if hardMult <= softMult {
		hardMult = softMult * 1.5
	}
	if marketMult <= 0 {
		marketMult = 2
	}
	soft := cfg.BaseSize * softMult
	hard := cfg.BaseSize * hardMult
	chunk := cfg.BaseSize * marketMult
	ctrl := &reduceOnlyController{
		soft:     soft,
		hard:     hard,
		chunk:    chunk,
		boost:    1.3,
		symbol:   symbol,
		placer:   placer,
		cooldown: 5 * time.Second,
		sink:     sink,
	}
	metrics.RunnerRiskState.WithLabelValues(symbol).Set(0)
	metrics.ReduceOnlyForceCount.WithLabelValues(symbol).Add(0)
	return ctrl
}

func (r *reduceOnlyController) Apply(position, mid float64, buys, sells []strategy.Quote) ([]strategy.Quote, []strategy.Quote) {
	if r == nil {
		return buys, sells
	}
	state := r.evaluateState(position)
	metrics.RunnerRiskState.WithLabelValues(r.symbol).Set(float64(state))
	if state != r.lastState {
		log.Printf("reduce-only state=%d pos=%.4f chunk=%.4f", state, position, r.chunk)
		if r.sink != nil {
			r.sink("risk_state_change", map[string]interface{}{
				"state":    state,
				"position": position,
			})
		}
		r.lastState = state
	}
	if state == 0 {
		return buys, sells
	}
	if position > 0 {
		buys = nil
		sells = r.boostQuotes(sells)
	} else if position < 0 {
		sells = nil
		buys = r.boostQuotes(buys)
	}
	if state == 2 {
		r.forceFlatten(position)
	}
	return buys, sells
}

func (r *reduceOnlyController) evaluateState(position float64) int {
	absPos := math.Abs(position)
	if r.soft > 0 && absPos >= r.soft {
		if r.hard > 0 && absPos >= r.hard {
			return 2
		}
		return 1
	}
	return 0
}

func (r *reduceOnlyController) boostQuotes(quotes []strategy.Quote) []strategy.Quote {
	if len(quotes) == 0 || r.boost <= 1 {
		return quotes
	}
	out := make([]strategy.Quote, len(quotes))
	for i, q := range quotes {
		q.Size *= r.boost
		out[i] = q
	}
	return out
}

func (r *reduceOnlyController) forceFlatten(position float64) {
	if r.placer == nil {
		return
	}
	if time.Since(r.lastForce) < r.cooldown {
		return
	}
	qty := r.chunk
	absPos := math.Abs(position)
	if qty <= 0 || qty > absPos {
		qty = absPos
	}
	if qty <= 0 {
		return
	}
	side := "SELL"
	if position < 0 {
		side = "BUY"
	}
	r.lastForce = time.Now()
	go func(side string, qty float64) {
		if err := r.placer.PlaceMarket(r.symbol, side, qty); err != nil {
			log.Printf("reduce-only market %s %.4f err: %v", side, qty, err)
			if r.sink != nil {
				r.sink("reduce_only_force_error", map[string]interface{}{
					"side":  side,
					"qty":   qty,
					"error": err.Error(),
				})
			}
		} else {
			log.Printf("reduce-only market %s %.4f triggered", side, qty)
			metrics.ReduceOnlyForceCount.WithLabelValues(r.symbol).Inc()
			if r.sink != nil {
				r.sink("reduce_only_force", map[string]interface{}{
					"side": side,
					"qty":  qty,
				})
			}
		}
	}(side, qty)
}

// runGrindingLoop 每 55 秒检查磨成本。
func runGrindingLoop(ctx context.Context, grinder *risk.GrindingEngine, st *store.Store) {
	ticker := time.NewTicker(55 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("grind loop stopped")
			return
		case <-ticker.C:
		}

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

func runEventSnapshotLoop(ctx context.Context, st *store.Store, logger *eventLogger) {
	if logger == nil {
		return
	}
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logger.Log("runner_snapshot", map[string]interface{}{
				"mid":          st.MidPrice(),
				"position":     st.Position(),
				"pending_buy":  st.PendingBuySize(),
				"pending_sell": st.PendingSellSize(),
			})
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

// wsLimitClient 为智能订单管理器提供 WSS 优先的下单/撤单能力，并将操作写入事件日志。
type wsLimitClient struct {
	rest    *gateway.BinanceRESTClient
	tradeWS *exchange.TradeWSClient
	sink    store.EventSink
}

func (c *wsLimitClient) PlaceLimit(symbol, side, tif string, price, qty float64, reduceOnly, postOnly bool, clientID string) (string, error) {
	if dryRun {
		c.log("order_submit_result", map[string]interface{}{
			"symbol": symbol,
			"side":   side,
			"price":  price,
			"qty":    qty,
			"type":   "LIMIT",
			"mode":   "DRY_RUN",
			"note":   "skipped",
		})
		return "", nil
	}
	baseFields := map[string]interface{}{
		"symbol":       symbol,
		"side":         side,
		"price":        price,
		"qty":          qty,
		"type":         "LIMIT",
		"reduce_only":  reduceOnly,
		"post_only":    postOnly,
		"time_inforce": strings.ToUpper(tif),
	}
	if clientID != "" {
		baseFields["client_id"] = clientID
	}
	if c.tradeWS != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		wsFields := cloneFields(baseFields)
		wsFields["mode"] = "WSS"
		c.log("order_submit", wsFields)
		resp, err := c.tradeWS.PlaceOrder(ctx, exchange.TradeOrderParams{
			Symbol:        strings.ToUpper(symbol),
			Side:          side,
			Type:          "LIMIT",
			TimeInForce:   strings.ToUpper(tif),
			Quantity:      qty,
			Price:         price,
			ReduceOnly:    reduceOnly,
			PostOnly:      postOnly,
			ClientOrderID: clientID,
		})
		if err == nil {
			orderID := parseWSOrderID(resp)
			if orderID != "" {
				wsFields["order_id"] = orderID
			}
			c.log("order_submit_result", wsFields)
			return orderID, nil
		}
		wsFields["error"] = err.Error()
		c.log("order_submit_result", wsFields)
	}
	start := time.Now()
	restFields := cloneFields(baseFields)
	restFields["mode"] = "REST"
	orderID, err := c.rest.PlaceLimit(symbol, side, tif, price, qty, reduceOnly, postOnly, clientID)
	restFields["duration_ms"] = time.Since(start).Milliseconds()
	if err != nil {
		restFields["error"] = err.Error()
	} else if orderID != "" {
		restFields["order_id"] = orderID
	}
	c.log("order_submit_result", restFields)
	return orderID, err
}

func (c *wsLimitClient) CancelOrder(symbol, orderID string) error {
	if dryRun {
		c.log("order_cancel_result", map[string]interface{}{
			"symbol": symbol,
			"order":  orderID,
			"mode":   "DRY_RUN",
			"note":   "skipped",
		})
		return nil
	}
	fields := map[string]interface{}{
		"symbol": symbol,
		"order":  orderID,
	}
	if c.tradeWS != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		numID, _ := strconv.ParseInt(orderID, 10, 64)
		if numID > 0 {
			wsFields := cloneFields(fields)
			wsFields["mode"] = "WSS"
			c.log("order_cancel", wsFields)
			_, err := c.tradeWS.CancelOrder(ctx, exchange.TradeCancelParams{
				Symbol:  strings.ToUpper(symbol),
				OrderID: numID,
			})
			if err == nil {
				c.log("order_cancel_result", wsFields)
				return nil
			}
			wsFields["error"] = err.Error()
			c.log("order_cancel_result", wsFields)
		}
	}
	start := time.Now()
	fields["mode"] = "REST"
	err := c.rest.CancelOrder(symbol, orderID)
	fields["duration_ms"] = time.Since(start).Milliseconds()
	if err != nil {
		fields["error"] = err.Error()
	}
	c.log("order_cancel_result", fields)
	return err
}

func (c *wsLimitClient) CancelAll(symbol string) error {
	if dryRun {
		c.log("order_cancel_all", map[string]interface{}{
			"symbol": symbol,
			"mode":   "DRY_RUN",
			"note":   "skipped",
		})
		return nil
	}
	if c.tradeWS != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		fields := map[string]interface{}{
			"symbol": symbol,
			"mode":   "WSS",
		}
		c.log("order_cancel_all", fields)
		_, err := c.tradeWS.CancelAll(ctx, exchange.TradeCancelAllParams{
			Symbol: strings.ToUpper(symbol),
		})
		if err == nil {
			c.log("order_cancel_all_result", fields)
			return nil
		}
		fields["error"] = err.Error()
		c.log("order_cancel_all_result", fields)
	}
	start := time.Now()
	fields := map[string]interface{}{
		"symbol": symbol,
		"mode":   "REST",
	}
	err := c.rest.CancelAll(symbol)
	fields["duration_ms"] = time.Since(start).Milliseconds()
	if err != nil {
		fields["error"] = err.Error()
	}
	c.log("order_cancel_all_result", fields)
	return err
}

func (c *wsLimitClient) log(event string, fields map[string]interface{}) {
	if c == nil || c.sink == nil {
		return
	}
	c.sink(event, fields)
}

func parseWSOrderID(raw json.RawMessage) string {
	var payload struct {
		OrderID int64 `json:"orderId"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil && payload.OrderID > 0 {
		return fmt.Sprintf("%d", payload.OrderID)
	}
	return ""
}

func cloneFields(m map[string]interface{}) map[string]interface{} {
	c := make(map[string]interface{}, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// 事件日志
type eventLogger struct {
	file *os.File
	mu   sync.Mutex
}

func newEventLogger(path string) (*eventLogger, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &eventLogger{file: f}, nil
}

func (l *eventLogger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}

func (l *eventLogger) Log(event string, fields map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	entry := map[string]interface{}{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"event": event,
	}
	for k, v := range fields {
		entry[k] = v
	}
	bytes, _ := json.Marshal(entry)
	l.file.Write(bytes)
	l.file.WriteString("\n")
}

// 辅助函数：平仓
func flattenPosition(client *gateway.BinanceRESTClient, symbol string) error {
	// 查询当前仓位
	info, err := client.AccountInfo()
	if err != nil {
		return err
	}
	var posAmt float64
	for _, p := range info.Positions {
		if p.Symbol == symbol {
			posAmt = p.PositionAmt
			break
		}
	}
	if posAmt == 0 {
		return nil
	}
	side := "SELL"
	if posAmt < 0 {
		side = "BUY"
	}
	qty := math.Abs(posAmt)
	_, err = client.PlaceMarket(symbol, side, qty, true, "")
	return err
}

// 验证配置
func validateConfig(cfg *Round8Config) error {
	if cfg.Symbol == "" {
		return fmt.Errorf("symbol required")
	}
	if cfg.BaseSize <= 0 {
		return fmt.Errorf("base_size must be > 0")
	}
	if cfg.NetMax <= 0 {
		return fmt.Errorf("net_max must be > 0")
	}
	return nil
}

// orderPlacer 实现 OrderPlacer 接口（简化版）。
type orderPlacer struct {
	client  *gateway.BinanceRESTClient
	tradeWS *exchange.TradeWSClient
	sink    store.EventSink
}

func (p *orderPlacer) PlaceMarket(symbol, side string, qty float64) error {
	if dryRun {
		log.Printf("DRY-RUN: Market %s %.6f", side, qty)
		return nil
	}
	if p.tradeWS != nil && p.tradeWS.Healthy() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p.log("order_submit", map[string]interface{}{
			"symbol": symbol,
			"side":   side,
			"qty":    qty,
			"type":   "MARKET",
			"mode":   "WSS",
		})
		_, err := p.tradeWS.PlaceOrder(ctx, exchange.TradeOrderParams{
			Symbol:   strings.ToUpper(symbol),
			Side:     side,
			Type:     "MARKET",
			Quantity: qty,
		})
		if err == nil {
			p.log("order_submit_result", map[string]interface{}{
				"symbol": symbol,
				"side":   side,
				"qty":    qty,
				"type":   "MARKET",
				"mode":   "WSS",
			})
			return nil
		}
		p.log("order_submit_result", map[string]interface{}{
			"symbol": symbol,
			"side":   side,
			"qty":    qty,
			"type":   "MARKET",
			"mode":   "WSS",
			"error":  err.Error(),
		})
	}
	start := time.Now()
	orderID, err := p.client.PlaceMarket(symbol, side, qty, false, "")
	fields := map[string]interface{}{
		"symbol":      symbol,
		"side":        side,
		"qty":         qty,
		"type":        "MARKET",
		"mode":        "REST",
		"duration_ms": time.Since(start).Milliseconds(),
	}
	if err != nil {
		fields["error"] = err.Error()
	} else {
		fields["order_id"] = orderID
	}
	p.log("order_submit_result", fields)
	return err
}

func (p *orderPlacer) PlaceLimit(symbol, side string, price, qty float64) error {
	if dryRun {
		log.Printf("DRY-RUN: Limit %s %.6f @ %.2f", side, qty, price)
		return nil
	}
	if p.tradeWS != nil && p.tradeWS.Healthy() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p.log("order_submit", map[string]interface{}{
			"symbol": symbol,
			"side":   side,
			"qty":    qty,
			"price":  price,
			"type":   "LIMIT",
			"mode":   "WSS",
		})
		_, err := p.tradeWS.PlaceOrder(ctx, exchange.TradeOrderParams{
			Symbol:      strings.ToUpper(symbol),
			Side:        side,
			Type:        "LIMIT",
			TimeInForce: "GTX",
			Quantity:    qty,
			Price:       price,
			ReduceOnly:  false,
			PostOnly:    true,
		})
		if err == nil {
			p.log("order_submit_result", map[string]interface{}{
				"symbol": symbol,
				"side":   side,
				"qty":    qty,
				"price":  price,
				"type":   "LIMIT",
				"mode":   "WSS",
			})
			return nil
		}
		p.log("order_submit_result", map[string]interface{}{
			"symbol": symbol,
			"side":   side,
			"qty":    qty,
			"price":  price,
			"type":   "LIMIT",
			"mode":   "WSS",
			"error":  err.Error(),
		})
	}
	start := time.Now()
	orderID, err := p.client.PlaceLimit(symbol, side, "GTC", price, qty, false, true, "")
	fields := map[string]interface{}{
		"symbol":      symbol,
		"side":        side,
		"qty":         qty,
		"price":       price,
		"type":        "LIMIT",
		"mode":        "REST",
		"duration_ms": time.Since(start).Milliseconds(),
	}
	if err != nil {
		fields["error"] = err.Error()
	} else {
		fields["order_id"] = orderID
	}
	p.log("order_submit_result", fields)
	return err
}

func (p *orderPlacer) log(event string, fields map[string]interface{}) {
	if p == nil || p.sink == nil {
		return
	}
	p.sink(event, fields)
}
