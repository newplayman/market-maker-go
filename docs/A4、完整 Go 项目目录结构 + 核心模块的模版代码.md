好，下面我直接帮你“搭好脚手架”：给出**完整 Go 项目目录结构 + 核心模块的模版代码**，你改成自己的仓库名后就可以 `go build` / 往里填实现了。

> 模块名我先用：`github.com/yourname/mmbot`  
> 你实际用的时候改一下就行（`go.mod` 和 import 一起改）。

---

## 一、项目目录结构建议

```txt
mmbot/
├── go.mod
├── go.sum                     # 生成即可
├── cmd
│   ├── bot
│   │   └── main.go            # 实盘机器人入口
│   └── simulator
│       └── main.go            # 仿真/回放入口（以后用）
├── pkg
│   ├── config                 # 配置 & 热更新
│   │   ├── config.go
│   │   └── types.go
│   ├── logx                   # 日志封装
│   │   └── logx.go
│   ├── monitor                # metrics / Prometheus
│   │   └── metrics.go
│   ├── gateway                # API Gateway（Binance等）
│   │   ├── gateway.go
│   │   ├── binance_rest.go
│   │   └── binance_ws.go
│   ├── marketdata             # 行情处理
│   │   └── service.go
│   ├── state                  # 账户/仓位/挂单快照
│   │   └── state.go
│   ├── oms                    # 订单管理系统
│   │   └── oms.go
│   ├── risk                   # 风控引擎
│   │   └── engine.go
│   ├── strategy               # 策略层
│   │   └── phase1
│   │       └── phase1.go
│   ├── webapi                 # WebUI 后端 API（基础）
│   │   └── server.go
│   └── common                 # 公共工具
│       ├── id.go
│       └── timeutil.go
├── configs
│   └── config.example.yaml    # 配置示例
└── scripts
    └── run_bot.sh             # 启动脚本示例
```

下面给你**关键文件的模板代码**，照抄就能起项目，然后再往里填业务逻辑。

---

## 二、`go.mod`

```go
module github.com/yourname/mmbot

go 1.21

require (
    go.uber.org/zap v1.27.0
    gopkg.in/yaml.v3 v3.0.1
    github.com/gorilla/websocket v1.5.1
    github.com/prometheus/client_golang v1.19.0
)
```

> 版本大致写了一个合理区间，实际你可以 `go get -u` 调整。

---

## 三、主程序入口 `cmd/bot/main.go`

这是“实盘 bot”入口，负责：加载配置 → 初始化 logger → 启动各个服务 → 跑主循环。

```go
package main

import (
    "context"
    "flag"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/yourname/mmbot/pkg/config"
    "github.com/yourname/mmbot/pkg/gateway"
    "github.com/yourname/mmbot/pkg/logx"
    "github.com/yourname/mmbot/pkg/marketdata"
    "github.com/yourname/mmbot/pkg/monitor"
    "github.com/yourname/mmbot/pkg/oms"
    "github.com/yourname/mmbot/pkg/risk"
    "github.com/yourname/mmbot/pkg/state"
    "github.com/yourname/mmbot/pkg/strategy/phase1"
    "github.com/yourname/mmbot/pkg/webapi"
)

func main() {
    cfgPath := flag.String("config", "configs/config.yaml", "config file path")
    flag.Parse()

    // 全局 Context
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 捕获退出信号
    go func() {
        ch := make(chan os.Signal, 1)
        signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
        <-ch
        cancel()
    }()

    // 1) 加载配置
    cfg, err := config.Load(*cfgPath)
    if err != nil {
        panic("load config failed: " + err.Error())
    }

    // 2) 初始化日志
    logger := logx.NewLogger(cfg.System.LogLevel)
    defer logger.Sync()
    logx.ReplaceGlobal(logger)

    logger.Info("mmbot starting", zap.String("env", cfg.System.Environment))

    // 3) 初始化监控 (Prometheus)
    monitor.MustInit(cfg.Monitor)

    // 4) 初始化 API Gateway（Binance）
    gw, err := gateway.NewBinanceGateway(ctx, cfg.Exchange, logger)
    if err != nil {
        logger.Fatal("failed to init gateway", zap.Error(err))
    }

    // 5) 初始化 State / MarketData / OMS / Risk / Strategy
    st := state.NewStateStore()
    mds := marketdata.NewService(ctx, gw, st, cfg.MarketData, logger)
    riskEngine := risk.NewEngine(cfg.Risk, st, logger)
    omsSvc := oms.NewService(ctx, gw, st, riskEngine, cfg.OMS, logger)
    strat := phase1.NewStrategy(cfg.Strategy.Phase1, st, omsSvc, riskEngine, logger)

    // 6) 启动 WebAPI（监控 & 控制）
    go webapi.Serve(ctx, cfg.WebAPI, st, strat, riskEngine, logger)

    // 7) 启动行情服务
    go mds.Run()

    // 8) 启动策略主循环
    go strat.Run(ctx)

    logger.Info("mmbot started")

    // 阻塞直到 context 结束
    <-ctx.Done()
    logger.Info("mmbot shutting down")

    // 给各模块一点时间做收尾
    time.Sleep(1 * time.Second)
}
```

