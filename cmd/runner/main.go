package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"market-maker-go/config"
	"market-maker-go/gateway"
	"market-maker-go/inventory"
	"market-maker-go/market"
	"market-maker-go/metrics"
	"market-maker-go/order"
	"market-maker-go/risk"
	"market-maker-go/sim"
	"market-maker-go/strategy"
	"market-maker-go/strategy/asmm"
)

func main() {
	configPath := flag.String("config", "", "path to config yaml")
	symbol := flag.String("symbol", "ETHUSDC", "交易对（例如 ETHUSDC）")
	dryRun := flag.Bool("dryRun", false, "enable dry-run mode")
	restRate := flag.Float64("restRate", 5, "REST 限流：每秒令牌数")
	restBurst := flag.Int("restBurst", 10, "REST 限流：最大突发令牌数")
	metricsAddr := flag.String("metricsAddr", ":8080", "address for prometheus metrics endpoint")
	flag.Parse()
	if *configPath == "" {
		// Try to find config in common locations
		possiblePaths := []string{
			"configs/config.yaml",
			"config.yaml",
			"../configs/config.yaml",
		}
		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				*configPath = path
				break
			}
		}
		if *configPath == "" {
			log.Fatalf("config file not found")
		}
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(*configPath)
	if err != nil {
		log.Fatalf("failed to get absolute path for config: %v", err)
	}

	cfg, err := config.Load(absPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	fmt.Printf("config loaded: %+v\n", cfg)

	// Start metrics server
	metrics.StartMetricsServer(*metricsAddr)
	fmt.Printf("metrics server started on %s\n", *metricsAddr)

	symbolUpper := strings.ToUpper(*symbol)
	symConf, ok := cfg.Symbols[symbolUpper]
	if !ok {
		log.Fatalf("symbol %s not found in config", symbolUpper)
	}
	
	// 创建策略工厂
	factory := strategy.NewStrategyFactory()
	
	// 根据配置创建策略
	var engine interface{}
	var stratParams config.StrategyParams
	
	if symConf.Strategy.Type == "asmm" {
		// 创建ASMM策略配置
		asmmConfig := asmm.DefaultASMMConfig()
		// 使用配置文件中的参数覆盖默认值
		if symConf.Strategy.QuoteIntervalMs > 0 {
			asmmConfig.QuoteIntervalMs = symConf.Strategy.QuoteIntervalMs
		}
		if symConf.Strategy.MinSpreadBps > 0 {
			asmmConfig.MinSpreadBps = symConf.Strategy.MinSpreadBps
		}
		if symConf.Strategy.MaxSpreadBps > 0 {
			asmmConfig.MaxSpreadBps = symConf.Strategy.MaxSpreadBps
		}
		if symConf.Strategy.MinSpacingBps > 0 {
			asmmConfig.MinSpacingBps = symConf.Strategy.MinSpacingBps
		}
		if symConf.Strategy.MaxLevels > 0 {
			asmmConfig.MaxLevels = symConf.Strategy.MaxLevels
		}
		if symConf.Strategy.BaseSize > 0 {
			asmmConfig.BaseSize = symConf.Strategy.BaseSize
		}
		if symConf.Strategy.SizeVolK >= 0 {
			asmmConfig.SizeVolK = symConf.Strategy.SizeVolK
		}
		if symConf.Strategy.TargetPosition != 0 {
			asmmConfig.TargetPosition = symConf.Strategy.TargetPosition
		}
		if symConf.Strategy.InvSoftLimit > 0 {
			asmmConfig.InvSoftLimit = symConf.Strategy.InvSoftLimit
		}
		if symConf.Strategy.InvHardLimit > 0 {
			asmmConfig.InvHardLimit = symConf.Strategy.InvHardLimit
		}
		if symConf.Strategy.InvSkewK >= 0 {
			asmmConfig.InvSkewK = symConf.Strategy.InvSkewK
		}
		if symConf.Strategy.VolK >= 0 {
			asmmConfig.VolK = symConf.Strategy.VolK
		}
		if symConf.Strategy.TrendSpreadMultiplier > 0 {
			asmmConfig.TrendSpreadMultiplier = symConf.Strategy.TrendSpreadMultiplier
		}
		if symConf.Strategy.HighVolSpreadMultiplier > 0 {
			asmmConfig.HighVolSpreadMultiplier = symConf.Strategy.HighVolSpreadMultiplier
		}
		
		engine, err = factory.CreateStrategy("asmm", asmmConfig)
		if err != nil {
			log.Fatalf("初始化ASMM策略失败: %v", err)
		}
		stratParams = symConf.Strategy
	} else {
		// 默认使用原有的网格策略
		engineConfig := strategy.EngineConfig{
			MinSpread:      symConf.Strategy.MinSpread,
			TargetPosition: symConf.Strategy.TargetPosition,
			MaxDrift:       symConf.Strategy.MaxDrift,
			BaseSize:       symConf.Strategy.BaseSize,
			EnableMultiLayer: symConf.Strategy.EnableMultiLayer,
			LayerCount:     symConf.Strategy.LayerCount,
			LayerSpacing:   symConf.Strategy.LayerSpacing,
		}
		engine, err = factory.CreateStrategy("grid", engineConfig)
		if err != nil {
			log.Fatalf("初始化策略失败: %v", err)
		}
		stratParams = symConf.Strategy
	}

	restClient := &gateway.BinanceRESTClient{
		BaseURL:      cfg.Gateway.BaseURL,
		APIKey:       cfg.Gateway.APIKey,
		Secret:       cfg.Gateway.APISecret,
		HTTPClient:   gateway.NewDefaultHTTPClient(),
		RecvWindowMs: 5000,
		Limiter:      gateway.NewTokenBucketLimiter(*restRate, *restBurst),
	}
	// 初始化指标收集器
	mc := &metricsCollector{
		quotesGenerated: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "mm_runner_quotes_generated_total",
			Help: "Total number of quotes generated",
		}, []string{"side"}),
		ordersPlaced: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "mm_runner_orders_placed_total",
			Help: "Total number of orders placed",
		}, []string{"side"}),
		fills: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "mm_runner_fills_total",
			Help: "Total number of fills",
		}, []string{"side"}),
		restRequests: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "mm_runner_rest_requests_total",
			Help: "Total number of REST requests",
		}, []string{"method"}),
		restErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "mm_runner_rest_errors_total",
			Help: "Total number of REST errors",
		}, []string{"method"}),
		restLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name: "mm_runner_rest_latency_seconds",
			Help: "REST request latency in seconds",
		}, []string{"method"}),
		wsConnects: promauto.NewCounter(prometheus.CounterOpts{
			Name: "mm_runner_ws_connects_total",
			Help: "Total number of WebSocket connects",
		}),
		wsFailures: promauto.NewCounter(prometheus.CounterOpts{
			Name: "mm_runner_ws_failures_total",
			Help: "Total number of WebSocket failures",
		}),
		midPrice: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "mm_runner_mid_price",
			Help: "Current mid price",
		}),
		position: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "mm_runner_position",
			Help: "Current position",
		}),
		pnl: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "mm_runner_pnl",
			Help: "Current PnL",
		}),
		riskRejects: promauto.NewCounter(prometheus.CounterOpts{
			Name: "mm_runner_risk_rejects_total",
			Help: "Total number of risk rejects",
		}),
		quotes: promauto.NewCounter(prometheus.CounterOpts{
			Name: "mm_runner_quotes_total",
			Help: "Total number of quotes",
		}),
	}
	
	// 初始化订单网关
	gw := &restOrderGateway{
		client:           restClient,
		dryRun:           *dryRun,
		symbolByID:       map[string]string{symbolUpper: symbolUpper},
		exchangeByClient: map[string]string{symbolUpper: "binance"},
		symbol:           symbolUpper,
		metrics:          mc, // 注入 metricsCollector
	}
	mgr := order.NewManager(gw)
	symbolConstraints := make(map[string]order.SymbolConstraints)
	for sym, sc := range cfg.Symbols {
		symbolConstraints[strings.ToUpper(sym)] = order.SymbolConstraints{
			TickSize:    sc.TickSize,
			StepSize:    sc.StepSize,
			MinQty:      sc.MinQty,
			MaxQty:      sc.MaxQty,
			MinNotional: sc.MinNotional,
		}
	}
	mgr.SetConstraints(symbolConstraints)

	inv := &inventory.Tracker{}
	book := market.NewOrderBook()
	
	// 类型断言获取正确的策略引擎
	var strategyEngine *strategy.Engine
	var asmmStrategy *asmm.ASMMStrategy
	
	if symConf.Strategy.Type == "asmm" {
		asmmStrategy = engine.(*asmm.ASMMStrategy)
	} else {
		strategyEngine = engine.(*strategy.Engine)
	}
	
	runner := sim.Runner{
		Symbol:   symbolUpper,
		Engine:   strategyEngine,
		ASMMStrategy: asmmStrategy,
		Inv:      inv,
		OrderMgr: mgr,
		Book:     book,
	}
	if sc, ok := symbolConstraints[symbolUpper]; ok {
		runner.Constraints = sc
	}
	runner.StopLoss = symConf.Risk.StopLoss
	runner.ShockThreshold = symConf.Risk.ShockPct
	runner.ReduceOnlyThreshold = symConf.Risk.ReduceOnlyThreshold
	runner.ReduceOnlyMaxSlippage = symConf.Risk.ReduceOnlyMaxSlippagePct
	if symConf.Risk.HaltSeconds > 0 {
		runner.HaltDuration = time.Duration(symConf.Risk.HaltSeconds) * time.Second
	}
	// 构造风控 Guard
	var guards []risk.Guard
	riskConf := symConf.Risk
	limits := &risk.Limits{
		SingleMax: riskConf.SingleMax,
		DailyMax:  riskConf.DailyMax,
		NetMax:    riskConf.NetMax,
	}
	guards = append(guards, risk.NewLimitChecker(limits, &trackerInventory{tr: inv}))
	if riskConf.LatencyMs > 0 {
		guards = append(guards, risk.NewLatencyGuard(time.Duration(riskConf.LatencyMs)*time.Millisecond))
	}
	if riskConf.PnLMin != 0 || riskConf.PnLMax != 0 {
		guards = append(guards, &risk.PnLGuard{
			MinPnL: riskConf.PnLMin,
			MaxPnL: riskConf.PnLMax,
			Source: risk.InventoryPnL{
				Tracker: inv,
				MidFn:   book.Mid,
			},
		})
	}
	runner.Risk = risk.MultiGuard{Guards: guards}
	runner.SetRiskStateListener(func(state sim.RiskState, reason string) {
		fields := map[string]interface{}{
			"symbol": symbolUpper,
			"state":  state.String(),
		}
		if reason != "" {
			fields["reason"] = reason
		}
		// metrics.VolatilityRegime.Set(float64(state))
		logEvent("risk_state_change", fields)
	})
	// info := engine.Info()
	// metrics.SpreadGauge.Set(info.Spread)
	// metrics.QuoteIntervalGauge.Set(info.Interval.Seconds())
	// TODO: 修复这些指标引用
	// metrics.SpreadGauge.WithLabelValues(symbolUpper).Set(0) // Placeholder
	// metrics.QuoteIntervalGauge.WithLabelValues(symbolUpper).Set(0) // Placeholder
	runner.SetStrategyAdjustListener(func(info sim.StrategyAdjustInfo) {
		metrics.SpreadGauge.WithLabelValues(symbolUpper).Set(info.Spread)
		metrics.QuoteIntervalGauge.WithLabelValues(symbolUpper).Set(info.Interval.Seconds())
		fields := map[string]interface{}{
			"symbol":          symbolUpper,
			"mid":             info.Mid,
			"spread":          info.Spread,
			"spreadRatio":     info.SpreadRatio,
			"volFactor":       info.VolFactor,
			"inventoryFactor": info.InventoryFactor,
			"intervalMs":      info.Interval.Milliseconds(),
			"net":             info.NetExposure,
			"reduceOnly":      info.ReduceOnly,
		}
		if info.TakeProfitActive {
			fields["takeProfit"] = true
		}
		if info.DepthFillPrice > 0 {
			fields["depthFillPrice"] = info.DepthFillPrice
			fields["depthFillAvailable"] = info.DepthFillAvailable
		}
		if info.DepthSlippage > 0 {
			fields["depthSlippage"] = info.DepthSlippage
		}
		logEvent("strategy_adjust", fields)
	})

	if bid, ask, err := restClient.GetBestBidAsk(symbolUpper, 50); err != nil {
		logEvent("depth_snapshot_error", map[string]interface{}{"symbol": symbolUpper, "error": err.Error()})
	} else {
		book.ApplyDelta(map[float64]float64{bid: 1}, map[float64]float64{ask: 1})
		logEvent("depth_snapshot", map[string]interface{}{"symbol": symbolUpper, "bid": bid, "ask": ask})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 初始化 listenKey + WS
	var ws *gateway.BinanceWSReal
	var listenKey string
	
	if !*dryRun {
		lkClient := &gateway.ListenKeyClient{
			BaseURL:    cfg.Gateway.BaseURL,
			APIKey:     cfg.Gateway.APIKey,
			HTTPClient: gateway.NewListenKeyHTTPClient(),
		}
		var err error
		listenKey, err = lkClient.NewListenKey()
		if err != nil {
			log.Fatalf("创建 listenKey 失败: %v", err)
		}
		logEvent("listenkey_created", map[string]interface{}{"listenKey": listenKey})
		defer lkClient.CloseListenKey(listenKey)
		go keepAliveLoop(ctx, lkClient, listenKey)

		depthHandler := &gateway.BinanceWSHandler{Book: book}
		userHandler := &gateway.BinanceUserHandler{
			OnOrderUpdate: func(o gateway.OrderUpdate) {
				switch o.Status {
				case "FILLED":
					_ = mgr.Update(o.ClientOrderID, order.StatusFilled)
				case "PARTIALLY_FILLED":
					_ = mgr.Update(o.ClientOrderID, order.StatusPartial)
				case "CANCELED":
					_ = mgr.Update(o.ClientOrderID, order.StatusCanceled)
				case "REJECTED":
					_ = mgr.Update(o.ClientOrderID, order.StatusRejected)
				case "EXPIRED", "EXPIRED_IN_MATCH", "EXPIRED_IN_CANCEL":
					_ = mgr.Update(o.ClientOrderID, order.StatusExpired)
				}
				logEvent("order_update", map[string]interface{}{
					"symbol":        o.Symbol,
					"status":        o.Status,
					"clientOrderId": o.ClientOrderID,
					"orderId":       o.OrderID,
					"lastQty":       o.LastFilledQty,
					"lastPrice":     o.LastFilledPrice,
					"pnl":           o.RealizedPnL,
				})
			},
			OnAccountUpdate: func(a gateway.AccountUpdate) {
				for _, p := range a.Positions {
					if strings.ToUpper(p.Symbol) == symbolUpper {
						inv.SetExposure(p.PositionAmt, p.EntryPrice)
					}
				}
				logEvent("account_update", map[string]interface{}{"reason": a.Reason})
			},
		}
		wsMux := &wsMultiplexer{depth: depthHandler, user: userHandler}
		ws = gateway.NewBinanceWSReal()
		ws.OnConnect(func() {
			mc.wsConnects.Inc()
			logEvent("ws_connect", map[string]interface{}{"symbol": symbolUpper})
		})
		ws.OnDisconnect(func(err error) {
			mc.wsFailures.Inc()
			logEvent("ws_disconnect", map[string]interface{}{"error": err.Error()})
		})
		if err := ws.SubscribeDepth(symbolUpper); err != nil {
			log.Fatalf("订阅 depth 失败: %v", err)
		}
		if err := ws.SubscribeUserData(listenKey); err != nil {
			log.Fatalf("订阅用户流失败: %v", err)
		}
		go func() {
			if err := ws.Run(wsMux); err != nil {
				logEvent("ws_exit", map[string]interface{}{"error": err.Error()})
				cancel()
			}
		}()
	} else {
		// 在dryRun模式下，使用模拟数据填充订单簿
		log.Println("Dry-run mode: using simulated order book data")
		// 模拟初始订单簿数据
		book.SetBest(2940.0, 2941.0)
	}

	quoteInterval := time.Duration(stratParams.QuoteIntervalMs) * time.Millisecond
	if quoteInterval <= 0 {
		quoteInterval = 2 * time.Second
	}
	runner.BaseInterval = quoteInterval
	runner.BaseSpread = stratParams.MinSpread
	runner.TakeProfitPct = stratParams.TakeProfitPct
	runner.NetMax = symConf.Risk.NetMax
	if stratParams.BaseSize > 0 && symConf.Risk.ReduceOnlyThreshold > 0 {
		reduceCap := stratParams.BaseSize * symConf.Risk.ReduceOnlyThreshold
		if runner.NetMax == 0 || reduceCap < runner.NetMax {
			runner.NetMax = reduceCap
		}
	}
	if runner.NetMax <= 0 {
		runner.NetMax = symConf.Risk.NetMax
	}
	runner.StaticFraction = stratParams.StaticFraction
	runner.StaticThresholdTicks = stratParams.StaticTicks
	runner.ReduceOnlyMarketTrigger = symConf.Risk.ReduceOnlyMarketTriggerPct
	if stratParams.StaticRestMs > 0 {
		runner.StaticRestDuration = time.Duration(stratParams.StaticRestMs) * time.Millisecond
	} else if runner.BaseInterval > 0 {
		runner.StaticRestDuration = 2 * runner.BaseInterval
	}
	if stratParams.DynamicRestMs > 0 {
		runner.DynamicRestDuration = time.Duration(stratParams.DynamicRestMs) * time.Millisecond
	}
	if stratParams.DynamicRestTicks > 0 {
		runner.DynamicThresholdTicks = stratParams.DynamicRestTicks
	}

	go func() {
		baseStale := 1200 * time.Millisecond
		step := quoteInterval / 2
		if step <= 0 {
			step = quoteInterval
		}
		ticker := time.NewTicker(step)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mid := book.Mid()
				// WebSocket应该实时更新book，只在数据过期时才用REST备份
				staleThreshold := baseStale
				if runner.RiskStateUnsafe() == sim.RiskStateReduceOnly {
					staleThreshold = 500 * time.Millisecond
				}
				// 只有当book数据过期或无效时才回退到REST API
				if mid == 0 || time.Since(book.LastUpdate()) > staleThreshold {
					if restClient != nil {
						if bid, ask, err := restClient.GetBestBidAsk(symbolUpper, 5); err == nil {
							book.SetBest(bid, ask)
							mid = book.Mid()
						} else {
							logEvent("depth_refresh_error", map[string]interface{}{"symbol": symbolUpper, "error": err.Error()})
						}
					}
				}
				if mid == 0 {
					continue
				}
				if !runner.ReadyForNext(mid) {
					continue
				}
				mc.midPrice.Set(mid)
				net, pnl := inv.Valuation(mid)
				mc.position.Set(net)
				mc.pnl.Set(pnl)
				if err := runner.OnTick(mid); err != nil {
					mc.riskRejects.Inc()
					logEvent("quote_error", map[string]interface{}{"symbol": symbolUpper, "error": err.Error()})
				} else {
					mc.quotes.Inc()
				}
			}
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	cancel()
	logEvent("runner_exit", map[string]interface{}{"symbol": symbolUpper})
}

