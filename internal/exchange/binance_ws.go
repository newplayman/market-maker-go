package exchange

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"market-maker-go/gateway"
	"market-maker-go/internal/store"
	"market-maker-go/metrics"
)

// BinanceUserStream 管理 UserStream WebSocket，含 listenKey keepalive 与自动重连。
type BinanceUserStream struct {
	BaseURL      string
	WSEndpoint   string
	APIKey       string
	APISecret    string
	store        *store.Store
	lkClient     *gateway.ListenKeyClient
	restClient   *gateway.BinanceRESTClient
	listenKey    string
	conn         *websocket.Conn
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	onConnected  func()
	onFatalError func(error)
	maxRetries   int
	retryBackoff time.Duration
	eventSink    func(string, map[string]interface{})
}

func NewBinanceUserStream(baseURL, wsEndpoint, apiKey, apiSecret string, st *store.Store) *BinanceUserStream {
	lk := &gateway.ListenKeyClient{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		HTTPClient: gateway.NewListenKeyHTTPClient(),
	}
	restClient := &gateway.BinanceRESTClient{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		Secret:       apiSecret,
		HTTPClient:   gateway.NewDefaultHTTPClient(),
		RecvWindowMs: 5000,
	}
	return &BinanceUserStream{
		BaseURL:      baseURL,
		WSEndpoint:   wsEndpoint,
		APIKey:       apiKey,
		APISecret:    apiSecret,
		store:        st,
		lkClient:     lk,
		restClient:   restClient,
		maxRetries:   5,
		retryBackoff: 3 * time.Second,
	}
}

// Start 启动 UserStream（后台 goroutine）。
func (b *BinanceUserStream) Start() error {
	// 创建 listenKey
	key, err := b.lkClient.NewListenKey()
	if err != nil {
		return fmt.Errorf("new listenKey: %w", err)
	}
	b.listenKey = key
	log.Printf("WebSocket UserStream listenKey=%s", key)

	ctx, cancel := context.WithCancel(context.Background())
	b.ctx = ctx
	b.cancel = cancel

	go b.runKeepalive()
	go b.runWS()
	return nil
}

// Stop 停止 WebSocket 连接。
func (b *BinanceUserStream) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
	b.mu.Lock()
	if b.conn != nil {
		_ = b.conn.Close()
		b.conn = nil
	}
	b.mu.Unlock()
}

// SetEventSink 设置事件回调（例如记录连接状态）
func (b *BinanceUserStream) SetEventSink(fn func(string, map[string]interface{})) {
	b.eventSink = fn
}

// SetFatalErrorHandler 设置致命错误回调（用于通知主程序触发优雅退出）
func (b *BinanceUserStream) SetFatalErrorHandler(fn func(error)) {
	b.onFatalError = fn
}

// runKeepalive 每 25 分钟 PUT keepalive。
func (b *BinanceUserStream) runKeepalive() {
	ticker := time.NewTicker(25 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			if err := b.lkClient.KeepAlive(b.listenKey); err != nil {
				log.Printf("listenKey keepalive failed: %v", err)
			}
		}
	}
}

// runWS 启动 WS 连接，自动重连。
func (b *BinanceUserStream) runWS() {
	retries := 0
	for {
		select {
		case <-b.ctx.Done():
			return
		default:
		}
		wsURL := fmt.Sprintf("%s/ws/%s", b.WSEndpoint, b.listenKey)
		u, _ := url.Parse(wsURL)
		dialer := websocket.DefaultDialer
		conn, _, err := dialer.Dial(u.String(), nil)
		if err != nil {
			if retries >= b.maxRetries {
				fatalErr := fmt.Errorf("websocket reconnection failed after %d retries: %w", b.maxRetries, err)
				log.Printf("❌ %v", fatalErr)
				if b.onFatalError != nil {
					b.onFatalError(fatalErr)
				}
				return
			}
			retries++
			backoff := time.Duration(retries) * b.retryBackoff
			log.Printf("ws dial failed (%d/%d): %v, retry in %s", retries, b.maxRetries, err, backoff)
			time.Sleep(backoff)
			continue
		}
		b.mu.Lock()
		b.conn = conn
		b.mu.Unlock()

		metrics.WSConnected.Set(1)
		log.Println("WebSocket UserStream connected")
		if b.eventSink != nil {
			b.eventSink("ws_connected", map[string]interface{}{
				"listenKey": b.listenKey,
			})
		}

		// 关键修复：重连后立即同步订单状态（防止状态不一致）
		if err := b.syncOrderState(); err != nil {
			log.Printf("⚠️ 订单状态同步失败: %v", err)
			metrics.RestFallbackCount.Inc()
		}

		if b.onConnected != nil {
			b.onConnected()
		}
		retries = 0

		// 读取循环
		b.readLoop(conn)

		// 断开则重连
		b.mu.Lock()
		b.conn = nil
		b.mu.Unlock()
		metrics.WSConnected.Set(0)
		log.Println("WebSocket UserStream disconnected, reconnecting...")
		if b.eventSink != nil {
			b.eventSink("ws_disconnected", map[string]interface{}{})
		}
		time.Sleep(b.retryBackoff)
	}
}

