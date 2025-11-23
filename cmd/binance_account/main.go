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
	symbol := flag.String("symbol", "ETHUSDC", "需要查看杠杆档位的合约符号")
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

	info, err := client.AccountInfo()
	if err != nil {
		log.Fatalf("获取账户信息失败: %v", err)
	}
	fmt.Printf("账户总余额: %.4f USDC, 可用余额: %.4f, 总浮盈亏: %.4f\n",
		info.TotalWalletBalance, info.AvailableBalance, info.TotalUnrealizedProfit)
	for _, a := range info.Assets {
		if a.Asset == "USDC" {
			fmt.Printf("USDC 余额: %.4f, 可提现: %.4f\n", a.WalletBalance, a.MaxWithdraw)
		}
	}

	filter := strings.ToUpper(strings.TrimSpace(*symbol))
	brackets, err := client.LeverageBrackets(filter)
	if err != nil {
		log.Fatalf("获取杠杆档位失败: %v", err)
	}
	for _, lb := range brackets {
		if strings.ToUpper(lb.Symbol) != filter {
			continue
		}
		fmt.Printf("%s 杠杆档位:\n", lb.Symbol)
		for _, b := range lb.Brackets {
			fmt.Printf("  档位%d: 初始杠杆=%.0f, 名义区间=%.0f~%.0f, 维持保证金率=%.4f\n",
				b.Bracket, b.InitialLeverage, b.NotionalFloor, b.NotionalCap, b.MaintMarginRate)
		}
		return
	}
	fmt.Printf("未找到 %s 的杠杆档位数据\n", filter)
}
