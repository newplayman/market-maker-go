package risk

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"market-maker-go/internal/store"
	"market-maker-go/metrics"
)

// GrindingConfig 磨成本配置。
type GrindingConfig struct {
	Enabled           bool
	TriggerRatio      float64 // 仓位 ≥ TriggerRatio * NetMax 才触发
	RangeStdThreshold float64 // 30分钟价格标准差阈值（<该值才横盘）
	GrindSizePct      float64 // 每次磨成本的反向 taker 占当前仓位的百分比
	ReentrySpreadBps  float64 // 磨成本后重新挂 maker 的有利偏移（基点）
	MaxGrindPerHour   int
	MinIntervalSec    int
	FundingBoost      bool
	FundingFavorMult  float64
}

// GrindingEngine 磨成本引擎。
type GrindingEngine struct {
	cfg       GrindingConfig
	store     *store.Store
	netMax    float64
	placer    OrderPlacer
	mu        sync.Mutex
	grindLog  []time.Time
	lastGrind time.Time

	costSaved         float64
	baseSize          float64
	pinSizeMultiplier float64
}

type OrderPlacer interface {
	PlaceMarket(symbol, side string, qty float64) error
	PlaceLimit(symbol, side string, price, qty float64) error
}

func NewGrindingEngine(cfg GrindingConfig, st *store.Store, netMax, baseSize, pinSizeMultiplier float64, placer OrderPlacer) *GrindingEngine {
	return &GrindingEngine{
		cfg:               cfg,
		store:             st,
		netMax:            netMax,
		baseSize:          baseSize,
		pinSizeMultiplier: pinSizeMultiplier,
		placer:            placer,
		grindLog:          make([]time.Time, 0, 100),
	}
}

// MaybeGrind 每 55 秒调用一次，检查是否磨成本。
func (g *GrindingEngine) MaybeGrind(position, mid float64) error {
	if !g.cfg.Enabled {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	// 频率限制：间隔 <42s 禁止
	if time.Since(g.lastGrind) < time.Duration(g.cfg.MinIntervalSec)*time.Second {
		return nil
	}
	// 小时限制：统计最近1小时内磨成本次数
	now := time.Now()
	cutoff := now.Add(-1 * time.Hour)
	var recentCount int
	for _, t := range g.grindLog {
		if t.After(cutoff) {
			recentCount++
		}
	}
	if recentCount >= g.cfg.MaxGrindPerHour {
		return nil
	}

	// 检查仓位是否达到阈值
	absPos := math.Abs(position)
	if absPos < g.cfg.TriggerRatio*g.netMax {
		return nil
	}

	// 检查横盘条件：价格标准差 <阈值
	stdDev := g.store.PriceStdDev30m()
	if stdDev >= g.cfg.RangeStdThreshold {
		return nil
	}

	// 资金费率有利时自动放大
	fundingRate := g.store.PredictedFundingRate()
	mult := 1.0
	if g.cfg.FundingBoost {
		// 正费率→多头吃亏；若持多头且费率正→有利磨
		if position > 0 && fundingRate > 0 {
			mult = g.cfg.FundingFavorMult
		} else if position < 0 && fundingRate < 0 {
			mult = g.cfg.FundingFavorMult
		}
	}

	// 计算磨成本尺寸
	grindSize := absPos * g.cfg.GrindSizePct * mult
	if grindSize <= 0 {
		return nil
	}

	log.Printf("Grinding triggered: pos=%.4f mid=%.2f stdDev=%.4f fundingRate=%.6f grindSize=%.4f",
		position, mid, stdDev, fundingRate, grindSize)

	metrics.GrindActive.Set(1)
	defer metrics.GrindActive.Set(0)

	// 多头磨：先市价卖 → 立即挂买单（低4.2bps）
	// 空头磨：先市价买 → 立即挂卖单（高4.2bps）
	symbol := g.store.Symbol
	var side, reentSide string
	var reentPrice float64
	if position > 0 {
		side = "SELL"
		reentSide = "BUY"
		reentPrice = mid * (1 - g.cfg.ReentrySpreadBps/10000)
	} else {
		side = "BUY"
		reentSide = "SELL"
		reentPrice = mid * (1 + g.cfg.ReentrySpreadBps/10000)
	}

	// 市价单
	if err := g.placer.PlaceMarket(symbol, side, grindSize); err != nil {
		return fmt.Errorf("grind market %s %.4f: %w", side, grindSize, err)
	}

	// 追 maker 单（使用钉子模式倍数）
	reentQty := g.baseSize * g.pinSizeMultiplier
	if err := g.placer.PlaceLimit(symbol, reentSide, reentPrice, reentQty); err != nil {
		log.Printf("grind reentry limit %s %.4f @ %.2f failed: %v", reentSide, reentQty, reentPrice, err)
	}

	// 记录
	g.lastGrind = now
	g.grindLog = append(g.grindLog, now)
	// 估算节省成本（粗略：减少持仓对资金费率暴露）
	g.costSaved += grindSize * mid * math.Abs(fundingRate)
	metrics.GrindCountTotal.Inc()
	metrics.GrindCostSaved.Set(g.costSaved)

	return nil
}
