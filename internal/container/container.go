package container

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"

	"market-maker-go/config"
	"market-maker-go/gateway"
	"market-maker-go/infrastructure/logger"
	"market-maker-go/infrastructure/monitor"
	"market-maker-go/inventory"
	"market-maker-go/market"
	"market-maker-go/order"
)

// Container 依赖注入容器，管理所有组件的生命周期
type Container struct {
	// 配置
	cfg *config.AppConfig

	// 基础设施
	logger  *logger.Logger
	monitor *monitor.Monitor

	// 交易所网关
	restClient *gateway.BinanceRESTClient

	// 核心服务
	marketData   *market.Service
	inventory    *inventory.Tracker
	orderManager *order.Manager

	// HTTP服务器
	metricsServer *http.Server

	// 生命周期管理
	lifecycle *LifecycleManager
}

// New 创建新的Container实例
func New(configPath string) (*Container, error) {
	cfg, err := config.LoadWithEnvOverrides(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config failed: %w", err)
	}

	return &Container{
		cfg:       &cfg,
		lifecycle: NewLifecycleManager(),
	}, nil
}

// Build 构建所有组件
func (c *Container) Build() error {
	if err := c.buildInfrastructure(); err != nil {
		return fmt.Errorf("build infrastructure failed: %w", err)
	}

	if err := c.buildGateway(); err != nil {
		return fmt.Errorf("build gateway failed: %w", err)
	}

	if err := c.buildCoreServices(); err != nil {
		return fmt.Errorf("build core services failed: %w", err)
	}

	c.registerLifecycleComponents()
	c.logger.Info("container built successfully")
	return nil
}

func (c *Container) buildInfrastructure() error {
	logCfg := logger.Config{
		Level:      "info",
		Outputs:    []string{"stdout", "file"},
		OutputFile: "/var/log/market-maker/trader.log",
		ErrorFile:  "/var/log/market-maker/trader_errors.log",
		Format:     "json",
	}

	var err error
	c.logger, err = logger.New(logCfg)
	if err != nil {
		return fmt.Errorf("create logger failed: %w", err)
	}

	monitorCfg := monitor.DefaultConfig()
	c.monitor = monitor.New(monitorCfg)

	c.logger.Info("infrastructure built")
	return nil
}

func (c *Container) buildGateway() error {
	c.restClient = &gateway.BinanceRESTClient{
		BaseURL:      c.cfg.Gateway.BaseURL,
		APIKey:       c.cfg.Gateway.APIKey,
		Secret:       c.cfg.Gateway.APISecret,
		HTTPClient:   gateway.NewDefaultHTTPClient(),
		RecvWindowMs: 5000,
		Limiter:      gateway.NewCompositeLimiter(25.0, 50, 280, 2200),
		MaxRetries:   3,
		RetryDelay:   200 * time.Millisecond,
	}

	c.logger.Info("gateway built")
	return nil
}

func (c *Container) buildCoreServices() error {
	publisher := market.NewPublisher()
	c.marketData = market.NewService(publisher)

	orderGw := &orderGatewayAdapter{
		client:  c.restClient,
		logger:  c.logger,
		monitor: c.monitor,
	}
	c.orderManager = order.NewManager(orderGw)

	symbolConstraints := make(map[string]order.SymbolConstraints)
	for sym, sc := range c.cfg.Symbols {
		symbolConstraints[sym] = order.SymbolConstraints{
			TickSize:    sc.TickSize,
			StepSize:    sc.StepSize,
			MinQty:      sc.MinQty,
			MaxQty:      sc.MaxQty,
			MinNotional: sc.MinNotional,
		}
	}
	c.orderManager.SetConstraints(symbolConstraints)

	c.logger.Info("core services built")
	return nil
}

func (c *Container) registerLifecycleComponents() {
	if c.monitor != nil {
		c.lifecycle.Register(&httpServerComponent{
			name:    "metrics_server",
			handler: c.monitor.Handler(),
			addr:    ":9100",
			logger:  c.logger,
			server:  &c.metricsServer,
		})
	}
}

