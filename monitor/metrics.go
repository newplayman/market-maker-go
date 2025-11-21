package monitor

// 指标名称占位，便于后续 Prometheus/日志收集使用。
const (
	MetricWsReconnects    = "ws_reconnects_total"
	MetricRestErrors      = "rest_errors_total"
	MetricOrderRejects    = "order_rejects_total"
	MetricRiskTriggered   = "risk_triggers_total"
	MetricLatencyExceeded = "order_latency_exceeded_total"
)

// RecordCounter 占位接口，实际可接 Prometheus/Register。
type Recorder interface {
	Inc(name string, labels map[string]string)
}
