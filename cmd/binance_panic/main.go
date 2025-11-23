package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"strings"

	"market-maker-go/config"
	"market-maker-go/gateway"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	symbol := flag.String("symbol", "ETHUSDC", "合约代码")
	cancelAll := flag.Bool("cancel", false, "取消该合约全部挂单")
	closePosition := flag.Bool("close", false, "市价 reduce-only 平掉当前仓位")
	flag.Parse()

	if !*cancelAll && !*closePosition {
		log.Fatalf("请至少指定 -cancel 或 -close 其中之一")
	}

	cfg, err := config.LoadWithEnvOverrides(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	client := &gateway.BinanceRESTClient{
		BaseURL:      cfg.Gateway.BaseURL,
		APIKey:       cfg.Gateway.APIKey,
		Secret:       cfg.Gateway.APISecret,
		HTTPClient:   gateway.NewDefaultHTTPClient(),
		RecvWindowMs: 5000,
	}

	if *cancelAll {
		if err := client.CancelAll(*symbol); err != nil {
			log.Fatalf("取消挂单失败: %v", err)
		}
		fmt.Printf("[%s] 所有挂单已提交撤销\n", *symbol)
	}

	if *closePosition {
		if err := flattenPosition(client, *symbol); err != nil {
			log.Fatalf("平仓失败: %v", err)
		}
	}
}

func flattenPosition(client *gateway.BinanceRESTClient, symbol string) error {
	positions, err := client.PositionRisk(symbol)
	if err != nil {
		return fmt.Errorf("查询持仓失败: %w", err)
	}
	var amt float64
	for _, p := range positions {
		if strings.EqualFold(p.Symbol, symbol) {
			amt = p.PositionAmt
			break
		}
	}
	if math.Abs(amt) < 1e-8 {
		fmt.Println("当前无持仓，无需平仓")
		return nil
	}
	side := "SELL"
	if amt < 0 {
		side = "BUY"
	}
	qty := math.Abs(amt)
	if _, err := client.PlaceMarket(symbol, side, qty, true, "panic-close"); err != nil {
		return fmt.Errorf("提交市价单失败: %w", err)
	}
	fmt.Printf("已提交 reduce-only %s 市价单，数量 %.6f\n", side, qty)
	return nil
}
