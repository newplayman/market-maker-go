package main

import (
	"flag"
	"fmt"
	"log"

	"market-maker-go/config"
	"market-maker-go/gateway"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	symbol := flag.String("symbol", "ETHUSDC", "交易对（例如 ETHUSDC）")
	marginType := flag.String("margin", "", "设置保证金模式：CROSSED 或 ISOLATED（留空只查询）")
	leverage := flag.Int("leverage", 0, "设置杠杆倍数（0 表示不调整）")
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
	if *marginType != "" {
		if err := client.SetMarginType(*symbol, *marginType); err != nil {
			log.Fatalf("设置保证金模式失败: %v", err)
		}
		fmt.Printf("已设置 %s 的 marginType=%s\n", *symbol, *marginType)
	}
	if *leverage > 0 {
		if err := client.SetLeverage(*symbol, *leverage); err != nil {
			log.Fatalf("设置杠杆失败: %v", err)
		}
		fmt.Printf("已设置 %s 杠杆=%dx\n", *symbol, *leverage)
	}
	mode, err := client.GetDualPosition()
	if err != nil {
		log.Fatalf("查询持仓模式失败: %v", err)
	}
	fmt.Printf("当前 dualSidePosition=%v (%s)\n", mode, describeMode(mode))
}

func describeMode(dual bool) string {
	if dual {
		return "双向持仓（Hedge）"
	}
	return "单向持仓（One-way）"
}