type restOrderGateway struct {
	client           *gateway.BinanceRESTClient
	dryRun           bool
	symbolByID       map[string]string
	exchangeByClient map[string]string
	mu               sync.Mutex
	symbol           string          // 添加 symbol 字段用于指标标签
	metrics          *metricsCollector // 注入 metricsCollector
}

func (g *restOrderGateway) Place(o order.Order) (string, error) {
	start := time.Now()
	g.metrics.restRequests.WithLabelValues("place").Inc()
	// metrics.RestRequestsCounter.WithLabelValues("place", g.symbol).Inc()

	if g.dryRun {
		// Dry-run mode: simulate order placement
		time.Sleep(50 * time.Millisecond)
		orderID := fmt.Sprintf("dryrun_%d", time.Now().UnixNano())
		g.metrics.restLatency.WithLabelValues("place").Observe(time.Since(start).Seconds())
		// metrics.RestLatencyHistogram.WithLabelValues("place", g.symbol).Observe(time.Since(start).Seconds())
		g.metrics.incOrdersPlaced(string(o.Side))
		// metrics.OrdersPlacedCounter.WithLabelValues(g.symbol).Inc()
		return orderID, nil
	}

	// Real mode: place order via Binance REST API
	clientOrderID := fmt.Sprintf("mm_%d", time.Now().UnixNano())
	exchangeOrderID, err := g.client.PlaceLimit(g.symbolByID[g.symbol], string(o.Side), "GTC", o.Price, o.Quantity, false, o.PostOnly, clientOrderID)
	if err != nil {
		g.metrics.restErrors.WithLabelValues("place").Inc()
		g.metrics.restLatency.WithLabelValues("place").Observe(time.Since(start).Seconds())
		// metrics.RestErrorsCounter.WithLabelValues("place", g.symbol).Inc()
		// metrics.RestLatencyHistogram.WithLabelValues("place", g.symbol).Observe(time.Since(start).Seconds())
		return "", err
	}

	g.metrics.restLatency.WithLabelValues("place").Observe(time.Since(start).Seconds())
	// metrics.RestLatencyHistogram.WithLabelValues("place", g.symbol).Observe(time.Since(start).Seconds())
	g.metrics.incOrdersPlaced(string(o.Side))
	// metrics.OrdersPlacedCounter.WithLabelValues(g.symbol).Inc()
	return exchangeOrderID, nil
}

