package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// BinanceRESTClient 一个可签名的简化客户端；默认不发起真实网络调用，HTTPClient 可注入 httptest。
type BinanceRESTClient struct {
	BaseURL    string
	APIKey     string
	Secret     string
	HTTPClient *http.Client
}

type placeResp struct {
	OrderID string `json:"orderId"`
}

// PlaceLimit 调用 /fapi/v1/order 下单（LIMIT）。
func (c *BinanceRESTClient) PlaceLimit(symbol, side, tif string, price, qty float64, reduceOnly, postOnly bool, clientID string) (string, error) {
	if c == nil || c.HTTPClient == nil {
		return "", fmt.Errorf("http client not set")
	}
	params := map[string]string{
		"symbol":      symbol,
		"side":        side,
		"type":        "LIMIT",
		"timeInForce": tif,
		"price":       fmt.Sprintf("%f", price),
		"quantity":    fmt.Sprintf("%f", qty),
	}
	if reduceOnly {
		params["reduceOnly"] = "true"
	}
	if postOnly {
		params["timeInForce"] = "GTX"
	}
	if clientID != "" {
		params["newClientOrderId"] = clientID
	}
	query, sig := SignParams(params, c.Secret)
	endpoint := c.BaseURL + "/fapi/v1/order?" + query + "&signature=" + url.QueryEscape(sig)
	req, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(nil))
	req.Header.Set("X-MBX-APIKEY", c.APIKey)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("place limit status %d", resp.StatusCode)
	}
	var pr placeResp
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return "", err
	}
	if pr.OrderID == "" {
		return "", fmt.Errorf("empty orderId")
	}
	return pr.OrderID, nil
}

// CancelOrder 调用 /fapi/v1/order 取消。
func (c *BinanceRESTClient) CancelOrder(symbol, orderID string) error {
	if c == nil || c.HTTPClient == nil {
		return fmt.Errorf("http client not set")
	}
	params := map[string]string{
		"symbol":  symbol,
		"orderId": orderID,
	}
	query, sig := SignParams(params, c.Secret)
	endpoint := c.BaseURL + "/fapi/v1/order?" + query + "&signature=" + url.QueryEscape(sig)
	req, _ := http.NewRequest(http.MethodDelete, endpoint, bytes.NewBuffer(nil))
	req.Header.Set("X-MBX-APIKEY", c.APIKey)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("cancel status %d", resp.StatusCode)
	}
	return nil
}

// NewDefaultHTTPClient 提供一个带超时的 http.Client。
func NewDefaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}
