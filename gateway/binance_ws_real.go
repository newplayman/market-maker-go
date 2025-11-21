package gateway

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// BinanceWSReal 组合订阅深度/用户数据流并连接真实 WS（执行者确保网络可达）。
// 仅提供最小骨架：连接 + 简单读取；业务层可在 handler 中扩展解析。
type BinanceWSReal struct {
	BaseEndpoint string // 默认 wss://fstream.binance.com
	depthStreams []string
	userStream   string
	Dialer       *websocket.Dialer
}

func NewBinanceWSReal() *BinanceWSReal {
	return &BinanceWSReal{
		BaseEndpoint: BinanceFuturesWSEndpoint,
		Dialer:       websocket.DefaultDialer,
	}
}

func (b *BinanceWSReal) SubscribeDepth(symbol string) error {
	if symbol == "" {
		return fmt.Errorf("symbol required")
	}
	stream := strings.ToLower(symbol) + "@depth20@100ms"
	b.depthStreams = append(b.depthStreams, stream)
	return nil
}

func (b *BinanceWSReal) SubscribeUserData(listenKey string) error {
	if listenKey == "" {
		return fmt.Errorf("listenKey required")
	}
	b.userStream = listenKey
	return nil
}

// Run 构建 combined stream 并读取消息；对消息不做解析，业务可扩展。
func (b *BinanceWSReal) Run(handler WSHandler) error {
	streams := make([]string, 0, len(b.depthStreams)+1)
	streams = append(streams, b.depthStreams...)
	if b.userStream != "" {
		streams = append(streams, b.userStream)
	}
	if len(streams) == 0 {
		return fmt.Errorf("no streams subscribed")
	}
	u := url.URL{
		Scheme: "wss",
		Host:   strings.TrimPrefix(b.BaseEndpoint, "wss://"),
		Path:   "/stream",
	}
	q := u.Query()
	q.Set("streams", strings.Join(streams, "/"))
	u.RawQuery = q.Encode()

	conn, _, err := b.Dialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		// 如果提供 handler，可以在外部解析或者使用 BinanceWSHandler.OnRawMessage
		if handler != nil {
			if h, ok := handler.(interface{ OnRawMessage([]byte) }); ok {
				h.OnRawMessage(message)
			}
		} else {
			log.Printf("binance ws recv: %s", string(message))
		}
	}
}
