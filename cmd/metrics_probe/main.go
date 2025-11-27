package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"market-maker-go/metrics"
)

func main() {
	addr := flag.String("metricsAddr", ":9100", "Prometheus 指标监听地址")
	ws := flag.Bool("ws", true, "是否模拟 WS 已连接")
	pf := flag.Float64("predictedFunding", 0.0001, "模拟预测资金费率")
	acc := flag.Float64("fundingPnlAcc", 0.0, "模拟累计资金费率盈亏")
	supp := flag.Bool("suppressed", false, "是否模拟 worst-case 抑制报价")
	std := flag.Float64("stddev30m", 0.0012, "模拟30分钟价格标准差")
	flag.Parse()

	metrics.StartMetricsServer(*addr)
	fmt.Printf("metrics_probe started at %s\n", *addr)

	// 额外注册一个探针指标，确保 /metrics 可见 mm_* 前缀
	probe := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "mm_probe_test",
		Help: "Probe test metric",
	})
	prometheus.MustRegister(probe)
	probe.Set(1)

	// 初始设置一批核心指标，便于 Prometheus/Grafana 验证
	if *ws {
		metrics.WSConnected.Set(1)
	} else {
		metrics.WSConnected.Set(0)
	}
	metrics.PredictedFundingRate.Set(*pf)
	metrics.FundingPnlAccum.Set(*acc)
	metrics.PriceStdDev30m.Set(*std)
	if *supp {
		metrics.QuoteSuppressed.Set(1)
	} else {
		metrics.QuoteSuppressed.Set(0)
	}

	// 周期性微调，观察值变化
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	boost := 0.0
	for {
		select {
		case <-ticker.C:
			boost += 0.00001
			metrics.PredictedFundingRate.Set(*pf + boost)
			metrics.FundingPnlAccum.Set(*acc + boost*100.0)
		}
	}
}
