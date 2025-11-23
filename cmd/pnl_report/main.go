package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type stats struct {
	trades       int
	buyNotional  float64
	sellNotional float64
	realizedPnL  float64
}

func (s *stats) add(side string, price, qty, pnl float64) {
	if qty <= 0 || price <= 0 {
		return
	}
	notion := price * qty
	s.trades++
	switch strings.ToUpper(side) {
	case "BUY":
		s.buyNotional += notion
	case "SELL":
		s.sellNotional += notion
	default:
		s.buyNotional += notion / 2
		s.sellNotional += notion / 2
	}
	s.realizedPnL += pnl
}

func main() {
	logPath := flag.String("log", "/var/log/market-maker/runner.log", "runner 日志路径")
	symbol := flag.String("symbol", "", "仅统计指定交易对 (默认全量)")
	sinceStr := flag.String("since", "", "仅统计此时间之后的记录 (RFC3339，例如 2025-11-22T00:00:00Z)")
	flag.Parse()

	var since time.Time
	var err error
	if *sinceStr != "" {
		since, err = time.Parse(time.RFC3339Nano, *sinceStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "解析 since 参数失败: %v\n", err)
			os.Exit(1)
		}
	}

	f, err := os.Open(*logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法读取日志: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	orderSides := make(map[string]string)
	st := stats{}

	for scanner.Scan() {
		line := scanner.Text()
		idx := strings.Index(line, "{")
		if idx == -1 {
			continue
		}
		payload := line[idx:]
		var evt map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			continue
		}
		evtName, _ := evt["event"].(string)
		if evtName == "" {
			continue
		}
		if *symbol != "" {
			if sym, ok := evt["symbol"].(string); ok && sym != *symbol {
				continue
			}
		}
		if !since.IsZero() {
			if tsStr, ok := evt["ts"].(string); ok {
				if ts, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
					if ts.Before(since) {
						continue
					}
				}
			}
		}

		switch evtName {
		case "order_place":
			orderID := fmt.Sprint(evt["orderId"])
			side, _ := evt["side"].(string)
			orderSides[orderID] = side
		case "order_update":
			status, _ := evt["status"].(string)
			if status != "FILLED" && status != "PARTIALLY_FILLED" {
				continue
			}
			orderID := fmt.Sprint(evt["orderId"])
			side := orderSides[orderID]
			price := toFloat(evt["lastPrice"])
			qty := toFloat(evt["lastQty"])
			pnl := toFloat(evt["pnl"])
			st.add(side, price, qty, pnl)
			delete(orderSides, orderID)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "读取日志出错: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("统计文件: %s\n", *logPath)
	if *symbol != "" {
		fmt.Printf("交易对: %s\n", *symbol)
	}
	if !since.IsZero() {
		fmt.Printf("起始时间: %s\n", since.Format(time.RFC3339))
	}
	fmt.Printf("成交笔数: %d\n", st.trades)
	fmt.Printf("买单名义: %.4f USDC\n", st.buyNotional)
	fmt.Printf("卖单名义: %.4f USDC\n", st.sellNotional)
	fmt.Printf("净成交差额: %.4f USDC\n", st.sellNotional-st.buyNotional)
	fmt.Printf("Realized PnL (来自 Binance 回报): %.6f USDC\n", st.realizedPnL)
}

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}