func (g *restOrderGateway) Cancel(clientOrderID string) error {
	start := time.Now()
	g.metrics.restRequests.WithLabelValues("cancel").Inc()
	// metrics.RestRequestsCounter.WithLabelValues("cancel", g.symbol).Inc()

	if g.dryRun {
		// Dry-run mode: simulate order cancellation
		time.Sleep(50 * time.Millisecond)
		g.metrics.restLatency.WithLabelValues("cancel").Observe(time.Since(start).Seconds())
		// metrics.RestLatencyHistogram.WithLabelValues("cancel", g.symbol).Observe(time.Since(start).Seconds())
		return nil
	}

	// Real mode: cancel order via Binance REST API
	exchangeID, _ := g.lookupMapping(clientOrderID)
	if exchangeID == "" {
		return fmt.Errorf("无法找到订单映射: %s", clientOrderID)
	}
	err := g.client.CancelOrder(g.symbolByID[g.symbol], exchangeID)
	if err != nil {
		g.metrics.restErrors.WithLabelValues("cancel").Inc()
		g.metrics.restLatency.WithLabelValues("cancel").Observe(time.Since(start).Seconds())
		// metrics.RestErrorsCounter.WithLabelValues("cancel", g.symbol).Inc()
		// metrics.RestLatencyHistogram.WithLabelValues("cancel", g.symbol).Observe(time.Since(start).Seconds())
		return err
	}

	g.metrics.restLatency.WithLabelValues("cancel").Observe(time.Since(start).Seconds())
	// metrics.RestLatencyHistogram.WithLabelValues("cancel", g.symbol).Observe(time.Since(start).Seconds())
	return nil
}