> 这里用的是 zap，全局 logger 用 `logx.ReplaceGlobal` 包一下。

---

## 四、配置模块 `pkg/config/types.go` & `config.go`

### `types.go`

```go
package config

type SystemConfig struct {
    Environment string `yaml:"environment"`
    LogLevel    string `yaml:"log_level"`
}

type ExchangeConfig struct {
    Name       string `yaml:"name"` // "binance"
    ApiKey     string `yaml:"api_key"`
    SecretKey  string `yaml:"secret_key"`
    BaseURL    string `yaml:"base_url"`
    WsURL      string `yaml:"ws_url"`
    RecvWindow int64  `yaml:"recv_window"`
}

type MarketDataConfig struct {
    Symbols     []string `yaml:"symbols"`
    DepthLevel  int      `yaml:"depth_level"`
    DepthUpdateMs int    `yaml:"depth_update_ms"`
}

type RiskConfig struct {
    MaxInventory        float64 `yaml:"max_inventory"`
    MaxDailyLossPercent float64 `yaml:"max_daily_loss_percent"`
}

type OMSConfig struct {
    MaxOrdersPerSecond  int `yaml:"max_orders_per_second"`
    MaxCancelsPerSecond int `yaml:"max_cancels_per_second"`
}

type Phase1StrategyConfig struct {
    Symbol          string  `yaml:"symbol"`
    BaseOrderSize   float64 `yaml:"base_order_size"`
    MinSpreadTicks  int     `yaml:"min_spread_ticks"`
    QuoteOffsetTicks int    `yaml:"quote_offset_ticks"`
    TickSize        float64 `yaml:"tick_size"`
}

type StrategyConfig struct {
    Phase1 Phase1StrategyConfig `yaml:"phase1"`
}

type MonitorConfig struct {
    Addr string `yaml:"addr"` // :2112
}

type WebAPIConfig struct {
    Addr string `yaml:"addr"`
}

type Config struct {
    System     SystemConfig     `yaml:"system"`
    Exchange   ExchangeConfig   `yaml:"exchange"`
    MarketData MarketDataConfig `yaml:"market_data"`
    Risk       RiskConfig       `yaml:"risk"`
    OMS        OMSConfig        `yaml:"oms"`
    Strategy   StrategyConfig   `yaml:"strategy"`
    Monitor    MonitorConfig    `yaml:"monitor"`
    WebAPI     WebAPIConfig     `yaml:"webapi"`
}
```

### `config.go`

```go
package config

import (
    "io/ioutil"

    "gopkg.in/yaml.v3"
)

func Load(path string) (*Config, error) {
    data, err := ioutil.ReadFile(path)
    if err != nil {
        return nil, err
    }
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, err
    }
    // TODO: 做一些简单的合法性校验
    return &cfg, nil
}
```

---

## 五、日志封装 `pkg/logx/logx.go`

```go
package logx

import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

var global *zap.Logger

func NewLogger(level string) *zap.Logger {
    cfg := zap.NewProductionConfig()
    cfg.Encoding = "json"
    cfg.EncoderConfig.TimeKey = "ts"
    cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

    switch level {
    case "debug":
        cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
    case "info":
        cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
    case "warn":
        cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
    default:
        cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
    }

    logger, err := cfg.Build()
    if err != nil {
        panic(err)
    }
    return logger
}

func ReplaceGlobal(l *zap.Logger) {
    global = l
    zap.ReplaceGlobals(l)
}

func L() *zap.Logger {
    if global != nil {
        return global
    }
    l, _ := zap.NewProduction()
    return l
}
```

---

## 六、监控指标 `pkg/monitor/metrics.go`

