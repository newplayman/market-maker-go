package strategy

import (
	"errors"
	"time"
)

// Quote represents a bid/ask decision.
type Quote struct {
	Bid float64
	Ask float64
	// Size is symmetric for simplicity; real impl 可拆分 bid/ask size。
	Size float64
}

// EngineConfig 控制最小价差、目标仓位等核心参数。
type EngineConfig struct {
	MinSpread      float64 // 最小价差（如 0.0005 表示 5bps）
	TargetPosition float64 // 目标仓位（正=多，负=空）
	MaxDrift       float64 // 可接受的仓位偏移
	BaseSize       float64 // 报价基础数量
}

// MarketSnapshot 提供 mid 价与时间，实际应含更多行情字段。
type MarketSnapshot struct {
	Mid float64
	Ts  time.Time
}

// Inventory 提供当前净仓位。
type Inventory interface {
	NetExposure() float64
}

// Engine 负责根据行情和仓位生成报价。
type Engine struct {
	cfg EngineConfig
}

func NewEngine(cfg EngineConfig) (*Engine, error) {
	if cfg.MinSpread <= 0 || cfg.BaseSize <= 0 {
		return nil, errors.New("invalid engine config")
	}
	return &Engine{cfg: cfg}, nil
}

// QuoteZeroInventory 基于零库存策略生成报价：围绕 mid 对称挂单，满足最小价差。
func (e *Engine) QuoteZeroInventory(s MarketSnapshot, inv Inventory) Quote {
	spread := e.cfg.MinSpread * s.Mid
	if spread <= 0 {
		spread = 0.0001
	}
	bid := s.Mid - spread/2
	ask := s.Mid + spread/2

	// 按仓位偏移调整：如果多头过多，下移 bid/ask，反之上移。
	drift := 0.0
	if inv != nil {
		pos := inv.NetExposure()
		diff := pos - e.cfg.TargetPosition
		if diff > e.cfg.MaxDrift {
			drift = spread * 0.25
		} else if diff < -e.cfg.MaxDrift {
			drift = -spread * 0.25
		}
	}
	return Quote{
		Bid:  bid - drift,
		Ask:  ask - drift,
		Size: e.cfg.BaseSize,
	}
}

// BacktestUpdate 用于离线回测：输入 mid 序列和仓位序列，输出报价曲线（供测试/调参）。
func (e *Engine) BacktestUpdate(snaps []MarketSnapshot, invs []float64) []Quote {
	res := make([]Quote, 0, len(snaps))
	for i, s := range snaps {
		var inv Inventory
		if i < len(invs) {
			inv = staticInv{net: invs[i]}
		}
		res = append(res, e.QuoteZeroInventory(s, inv))
	}
	return res
}

type staticInv struct{ net float64 }

func (s staticInv) NetExposure() float64 { return s.net }
