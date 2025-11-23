好，那我就顺着刚才的项目骨架，**把 Binance Gateway 这一层往前实打实推进一大步**：给你一个可用的 REST 客户端 + WS 行情/账户流骨架，后面你只要补少量细节就能真连币安。

> 下面所有代码都默认你已经用的是前面那套目录结构。  
> 模块路径还是 `github.com/yourname/mmbot`，你自己换掉就行。

---

## 一、REST 客户端实现：`pkg/gateway/binance_rest.go`

功能目标：

- 支持 **签名请求**
    
- 自动附加 `timestamp` 和 `recvWindow`
    
- 支持下单 / 撤单 / 查 openOrders / 查账户 / 查持仓
    
- 包一个简单的错误处理
    

```go
package gateway

import (
    "context"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strconv"
    "strings"
    "time"

    "go.uber.org/zap"

    "github.com/yourname/mmbot/pkg/config"
)

type binanceREST struct {
    baseURL    string
    apiKey     string
    secretKey  string
    recvWindow int64
    client     *http.Client
    log        *zap.Logger

    // 时间偏移（本地时间 + offset = 交易所时间）
    timeOffsetMs int64
}

func newBinanceREST(cfg config.ExchangeConfig, logger *zap.Logger) *binanceREST {
    return &binanceREST{
        baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
        apiKey:     cfg.ApiKey,
        secretKey:  cfg.SecretKey,
        recvWindow: cfg.RecvWindow,
        client: &http.Client{
            Timeout: 5 * time.Second,
        },
        log: logger.Named("binance_rest"),
    }
}

// --- 工具方法 ---

func (b *binanceREST) signedRequest(ctx context.Context, method, path string, params url.Values) ([]byte, error) {
    if params == nil {
        params = url.Values{}
    }
    now := time.Now().UnixMilli() + b.timeOffsetMs
    params.Set("timestamp", strconv.FormatInt(now, 10))
    if b.recvWindow > 0 {
        params.Set("recvWindow", strconv.FormatInt(b.recvWindow, 10))
    }

    query := params.Encode()
    sig := b.sign(query)
    query += "&signature=" + sig

    req, err := http.NewRequestWithContext(ctx, method, b.baseURL+path+"?"+query, nil)
    if err != nil {
        return nil, err
    }
    req.Header.Set("X-MBX-APIKEY", b.apiKey)

    resp, err := b.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)

    if resp.StatusCode >= 400 {
        // 简单错误包装，你可以后面改成结构化
        b.log.Warn("binance error", zap.Int("status", resp.StatusCode), zap.ByteString("body", body))
        return nil, fmt.Errorf("binance error: status=%d body=%s", resp.StatusCode, string(body))
    }

    return body, nil
}

func (b *binanceREST) sign(payload string) string {
    mac := hmac.New(sha256.New, []byte(b.secretKey))
    mac.Write([]byte(payload))
    return hex.EncodeToString(mac.Sum(nil))
}

// 和 Binance /fapi/v1/time 做时间同步
func (b *binanceREST) syncTime() error {
    resp, err := b.client.Get(b.baseURL + "/fapi/v1/time")
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    body, _ := io.ReadAll(resp.Body)
    var data struct {
        ServerTime int64 `json:"serverTime"`
    }
    if err := json.Unmarshal(body, &data); err != nil {
        return err
    }
    local := time.Now().UnixMilli()
    b.timeOffsetMs = data.ServerTime - local
    b.log.Info("binance time synced", zap.Int64("offset_ms", b.timeOffsetMs))
    return nil
}
```

### 下单 / 撤单 / 查询：REST 具体方法

