package risk

import (
	"time"
)

// DrawdownManager 提供浮亏分层减仓的决策，不直接下单，仅给出建议方案。
// 由上层引擎在获得方案后以 Maker 优先的 reduce-only 方式执行，必要时允许小比例市价。
type DrawdownManager struct {
	Bands      []float64      // 浮亏档位（%），例如 [5,8,12]
	Fractions  []float64      // 每档对应的减仓比例，例如 [0.15,0.25,0.40]
	Mode       string         // "maker_first_then_taker"
	Cooldown   time.Duration  // 两次触发的最小间隔
	lastAction time.Time

	// 依赖
	PnL    PnLSource          // 提供当前浮盈亏（USDT 或权益百分比计算由调用方统一）
	Pos    interface{ NetExposure() float64 } // 当前净仓
	NetMax float64            // 风控净仓上限（用于计算最大可减仓量）
	Base   float64            // 基础下单量（用于最小动作颗粒度）
}

// Plan 返回建议的减仓数量（绝对值）与是否优先 Maker。
// drawdownPct 传入当前浮亏占权益的百分比（正值表示亏损百分比）。
func (d *DrawdownManager) Plan(symbol string, drawdownPct float64) (reduceQty float64, preferMaker bool, triggeredBand float64) {
	if d == nil || len(d.Bands) == 0 || len(d.Fractions) == 0 || d.Pos == nil {
		return 0, true, 0
	}
	if d.Cooldown > 0 && time.Since(d.lastAction) < d.Cooldown {
		return 0, true, 0
	}
	// 找到最高已跨越的档位
	bandIdx := -1
	for i := range d.Bands {
		if drawdownPct >= d.Bands[i] {
			bandIdx = i
		}
	}
	if bandIdx < 0 {
		return 0, true, 0
	}
	fraction := d.Fractions[bandIdx]
	if fraction <= 0 {
		return 0, true, 0
	}
	// 计算目标减仓量：按当前净仓的 fraction
	net := d.Pos.NetExposure()
	target := absFloat(net) * fraction
	// 限制到 NetMax 范围（避免负向过度）
	if d.NetMax > 0 && target > d.NetMax {
		target = d.NetMax
	}
	// 最小颗粒度：至少一个 base
	if d.Base > 0 && target < d.Base {
		target = d.Base
	}
	preferMaker = (d.Mode == "maker_first_then_taker")
	d.lastAction = time.Now() // 记录触发时间
	return target, preferMaker, d.Bands[bandIdx]
}

func (d *DrawdownManager) MarkAction() {
	d.lastAction = time.Now()
}

func absFloat(x float64) float64 {
	if x < 0 { return -x }
	return x
}