```go
package monitor

import (
    "log"
    "net/http"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"

    "github.com/yourname/mmbot/pkg/config"
)

var (
    OrderLatency = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name: "oms_order_latency_ms",
            Help: "Order send latency in ms",
            Buckets: []float64{1, 2, 5, 10, 20, 50, 100},
        },
    )
)

func MustInit(cfg config.MonitorConfig) {
    prometheus.MustRegister(OrderLatency)

    go func() {
        http.Handle("/metrics", promhttp.Handler())
        if err := http.ListenAndServe(cfg.Addr, nil); err != nil {
            log.Printf("metrics server error: %v", err)
        }
    }()
}
```

---

## 七、Gateway 接口与 Binance 桩 `pkg/gateway/gateway.go`

```go
package gateway

import (
    "context"

    "go.uber.org/zap"

    "github.com/yourname/mmbot/pkg/config"
)

type Side string

const (
    SideBuy  Side = "BUY"
    SideSell Side = "SELL"
)

type OrderType string

const (
    OrderTypeLimit OrderType = "LIMIT"
)

type OrderRequest struct {
    Symbol string
    Side   Side
    Type   OrderType
    Price  float64
    Qty    float64
    // 还有 timeInForce 等，可后续扩展
}

type OrderResponse struct {
    OrderID        string
    ClientOrderID  string
    Symbol         string
    Status         string
    TransactTimeMs int64
}

type TradeFill struct {
    TradeID string
    OrderID string
    Symbol  string
    Price   float64
    Qty     float64
    IsMaker bool
    TsMs    int64
}

type AccountSnapshot struct {
    Balances map[string]float64
}

type PositionSnapshot struct {
    Symbol string
    Qty    float64
    Price  float64
}

type MarketDataHandler interface {
    OnDepth(symbol string, bids, asks [][2]float64)
    OnTrade(symbol string, trade TradeFill)
}

type AccountHandler interface {
    OnOrderUpdate(or OrderResponse)
    OnTradeUpdate(tf TradeFill)
    OnAccountUpdate(as AccountSnapshot)
}

// Gateway 抽象接口
type Gateway interface {
    PlaceOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error)
    CancelOrder(ctx context.Context, symbol, orderID string) error
    CancelAll(ctx context.Context, symbol string) error

    GetOpenOrders(ctx context.Context, symbol string) ([]OrderResponse, error)
    GetPositions(ctx context.Context) ([]PositionSnapshot, error)

    // 行情订阅由实现内部处理
}

func NewBinanceGateway(ctx context.Context, cfg config.ExchangeConfig, logger *zap.Logger) (Gateway, error) {
    // 这里只做 stub，后面可以填充 REST/WS 细节
    return NewBinanceStub(ctx, cfg, logger), nil
}
```

### 简单 stub：`binance_rest.go` / `binance_ws.go`

你可以先用一个“假实现”方便跑通流程：

```go
// binance_stub.go
package gateway

import (
    "context"

    "go.uber.org/zap"

    "github.com/yourname/mmbot/pkg/config"
)

type binanceStub struct {
    log *zap.Logger
}

func NewBinanceStub(ctx context.Context, cfg config.ExchangeConfig, logger *zap.Logger) Gateway {
    logger.Info("binanceStub gateway initialized (no real API calls)")
    return &binanceStub{log: logger}
}

func (b *binanceStub) PlaceOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
    b.log.Info("PlaceOrder stub", zap.Any("req", req))
    return &OrderResponse{
        OrderID:       "stub-order-id",
        ClientOrderID: "stub-client-id",
        Symbol:        req.Symbol,
        Status:        "NEW",
    }, nil
}

func (b *binanceStub) CancelOrder(ctx context.Context, symbol, orderID string) error {
    b.log.Info("CancelOrder stub", zap.String("symbol", symbol), zap.String("order_id", orderID))
    return nil
}

func (b *binanceStub) CancelAll(ctx context.Context, symbol string) error {
    b.log.Info("CancelAll stub", zap.String("symbol", symbol))
    return nil
}

func (b *binanceStub) GetOpenOrders(ctx context.Context, symbol string) ([]OrderResponse, error) {
    return nil, nil
}

func (b *binanceStub) GetPositions(ctx context.Context) ([]PositionSnapshot, error) {
    return nil, nil
}
```

> 等你把整个架构跑通，再把 stub 替换成真实 REST/WS 实现。

---

## 八、状态存储 `pkg/state/state.go`

