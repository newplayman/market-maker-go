package strategy

import (
	"fmt"
	"math"
	"sync"
)

// Quote 报价
type Quote struct {
	Side  string // "BUY" or "SELL"
	Price float64
	Size  float64
}

// Context 策略上下文
type Context struct {
	Symbol       string
	Mid          float64 // 中间价
	Inventory    float64 // 当前仓位
	MaxInventory float64 // 最大仓位
	Volatility   float64 // 波动率（可选）
}

// Fill 成交信息
type Fill struct {
	Side  string
	Price float64
	Size  float64
}

// BasicMarketMaking 基础做市策略
type BasicMarketMaking struct {
	config Config

	// 统计信息
	totalBuyFills  int
	totalSellFills int
	totalVolume    float64

	mu sync.RWMutex
}

// Config 策略配置
type Config struct {
	BaseSpread   float64 // 基础价差（如0.0005 = 0.05%）
	BaseSize     float64 // 基础下单数量
	MaxInventory float64 // 最大库存限制
	SkewFactor   float64 // 库存倾斜因子（0-1之间）
	MinSpread    float64 // 最小价差
	MaxSpread    float64 // 最大价差
	// 新增：多层持仓参数
	EnableMultiLayer bool    // 是否启用多层持仓
	LayerCount       int     // 层数（2-3）
	LayerSpacing     float64 // 层间距（以百分比表示，如0.15%）
}

// NewBasicMarketMaking 创建基础做市策略
func NewBasicMarketMaking(config Config) *BasicMarketMaking {
	// 参数验证
	if config.BaseSpread <= 0 {
		config.BaseSpread = 0.0005 // 默认0.05%
	}
	if config.BaseSize <= 0 {
		config.BaseSize = 0.01 // 默认0.01个
	}
	if config.MaxInventory <= 0 {
		config.MaxInventory = 0.05 // 默认0.05个
	}
	if config.SkewFactor <= 0 || config.SkewFactor > 1 {
		config.SkewFactor = 0.3 // 默认0.3
	}
	if config.MinSpread <= 0 {
		config.MinSpread = config.BaseSpread * 0.5
	}
	if config.MaxSpread <= 0 {
		config.MaxSpread = config.BaseSpread * 2.0
	}

	return &BasicMarketMaking{
		config: config,
	}
}

// GenerateQuotes 生成买卖报价 - 支持多层持仓
func (s *BasicMarketMaking) GenerateQuotes(ctx Context) ([]Quote, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 参数验证
	if ctx.Mid <= 0 {
		return nil, fmt.Errorf("invalid mid price: %f", ctx.Mid)
	}
	if ctx.MaxInventory <= 0 {
		ctx.MaxInventory = s.config.MaxInventory
	}

	// 1. 计算基础spread（以价格为单位）
	baseSpreadPrice := s.config.BaseSpread * ctx.Mid
	halfSpread := baseSpreadPrice / 2.0

	// 2. 计算库存倾斜
	// inventoryRatio: -1 (满仓空头) 到 +1 (满仓多头)
	inventoryRatio := 0.0

	// 3. 检查是否启用多层持仓
	if s.config.EnableMultiLayer && s.config.LayerCount > 1 {
		return s.generateMultiLayerQuotes(ctx, baseSpreadPrice, halfSpread, inventoryRatio)
	}

	// 否则使用单层报价
	return s.generateSingleLayerQuotes(ctx, baseSpreadPrice, halfSpread, inventoryRatio)
}

// generateMultiLayerQuotes 生成多层网格报价
func (s *BasicMarketMaking) generateMultiLayerQuotes(
	ctx Context,
	baseSpreadPrice float64,
	halfSpread float64,
	inventoryRatio float64) ([]Quote, error) {

	var quotes []Quote
	layerCount := s.config.LayerCount
	if layerCount < 2 {
		layerCount = 2
	}
	if layerCount > 3 {
		layerCount = 3
	}

	// 计算每层的订单大小
	sizePerLayer := s.config.BaseSize / float64(layerCount)

	// 层间距（价格单位）
	layerSpacingPrice := s.config.LayerSpacing * ctx.Mid
	if layerSpacingPrice <= 0 {
		layerSpacingPrice = 0.001 * ctx.Mid // 默认0.1%
	}

	// 库存倾斜调整
	skewAdjust := inventoryRatio * halfSpread * s.config.SkewFactor

	// 生成多个BUY层
	for layer := 0; layer < layerCount; layer++ {
		buyPrice := ctx.Mid - halfSpread - float64(layer)*layerSpacingPrice - skewAdjust
		if buyPrice > 0 {
			quotes = append(quotes, Quote{
				Side:  "BUY",
				Price: buyPrice,
				Size:  sizePerLayer,
			})
		}
	}

	// 生成多个SELL层
	for layer := 0; layer < layerCount; layer++ {
		sellPrice := ctx.Mid + halfSpread + float64(layer)*layerSpacingPrice - skewAdjust
		if sellPrice > 0 {
			quotes = append(quotes, Quote{
				Side:  "SELL",
				Price: sellPrice,
				Size:  sizePerLayer,
			})
		}
	}

	// 应用spread限制
	for i := range quotes {
		switch quotes[i].Side {
		case "BUY":
			if quotes[i].Price > ctx.Mid {
				quotes[i].Price = ctx.Mid - s.config.MinSpread*ctx.Mid/2
			}
		case "SELL":
			if quotes[i].Price < ctx.Mid {
				quotes[i].Price = ctx.Mid + s.config.MinSpread*ctx.Mid/2
			}
		}
	}

	return quotes, nil
}

