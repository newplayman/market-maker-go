package gateway

import "net/http"

// BuildRealBinanceClients 根据环境变量快速构建 REST/WS/ListenKey 客户端（仅骨架，不发起连接）。
// 调用方可传入自定义 http.Client（带代理/超时），否则使用默认。
func BuildRealBinanceClients(httpCli *http.Client) (*BinanceRESTClient, *ListenKeyClient, *BinanceWSReal) {
	env := LoadEnvConfig()
	if httpCli == nil {
		httpCli = NewDefaultHTTPClient()
	}
	rest := &BinanceRESTClient{
		BaseURL:    env.RestURL,
		APIKey:     env.APIKey,
		Secret:     env.APISecret,
		HTTPClient: httpCli,
	}
	lk := &ListenKeyClient{
		BaseURL:    env.RestURL,
		APIKey:     env.APIKey,
		HTTPClient: NewListenKeyHTTPClient(),
	}
	ws := NewBinanceWSReal()
	ws.BaseEndpoint = env.WSEndpoint
	return rest, lk, ws
}