```go
package state

import (
    "sync"
)

type Position struct {
    Symbol string
    Qty    float64
    Price  float64
}

type Order struct {
    OrderID   string
    Symbol    string
    Price     float64
    Qty       float64
    FilledQty float64
    Side      string
    Status    string
}

type StateStore struct {
    mu        sync.RWMutex
    positions map[string]Position
    orders    map[string]Order
}

func NewStateStore() *StateStore {
    return &StateStore{
        positions: make(map[string]Position),
        orders:    make(map[string]Order),
    }
}

func (s *StateStore) GetPosition(symbol string) Position {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.positions[symbol]
}

func (s *StateStore) SetPosition(p Position) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.positions[p.Symbol] = p
}

func (s *StateStore) UpsertOrder(o Order) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.orders[o.OrderID] = o
}

func (s *StateStore) GetOpenOrders(symbol string) []Order {
    s.mu.RLock()
    defer s.mu.RUnlock()
    res := make([]Order, 0)
    for _, o := range s.orders {
        if o.Symbol == symbol && o.Status == "NEW" {
            res = append(res, o)
        }
    }
    return res
}
```

---

## 九、OMS 模板 `pkg/oms/oms.go`

```go
package oms

import (
    "context"
    "time"

    "go.uber.org/zap"

    "github.com/yourname/mmbot/pkg/gateway"
    "github.com/yourname/mmbot/pkg/config"
    "github.com/yourname/mmbot/pkg/state"
)

type Service struct {
    gw      gateway.Gateway
    st      *state.StateStore
    cfg     config.OMSConfig
    logger  *zap.Logger
}

func NewService(ctx context.Context, gw gateway.Gateway, st *state.StateStore, riskEngine RiskChecker, cfg config.OMSConfig, logger *zap.Logger) *Service {
    // riskEngine 以后可用于 pre-check
    return &Service{
        gw:     gw,
        st:     st,
        cfg:    cfg,
        logger: logger.Named("oms"),
    }
}

type RiskChecker interface {
    AllowNewOrder(symbol string, side gateway.Side, price, qty float64) bool
}

func (s *Service) PlaceLimitMaker(ctx context.Context, symbol string, side gateway.Side, price, qty float64) (*gateway.OrderResponse, error) {
    // TODO: 调用 riskEngine 做预检
    start := time.Now()
    resp, err := s.gw.PlaceOrder(ctx, gateway.OrderRequest{
        Symbol: symbol,
        Side:   side,
        Type:   gateway.OrderTypeLimit,
        Price:  price,
        Qty:    qty,
    })
    latencyMs := float64(time.Since(start).Milliseconds())
    s.logger.Debug("PlaceLimitMaker done", zap.Float64("latency_ms", latencyMs), zap.Error(err))
    // TODO: 更新 state
    return resp, err
}

func (s *Service) CancelAll(ctx context.Context, symbol string) error {
    return s.gw.CancelAll(ctx, symbol)
}
```

---

## 十、Risk Engine 模板 `pkg/risk/engine.go`

```go
package risk

import (
    "go.uber.org/zap"

    "github.com/yourname/mmbot/pkg/config"
    "github.com/yourname/mmbot/pkg/gateway"
    "github.com/yourname/mmbot/pkg/state"
)

type Engine struct {
    cfg config.RiskConfig
    st  *state.StateStore
    log *zap.Logger
}

func NewEngine(cfg config.RiskConfig, st *state.StateStore, logger *zap.Logger) *Engine {
    return &Engine{
        cfg: cfg,
        st:  st,
        log: logger.Named("risk"),
    }
}

func (e *Engine) AllowNewOrder(symbol string, side gateway.Side, price, qty float64) bool {
    // TODO: 使用 inventory/PnL 进行检测
    return true
}
```

---

## 十一、Phase1 策略模板 `pkg/strategy/phase1/phase1.go`

