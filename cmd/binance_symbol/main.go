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
	symbol := flag.String("symbol", "ETHUSDC", "查询的交易对(如 ETHUSDC)")
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
	info, err := client.ExchangeInfo(filter)
	if err != nil {
		log.Fatalf("获取交易对信息失败: %v", err)
	}
	for _, s := range info {
		if strings.ToUpper(s.Symbol) != filter {
			continue
		}
		fmt.Printf("%s 状态=%s 价格精度=%d 数量精度=%d\n", s.Symbol, s.Status, s.PricePrecision, s.QuantityPrecision)
		fmt.Printf("  TickSize=%.8f MinPrice=%.8f MaxPrice=%.1f\n", s.TickSize, s.MinPrice, s.MaxPrice)
		fmt.Printf("  StepSize=%.8f MinQty=%.8f MaxQty=%.2f\n", s.StepSize, s.MinQty, s.MaxQty)
		fmt.Printf("  MinNotional=%.4f\n", s.MinNotional)
		return
	}
	fmt.Printf("未找到 %s 的交易对信息\n", filter)
}
