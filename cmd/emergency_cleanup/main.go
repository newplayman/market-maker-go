package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
	"crypto/hmac"
	"crypto/sha256"
)

func main() {
	apiKey := os.Getenv("BINANCE_API_KEY")
	apiSecret := os.Getenv("BINANCE_API_SECRET")
	if apiKey == "" || apiSecret == "" {
		log.Fatal("需要 BINANCE_API_KEY 和 BINANCE_API_SECRET")
	}

	symbol := "ETHUSDC"
	baseURL := "https://fapi.binance.com"

	// 1. 取消所有挂单
	fmt.Println("🔸 取消所有挂单...")
	if err := cancelAll(baseURL, apiKey, apiSecret, symbol); err != nil {
		log.Printf("取消挂单失败: %v", err)
	} else {
		fmt.Println("✅ 所有挂单已取消")
	}

	// 2. 查询当前仓位
	fmt.Println("\n🔸 查询当前仓位...")
	position, err := getPosition(baseURL, apiKey, apiSecret, symbol)
	if err != nil {
		log.Fatalf("查询仓位失败: %v", err)
	}

	fmt.Printf("当前仓位: %.4f ETH\n", position)

	if position == 0 {
		fmt.Println("✅ 没有持仓，无需平仓")
		return
	}

	// 3. 平仓
	fmt.Printf("\n🔸 平仓 %.4f ETH...\n", position)
	side := "SELL"
	if position < 0 {
		side = "BUY"
		position = -position
	}

	if err := placeMarket(baseURL, apiKey, apiSecret, symbol, side, position); err != nil {
		log.Fatalf("平仓失败: %v", err)
	}

	fmt.Println("✅ 平仓订单已提交")
	
	// 等待3秒后再次查询
	time.Sleep(3 * time.Second)
	finalPos, _ := getPosition(baseURL, apiKey, apiSecret, symbol)
	fmt.Printf("\n最终仓位: %.4f ETH\n", finalPos)
}

func sign(secret, query string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(query))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func cancelAll(baseURL, apiKey, secret, symbol string) error {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	query := fmt.Sprintf("symbol=%s&timestamp=%s", symbol, ts)
	sig := sign(secret, query)
	
	reqURL := fmt.Sprintf("%s/fapi/v1/allOpenOrders?%s&signature=%s", baseURL, query, sig)
	req, _ := http.NewRequest("DELETE", reqURL, nil)
	req.Header.Set("X-MBX-APIKEY", apiKey)
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	return nil
}

func getPosition(baseURL, apiKey, secret, symbol string) (float64, error) {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	query := fmt.Sprintf("timestamp=%s", ts)
	sig := sign(secret, query)
	
	reqURL := fmt.Sprintf("%s/fapi/v2/positionRisk?%s&signature=%s", baseURL, query, sig)
	req, _ := http.NewRequest("GET", reqURL, nil)
	req.Header.Set("X-MBX-APIKEY", apiKey)
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	var positions []map[string]interface{}
	if err := json.Unmarshal(body, &positions); err != nil {
		return 0, err
	}
	
	for _, p := range positions {
		if p["symbol"] == symbol {
			amt, _ := strconv.ParseFloat(p["positionAmt"].(string), 64)
			return amt, nil
		}
	}
	return 0, nil
}

func placeMarket(baseURL, apiKey, secret, symbol, side string, qty float64) error {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", side)
	params.Set("type", "MARKET")
	params.Set("quantity", fmt.Sprintf("%.3f", qty))
	params.Set("reduceOnly", "true")
	params.Set("timestamp", ts)
	
	query := params.Encode()
	sig := sign(secret, query)
	
	reqURL := fmt.Sprintf("%s/fapi/v1/order?%s&signature=%s", baseURL, query, sig)
	req, _ := http.NewRequest("POST", reqURL, nil)
	req.Header.Set("X-MBX-APIKEY", apiKey)
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	return nil
}