```go
package phase1

import (
    "context"
    "time"

    "go.uber.org/zap"

    "github.com/yourname/mmbot/pkg/config"
    "github.com/yourname/mmbot/pkg/gateway"
    "github.com/yourname/mmbot/pkg/oms"
    "github.com/yourname/mmbot/pkg/state"
)

type Strategy struct {
    cfg   config.Phase1StrategyConfig
    st    *state.StateStore
    oms   *oms.Service
    log   *zap.Logger
}

func NewStrategy(cfg config.Phase1StrategyConfig, st *state.StateStore, o *oms.Service, riskEngine interface{}, logger *zap.Logger) *Strategy {
    return &Strategy{
        cfg: cfg,
        st:  st,
        oms: o,
        log: logger.Named("strategy.phase1"),
    }
}

func (s *Strategy) Run(ctx context.Context) {
    ticker := time.NewTicker(200 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            s.log.Info("strategy stopped")
            return
        case <-ticker.C:
            s.onTick(ctx)
        }
    }
}

func (s *Strategy) onTick(ctx context.Context) {
    // TODO: 从 state 中获取当前 mid / spread / inventory
    mid := 100.0 // stub
    tick := s.cfg.TickSize
    spreadTicks := s.cfg.MinSpreadTicks

    bid := mid - float64(spreadTicks/2)*tick
    ask := mid + float64(spreadTicks/2)*tick

    s.log.Debug("quote", zap.Float64("bid", bid), zap.Float64("ask", ask))

    // TODO: 实际上需要先撤销旧单，再挂新单（或者只做 diff）
    _, _ = s.oms.PlaceLimitMaker(ctx, s.cfg.Symbol, gateway.SideBuy, bid, s.cfg.BaseOrderSize)
    _, _ = s.oms.PlaceLimitMaker(ctx, s.cfg.Symbol, gateway.SideSell, ask, s.cfg.BaseOrderSize)
}
```

---

## 十二、Web API 模板 `pkg/webapi/server.go`

```go
package webapi

import (
    "context"
    "encoding/json"
    "net/http"

    "go.uber.org/zap"

    "github.com/yourname/mmbot/pkg/config"
    "github.com/yourname/mmbot/pkg/state"
)

type StrategyController interface {
    // 以后可以加 Start/Stop/Pause 等接口
}

func Serve(ctx context.Context, cfg config.WebAPIConfig, st *state.StateStore, strat StrategyController, risk interface{}, logger *zap.Logger) {
    mux := http.NewServeMux()

    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok"))
    })

    mux.HandleFunc("/state/positions", func(w http.ResponseWriter, r *http.Request) {
        // 简单 demo：返回所有 position
        // 实际可扩展
        resp := map[string]any{
            "ok": true,
        }
        _ = json.NewEncoder(w).Encode(resp)
    })

    srv := &http.Server{
        Addr:    cfg.Addr,
        Handler: mux,
    }

    go func() {
        logger.Info("webapi listening", zap.String("addr", cfg.Addr))
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logger.Error("webapi server error", zap.Error(err))
        }
    }()

    go func() {
        <-ctx.Done()
        _ = srv.Shutdown(context.Background())
    }()
}
```

---

## 十三、示例配置 `configs/config.example.yaml`

```yaml
system:
  environment: "dev"
  log_level: "debug"

exchange:
  name: "binance"
  api_key: "YOUR_API_KEY"
  secret_key: "YOUR_SECRET"
  base_url: "https://fapi.binance.com"
  ws_url: "wss://fstream.binance.com/ws"
  recv_window: 5000

market_data:
  symbols: ["BTCUSDC"]
  depth_level: 10
  depth_update_ms: 100

risk:
  max_inventory: 0.02
  max_daily_loss_percent: 0.03

oms:
  max_orders_per_second: 10
  max_cancels_per_second: 10

strategy:
  phase1:
    symbol: "BTCUSDC"
    base_order_size: 0.001
    min_spread_ticks: 4
    quote_offset_ticks: 1
    tick_size: 0.1

monitor:
  addr: ":2112"

webapi:
  addr: ":8080"
```

---

## 接下来你可以怎么干？

1. **把这些文件和目录按原样建起来**
    
2. 修改 `module github.com/yourname/mmbot` 为你自己的路径
    
3. `go mod tidy`
    
4. 先用 `binanceStub` 跑通：
    
    ```bash
    go run ./cmd/bot -config=configs/config.example.yaml
    ```
    
    看日志 /health /metrics。
    
5. 跑通之后，再逐步把：
    
    - Gateway stub → 真正的 REST / WS 实现
        
    - MarketData Service 填上深度/成交处理
        
    - Strategy 的 onTick 从 state 里读真实 mid/盘口
        

如果你愿意，下一步我可以专门帮你把 **Binance REST/WS 实现设计出来（含签名、限频和深度同步逻辑的 Go 版代码骨架）**，或者直接从 **Phase1 策略的实盘版本** 开始细化。你更想先推进哪块？