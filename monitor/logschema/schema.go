package logschema

import (
	"fmt"
	"sort"
	"strings"
)

// Schema 定义每个日志事件所需的关键字段，便于集中校验。
type Schema struct {
	Event    string
	Required []string
}

var schemas = map[string]Schema{
	"strategy_adjust": {
		Event:    "strategy_adjust",
		Required: []string{"symbol", "mid", "spread", "spreadRatio", "intervalMs"},
	},
	"risk_event": {
		Event:    "risk_event",
		Required: []string{"symbol", "state"},
	},
	"order_update": {
		Event:    "order_update",
		Required: []string{"symbol", "status", "clientOrderId"},
	},
	"depth_snapshot": {
		Event:    "depth_snapshot",
		Required: []string{"symbol", "bid", "ask"},
	},
}

// Known 返回所有事件名，便于外部生成文档。
func Known() []string {
	names := make([]string, 0, len(schemas))
	for k := range schemas {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// Validate 检查日志字段是否包含 schema 中要求的 key。
func Validate(event string, fields map[string]interface{}) error {
	s, ok := schemas[event]
	if !ok {
		return nil
	}
	var missing []string
	for _, key := range s.Required {
		if _, exists := fields[key]; !exists {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing fields: %s", strings.Join(missing, ","))
	}
	return nil
}