func (c *Container) Start(ctx context.Context) error {
	c.logger.Info("starting container...")

	if err := c.lifecycle.StartAll(ctx); err != nil {
		return fmt.Errorf("start failed: %w", err)
	}

	c.logger.Info("container started")
	return nil
}

func (c *Container) Stop() error {
	c.logger.Info("stopping container...")

	if err := c.lifecycle.StopAll(); err != nil {
		c.logger.LogError(err, map[string]interface{}{"action": "stop"})
		return err
	}

	// 安全清场：撤单 + 平仓
	for sym := range c.cfg.Symbols {
		// 撤销所有挂单
		if err := c.restClient.CancelAll(sym); err != nil {
			c.logger.LogError(err, map[string]interface{}{"action": "cancel_all", "symbol": sym})
		} else {
			c.logger.Logger.Info(fmt.Sprintf("[%s] 所有挂单已撤销", sym))
		}
		// 查询持仓并平仓（reduce-only 市价）
		positions, err := c.restClient.PositionRisk(sym)
		if err != nil {
			c.logger.LogError(err, map[string]interface{}{"action": "position_risk", "symbol": sym})
			continue
		}
		var net float64
		for _, p := range positions {
			net += p.PositionAmt
		}
		if math.Abs(net) > 1e-8 {
			side := "SELL"
			qty := math.Abs(net)
			if net < 0 { side = "BUY" }
			if _, err := c.restClient.PlaceMarket(sym, side, qty, true, "shutdown-clean"); err != nil {
				c.logger.LogError(err, map[string]interface{}{"action": "flatten", "symbol": sym, "qty": qty})
			} else {
				c.logger.Logger.Info(fmt.Sprintf("[%s] 已提交 reduce-only %s 市价平仓，数量 %.6f", sym, side, qty))
			}
		}
	}

	if c.logger != nil {
		c.logger.Close()
	}

	return nil
}

func (c *Container) HealthCheck() error {
	return c.lifecycle.CheckHealth()
}

// orderGatewayAdapter 适配器
type orderGatewayAdapter struct {
	client  *gateway.BinanceRESTClient
	logger  *logger.Logger
	monitor *monitor.Monitor
}

func (a *orderGatewayAdapter) Place(o order.Order) (string, error) {
	start := time.Now()
	a.monitor.RecordRESTRequest("place")

	var orderID string
	var err error

	if o.Type == "MARKET" {
		orderID, err = a.client.PlaceMarket(o.Symbol, o.Side, o.Quantity, o.ReduceOnly, o.ID)
	} else {
		tif := o.TimeInForce
		if tif == "" {
			tif = "GTC"
		}
		orderID, err = a.client.PlaceLimit(o.Symbol, o.Side, tif, o.Price, o.Quantity, o.ReduceOnly, o.PostOnly, o.ID)
	}

	elapsed := time.Since(start).Seconds()
	a.monitor.RecordRESTLatency("place", elapsed)

	if err != nil {
		a.monitor.RecordRESTError("place")
		a.logger.LogError(err, map[string]interface{}{
			"action": "place_order",
			"symbol": o.Symbol,
		})
		return "", err
	}

	a.monitor.RecordOrderPlaced()
	a.logger.LogOrder("order_placed", orderID, map[string]interface{}{
		"symbol": o.Symbol,
		"side":   o.Side,
		"price":  o.Price,
		"qty":    o.Quantity,
	})

	return orderID, nil
}

func (a *orderGatewayAdapter) Cancel(orderID string) error {
	start := time.Now()
	a.monitor.RecordRESTRequest("cancel")

	// 这里需要symbol，简化处理先用空字符串
	err := a.client.CancelOrder("", orderID)

	elapsed := time.Since(start).Seconds()
	a.monitor.RecordRESTLatency("cancel", elapsed)

	if err != nil {
		a.monitor.RecordRESTError("cancel")
		return err
	}

	a.monitor.RecordOrderCanceled()
	return nil
}