```go
// 下单
func (b *binanceREST) newOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
    params := url.Values{}
    params.Set("symbol", req.Symbol)
    params.Set("side", string(req.Side))
    params.Set("type", string(req.Type))
    params.Set("timeInForce", "GTC")
    params.Set("quantity", formatFloat(req.Qty))
    params.Set("price", formatFloat(req.Price))
    params.Set("newOrderRespType", "RESULT")

    body, err := b.signedRequest(ctx, http.MethodPost, "/fapi/v1/order", params)
    if err != nil {
        return nil, err
    }

    var res struct {
        OrderId       int64  `json:"orderId"`
        ClientOrderId string `json:"clientOrderId"`
        Symbol        string `json:"symbol"`
        Status        string `json:"status"`
        UpdateTime    int64  `json:"updateTime"`
    }
    if err := json.Unmarshal(body, &res); err != nil {
        return nil, err
    }

    return &OrderResponse{
        OrderID:        fmt.Sprintf("%d", res.OrderId),
        ClientOrderID:  res.ClientOrderId,
        Symbol:         res.Symbol,
        Status:         res.Status,
        TransactTimeMs: res.UpdateTime,
    }, nil
}

// 撤单
func (b *binanceREST) cancelOrder(ctx context.Context, symbol, orderID string) error {
    params := url.Values{}
    params.Set("symbol", symbol)
    params.Set("orderId", orderID)

    _, err := b.signedRequest(ctx, http.MethodDelete, "/fapi/v1/order", params)
    return err
}

// 撤全部（简化版：循环 openOrders 撤）
func (b *binanceREST) cancelAll(ctx context.Context, symbol string) error {
    params := url.Values{}
    params.Set("symbol", symbol)
    _, err := b.signedRequest(ctx, http.MethodDelete, "/fapi/v1/allOpenOrders", params)
    return err
}

// 查询 openOrders
func (b *binanceREST) getOpenOrders(ctx context.Context, symbol string) ([]OrderResponse, error) {
    params := url.Values{}
    params.Set("symbol", symbol)
    body, err := b.signedRequest(ctx, http.MethodGet, "/fapi/v1/openOrders", params)
    if err != nil {
        return nil, err
    }
    var raws []struct {
        OrderId       int64  `json:"orderId"`
        ClientOrderId string `json:"clientOrderId"`
        Symbol        string `json:"symbol"`
        Status        string `json:"status"`
        UpdateTime    int64  `json:"updateTime"`
        Price         string `json:"price"`
        OrigQty       string `json:"origQty"`
        ExecutedQty   string `json:"executedQty"`
        Side          string `json:"side"`
    }
    if err := json.Unmarshal(body, & raws); err != nil {
        return nil, err
    }
    res := make([]OrderResponse, 0, len(raws))
    for _, r := range raws {
        res = append(res, OrderResponse{
            OrderID:        fmt.Sprintf("%d", r.OrderId),
            ClientOrderID:  r.ClientOrderId,
            Symbol:         r.Symbol,
            Status:         r.Status,
            TransactTimeMs: r.UpdateTime,
        })
    }
    return res, nil
}
```

> `formatFloat` 自己写一个简单辅助（保留合适的小数位）。

```go
func formatFloat(v float64) string {
    return strconv.FormatFloat(v, 'f', -1, 64)
}
```

持仓 / 账户可以后面慢慢加；先把下单通路打通最重要。

---

## 二、把 REST 灌进 Gateway：`pkg/gateway/binance_gateway.go`

我们之前的 `NewBinanceGateway` 返回的是 stub，现在改成真正的实现结构体，把 REST 封装进去，并预留 WS：

```go
package gateway

import (
    "context"

    "go.uber.org/zap"

    "github.com/yourname/mmbot/pkg/config"
)

type binanceGateway struct {
    rest *binanceREST
    // 以后加 ws 行情 & 账户流：
    // mdWS  *binanceMDWS
    // accWS *binanceAccountWS
    log  *zap.Logger
}

func NewBinanceGateway(ctx context.Context, cfg config.ExchangeConfig, logger *zap.Logger) (Gateway, error) {
    rest := newBinanceREST(cfg, logger)
    if err := rest.syncTime(); err != nil {
        logger.Warn("binance time sync failed", zap.Error(err))
    }

    g := &binanceGateway{
        rest: rest,
        log:  logger.Named("binance_gateway"),
    }

    // TODO: 初始化 WS 行情、帐户流，在这里启动 goroutine

    return g, nil
}

// 实现 Gateway 接口：

func (g *binanceGateway) PlaceOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
    return g.rest.newOrder(ctx, req)
}

func (g *binanceGateway) CancelOrder(ctx context.Context, symbol, orderID string) error {
    return g.rest.cancelOrder(ctx, symbol, orderID)
}

func (g *binanceGateway) CancelAll(ctx context.Context, symbol string) error {
    return g.rest.cancelAll(ctx, symbol)
}

func (g *binanceGateway) GetOpenOrders(ctx context.Context, symbol string) ([]OrderResponse, error) {
    return g.rest.getOpenOrders(ctx, symbol)
}

func (g *binanceGateway) GetPositions(ctx context.Context) ([]PositionSnapshot, error) {
    // TODO: 使用 /fapi/v2/positionRisk 实现
    return nil, nil
}
```

