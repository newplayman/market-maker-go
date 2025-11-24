package monitor

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Monitor Prometheus监控指标收集器
type Monitor struct {
	registry *prometheus.Registry
	
	// 订单指标
	ordersPlaced    prometheus.Counter
	ordersCanceled  prometheus.Counter
	ordersFilled    prometheus.Counter
	ordersRejected  prometheus.Counter
	orderLatency    prometheus.Histogram
	
	// 交易指标
	tradesTotal     prometheus.Counter
	tradedVolume    prometheus.Counter
	
	// 仓位指标
	position        prometheus.Gauge
	unrealizedPnL   prometheus.Gauge
	realizedPnL     prometheus.Gauge
	
	// 市场指标
	midPrice        prometheus.Gauge
	spread          prometheus.Gauge
	bidPrice        prometheus.Gauge
	askPrice        prometheus.Gauge
	
	// 风控指标
	riskState       prometheus.Gauge
	riskRejects     prometheus.Counter
	positionLimit   prometheus.Gauge
	
	// 策略指标
	quoteInterval   prometheus.Gauge
	quotesGenerated prometheus.Counter
	
	// 系统指标
	wsConnections   prometheus.Counter
	wsDisconnects   prometheus.Counter
	restRequests    *prometheus.CounterVec
	restErrors      *prometheus.CounterVec
	restLatency     *prometheus.HistogramVec
	
	mu sync.RWMutex
}

// Config 监控配置
type Config struct {
	Namespace string
	Subsystem string
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		Namespace: "mm",
		Subsystem: "trading",
	}
}

// New 创建新的Monitor实例
func New(cfg Config) *Monitor {
	reg := prometheus.NewRegistry()
	
	// 创建factory
	factory := promauto.With(reg)
	
	m := &Monitor{
		registry: reg,
		
		// 订单指标
		ordersPlaced: factory.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "orders_placed_total",
			Help:      "订单下单总数",
		}),
		ordersCanceled: factory.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "orders_canceled_total",
			Help:      "订单撤单总数",
		}),
		ordersFilled: factory.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "orders_filled_total",
			Help:      "订单成交总数",
		}),
		ordersRejected: factory.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "orders_rejected_total",
			Help:      "订单拒绝总数",
		}),
		orderLatency: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "order_latency_seconds",
			Help:      "订单延迟分布（秒）",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		}),
		
		// 交易指标
		tradesTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "trades_total",
			Help:      "成交笔数总数",
		}),
		tradedVolume: factory.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "traded_volume_total",
			Help:      "累计成交量",
		}),
		
		// 仓位指标
		position: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "position",
			Help:      "当前净仓位",
		}),
		unrealizedPnL: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "unrealized_pnl",
			Help:      "未实现盈亏",
		}),
		realizedPnL: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "realized_pnl",
			Help:      "已实现盈亏",
		}),
		
		// 市场指标
		midPrice: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "mid_price",
			Help:      "当前中间价",
		}),
		spread: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "spread",
			Help:      "当前价差",
		}),
		bidPrice: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "bid_price",
			Help:      "当前买一价",
		}),
		askPrice: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "ask_price",
			Help:      "当前卖一价",
		}),
		
		// 风控指标
		riskState: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "risk_state",
			Help:      "风控状态(0=正常,1=只减仓,2=暂停)",
		}),
		riskRejects: factory.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "risk_rejects_total",
			Help:      "风控拒单总数",
		}),
		positionLimit: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "position_limit",
			Help:      "仓位限制",
		}),
		
		// 策略指标
		quoteInterval: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "quote_interval_seconds",
			Help:      "报价间隔（秒）",
		}),
		quotesGenerated: factory.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "quotes_generated_total",
			Help:      "策略生成报价总数",
		}),
		
		// 系统指标
		wsConnections: factory.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "ws_connections_total",
			Help:      "WebSocket连接次数",
		}),
		wsDisconnects: factory.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "ws_disconnects_total",
			Help:      "WebSocket断开次数",
		}),
		restRequests: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: cfg.Namespace,
				Subsystem: cfg.Subsystem,
				Name:      "rest_requests_total",
				Help:      "REST请求总数",
			},
			[]string{"action"},
		),
		restErrors: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: cfg.Namespace,
				Subsystem: cfg.Subsystem,
				Name:      "rest_errors_total",
				Help:      "REST错误总数",
			},
			[]string{"action"},
		),
		restLatency: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: cfg.Namespace,
				Subsystem: cfg.Subsystem,
				Name:      "rest_latency_seconds",
				Help:      "REST请求延迟（秒）",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"action"},
		),
	}
	
	return m
}

// 订单相关方法
func (m *Monitor) RecordOrderPlaced() {
	m.ordersPlaced.Inc()
}

func (m *Monitor) RecordOrderCanceled() {
	m.ordersCanceled.Inc()
}

func (m *Monitor) RecordOrderFilled() {
	m.ordersFilled.Inc()
}

func (m *Monitor) RecordOrderRejected() {
	m.ordersRejected.Inc()
}

func (m *Monitor) RecordOrderLatency(seconds float64) {
	m.orderLatency.Observe(seconds)
}

// 交易相关方法
func (m *Monitor) RecordTrade(volume float64) {
	m.tradesTotal.Inc()
	m.tradedVolume.Add(volume)
}

// 仓位相关方法
func (m *Monitor) UpdatePosition(value float64) {
	m.position.Set(value)
}

func (m *Monitor) UpdateUnrealizedPnL(value float64) {
	m.unrealizedPnL.Set(value)
}

func (m *Monitor) UpdateRealizedPnL(value float64) {
	m.realizedPnL.Set(value)
}

// 市场相关方法
func (m *Monitor) UpdateMidPrice(value float64) {
	m.midPrice.Set(value)
}

func (m *Monitor) UpdateSpread(value float64) {
	m.spread.Set(value)
}

func (m *Monitor) UpdateBidAsk(bid, ask float64) {
	m.bidPrice.Set(bid)
	m.askPrice.Set(ask)
}

// 风控相关方法
func (m *Monitor) UpdateRiskState(state int) {
	m.riskState.Set(float64(state))
}

func (m *Monitor) RecordRiskReject() {
	m.riskRejects.Inc()
}

func (m *Monitor) UpdatePositionLimit(value float64) {
	m.positionLimit.Set(value)
}

// 策略相关方法
func (m *Monitor) UpdateQuoteInterval(seconds float64) {
	m.quoteInterval.Set(seconds)
}

func (m *Monitor) RecordQuoteGenerated() {
	m.quotesGenerated.Inc()
}

// 系统相关方法
func (m *Monitor) RecordWSConnection() {
	m.wsConnections.Inc()
}

func (m *Monitor) RecordWSDisconnect() {
	m.wsDisconnects.Inc()
}

func (m *Monitor) RecordRESTRequest(action string) {
	m.restRequests.WithLabelValues(action).Inc()
}

func (m *Monitor) RecordRESTError(action string) {
	m.restErrors.WithLabelValues(action).Inc()
}

func (m *Monitor) RecordRESTLatency(action string, seconds float64) {
	m.restLatency.WithLabelValues(action).Observe(seconds)
}

// Handler 返回HTTP handler用于暴露指标
func (m *Monitor) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Registry 返回prometheus registry
func (m *Monitor) Registry() *prometheus.Registry {
	return m.registry
}
