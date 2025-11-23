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
	set := flag.Bool("set", false, "是否设置模式（默认仅查询）")
	enableDual := flag.Bool("dual", false, "当 -set=true 时，dual=true 表示双向持仓，false 表示单向")
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
	if *set {
		if err := client.SetDualPosition(*enableDual); err != nil {
			log.Fatalf("设置持仓模式失败: %v", err)
		}
		fmt.Printf("已设置 dualSidePosition=%v\n", *enableDual)
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