这样一来：

- 你的 `OMS` 已经可以真实下单/撤单
    
- 策略里调 `oms.PlaceLimitMaker` 会真正打到币安
    
- 接下来只要把 WS 行情补上，就能跑 Phase1 做市（虽然还没风控和撤单 diff 优化）
    

---

## 三、WS 行情（MarketData）骨架：`pkg/gateway/binance_ws.go`

我们先做一个最小可用版：  
只订阅单 symbol 的 depth stream，把最新的 `bids/asks` 推给 `MarketDataHandler`。

```go
package gateway

import (
    "context"
    "encoding/json"
    "fmt"
    "net/url"
    "strings"
    "time"

    "github.com/gorilla/websocket"
    "go.uber.org/zap"

    "github.com/yourname/mmbot/pkg/config"
)

type binanceMDWS struct {
    ctx      context.Context
    cancel   context.CancelFunc
    wsURL    string
    symbols  []string
    handler  MarketDataHandler
    log      *zap.Logger
}

func newBinanceMDWS(parent context.Context, cfg config.ExchangeConfig, symbols []string, h MarketDataHandler, logger *zap.Logger) *binanceMDWS {
    ctx, cancel := context.WithCancel(parent)
    return &binanceMDWS{
        ctx:     ctx,
        cancel:  cancel,
        wsURL:   cfg.WsURL,
        symbols: symbols,
        handler: h,
        log:     logger.Named("binance_mdws"),
    }
}

func (w *binanceMDWS) Run() {
    // 简化版：聚合多个 depth stream 到一个 multiplex
    // fstream: wss://fstream.binance.com/stream?streams=btcusdc@depth10@100ms
    streams := make([]string, 0, len(w.symbols))
    for _, s := range w.symbols {
        streams = append(streams, strings.ToLower(s)+"@depth10@100ms")
    }
    streamPath := strings.Join(streams, "/")

    u, _ := url.Parse(w.wsURL)
    u.Path = "/stream"
    q := u.Query()
    q.Set("streams", streamPath)
    u.RawQuery = q.Encode()

    for {
        select {
        case <-w.ctx.Done():
            return
        default:
        }

        w.log.Info("connecting md ws", zap.String("url", u.String()))
        conn, _, err := websocket.DefaultDialer.DialContext(w.ctx, u.String(), nil)
        if err != nil {
            w.log.Error("md ws dial failed", zap.Error(err))
            time.Sleep(2 * time.Second)
            continue
        }

        w.readLoop(conn)

        conn.Close()
        time.Sleep(1 * time.Second)
    }
}

func (w *binanceMDWS) readLoop(conn *websocket.Conn) {
    for {
        select {
        case <-w.ctx.Done():
            return
        default:
        }

        _, msg, err := conn.ReadMessage()
        if err != nil {
            w.log.Error("md ws read error", zap.Error(err))
            return
        }

        var envelope struct {
            Stream string          `json:"stream"`
            Data   json.RawMessage `json:"data"`
        }
        if err := json.Unmarshal(msg, &envelope); err != nil {
            w.log.Warn("unmarshal envelope failed", zap.Error(err))
            continue
        }

        var depth struct {
            Symbol string     `json:"s"`
            Bids   [][2]string `json:"b"`
            Asks   [][2]string `json:"a"`
        }
        if err := json.Unmarshal(envelope.Data, &depth); err != nil {
            w.log.Warn("unmarshal depth failed", zap.Error(err))
            continue
        }

        bids := convertPriceLevels(depth.Bids)
        asks := convertPriceLevels(depth.Asks)

        w.handler.OnDepth(depth.Symbol, bids, asks)
    }
}

func convertPriceLevels(src [][2]string) [][2]float64 {
    res := make([][2]float64, 0, len(src))
    for _, lv := range src {
        if len(lv) != 2 {
            continue
        }
        p, err1 := strconv.ParseFloat(lv[0], 64)
        q, err2 := strconv.ParseFloat(lv[1], 64)
        if err1 != nil || err2 != nil {
            continue
        }
        res = append(res, [2]float64{p, q})
    }
    return res
}
```

> 这里省略了一些 import（`strconv` 等），你加上即可。  
> 这个实现是“够用但不完全严谨”的版本：  
> 之后你可以加上 `lastUpdateId` 校验、snapshot 组合等高级逻辑。