// generateSingleLayerQuotes 生成单层报价
func (s *BasicMarketMaking) generateSingleLayerQuotes(
	ctx Context,
	baseSpreadPrice float64,
	halfSpread float64,
	inventoryRatio float64) ([]Quote, error) {
	if ctx.MaxInventory > 0 {
		inventoryRatio = ctx.Inventory / ctx.MaxInventory
		// 限制在 [-1, 1] 范围内
		inventoryRatio = math.Max(-1.0, math.Min(1.0, inventoryRatio))
	}

	// 倾斜量：库存过多时，向下倾斜（降低卖价，提高买价促进卖出）
	skew := inventoryRatio * s.config.SkewFactor * baseSpreadPrice

	// 3. 计算买卖价格
	// 当inventory为正（持有多头）时，skew为正
	// buyPrice应该更低（减少买入），sellPrice应该更低（促进卖出）
	buyPrice := ctx.Mid - halfSpread - skew
	sellPrice := ctx.Mid + halfSpread - skew

	// 4. 确保价格有效
	if buyPrice <= 0 || sellPrice <= 0 {
		return nil, fmt.Errorf("invalid prices: buy=%f, sell=%f", buyPrice, sellPrice)
	}
	if buyPrice >= sellPrice {
		return nil, fmt.Errorf("invalid spread: buy=%f >= sell=%f", buyPrice, sellPrice)
	}

	// 5. 计算下单数量（可以根据库存调整）
	buySize := s.config.BaseSize
	sellSize := s.config.BaseSize

	// 库存接近限制时减少相应方向的数量
	if inventoryRatio > 0.8 {
		// 持仓过多，减少买单数量
		buySize = buySize * (1 - inventoryRatio)
	}
	if inventoryRatio < -0.8 {
		// 持仓过空，减少卖单数量
		sellSize = sellSize * (1 + inventoryRatio)
	}

	// 6. 生成报价
	quotes := []Quote{
		{
			Side:  "BUY",
			Price: s.roundPrice(buyPrice, ctx.Mid),
			Size:  s.roundSize(buySize),
		},
		{
			Side:  "SELL",
			Price: s.roundPrice(sellPrice, ctx.Mid),
			Size:  s.roundSize(sellSize),
		},
	}

	return quotes, nil
}

// OnFill 成交回调
func (s *BasicMarketMaking) OnFill(fill Fill) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch fill.Side {
	case "BUY":
		s.totalBuyFills++
	case "SELL":
		s.totalSellFills++
	}
	s.totalVolume += fill.Size
}

// UpdateParameters 动态更新策略参数
func (s *BasicMarketMaking) UpdateParameters(params map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if spread, ok := params["base_spread"].(float64); ok && spread > 0 {
		s.config.BaseSpread = spread
	}
	if size, ok := params["base_size"].(float64); ok && size > 0 {
		s.config.BaseSize = size
	}
	if maxInv, ok := params["max_inventory"].(float64); ok && maxInv > 0 {
		s.config.MaxInventory = maxInv
	}
	if skew, ok := params["skew_factor"].(float64); ok && skew >= 0 && skew <= 1 {
		s.config.SkewFactor = skew
	}

	return nil
}

// GetStatistics 获取策略统计信息
func (s *BasicMarketMaking) GetStatistics() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"total_buy_fills":  s.totalBuyFills,
		"total_sell_fills": s.totalSellFills,
		"total_volume":     s.totalVolume,
		"config":           s.config,
	}
}

// GetConfig 获取当前配置
func (s *BasicMarketMaking) GetConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// roundPrice 价格取整（简化处理，实际应根据交易所tick size）
func (s *BasicMarketMaking) roundPrice(price, reference float64) float64 {
	// 根据参考价格确定精度
	tickSize := 0.01
	if reference > 1000 {
		tickSize = 0.1
	} else if reference > 100 {
		tickSize = 0.01
	} else if reference > 10 {
		tickSize = 0.001
	} else {
		tickSize = 0.0001
	}

	return math.Round(price/tickSize) * tickSize
}

// roundSize 数量取整
func (s *BasicMarketMaking) roundSize(size float64) float64 {
	// 简化处理，保留3位小数
	return math.Round(size*1000) / 1000
}
