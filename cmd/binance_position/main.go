package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"market-maker-go/config"
	"market-maker-go/gateway"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	symbol := flag.String("symbol", "ETHUSDC", "查询的合约符号（如 ETHUSDC）")
	flag.Parse()

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

	filter := strings.ToUpper(strings.TrimSpace(*symbol))
	positions, err := client.PositionRisk(filter)
	if err != nil {
		log.Fatalf("查询持仓失败: %v", err)
	}
	if len(positions) == 0 {
		fmt.Printf("未找到 %s 的持仓记录\n", filter)
		return
	}
	for _, p := range positions {
		fmt.Printf("%s qty=%.6f entry=%.4f mark=%.4f pnl=%.4f side=%s margin=%s\n",
			p.Symbol, p.PositionAmt, p.EntryPrice, p.MarkPrice, p.UnrealizedProfit, p.PositionSide, p.MarginType)
	}
}