然后在 `binanceGateway` 的 `NewBinanceGateway` 里，把 `mdWS.Run()` 用 goroutine 跑起来，传给 `marketdata.Service` 做 handler 即可。

---

## 四、MarketData Service 接口对接：`pkg/marketdata/service.go`

配合 `MarketDataHandler`，写一个简单实现，把盘口 mid/last 存到 `StateStore` 里，供策略读取。

```go
package marketdata

import (
    "context"
    "sync/atomic"

    "go.uber.org/zap"

    "github.com/yourname/mmbot/pkg/gateway"
    "github.com/yourname/mmbot/pkg/config"
    "github.com/yourname/mmbot/pkg/state"
)

type Service struct {
    ctx    context.Context
    st     *state.StateStore
    cfg    config.MarketDataConfig
    log    *zap.Logger

    // 你可以用 atomic 保存一些全局指标
    lastTickTsMs atomic.Int64
}

// 确保 Service 实现 gateway.MarketDataHandler
var _ gateway.MarketDataHandler = (*Service)(nil)

func NewService(ctx context.Context, gw gateway.Gateway, st *state.StateStore, cfg config.MarketDataConfig, logger *zap.Logger) *Service {
    svc := &Service{
        ctx: ctx,
        st:  st,
        cfg: cfg,
        log: logger.Named("mds"),
    }
    // 实际上应该让 gateway 知道 handler是谁
    // 目前我们是手动在 NewBinanceGateway 那边 new ws 时传 handler
    _ = gw
    return svc
}

func (s *Service) Run() {
    s.log.Info("marketdata service started")
    <-s.ctx.Done()
    s.log.Info("marketdata service stopped")
}

// OnDepth 被 gateway 的 WS 调用
func (s *Service) OnDepth(symbol string, bids, asks [][2]float64) {
    if len(bids) == 0 || len(asks) == 0 {
        return
    }
    bestBid := bids[0][0]
    bestAsk := asks[0][0]
    mid := (bestBid + bestAsk) / 2

    // 你可以在 state 里加一个 MarketSnapshot 结构，这里简单打印
    s.log.Debug("depth", zap.String("symbol", symbol), zap.Float64("bid", bestBid), zap.Float64("ask", bestAsk), zap.Float64("mid", mid))
    s.lastTickTsMs.Store(time.Now().UnixMilli())
}

// OnTrade 以后用逐笔数据时再填
func (s *Service) OnTrade(symbol string, trade gateway.TradeFill) {
    // TODO
}
```

> 上面 `NewService` 与 `Gateway` 的 handler wiring 这里只是勾了一个接口，实际你可以这样做：
> 
> - 把 `MarketDataHandler` 传给 `NewBinanceMDWS`
>     
> - 在 `NewBinanceGateway` 里 new `binanceMDWS` 时注入 `mds`。
>     

---

## 五、你现在手里有什么？

到这一刻，你已经有了：

- 完整 Go 项目骨架
    
- 可用的配置系统
    
- 可用的日志 & metrics
    
- 一个**真实的 Binance REST 客户端**（下单/撤单/openOrders）
    
- 一个**最小可用版 WS depth 行情**管线骨架
    
- Phase1 策略主循环模板（现在可以开始读 mid/盘口并下真单）
    

你接下来可以做的几件具体事：

1. 把 `binanceStub` 替换为上面的 `binanceGateway` 和 `binanceREST`。
    
2. 在 `NewBinanceGateway` 里真正 new 一个 `binanceMDWS`，把 `marketdata.Service` 作为 handler 传进去。
    
3. 在 `Phase1Strategy.onTick` 里，从你的 `state` 或 `MarketData` 中拿到 mid/bid/ask，做真正的 quote，而不是写死 `mid := 100`。
    
4. 加上 riskEngine 的一个简单检查（比如 MaxInventory）。
    

如果你愿意，下一步我可以帮你：

- 把 **Phase1 策略的订单管理逻辑写完整**（自动 diff 挂单、只更新需要改的价位），  
    或者
    
- 把 **Binance 账户 / 订单回报 WS（userDataStream）** 的实现补完，让 `StateStore` 跟实际成交同步。
    

你自己不挑的话，我会优先建议下一步搞 **用户流 / 订单回报 WS**，因为实盘必须要这条链路来保证状态一致。