type wsMultiplexer struct {
	depth *gateway.BinanceWSHandler
	user  *gateway.BinanceUserHandler
}

func (m *wsMultiplexer) OnDepth(symbol string, bid, ask float64) {}

func (m *wsMultiplexer) OnTrade(symbol string, price, qty float64) {}

func (m *wsMultiplexer) OnRawMessage(msg []byte) {
	if m.user != nil {
		m.user.OnRawMessage(msg)
	}
	if m.depth != nil {
		m.depth.OnRawMessage(msg)
	}
}

func keepAliveLoop(ctx context.Context, cli *gateway.ListenKeyClient, key string) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := cli.KeepAlive(key); err != nil {
				logEvent("listenkey_keepalive_error", map[string]interface{}{"listenKey": key, "error": err.Error()})
				time.Sleep(5 * time.Second)
				if err = cli.KeepAlive(key); err != nil {
					logEvent("listenkey_keepalive_retry_failed", map[string]interface{}{"listenKey": key, "error": err.Error()})
				} else {
					logEvent("listenkey_keepalive_retry_ok", map[string]interface{}{"listenKey": key})
				}
			} else {
				logEvent("listenkey_keepalive_ok", map[string]interface{}{"listenKey": key})
			}
		}
	}
}

func (g *restOrderGateway) storeMapping(clientID, exchangeID, symbol string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if clientID != "" && exchangeID != "" {
		g.exchangeByClient[clientID] = exchangeID
	}
	if exchangeID != "" {
		g.symbolByID[exchangeID] = symbol
	}
}

