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
	store        *store.Store
	lkClient     *gateway.ListenKeyClient
	listenKey    string
	conn         *websocket.Conn
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	onConnected  func()
	maxRetries   int
	retryBackoff time.Duration
}

func NewBinanceUserStream(baseURL, wsEndpoint, apiKey string, st *store.Store) *BinanceUserStream {
	lk := &gateway.ListenKeyClient{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		HTTPClient: gateway.NewListenKeyHTTPClient(),
	}
	return &BinanceUserStream{
		BaseURL:      baseURL,
		WSEndpoint:   wsEndpoint,
		APIKey:       apiKey,
		store:        st,
		lkClient:     lk,
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
				log.Printf("ws dial exceeded max retries, stopping")
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
