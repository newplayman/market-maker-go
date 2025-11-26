// Package metrics provides Prometheus metrics for the market maker
package metrics

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// StartMetricsServer 启动Prometheus指标服务器
func StartMetricsServer(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{}))
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("metrics server listen error: %v", err)
		}
	}()
}