func (g *restOrderGateway) lookupMapping(id string) (exchangeID, symbol string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	// id 可能是 exchangeID 或 clientID
	if sym, ok := g.symbolByID[id]; ok {
		return id, sym
	}
	if exch, ok := g.exchangeByClient[id]; ok {
		return exch, g.symbolByID[exch]
	}
	return "", ""
}

type trackerInventory struct {
	tr *inventory.Tracker
}

func (t *trackerInventory) NetExposure() float64 {
	return t.tr.NetExposure()
}

func (t *trackerInventory) Position() float64 {
	return t.tr.NetExposure()
}

func (t *trackerInventory) AddFilled(symbol string, qty float64) {
	// 这里我们简化处理，忽略价格参数
	// 在实际应用中，您可能需要从其他地方获取价格信息
	price := 0.0 // 占位符价格
	var delta float64
	// 假设我们可以通过某种方式确定订单方向，这里简化处理
	if qty > 0 {
		delta = qty
	} else {
		delta = -qty
	}
	t.tr.Update(delta, price)
}

func (t *trackerInventory) GetDailyFilled(symbol string) float64 {
	// 简化实现：返回0
	return 0
}

func logEvent(eventType string, fields map[string]interface{}) {
	// 简化实现，直接打印到控制台
	fmt.Printf("[%s] %s\n", eventType, fmt.Sprintf("%+v", fields))
}

