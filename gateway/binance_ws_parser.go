package gateway

import (
	"encoding/json"
)

// CombinedMessage 对应 binance combined stream 包装。
type CombinedMessage struct {
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

// DepthUpdate 提取 depth@100ms 消息的核心字段。
type DepthUpdate struct {
	Symbol string           `json:"s"`
	Bids   [][2]json.Number `json:"b"`
	Asks   [][2]json.Number `json:"a"`
}

// ParseCombinedDepth 解析 combined stream 的 depth 消息，返回符号、最好 bid/ask。
func ParseCombinedDepth(raw []byte) (symbol string, bestBid, bestAsk float64, err error) {
	var msg CombinedMessage
	if err = json.Unmarshal(raw, &msg); err != nil {
		return
	}
	var depth DepthUpdate
	if err = json.Unmarshal(msg.Data, &depth); err != nil {
		return
	}
	symbol = depth.Symbol
	if len(depth.Bids) > 0 {
		bestBid, _ = depth.Bids[0][0].Float64()
	}
	if len(depth.Asks) > 0 {
		bestAsk, _ = depth.Asks[0][0].Float64()
	}
	return
}
