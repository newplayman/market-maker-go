// Package metrics provides Prometheus metrics for the market maker
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// StartMetricsServer 启动Prometheus指标服务器
func StartMetricsServer(addr string) {
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		_ = http.ListenAndServe(addr, nil)
	}()
}