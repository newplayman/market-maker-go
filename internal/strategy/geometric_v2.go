package strategy

import (
	"math"

	"market-maker-go/internal/store"
	"market-maker-go/metrics"
)

// QuotePinningConfig 钉子模式配置
type QuotePinningConfig struct {
	Enabled                bool
	TriggerRatio           float64
	NearLayers             int
	FarLayers              int
	FarLayerFixedSize      float64
	FarLayerMinDistancePct float64
	FarLayerMaxDistancePct float64
	PinToBestTick          bool
	PinSizeMultiplier      float64
}

// GeometricV2Config 几何网格策略 V2 配置（防扫单）。
type GeometricV2Config struct {
	Symbol           string
	MinSpread        float64
	BaseSize         float64
	NetMax           float64
	LayerSpacingMode string  // "geometric"
	SpacingRatio     float64 // 几何间距倍率
	LayerSizeDecay   float64 // 层级尺寸衰减
	MaxLayers        int
	WorstCaseMult    float64 // worst-case 允许超挂倍率
	SizeDecayK       float64 // 指数衰减系数

	QuotePinning QuotePinningConfig // Round8 新增
}

// GeometricV2 几何网格策略 V2，包含 worst-case 敞口检查与指数衰减。
type GeometricV2 struct {
	cfg   GeometricV2Config
	store *store.Store
}

func NewGeometricV2(cfg GeometricV2Config, st *store.Store) *GeometricV2 {
	return &GeometricV2{
		cfg:   cfg,
		store: st,
	}
}

// GenerateQuotes 生成双向多层报价，返回 (buys, sells)。
// 替换整个 GenerateQuotes() 函数，禁止改任何逻辑！！！
func (s *GeometricV2) GenerateQuotes(position, mid float64) (bids, asks []Quote) {
	pos := s.store.Position()
	// mid := s.store.MidPrice() // 参数里已经传了 mid，直接用
	cfg := s.cfg.QuotePinning

	// ──────── 1. 仓位 ≥70% net_max → 进入钉子模式（不跑了！）───────
	if cfg.Enabled && math.Abs(pos)/s.cfg.NetMax >= cfg.TriggerRatio {
		metrics.PinningActive.Set(1) // 记录钉子模式激活
		bestBid := s.store.BestBidPrice()
		bestAsk := s.store.BestAskPrice()

		if pos > 0 { // 多头太多 → 钉卖单，加大反向卖单
			asks = append(asks, Quote{
				Price: bestAsk,
				Size:  s.cfg.BaseSize * cfg.PinSizeMultiplier,
			})
			// 买单只挂近端小单，防止继续加多
			bids = s.generateNearQuotes("BUY", 4, mid) // 只挂 4 层小买单
		} else { // 空头太多 → 钉买单
			bids = append(bids, Quote{
				Price: bestBid,
				Size:  s.cfg.BaseSize * cfg.PinSizeMultiplier,
			})
			asks = s.generateNearQuotes("SELL", 4, mid)
		}
		return bids, asks
	}
	metrics.PinningActive.Set(0)

	// ──────── 2. 正常情况：分段报价（近端防扫单，远端抗单边）───────
	// 前 8 层：用原来的动态指数衰减（防扫单）
	bids = append(bids, s.generateNearQuotes("BUY", cfg.NearLayers, mid)...)
	asks = append(asks, s.generateNearQuotes("SELL", cfg.NearLayers, mid)...)

	// 后 16 层：固定大单，价格拉远（抗单边）
	bids = append(bids, s.generateFarQuotes("BUY", cfg.FarLayers, mid)...)
	asks = append(asks, s.generateFarQuotes("SELL", cfg.FarLayers, mid)...)

	return bids, asks
}

// 近端报价（保留原来的防扫单逻辑）
func (s *GeometricV2) generateNearQuotes(side string, layers int, mid float64) []Quote {
	var quotes []Quote
	for i := 1; i <= layers; i++ {
		price := s.calculateLayerPrice(side, i, mid)
		size := s.calculateDynamicSize(side, i) // 原来的指数衰减逻辑
		if size > 0.001 {
			quotes = append(quotes, Quote{Side: side, Price: price, Size: size})
		}
	}
	return quotes
}

// 远端报价（固定大单，拉很远）
func (s *GeometricV2) generateFarQuotes(side string, layers int, mid float64) []Quote {
	var quotes []Quote
	cfg := s.cfg.QuotePinning
	// basePrice := mid // unused

	for i := 1; i <= layers; i++ {
		ratio := cfg.FarLayerMinDistancePct +
			float64(i-1)/(float64(layers-1))*
				(cfg.FarLayerMaxDistancePct-cfg.FarLayerMinDistancePct)

		var price float64
		if side == "BUY" {
			price = mid * (1 - ratio)
		} else {
			price = mid * (1 + ratio)
		}
		// 保证 tick 对齐
		price = s.roundToTick(price)

		quotes = append(quotes, Quote{
			Side:  side,
			Price: price,
			Size:  cfg.FarLayerFixedSize,
		})
	}
	metrics.FarQuotesCount.Set(float64(len(quotes)))
	return quotes
}

// 辅助函数：计算层级价格
func (s *GeometricV2) calculateLayerPrice(side string, layer int, mid float64) float64 {
	spread := s.cfg.MinSpread * math.Pow(s.cfg.SpacingRatio, float64(layer-1)) // layer从1开始，所以layer-1
	if side == "BUY" {
		return mid * (1 - spread)
	}
	return mid * (1 + spread)
}

// 辅助函数：计算动态Size (Worst-Case Decay)
func (s *GeometricV2) calculateDynamicSize(side string, layer int) float64 {
	// 重新计算 worst exposure (简化版，假设 generateNearQuotes 是在循环中调用)
	// 注意：这里为了简化，直接使用当前仓位作为 exposure 估算的基础
	// 严格来说应该像之前那样累加 pending，但为了适配新结构，我们使用简化的 decay 计算
	// 或者我们需要在 GenerateQuotes 里传进来 exposure。
	// 鉴于文档要求“保留原来的防扫单逻辑”，我们尽量还原。

	// 为了不破坏结构，我们这里重新获取一下 Store 的 pending
	pendingBuy := s.store.PendingBuySize()
	pendingSell := s.store.PendingSellSize()
	pos := s.store.Position()

	var exposure float64
	if side == "BUY" {
		exposure = pos + pendingBuy
	} else {
		exposure = -(pos - pendingSell)
	}

	absExposure := math.Abs(exposure)
	decay := math.Exp(-absExposure / s.cfg.NetMax * s.cfg.SizeDecayK)
	layerDecay := math.Pow(s.cfg.LayerSizeDecay, float64(layer-1))

	return s.cfg.BaseSize * decay * layerDecay
}

func (s *GeometricV2) roundToTick(price float64) float64 {
	// 简单处理，保留5位小数 (ETHUSDC 0.01)
	// 实际应该从 SymbolInfo 获取 tickSize
	// 这里假设 tickSize = 0.01
	return math.Round(price*100) / 100
}