func isErrorEvent(event string, fields map[string]interface{}) bool {
	if strings.Contains(event, "error") {
		return true
	}
	if _, ok := fields["error"]; ok {
		return true
	}
	return false
}

func appendErrorLog(line []byte) {
	const errorLogPath = "/var/log/market-maker/runner_errors.log"
	f, err := os.OpenFile(errorLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0664)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return
	}
}

type metricsCollector struct {
	quotesGenerated *prometheus.CounterVec
	ordersPlaced    *prometheus.CounterVec
	fills           *prometheus.CounterVec
	restRequests    *prometheus.CounterVec
	restErrors      *prometheus.CounterVec
	restLatency     *prometheus.HistogramVec
	wsConnects      prometheus.Counter
	wsFailures      prometheus.Counter
	midPrice        prometheus.Gauge
	position        prometheus.Gauge
	pnl             prometheus.Gauge
	riskRejects     prometheus.Counter
	quotes          prometheus.Counter
}

func (m *metricsCollector) incQuotesGenerated(side string) {
	m.quotesGenerated.WithLabelValues(side).Inc()
}

func (m *metricsCollector) incOrdersPlaced(side string) {
	m.ordersPlaced.WithLabelValues(side).Inc()
}

func (m *metricsCollector) incFills(side string) {
	m.fills.WithLabelValues(side).Inc()
}
