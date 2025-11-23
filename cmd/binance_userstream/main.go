package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"market-maker-go/config"
	"market-maker-go/gateway"
	"market-maker-go/inventory"
	"market-maker-go/order"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	depthSymbol := flag.String("depth", "", "可选：订阅一个公共 depth symbol，例如 ETHUSDC")
	flag.Parse()

	cfg, err := config.LoadWithEnvOverrides(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 初始化 order manager 和 inventory
	mgr := order.NewManager(nil)
	symbolConstraints := make(map[string]order.SymbolConstraints)
	for sym, sc := range cfg.Symbols {
		symbolConstraints[sym] = order.SymbolConstraints{
			TickSize:    sc.TickSize,
			StepSize:    sc.StepSize,
			MinQty:      sc.MinQty,
			MaxQty:      sc.MaxQty,
			MinNotional: sc.MinNotional,
		}
	}
	mgr.SetConstraints(symbolConstraints)
	inv := &inventory.Tracker{}

	lkClient := &gateway.ListenKeyClient{
		BaseURL:    cfg.Gateway.BaseURL,
		APIKey:     cfg.Gateway.APIKey,
		HTTPClient: gateway.NewListenKeyHTTPClient(),
	}
	listenKey, err := lkClient.NewListenKey()
	if err != nil {
		log.Fatalf("创建 listenKey 失败: %v", err)
	}
	log.Printf("listenKey=%s", listenKey)
	defer lkClient.CloseListenKey(listenKey)

	go keepAliveLoop(ctx, lkClient, listenKey)

	ws := gateway.NewBinanceWSReal()
	if *depthSymbol != "" {
		if err := ws.SubscribeDepth(strings.ToUpper(*depthSymbol)); err != nil {
			log.Fatalf("订阅 depth 失败: %v", err)
		}
	}
	if err := ws.SubscribeUserData(listenKey); err != nil {
		log.Fatalf("订阅用户流失败: %v", err)
	}

	userHandler := &gateway.BinanceUserHandler{
		OnOrderUpdate: func(o gateway.OrderUpdate) {
			switch o.Status {
			case "NEW":
				// 可以根据需要插入 order.Manager.Submit，这里仅更新状态
			case "FILLED":
				_ = mgr.Update(o.ClientOrderID, order.StatusFilled)
			case "PARTIALLY_FILLED":
				_ = mgr.Update(o.ClientOrderID, order.StatusPartial)
			case "CANCELED":
				_ = mgr.Update(o.ClientOrderID, order.StatusCanceled)
			}
			log.Printf("ORDER event: %+v", o)
		},
		OnAccountUpdate: func(a gateway.AccountUpdate) {
			for _, p := range a.Positions {
				if _, ok := symbolConstraints[p.Symbol]; ok {
					inv.SetExposure(p.PositionAmt, p.EntryPrice)
				}
			}
			log.Printf("ACCOUNT event: %+v", a)
		},
	}
	handler := &streamHandler{user: userHandler}

	go func() {
		if err := ws.Run(handler); err != nil {
			log.Printf("WS 运行失败: %v", err)
			cancel()
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	cancel()
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
				log.Printf("listenKey keepalive err: %v", err)
			} else {
				log.Printf("listenKey keepalive 成功")
			}
		}
	}
}

type streamHandler struct {
	user *gateway.BinanceUserHandler
}

func (h *streamHandler) OnRawMessage(msg []byte) {
	if h.user != nil {
		h.user.OnRawMessage(msg)
		return
	}
	fmt.Printf("WS事件: %s\n", string(msg))
}

func (h *streamHandler) OnDepth(symbol string, bid, ask float64) {
	log.Printf("DEPTH %s bid=%.2f ask=%.2f", symbol, bid, ask)
}

func (h *streamHandler) OnTrade(symbol string, price, qty float64) {}
