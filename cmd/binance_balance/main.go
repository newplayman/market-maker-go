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
	cfgPath := flag.String("config", "configs/config.yaml", "path to config file")
	assetFilter := flag.String("asset", "", "optional asset filter (e.g. USDC)")
	flag.Parse()

	cfg, err := config.LoadWithEnvOverrides(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	client := &gateway.BinanceRESTClient{
		BaseURL:      cfg.Gateway.BaseURL,
		APIKey:       cfg.Gateway.APIKey,
		Secret:       cfg.Gateway.APISecret,
		HTTPClient:   gateway.NewDefaultHTTPClient(),
		RecvWindowMs: 5000,
	}
	balances, err := client.AccountBalances()
	if err != nil {
		log.Fatalf("fetch balances: %v", err)
	}

	filter := strings.ToUpper(strings.TrimSpace(*assetFilter))
	shown := 0
	for _, b := range balances {
		if filter != "" && strings.ToUpper(b.Asset) != filter {
			continue
		}
		fmt.Printf("%s balance=%.8f available=%.8f\n", b.Asset, b.Balance, b.Available)
		shown++
	}
	if filter != "" && shown == 0 {
		fmt.Printf("no balances matched asset %s\n", filter)
	}
}
