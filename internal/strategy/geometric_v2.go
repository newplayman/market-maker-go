package strategy

import (
	"math"

	"market-maker-go/internal/store"
	"market-maker-go/metrics"
)

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
func (s *GeometricV2) GenerateQuotes(position, mid float64) ([]Quote, []Quote) {
	var buys, sells []Quote
	
	pendingBuy := s.store.PendingBuySize()
	pendingSell := s.store.PendingSellSize()
	
	// 计算最坏敞口
	worstLong := position + pendingBuy
	worstShort := position - pendingSell
	
	metrics.WorstCaseLong.Set(worstLong)
	metrics.WorstCaseShort.Set(math.Abs(worstShort))
	
	maxAllowedLong := s.cfg.NetMax * s.cfg.WorstCaseMult
	maxAllowedShort := -s.cfg.NetMax * s.cfg.WorstCaseMult
	
	suppressed := false
	for layer := 0; layer < s.cfg.MaxLayers; layer++ {
		// 计算买单
		if worstLong < maxAllowedLong {
			q := s.genQuote("BUY", layer, position, worstLong, mid)
			if q.Size > 0 {
				buys = append(buys, q)
				worstLong += q.Size
			}
		} else {
			suppressed = true
		}
		
		// 计算卖单
		if worstShort > maxAllowedShort {
			q := s.genQuote("SELL", layer, position, worstShort, mid)
			if q.Size > 0 {
				sells = append(sells, q)
				worstShort -= q.Size
			}
		} else {
			suppressed = true
		}
	}
	
	if suppressed {
		metrics.QuoteSuppressed.Set(1)
	} else {
		metrics.QuoteSuppressed.Set(0)
	}
	return buys, sells
}

// genQuote 生成单层报价（含指数衰减）。
func (s *GeometricV2) genQuote(side string, layer int, position, exposure, mid float64) Quote {
	// 计算档位间距（几何）
	spread := s.cfg.MinSpread * math.Pow(s.cfg.SpacingRatio, float64(layer))
	var price float64
	if side == "BUY" {
		price = mid * (1 - spread)
	} else {
		price = mid * (1 + spread)
	}

	// 指数衰减 size
	absExposure := math.Abs(exposure)
	decay := math.Exp(-absExposure / s.cfg.NetMax * s.cfg.SizeDecayK)
	layerDecay := math.Pow(s.cfg.LayerSizeDecay, float64(layer))
	size := s.cfg.BaseSize * decay * layerDecay

	metrics.DynamicDecayFactor.Set(decay)

	return Quote{
		Side:  side,
		Price: price,
		Size:  size,
	}
}