// readLoop 读取 WS 消息并分发事件。
func (b *BinanceUserStream) readLoop(conn *websocket.Conn) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		return nil
	})
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("ws read err: %v", err)
			return
		}
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		b.handleMessage(msg)
	}
}

// handleMessage 解析并分发 UserData 事件。
func (b *BinanceUserStream) handleMessage(raw []byte) {
	ev, err := gateway.ParseUserData(raw)
	if err != nil {
		if err != gateway.ErrNonUserData {
			log.Printf("parse user data: %v", err)
		}
		return
	}
	switch ev.EventType {
	case "ORDER_TRADE_UPDATE":
		if ev.Order != nil {
			b.store.HandleOrderUpdate(*ev.Order)
		}
	case "ACCOUNT_UPDATE":
		if ev.Account != nil {
			b.store.HandlePositionUpdate(*ev.Account)
			// 检查资金费率事件（FUNDING_FEE）
			if strings.ToUpper(ev.Account.Reason) == "FUNDING_FEE" {
				// 粗略估算当前费率：暂未提供费率直接字段，可用 0 占位或用外部接口
				b.store.HandleFundingRate(0)
			}
		}
	default:
		// 忽略其他
	}
}

// syncOrderState 重连后同步订单状态（审计报告P0级修复）
// 防止断线期间的订单成交/撤销事件丢失导致本地状态与交易所不一致
func (b *BinanceUserStream) syncOrderState() error {
	if b.restClient == nil {
		return fmt.Errorf("rest client not initialized")
	}

	// 1. 从交易所查询当前活跃订单
	info, err := b.restClient.AccountInfo()
	if err != nil {
		return fmt.Errorf("query account info: %w", err)
	}

	// 2. 更新本地仓位状态
	for _, p := range info.Positions {
		if p.Symbol == b.store.Symbol {
			log.Printf("仓位同步: %s = %.4f @ %.2f", p.Symbol, p.PositionAmt, p.EntryPrice)
			// 直接更新store中的仓位
			b.store.HandlePositionUpdate(gateway.AccountUpdate{
				Positions: []gateway.AccountPosition{{
					Symbol:      p.Symbol,
					PositionAmt: p.PositionAmt,
					EntryPrice:  p.EntryPrice,
					PnL:         p.UnrealizedProfit,
				}},
			})
			break
		}
	}

	// 3. 查询活跃订单并同步到store（关键！）
	// 注意：这里使用REST API查询未成交订单
	openOrders, err := b.getOpenOrders(b.store.Symbol)
	if err != nil {
		log.Printf("⚠️ 查询活跃订单失败: %v", err)
		return err
	}

	log.Printf("订单同步: 发现 %d 个活跃订单", len(openOrders))
	// 4. 更新store中的pending orders
	b.store.ReplacePendingOrders(openOrders)
	log.Printf("✅ 订单状态同步完成（BUY: %.4f, SELL: %.4f）", b.store.PendingBuySize(), b.store.PendingSellSize())
	return nil
}

// getOpenOrders 查询活跃订单（简化版REST调用）
func (b *BinanceUserStream) getOpenOrders(symbol string) ([]gateway.OrderUpdate, error) {
	if b.restClient == nil {
		return nil, fmt.Errorf("rest client not initialized")
	}
	rawOrders, err := b.restClient.OpenOrders(symbol)
	if err != nil {
		return nil, err
	}
	updates := make([]gateway.OrderUpdate, 0, len(rawOrders))
	for _, ro := range rawOrders {
		updates = append(updates, gateway.OrderUpdate{
			Symbol:         ro.Symbol,
			Side:           ro.Side,
			OrderType:      ro.OrderType,
			Status:         ro.Status,
			OrderID:        ro.OrderID,
			ClientOrderID:  ro.ClientOrderID,
			Price:          ro.Price,
			OrigQty:        ro.OrigQty,
			AccumulatedQty: ro.ExecutedQty,
			UpdateTime:     ro.UpdateTime,
		})
	}
	return updates, nil
}
