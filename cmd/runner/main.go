package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"market-maker-go/config"
	"market-maker-go/gateway"
	"market-maker-go/inventory"
	"market-maker-go/market"
	"market-maker-go/monitor/logschema"
	"market-maker-go/order"
	"market-maker-go/risk"
	"market-maker-go/sim"
	"market-maker-go/strategy"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	symbol := flag.String("symbol", "ETHUSDC", "交易对（例如 ETHUSDC）")
	dryRun := flag.Bool("dryRun", false, "仅日志输出，不真正下单")
	restRate := flag.Float64("restRate", 5, "REST 限流：每秒令牌数")
	restBurst := flag.Int("restBurst", 10, "REST 限流：最大突发令牌数")
	metricsAddr := flag.String("metricsAddr", ":9100", "Prometheus metrics 监听地址，留空则关闭")
	flag.Parse()

	cfg, err := config.LoadWithEnvOverrides(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	symbolUpper := strings.ToUpper(*symbol)
	metrics := newMetrics(symbolUpper)
	go serveMetrics(*metricsAddr)

	symConf, ok := cfg.Symbols[symbolUpper]
	if !ok {
		log.Fatalf("symbol %s not found in config", symbolUpper)
	}
	strat := symConf.Strategy
	engine, err := strategy.NewEngine(strategy.EngineConfig{
		MinSpread:      strat.MinSpread,
		TargetPosition: strat.TargetPosition,
		MaxDrift:       strat.MaxDrift,
		BaseSize:       strat.BaseSize,
	})
	if err != nil {
		log.Fatalf("初始化策略失败: %v", err)
	}

	restClient := &gateway.BinanceRESTClient{
		BaseURL:      cfg.Gateway.BaseURL,
		APIKey:       cfg.Gateway.APIKey,
		Secret:       cfg.Gateway.APISecret,
		HTTPClient:   gateway.NewDefaultHTTPClient(),
		RecvWindowMs: 5000,
		Limiter:      gateway.NewTokenBucketLimiter(*restRate, *restBurst),
	}
	orderGateway := &restOrderGateway{
		client:           restClient,
		dryRun:           *dryRun,
		symbolByID:       make(map[string]string),
		exchangeByClient: make(map[string]string),
		metrics:          metrics,
	}
	mgr := order.NewManager(orderGateway)
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
	runner := sim.Runner{
		Symbol:   symbolUpper,
		Engine:   engine,
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
		metrics.riskState.Set(float64(state))
		logEvent("risk_event", fields)
	})
	runner.SetStrategyAdjustListener(func(info sim.StrategyAdjustInfo) {
		metrics.spread.Set(info.Spread)
		metrics.quoteInterval.Set(info.Interval.Seconds())
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
	lkClient := &gateway.ListenKeyClient{
		BaseURL:    cfg.Gateway.BaseURL,
		APIKey:     cfg.Gateway.APIKey,
		HTTPClient: gateway.NewListenKeyHTTPClient(),
	}
	listenKey, err := lkClient.NewListenKey()
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
	ws := gateway.NewBinanceWSReal()
	ws.OnConnect(func() {
		metrics.wsConnects.Inc()
		logEvent("ws_connect", map[string]interface{}{"symbol": symbolUpper})
	})
	ws.OnDisconnect(func(err error) {
		metrics.wsFailures.Inc()
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

	quoteInterval := time.Duration(strat.QuoteIntervalMs) * time.Millisecond
	if quoteInterval <= 0 {
		quoteInterval = 2 * time.Second
	}
	runner.BaseInterval = quoteInterval
	runner.BaseSpread = strat.MinSpread
	runner.TakeProfitPct = strat.TakeProfitPct
	runner.NetMax = symConf.Risk.NetMax
	if strat.BaseSize > 0 && symConf.Risk.ReduceOnlyThreshold > 0 {
		reduceCap := strat.BaseSize * symConf.Risk.ReduceOnlyThreshold
		if runner.NetMax == 0 || reduceCap < runner.NetMax {
			runner.NetMax = reduceCap
		}
	}
	if runner.NetMax <= 0 {
		runner.NetMax = symConf.Risk.NetMax
	}
	runner.StaticFraction = strat.StaticFraction
	runner.StaticThresholdTicks = strat.StaticTicks
	runner.ReduceOnlyMarketTrigger = symConf.Risk.ReduceOnlyMarketTriggerPct
	if strat.StaticRestMs > 0 {
		runner.StaticRestDuration = time.Duration(strat.StaticRestMs) * time.Millisecond
	} else if runner.BaseInterval > 0 {
		runner.StaticRestDuration = 2 * runner.BaseInterval
	}
	if strat.DynamicRestMs > 0 {
		runner.DynamicRestDuration = time.Duration(strat.DynamicRestMs) * time.Millisecond
	}
	if strat.DynamicRestTicks > 0 {
		runner.DynamicThresholdTicks = strat.DynamicRestTicks
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
				metrics.midPrice.Set(mid)
				net, pnl := inv.Valuation(mid)
				metrics.position.Set(net)
				metrics.pnl.Set(pnl)
				if err := runner.OnTick(mid); err != nil {
					metrics.riskRejects.Inc()
					logEvent("quote_error", map[string]interface{}{"symbol": symbolUpper, "error": err.Error()})
				} else {
					metrics.quotes.Inc()
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
	metrics          *metricsCollector
}

func (g *restOrderGateway) Place(o order.Order) (string, error) {
	start := time.Now()
	g.metrics.restRequests.WithLabelValues("place").Inc()
	typ := strings.ToUpper(o.Type)
	if typ == "" {
		typ = "LIMIT"
	}
	if g.dryRun {
		event := map[string]interface{}{
			"symbol": o.Symbol, "side": o.Side, "qty": o.Quantity,
		}
		if typ == "LIMIT" {
			event["price"] = o.Price
		} else {
			event["type"] = "MARKET"
		}
		logEvent("order_place_dry_run", event)
		g.storeMapping(o.ID, o.ID, o.Symbol)
		g.metrics.restLatency.WithLabelValues("place").Observe(time.Since(start).Seconds())
		g.metrics.ordersPlaced.Inc()
		return o.ID, nil
	}
	side := strings.ToUpper(o.Side)
	var orderID string
	var err error
	switch typ {
	case "MARKET":
		if o.Quantity <= 0 {
			return "", fmt.Errorf("market qty must be > 0")
		}
		orderID, err = g.client.PlaceMarket(o.Symbol, side, o.Quantity, o.ReduceOnly, o.ID)
		if err == nil {
			logEvent("order_place", map[string]interface{}{
				"symbol": o.Symbol, "side": side, "qty": o.Quantity, "orderId": orderID, "type": "MARKET",
			})
		}
	default:
		postOnly := o.PostOnly
		tif := o.TimeInForce
		if tif == "" {
			if o.ReduceOnly && !postOnly {
				tif = "IOC"
			} else {
				tif = "GTC"
			}
		}
		orderID, err = g.client.PlaceLimit(o.Symbol, side, tif, o.Price, o.Quantity, o.ReduceOnly, postOnly, o.ID)
		if err == nil {
			logEvent("order_place", map[string]interface{}{
				"symbol": o.Symbol, "side": side, "price": o.Price, "qty": o.Quantity, "orderId": orderID,
			})
		}
	}
	if err != nil {
		g.metrics.restErrors.WithLabelValues("place").Inc()
		g.metrics.restLatency.WithLabelValues("place").Observe(time.Since(start).Seconds())
		return "", err
	}
	g.storeMapping(o.ID, orderID, o.Symbol)
	g.metrics.restLatency.WithLabelValues("place").Observe(time.Since(start).Seconds())
	g.metrics.ordersPlaced.Inc()
	return orderID, nil
}

func (g *restOrderGateway) Cancel(id string) error {
	start := time.Now()
	g.metrics.restRequests.WithLabelValues("cancel").Inc()
	if g.dryRun {
		logEvent("order_cancel_dry_run", map[string]interface{}{"orderId": id})
		g.metrics.restLatency.WithLabelValues("cancel").Observe(time.Since(start).Seconds())
		return nil
	}
	exchID, symbol := g.lookupMapping(id)
	if symbol == "" {
		return nil
	}
	if err := g.client.CancelOrder(symbol, exchID); err != nil {
		g.metrics.restErrors.WithLabelValues("cancel").Inc()
		g.metrics.restLatency.WithLabelValues("cancel").Observe(time.Since(start).Seconds())
		return err
	}
	logEvent("order_cancel", map[string]interface{}{"orderId": id, "symbol": symbol})
	g.metrics.restLatency.WithLabelValues("cancel").Observe(time.Since(start).Seconds())
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

func (t trackerInventory) NetExposure(symbol string) float64 {
	return t.tr.NetExposure()
}

func logEvent(event string, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	if err := logschema.Validate(event, fields); err != nil {
		fields["_schema_error"] = err.Error()
	}
	fields["event"] = event
	fields["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.Marshal(fields)
	if err != nil {
		log.Printf("%s %+v", event, fields)
		return
	}
	log.Println(string(data))
	if isErrorEvent(event, fields) {
		appendErrorLog(data)
	}
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
	restRequests  *prometheus.CounterVec
	restErrors    *prometheus.CounterVec
	restLatency   *prometheus.HistogramVec
	wsConnects    prometheus.Counter
	wsFailures    prometheus.Counter
	ordersPlaced  prometheus.Counter
	riskRejects   prometheus.Counter
	quotes        prometheus.Counter
	position      prometheus.Gauge
	pnl           prometheus.Gauge
	midPrice      prometheus.Gauge
	riskState     prometheus.Gauge
	spread        prometheus.Gauge
	quoteInterval prometheus.Gauge
}

func newMetrics(symbol string) *metricsCollector {
	return &metricsCollector{
		restRequests: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "runner_rest_requests_total",
			Help: "REST 请求数量",
		}, []string{"action"}),
		restErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "runner_rest_errors_total",
			Help: "REST 错误数量",
		}, []string{"action"}),
		restLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "runner_rest_latency_seconds",
			Help:    "REST 请求耗时",
			Buckets: prometheus.DefBuckets,
		}, []string{"action"}),
		wsConnects: promauto.NewCounter(prometheus.CounterOpts{
			Name: "runner_ws_connects_total",
			Help: "WS 连接次数",
		}),
		wsFailures: promauto.NewCounter(prometheus.CounterOpts{
			Name: "runner_ws_failures_total",
			Help: "WS 失败次数",
		}),
		ordersPlaced: promauto.NewCounter(prometheus.CounterOpts{
			Name: "runner_orders_placed_total",
			Help: "策略下单数量",
		}),
		riskRejects: promauto.NewCounter(prometheus.CounterOpts{
			Name: "runner_risk_rejects_total",
			Help: "风控拒单数量",
		}),
		quotes: promauto.NewCounter(prometheus.CounterOpts{
			Name: "runner_quotes_total",
			Help: "策略报价次数",
		}),
		position: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "runner_position",
			Help: "当前净仓位",
		}),
		pnl: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "runner_unrealized_pnl",
			Help: "当前未实现盈亏",
		}),
		midPrice: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "runner_mid_price",
			Help: "策略使用的 mid 价格",
		}),
		riskState: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "runner_risk_state",
			Help: "风险状态(0=normal,1=reduce_only,2=halted)",
		}),
		spread: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "runner_spread",
			Help: "当前挂单价差",
		}),
		quoteInterval: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "runner_quote_interval_seconds",
			Help: "当前动态报价间隔(秒)",
		}),
	}
}

func serveMetrics(addr string) {
	if addr == "" {
		return
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	go func() {
		logEvent("metrics_listen", map[string]interface{}{"addr": addr})
		if err := http.ListenAndServe(addr, mux); err != nil {
			logEvent("metrics_error", map[string]interface{}{"error": err.Error()})
		}
	}()
}
