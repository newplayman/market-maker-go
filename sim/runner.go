package sim

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"market-maker-go/inventory"
	"market-maker-go/market"
	"market-maker-go/metrics"
	"market-maker-go/order"
	"market-maker-go/posttrade"
	"market-maker-go/risk"
	"market-maker-go/strategy"
	"market-maker-go/strategy/asmm"
)

// RiskState 描述 Runner 当前的风险状态。
type RiskState int

const (
	RiskStateNormal RiskState = iota
	RiskStateReduceOnly
	RiskStateHalted
)

func (s RiskState) String() string {
	switch s {
	case RiskStateReduceOnly:
		return "reduce_only"
	case RiskStateHalted:
		return "halted"
	default:
		return "normal"
	}
}

// StrategyAdjustInfo 用于对外暴露策略调节参数。
type StrategyAdjustInfo struct {
	Mid                float64
	Spread             float64
	SpreadRatio        float64
	VolFactor          float64
	InventoryFactor    float64
	Interval           time.Duration
	NetExposure        float64
	ReduceOnly         bool
	TakeProfitActive   bool
	DepthFillPrice     float64
	DepthFillAvailable float64
	DepthSlippage      float64
}

// RiskGuard 用于下单前校验（可对接风险控制）。
type RiskGuard interface {
	PreOrder(symbol string, deltaQty float64) error
}

// Runner 将行情->策略->下单串起来，负责把 OrderBook/Inventory 状态与策略引擎的报价结果对齐，
// 并在内部管理静态/动态挂单、Reduce-only、止损等逻辑。cmd/runner 会使用真实 gateway 将其接入交易所。
type Runner struct {
	Symbol       string
	Engine       *strategy.Engine
	ASMMStrategy *asmm.ASMMStrategy
	Inv          *inventory.Tracker
	OrderMgr     *order.Manager
	Risk         RiskGuard
	Book         *market.OrderBook // 可选，供 VWAPGuard 使用
	// Constraints 用于在下单前对齐 tickSize/stepSize，并满足 minQty/minNotional。
	Constraints             order.SymbolConstraints
	BaseSpread              float64
	BaseInterval            time.Duration
	TakeProfitPct           float64
	NetMax                  float64
	StopLoss                float64
	HaltDuration            time.Duration
	ShockThreshold          float64
	ReduceOnlyThreshold     float64
	ReduceOnlyMaxSlippage   float64
	ReduceOnlyMarketTrigger float64
	reduceFailCount         map[string]int
	reduceFallbackUntil     time.Time
	reduceFallbackActive    bool
	reduceBackoffUntil      time.Time
	StaticFraction          float64
	StaticThresholdTicks    int
	StaticRestDuration      time.Duration
	DynamicRestDuration     time.Duration
	DynamicThresholdTicks   int
	PostOnlyCooldown        time.Duration
	staticBidID             string
	staticAskID             string
	staticBidPrice          float64
	staticAskPrice          float64
	staticBidPlacedAt       time.Time
	staticAskPlacedAt       time.Time
	lastBidPlacedAt         time.Time
	lastAskPlacedAt         time.Time
	postOnlyCooldown        map[string]time.Time
	reduceCooldownUntil     time.Time
	makerShiftTicks         map[string]int
	haltUntil               time.Time
	prevMid                 float64
	lastQuoteTime           time.Time
	lastBidID               string
	lastAskID               string
	lastBidPrice            float64
	lastAskPrice            float64
	riskState               RiskState
	onRiskStateChange       func(RiskState, string)
	onStrategyAdjust        func(StrategyAdjustInfo)
	// 多档动态挂单状态
	dynamicBids []levelState
	dynamicAsks []levelState
	// 自适应风控相关
	postTradeAnalyzer  *posttrade.Analyzer
	adaptiveRisk       *risk.AdaptiveRiskManager
	lastAdaptiveUpdate time.Time
	// 成交跟踪与撤单抑制
	fillTracker              *order.FillTracker
	cancelSuppressionEnabled bool
	fillRateThreshold        float64 // 成交率阈值（每分钟）
	recentFillsThreshold     int     // 近期成交次数阈值
}

// OnTick 是 Runner 的主循环：它会根据 mid 计算新的报价、处理 Reduce-only/静态挂单、调用 Risk Guard，
// 再将 buy/sell 两条腿提交给 order.Manager。任何错误都会反映为 risk_event/quote_error 供监控使用。

// levelState 维护单档挂单的状态
type levelState struct {
	id       string
	price    float64
	placedAt time.Time
}

func (r *Runner) OnTick(mid float64) error {
	if (r.Engine == nil && r.ASMMStrategy == nil) || r.OrderMgr == nil || r.Inv == nil {
		return errors.New("runner not initialized")
	}
	if mid <= 0 {
		return errors.New("invalid mid")
	}
	now := time.Now()
	if !r.haltUntil.IsZero() && now.Before(r.haltUntil) {
		return fmt.Errorf("halted until %s", r.haltUntil.UTC().Format(time.RFC3339))
	}
	if !r.haltUntil.IsZero() && !now.Before(r.haltUntil) && r.riskState == RiskStateHalted {
		r.haltUntil = time.Time{}
		r.setRiskState(RiskStateNormal, "halt_cleared")
	}
	if r.ShockThreshold > 0 && r.prevMid > 0 {
		if math.Abs(mid-r.prevMid)/r.prevMid >= r.ShockThreshold {
			return r.triggerHalt(fmt.Sprintf("volatility_halt pct=%.4f", math.Abs(mid-r.prevMid)/r.prevMid))
		}
	}
	r.prevMid = mid

	// 行情陈旧度守卫：若 OrderBook 更新过期则跳过本次报价
	if r.Book != nil {
		maxStale := r.BaseInterval
		if maxStale <= 0 {
			maxStale = 2 * time.Second
		}
		if time.Since(r.Book.LastUpdate()) > 3*maxStale {
			return fmt.Errorf("stale_orderbook staleness=%s", time.Since(r.Book.LastUpdate()))
		}
	}

	// 周期性更新自适应风控参数
	if r.adaptiveRisk != nil {
		adaptiveInterval := 5 * time.Minute
		if time.Since(r.lastAdaptiveUpdate) > adaptiveInterval {
			r.adaptiveRisk.Update()
			r.lastAdaptiveUpdate = now
		}
	}

	// 更新 FillTracker 指标
	if r.fillTracker != nil {
		stats := r.fillTracker.GetStats()
		suppressionActive := r.shouldSuppressCancel()
		metrics.UpdateFillTrackerMetrics(stats.RecentFillRate, stats.RecentFills, suppressionActive)
	}

	if r.StopLoss != 0 {
		_, pnl := r.Inv.Valuation(mid)
		if (r.StopLoss < 0 && pnl <= r.StopLoss) || (r.StopLoss > 0 && pnl >= r.StopLoss) {
			return r.triggerHalt(fmt.Sprintf("stop_loss pnl=%.2f", pnl))
		}
	}

	// 根据策略类型生成报价
	var bid, ask, size float64
	var quotes []asmm.Quote
	var asmmBidReduceOnly, asmmAskReduceOnly bool
	if r.ASMMStrategy != nil {
		// 基于ASMM策略生成报价
		var bestBid, bestAsk float64
		var imbalance float64
		if r.Book != nil {
			bestBid, bestAsk = r.Book.Best()
			imbalance = market.CalculateImbalanceFromOrderBook(r.Book, 3)
		}
		snap := market.Snapshot{
			Mid:       mid,
			BestBid:   bestBid,
			BestAsk:   bestAsk,
			Spread:    0,
			Imbalance: imbalance,
			Timestamp: time.Now().Unix(),
		}
		if bestBid > 0 && bestAsk > 0 && bestAsk > bestBid {
			snap.Spread = bestAsk - bestBid
		}
		quotes = r.ASMMStrategy.GenerateQuotes(snap, r.Inv.NetExposure())
		var bidFound, askFound bool
		var bidSize, askSize float64
		for _, q := range quotes {
			if q.Side == asmm.Bid && !bidFound {
				bid = q.Price
				bidSize = q.Size
				bidFound = true
				if q.ReduceOnly {
					asmmBidReduceOnly = true
				}
			} else if q.Side == asmm.Ask && !askFound {
				ask = q.Price
				askSize = q.Size
				askFound = true
				if q.ReduceOnly {
					asmmAskReduceOnly = true
				}
			}
			if bidFound && askFound {
				break
			}
		}
		// 统一下单尺寸：取两侧最小值（避免不一致）
		if bidFound || askFound {
			if bidFound && askFound {
				size = bidSize
				if askSize > 0 && askSize < size {
					size = askSize
				}
			} else if bidFound {
				size = bidSize
			} else {
				size = askSize
			}
		} else {
			return errors.New("asmm: no quotes generated")
		}
	} else {
		// 使用原有的网格策略
		snap := strategy.MarketSnapshot{Mid: mid, Ts: time.Now()}
		quote := r.Engine.QuoteZeroInventory(snap, invWrapper{r.Inv})
		size = quote.Size
		bid = quote.Bid
		ask = quote.Ask
	}

	if size <= 0 {
		return errors.New("invalid size")
	}

	spreadAbs, spreadRatio, volFactor, invFactor := r.computeSpread(mid)
	if spreadAbs <= 0 {
		if r.ASMMStrategy != nil {
			// ASMM策略计算
			spreadAbs = ask - bid
		} else {
			// 网格策略计算
			spreadAbs = (ask - bid)
		}
		if spreadAbs <= 0 {
			spreadAbs = mid * r.BaseSpread
		}
		if mid > 0 {
			spreadRatio = spreadAbs / mid
		}
	}

	// 如果是ASMM策略，bid和ask已经计算好了，不需要重新计算
	if r.ASMMStrategy == nil {
		bid = mid - spreadAbs/2
		ask = mid + spreadAbs/2
		bid, ask = r.applyInventorySkew(bid, ask, spreadAbs)
		bid, ask = r.applyTakeProfit(mid, bid, ask)
		bid, ask = r.applyInsertStrategy(bid, ask)
	}

	qty := size
	var err error
	bid, ask, qty, err = alignQuote(r.Constraints, bid, ask, qty)
	if err != nil {
		return err
	}

	allowBuy, allowSell := true, true
	net := r.Inv.NetExposure()
	reduceLimit := r.reduceOnlyLimit()
	hardReduce := false
	softReduce := false
	if reduceLimit > 0 {
		if net >= reduceLimit {
			allowBuy = false
			hardReduce = true
		} else if net <= -reduceLimit {
			allowSell = false
			hardReduce = true
		} else {
			softLimit := reduceLimit * 0.7
			if softLimit > 0 {
				if net >= softLimit {
					allowBuy = false
					softReduce = true
				} else if net <= -softLimit {
					allowSell = false
					softReduce = true
				}
			}
		}
	}
	reduceOnly := false
	if r.riskState != RiskStateHalted {
		if !allowBuy || !allowSell {
			reason := fmt.Sprintf("reduce_only net=%.4f", net)
			if !hardReduce && softReduce {
				reason = fmt.Sprintf("soft_reduce net=%.4f", net)
			}
			r.setRiskState(RiskStateReduceOnly, reason)
			reduceOnly = true
		} else if r.riskState == RiskStateReduceOnly {
			r.setRiskState(RiskStateNormal, "reduce_only_exit")
			r.resetReduceFailures()
			if r.BaseInterval > 0 {
				r.reduceCooldownUntil = time.Now().Add(2 * r.BaseInterval)
			} else {
				r.reduceCooldownUntil = time.Now().Add(500 * time.Millisecond)
			}
		}
	}
	if !reduceOnly {
		bid, ask = r.applyMakerShift(bid, ask)
	}
	if !reduceOnly {
		if !r.reduceCooldownUntil.IsZero() && time.Now().Before(r.reduceCooldownUntil) {
			if net > 0 {
				allowBuy = false
			} else if net < 0 {
				allowSell = false
			}
		} else {
			r.reduceCooldownUntil = time.Time{}
		}
	}
	buyReduceOnly := (reduceOnly && allowBuy && !allowSell) || asmmBidReduceOnly
	sellReduceOnly := (reduceOnly && allowSell && !allowBuy) || asmmAskReduceOnly
	bid, ask = r.applyReduceOnly(mid, bid, ask, allowBuy, allowSell)
	var depthPlan reducePlan
	buyPostOnly := r.postOnlyReady("BUY")
	sellPostOnly := r.postOnlyReady("SELL")
	buyTIF := ""
	sellTIF := ""
	profitPositive := r.hasPositivePnL(mid)
	if buyReduceOnly {
		buyPostOnly = false
		buyTIF = "IOC"
		if r.shouldFallbackReduceOnly() {
			buyPostOnly = false
			buyTIF = "IOC"
		}
		plan := r.planReduceOnlyPrice(true, mid, bid, qty)
		bid = plan.price
		if r.Constraints.TickSize > 0 {
			bid = snapDown(bid, r.Constraints.TickSize)
		}
		depthPlan = plan
	}
	if sellReduceOnly {
		sellPostOnly = false
		sellTIF = "IOC"
		if r.shouldFallbackReduceOnly() {
			sellPostOnly = false
			sellTIF = "IOC"
		}
		plan := r.planReduceOnlyPrice(false, mid, ask, qty)
		ask = plan.price
		if r.Constraints.TickSize > 0 {
			ask = snapUp(ask, r.Constraints.TickSize)
		}
		depthPlan = plan
	}

	// 若ASMM返回多档报价，走多档差分下发路径
	if r.ASMMStrategy != nil {
		var bidQuotes, askQuotes []asmm.Quote
		for _, q := range quotes {
			if q.Side == asmm.Bid {
				bidQuotes = append(bidQuotes, q)
			} else if q.Side == asmm.Ask {
				askQuotes = append(askQuotes, q)
			}
		}
		if len(bidQuotes) > 1 || len(askQuotes) > 1 {
			// 取消旧的单档动态订单
			r.cancelOutstanding(true, true)
			// 差分下发多档
			r.reconcileDynamicQuotes(mid, bidQuotes, askQuotes, allowBuy, allowSell)
			// 通知与静态挂单维护
			r.lastQuoteTime = time.Now()
			r.notifyStrategyAdjust(StrategyAdjustInfo{
				Mid:                mid,
				Spread:             ask - bid,
				SpreadRatio:        spreadRatio,
				VolFactor:          volFactor,
				InventoryFactor:    invFactor,
				Interval:           r.dynamicInterval(mid),
				NetExposure:        net,
				ReduceOnly:         reduceOnly,
				TakeProfitActive:   r.TakeProfitPct > 0 && net != 0,
				DepthFillPrice:     0,
				DepthFillAvailable: 0,
				DepthSlippage:      0,
			})
			metrics.SpreadGauge.WithLabelValues(r.Symbol).Set(ask - bid)
			metrics.QuoteIntervalGauge.WithLabelValues(r.Symbol).Set(float64(r.dynamicInterval(mid)) / float64(time.Second))
			r.manageStaticOrders(mid, spreadAbs, size, reduceOnly)
			return nil
		}
	}

	// 非多档路径，清理残留动态档位挂单
	if len(r.dynamicBids) > 0 {
		for i := range r.dynamicBids {
			_ = r.cancelDynamicLeg(true, i)
		}
		r.dynamicBids = nil
	}
	if len(r.dynamicAsks) > 0 {
		for i := range r.dynamicAsks {
			_ = r.cancelDynamicLeg(false, i)
		}
		r.dynamicAsks = nil
	}
	placeBuy := allowBuy
	placeSell := allowSell
	cancelBid := !allowBuy
	cancelAsk := !allowSell
	if allowBuy {
		cancelBid = true
		if buyReduceOnly {
			if !r.shouldReplaceReduceOrder(r.lastBidPrice, bid, mid, profitPositive) {
				cancelBid = false
				placeBuy = false
			}
		} else if placeBuy && !r.shouldReplacePassive(r.lastBidPrice, bid, r.lastBidPlacedAt) {
			cancelBid = false
			placeBuy = false
		}
	}
	if allowSell {
		cancelAsk = true
		if sellReduceOnly {
			if !r.shouldReplaceReduceOrder(r.lastAskPrice, ask, mid, profitPositive) {
				cancelAsk = false
				placeSell = false
			}
		} else if placeSell && !r.shouldReplacePassive(r.lastAskPrice, ask, r.lastAskPlacedAt) {
			cancelAsk = false
			placeSell = false
		}
	}

	r.cancelOutstanding(cancelBid, cancelAsk)
	if reduceOnly && r.tryMarketReduce(mid, net) {
		if net > 0 {
			placeSell = false
		} else if net < 0 {
			placeBuy = false
		}
	}

	// 买单
	if placeBuy {
		if r.Risk != nil {
			if err := r.Risk.PreOrder(r.Symbol, qty); err != nil {
				return err
			}
		}
		bidOrder, usedPostOnly, err := r.submitOrderWithFallback("BUY", order.Order{
			Symbol:      r.Symbol,
			Side:        "BUY",
			Price:       bid,
			Quantity:    qty,
			ReduceOnly:  buyReduceOnly,
			PostOnly:    buyPostOnly,
			TimeInForce: buyTIF,
		}, buyReduceOnly)
		if err != nil {
			if buyReduceOnly {
				r.recordReduceFailure("BUY")
			}
			return err
		}
		if buyReduceOnly {
			r.markReduceSuccess("BUY")
		} else {
			r.decayMakerShift("BUY")
			if usedPostOnly {
				r.clearPostOnlyCooldown("BUY")
			}
		}
		r.lastBidID = bidOrder.ID
		r.lastBidPrice = bid
		r.lastBidPlacedAt = time.Now()
		metrics.IncrementOrdersPlaced("buy")
	}

	// 卖单
	if placeSell {
		if r.Risk != nil {
			if err := r.Risk.PreOrder(r.Symbol, -qty); err != nil {
				return err
			}
		}
		askOrder, usedPostOnly, err := r.submitOrderWithFallback("SELL", order.Order{
			Symbol:      r.Symbol,
			Side:        "SELL",
			Price:       ask,
			Quantity:    qty,
			ReduceOnly:  sellReduceOnly,
			PostOnly:    sellPostOnly,
			TimeInForce: sellTIF,
		}, sellReduceOnly)
		if err != nil {
			if sellReduceOnly {
				r.recordReduceFailure("SELL")
			}
			return err
		}
		if sellReduceOnly {
			r.markReduceSuccess("SELL")
		} else {
			r.decayMakerShift("SELL")
			if usedPostOnly {
				r.clearPostOnlyCooldown("SELL")
			}
		}
		r.lastAskID = askOrder.ID
		r.lastAskPrice = ask
		r.lastAskPlacedAt = time.Now()
		metrics.IncrementOrdersPlaced("sell")
	}
	if !allowBuy && !allowSell {
		return fmt.Errorf("reduce_only_block net=%.4f", net)
	}
	r.lastQuoteTime = time.Now()
	r.notifyStrategyAdjust(StrategyAdjustInfo{
		Mid:                mid,
		Spread:             ask - bid,
		SpreadRatio:        spreadRatio,
		VolFactor:          volFactor,
		InventoryFactor:    invFactor,
		Interval:           r.dynamicInterval(mid),
		NetExposure:        net,
		ReduceOnly:         reduceOnly,
		TakeProfitActive:   r.TakeProfitPct > 0 && net != 0,
		DepthFillPrice:     depthPlan.depthPrice,
		DepthFillAvailable: depthPlan.depthAvailable,
		DepthSlippage:      depthPlan.slippage,
	})
	metrics.SpreadGauge.WithLabelValues(r.Symbol).Set(ask - bid)
	metrics.QuoteIntervalGauge.WithLabelValues(r.Symbol).Set(float64(r.dynamicInterval(mid)) / float64(time.Second))
	r.manageStaticOrders(mid, spreadAbs, size, reduceOnly)
	return nil
}

func (r *Runner) haltDuration() time.Duration {
	if r.HaltDuration > 0 {
		return r.HaltDuration
	}
	return 10 * time.Second
}

func (r *Runner) computeSpread(mid float64) (abs float64, ratio float64, volFactor float64, invFactor float64) {
	base := r.BaseSpread
	if base <= 0 {
		base = 0.001
	}
	volFactor = r.volatilityFactor(mid)
	ratio = base * (1 + volFactor)
	invFactor = 0
	if r.NetMax > 0 {
		invFactor = math.Max(math.Min(r.Inv.NetExposure()/r.NetMax, 1), -1)
		ratio *= 1 + math.Abs(invFactor)
	}
	abs = ratio * mid
	return
}

func (r *Runner) volatilityFactor(mid float64) float64 {
	if r.prevMid == 0 || mid == 0 {
		return 0
	}
	diff := math.Abs(mid-r.prevMid) / r.prevMid
	if diff == 0 {
		return 0
	}
	scale := r.ShockThreshold
	if scale <= 0 {
		scale = 0.002
	}
	return math.Min(diff/scale, 3)
}

func (r *Runner) applyInventorySkew(bid, ask, spread float64) (float64, float64) {
	if r.NetMax <= 0 {
		return bid, ask
	}
	net := r.Inv.NetExposure()
	if net == 0 {
		return bid, ask
	}
	factor := math.Max(math.Min(net/r.NetMax, 1), -1)
	shift := factor * spread * 0.5
	bid -= shift
	ask -= shift
	return bid, ask
}

func (r *Runner) applyTakeProfit(mid, bid, ask float64) (float64, float64) {
	if r.TakeProfitPct <= 0 {
		return bid, ask
	}
	net := r.Inv.NetExposure()
	if net == 0 {
		return bid, ask
	}
	cost := r.Inv.AvgCost()
	if cost <= 0 {
		return bid, ask
	}
	pnlPct := (mid - cost) / cost
	tp := r.TakeProfitPct
	if net > 0 && pnlPct > tp {
		ask = math.Min(ask, mid*(1+tp*0.5))
	} else if net < 0 && -pnlPct > tp {
		bid = math.Max(bid, mid*(1-tp*0.5))
	}
	return bid, ask
}

func (r *Runner) applyInsertStrategy(bid, ask float64) (float64, float64) {
	if r.Book == nil {
		return bid, ask
	}
	bestBid, bestAsk := r.Book.Best()
	if bestBid == 0 || bestAsk == 0 {
		return bid, ask
	}
	gap := bestAsk - bestBid
	if gap <= 0 {
		return bid, ask
	}
	spread := ask - bid
	tick := r.Constraints.TickSize
	if tick <= 0 {
		tick = 0.01
	}
	if gap > spread {
		bid = math.Max(bid, bestBid+tick)
		ask = math.Min(ask, bestAsk-tick)
	} else {
		bid = math.Max(bid, bestBid)
		ask = math.Min(ask, bestAsk)
	}
	if bid >= ask {
		bid = bestBid
		ask = bestAsk
	}
	return bid, ask
}

func (r *Runner) applyReduceOnly(mid, bid, ask float64, allowBuy, allowSell bool) (float64, float64) {
	if allowBuy && allowSell {
		return bid, ask
	}
	bestBid, bestAsk := 0.0, 0.0
	if r.Book != nil {
		bestBid, bestAsk = r.Book.Best()
	}
	if !allowBuy {
		if bestBid > 0 {
			ask = math.Min(ask, bestBid)
		} else {
			ask = math.Min(ask, mid)
		}
	}
	if !allowSell {
		if bestAsk > 0 {
			bid = math.Max(bid, bestAsk)
		} else {
			bid = math.Max(bid, mid)
		}
	}
	return bid, ask
}

type reducePlan struct {
	price          float64
	depthPrice     float64
	depthAvailable float64
	slippage       float64
}

func (r *Runner) hasPositivePnL(mid float64) bool {
	if mid <= 0 || r.Inv == nil {
		return false
	}
	cost := r.Inv.AvgCost()
	if cost <= 0 {
		return false
	}
	net := r.Inv.NetExposure()
	if net > 0 {
		return mid >= cost
	}
	if net < 0 {
		return mid <= cost
	}
	return false
}

func (r *Runner) currentPnLPct(mid float64) float64 {
	if mid <= 0 || r.Inv == nil {
		return 0
	}
	cost := r.Inv.AvgCost()
	if cost <= 0 {
		return 0
	}
	net := r.Inv.NetExposure()
	if net == 0 {
		return 0
	}
	if net > 0 {
		return (mid - cost) / cost
	}
	return (cost - mid) / cost
}

func (r *Runner) shouldReplaceReduceOrder(existing, target, mid float64, profitPositive bool) bool {
	if target <= 0 || mid <= 0 {
		return true
	}
	if existing == 0 {
		return true
	}
	tol := r.BaseSpread * 0.25
	if tol <= 0 {
		tol = 0.0002
	}
	if profitPositive {
		tol *= 2
	}
	diffRatio := math.Abs(existing-target) / mid
	return diffRatio > tol
}

// shouldReplacePassive 判断动态腿是否需要替换：若现有挂单价格与目标价差距不大，并且未超过 rest duration，则直接保留挂单避免“闪撤”。
func (r *Runner) shouldReplacePassive(existing, target float64, placedAt time.Time) bool {
	if target <= 0 {
		return true
	}
	if existing == 0 {
		return true
	}
	tick := r.Constraints.TickSize
	if r.DynamicThresholdTicks > 0 && tick > 0 {
		threshold := float64(r.DynamicThresholdTicks) * tick
		if threshold > 0 && math.Abs(existing-target) < threshold {
			if r.DynamicRestDuration <= 0 {
				return false
			}
			if placedAt.IsZero() || time.Since(placedAt) < r.DynamicRestDuration {
				return false
			}
		}
	} else if r.DynamicRestDuration > 0 && !placedAt.IsZero() && time.Since(placedAt) < r.DynamicRestDuration {
		return false
	}
	return true
}

func (r *Runner) planReduceOnlyPrice(isBuy bool, mid, current, qty float64) reducePlan {
	plan := reducePlan{price: current}
	if qty <= 0 {
		return plan
	}
	var bestBid, bestAsk float64
	if r.Book != nil {
		bestBid, bestAsk = r.Book.Best()
		var side market.DepthSide
		if isBuy {
			side = market.DepthSideAsk
		} else {
			side = market.DepthSideBid
		}
		depthPrice, depthAvail := r.Book.EstimateFillPrice(side, qty)
		plan.depthPrice = depthPrice
		plan.depthAvailable = depthAvail
		if depthPrice > 0 {
			current = depthPrice
		}
	}
	if current == 0 {
		current = mid
	}
	slip := r.ReduceOnlyMaxSlippage
	if slip <= 0 {
		slip = 0.002
	}
	if r.shouldFallbackReduceOnly() {
		slip *= 2
	}
	var limit float64
	if isBuy {
		limit = current
		if bestAsk > 0 && limit < bestAsk {
			limit = bestAsk
		}
		aggressive := mid * (1 + slip)
		if aggressive > 0 && (limit == 0 || limit < aggressive) {
			limit = aggressive
		}
		plan.price = limit
		if mid > 0 {
			plan.slippage = math.Max((limit-mid)/mid, 0)
		}
	} else {
		limit = current
		if bestBid > 0 && (limit == 0 || bestBid < limit) {
			limit = bestBid
		}
		aggressive := mid * (1 - slip)
		if aggressive > 0 && (limit == 0 || limit > aggressive) {
			limit = aggressive
		}
		plan.price = limit
		if mid > 0 {
			plan.slippage = math.Max((mid-limit)/mid, 0)
		}
	}
	if plan.price <= 0 && mid > 0 {
		if isBuy {
			plan.price = mid * (1 + slip)
			plan.slippage = slip
		} else {
			plan.price = mid * (1 - slip)
			plan.slippage = slip
		}
	}
	return plan
}

func (r *Runner) tryMarketReduce(mid, net float64) bool {
	if r.ReduceOnlyMarketTrigger <= 0 || mid <= 0 || net == 0 || r.OrderMgr == nil {
		return false
	}
	pnlPct := r.currentPnLPct(mid)
	if pnlPct < r.ReduceOnlyMarketTrigger {
		return false
	}
	qty := math.Abs(net)
	if r.Engine != nil {
		if base := r.Engine.BaseSize(); base > 0 && qty > base {
			qty = base
		}
	}
	if r.Constraints.StepSize > 0 {
		qty = roundToStep(qty, r.Constraints.StepSize)
	}
	if qty <= 0 {
		return false
	}
	side := "SELL"
	delta := -qty
	if net < 0 {
		side = "BUY"
		delta = qty
	}
	if r.Risk != nil {
		if err := r.Risk.PreOrder(r.Symbol, delta); err != nil {
			return false
		}
	}
	ord := order.Order{
		Symbol:     r.Symbol,
		Side:       side,
		Quantity:   qty,
		ReduceOnly: true,
		Type:       "MARKET",
	}
	if _, err := r.OrderMgr.Submit(ord); err != nil {
		return false
	}
	return true
}

// manageStaticOrders 负责维护“底仓”挂单：遵守 staticFraction/staticTicks/staticRest 的约束，
// 在 reduce-only 或暂停状态下会自动撤单，正常状态则尽量保持在盘口等待成交。
func (r *Runner) manageStaticOrders(mid, spreadAbs, baseSize float64, reduceOnly bool) {
	if r.StaticFraction <= 0 {
		r.cancelStaticOrders()
		return
	}
	if reduceOnly || r.riskState != RiskStateNormal {
		r.cancelStaticOrders()
		return
	}
	// 若存在多档动态腿，暂停静态挂单避免冲突
	if len(r.dynamicBids) > 0 || len(r.dynamicAsks) > 0 {
		r.cancelStaticOrders()
		return
	}
	staticQty := baseSize * r.StaticFraction
	if staticQty <= 0 {
		return
	}
	bid := mid - spreadAbs/2
	ask := mid + spreadAbs/2
	bid, ask, staticQty, err := alignQuote(r.Constraints, bid, ask, staticQty)
	if err != nil {
		return
	}
	threshold := spreadAbs
	tick := r.Constraints.TickSize
	if r.StaticThresholdTicks > 0 && tick > 0 {
		threshold = float64(r.StaticThresholdTicks) * tick
	} else if threshold <= 0 && tick > 0 {
		threshold = tick
	}
	if r.postOnlyReady("BUY") {
		r.ensureStaticOrder(&r.staticBidID, &r.staticBidPrice, &r.staticBidPlacedAt, bid, staticQty, "BUY", threshold)
	} else {
		r.cancelStaticByID(&r.staticBidID, &r.staticBidPlacedAt)
	}
	if r.postOnlyReady("SELL") {
		r.ensureStaticOrder(&r.staticAskID, &r.staticAskPrice, &r.staticAskPlacedAt, ask, staticQty, "SELL", threshold)
	} else {
		r.cancelStaticByID(&r.staticAskID, &r.staticAskPlacedAt)
	}
}

// ensureStaticOrder 在满足阈值的情况下提交静态挂单，并仅当价格/时间超过阈值时才撤单重挂。
func (r *Runner) ensureStaticOrder(id *string, price *float64, placedAt *time.Time, target float64, qty float64, side string, threshold float64) {
	now := time.Now()
	if r.staticOrderActive(*id) {
		if math.Abs(*price-target) <= threshold {
			return
		}
		if r.StaticRestDuration > 0 && !placedAt.IsZero() && now.Sub(*placedAt) < r.StaticRestDuration {
			return
		}
		r.cancelStaticByID(id, placedAt)
	}
	order := order.Order{
		Symbol:   r.Symbol,
		Side:     side,
		Price:    target,
		Quantity: qty,
		PostOnly: true,
	}
	res, err := r.OrderMgr.Submit(order)
	if err != nil {
		return
	}
	*id = res.ID
	*price = target
	*placedAt = now
}

func (r *Runner) staticOrderActive(id string) bool {
	if id == "" {
		return false
	}
	st, ok := r.OrderMgr.Status(id)
	if !ok {
		return false
	}
	switch st {
	case order.StatusCanceled, order.StatusFilled, order.StatusRejected:
		return false
	default:
		return true
	}
}

func (r *Runner) cancelStaticOrders() {
	r.cancelStaticByID(&r.staticBidID, &r.staticBidPlacedAt)
	r.cancelStaticByID(&r.staticAskID, &r.staticAskPlacedAt)
}

func (r *Runner) cancelStaticByID(id *string, placedAt *time.Time) {
	if *id == "" {
		return
	}
	if !r.staticOrderActive(*id) {
		*id = ""
		if placedAt != nil {
			*placedAt = time.Time{}
		}
		return
	}
	_ = r.OrderMgr.Cancel(*id)
	*id = ""
	if placedAt != nil {
		*placedAt = time.Time{}
	}
}

func (r *Runner) reduceOnlyLimit() float64 {
	thr := r.ReduceOnlyThreshold
	if r.Engine == nil {
		return thr
	}
	base := r.Engine.BaseSize()
	if base <= 0 {
		return thr
	}
	if thr == 0 {
		return base
	}
	if thr > base && thr > 1 {
		return thr * base
	}
	return thr
}

const (
	reduceFailThreshold    = 3
	reduceFallbackDuration = 2 * time.Second
	maxMakerShiftTicks     = 5
)

func (r *Runner) recordReduceFailure(side string) {
	if r.reduceFailCount == nil {
		r.reduceFailCount = make(map[string]int)
	}
	r.reduceFailCount[side]++
	if r.reduceFailCount[side] >= reduceFailThreshold {
		r.reduceFallbackActive = true
		r.reduceFallbackUntil = time.Now().Add(reduceFallbackDuration)
	}
}

func (r *Runner) markReduceSuccess(side string) {
	if r.reduceFailCount != nil {
		delete(r.reduceFailCount, side)
		if len(r.reduceFailCount) == 0 {
			r.reduceFallbackActive = false
			r.reduceFallbackUntil = time.Time{}
		}
	}
}

func (r *Runner) resetReduceFailures() {
	if r.reduceFailCount != nil {
		for k := range r.reduceFailCount {
			delete(r.reduceFailCount, k)
		}
	}
	r.reduceFallbackActive = false
	r.reduceFallbackUntil = time.Time{}
}

func (r *Runner) shouldFallbackReduceOnly() bool {
	if !r.reduceFallbackActive {
		return false
	}
	if time.Now().After(r.reduceFallbackUntil) {
		r.resetReduceFailures()
		return false
	}
	return true
}

func (r *Runner) applyMakerShift(bid, ask float64) (float64, float64) {
	if r.makerShiftTicks == nil {
		return bid, ask
	}
	tick := r.Constraints.TickSize
	if tick <= 0 {
		tick = 0.01
	}
	if shift := r.makerShiftTicks["BUY"]; shift > 0 {
		bid -= float64(shift) * tick
	}
	if shift := r.makerShiftTicks["SELL"]; shift > 0 {
		ask += float64(shift) * tick
	}
	return bid, ask
}

func (r *Runner) bumpMakerShift(side string) {
	if r.makerShiftTicks == nil {
		r.makerShiftTicks = make(map[string]int)
	}
	if r.makerShiftTicks[side] < maxMakerShiftTicks {
		r.makerShiftTicks[side]++
	}
}

func (r *Runner) decayMakerShift(side string) {
	if r.makerShiftTicks == nil {
		return
	}
	if r.makerShiftTicks[side] > 0 {
		r.makerShiftTicks[side]--
	}
}

func (r *Runner) postOnlyReady(side string) bool {
	if r.postOnlyCooldown == nil {
		return true
	}
	until, ok := r.postOnlyCooldown[side]
	if !ok {
		return true
	}
	if time.Now().After(until) {
		delete(r.postOnlyCooldown, side)
		return true
	}
	return false
}

func (r *Runner) enterPostOnlyCooldown(side string) {
	if r.postOnlyCooldown == nil {
		r.postOnlyCooldown = make(map[string]time.Time)
	}
	dur := r.PostOnlyCooldown
	if dur <= 0 {
		dur = 1500 * time.Millisecond
	}
	r.postOnlyCooldown[side] = time.Now().Add(dur)
}

func (r *Runner) clearPostOnlyCooldown(side string) {
	if r.postOnlyCooldown == nil {
		return
	}
	delete(r.postOnlyCooldown, side)
}

func (r *Runner) submitOrderWithFallback(side string, ord order.Order, reduceOnly bool) (*order.Order, bool, error) {
	if strings.ToUpper(ord.Type) == "MARKET" {
		res, err := r.OrderMgr.Submit(ord)
		return res, false, err
	}
	currentPostOnly := ord.PostOnly
	triedFallback := false
	for {
		res, err := r.OrderMgr.Submit(ord)
		if err == nil {
			return res, currentPostOnly, nil
		}
		if !reduceOnly && currentPostOnly && isPostOnlyReject(err) && !triedFallback {
			r.enterPostOnlyCooldown(side)
			r.bumpMakerShift(side)
			metrics.IncrementPostOnlyRejectFallback(strings.ToLower(side))
			currentPostOnly = false
			ord.PostOnly = false
			triedFallback = true
			continue
		}
		return nil, currentPostOnly, err
	}
}

func isPostOnlyReject(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Post Only") || strings.Contains(msg, "code\":-5022")
}

func (r *Runner) ReadyForNext(mid float64) bool {
	now := time.Now()
	if !r.haltUntil.IsZero() {
		if now.Before(r.haltUntil) {
			return false
		}
		if r.riskState == RiskStateHalted {
			r.haltUntil = time.Time{}
			r.setRiskState(RiskStateNormal, "halt_cleared")
		}
	}
	if r.BaseInterval <= 0 {
		return true
	}
	if r.lastQuoteTime.IsZero() {
		return true
	}
	required := r.dynamicInterval(mid)
	return time.Since(r.lastQuoteTime) >= required
}

func (r *Runner) dynamicInterval(mid float64) time.Duration {
	base := r.BaseInterval
	if base <= 0 {
		base = 2 * time.Second
	}
	factor := 1 + r.volatilityFactor(mid)
	if r.ReduceOnlyThreshold > 0 && math.Abs(r.Inv.NetExposure()) >= r.ReduceOnlyThreshold {
		factor *= 1.5
	}
	return time.Duration(float64(base) * factor)
}

// 多档差分下发与状态维护
func (r *Runner) reconcileDynamicQuotes(mid float64, bidQuotes []asmm.Quote, askQuotes []asmm.Quote, allowBuy, allowSell bool) {
	// 处理买侧
	if allowBuy {
		// 确保切片长度
		if len(r.dynamicBids) < len(bidQuotes) {
			r.dynamicBids = append(r.dynamicBids, make([]levelState, len(bidQuotes)-len(r.dynamicBids))...)
		}
		// 对每档进行差分
		buyPOAllowed := r.postOnlyReady("BUY")
		usedPOBuy := false
		for i := 0; i < len(bidQuotes); i++ {
			q := bidQuotes[i]
			price := q.Price
			qty := q.Size
			// 对齐精度
			var err error
			price, _, qty, err = alignQuote(r.Constraints, price, price+1e-9, qty)
			if err != nil || qty <= 0 {
				continue
			}
			st := r.dynamicBids[i]
			if q.ReduceOnly {
				profitPositive := r.hasPositivePnL(mid)
				if !r.shouldReplaceReduceOrder(st.price, price, mid, profitPositive) {
					continue
				}
			} else {
				if !r.shouldReplacePassive(st.price, price, st.placedAt) {
					continue
				}
			}
			_ = r.ensureDynamicOrder(true, i, price, qty, q.ReduceOnly, buyPOAllowed && !usedPOBuy)
			if buyPOAllowed && !usedPOBuy {
				metrics.IncrementPostOnlyUsage("buy")
			}
			usedPOBuy = true
		}
		// 取消多余档位
		for i := len(bidQuotes); i < len(r.dynamicBids); i++ {
			_ = r.cancelDynamicLeg(true, i)
			metrics.IncrementDynamicOrderCancel("buy")
		}
	}
	// 处理卖侧
	if allowSell {
		if len(r.dynamicAsks) < len(askQuotes) {
			r.dynamicAsks = append(r.dynamicAsks, make([]levelState, len(askQuotes)-len(r.dynamicAsks))...)
		}
		sellPOAllowed := r.postOnlyReady("SELL")
		usedPOSell := false
		for i := 0; i < len(askQuotes); i++ {
			q := askQuotes[i]
			price := q.Price
			qty := q.Size
			var err error
			_, price, qty, err = alignQuote(r.Constraints, price-1e-9, price, qty)
			if err != nil || qty <= 0 {
				continue
			}
			st := r.dynamicAsks[i]
			if q.ReduceOnly {
				profitPositive := r.hasPositivePnL(mid)
				if !r.shouldReplaceReduceOrder(st.price, price, mid, profitPositive) {
					continue
				}
			} else {
				if !r.shouldReplacePassive(st.price, price, st.placedAt) {
					continue
				}
			}
			_ = r.ensureDynamicOrder(false, i, price, qty, q.ReduceOnly, sellPOAllowed && !usedPOSell)
			if sellPOAllowed && !usedPOSell {
				metrics.IncrementPostOnlyUsage("sell")
			}
			usedPOSell = true
		}
		for i := len(askQuotes); i < len(r.dynamicAsks); i++ {
			_ = r.cancelDynamicLeg(false, i)
			metrics.IncrementDynamicOrderCancel("sell")
		}
	}
}

func (r *Runner) ensureDynamicOrder(isBuy bool, idx int, price, qty float64, reduceOnly bool, postOnlyAllowed bool) error {
	if r.OrderMgr == nil {
		return errors.New("order manager nil")
	}
	side := "SELL"
	if isBuy {
		side = "BUY"
	}
	postOnly := postOnlyAllowed
	if r.Risk != nil {
		delta := qty
		if !isBuy {
			delta = -qty
		}
		if err := r.Risk.PreOrder(r.Symbol, delta); err != nil {
			return err
		}
	}
	ord := order.Order{
		Symbol:      r.Symbol,
		Side:        side,
		Price:       price,
		Quantity:    qty,
		ReduceOnly:  reduceOnly,
		PostOnly:    postOnly && !reduceOnly,
		TimeInForce: "",
		ClientID:    fmt.Sprintf("dyn-%s-%d", strings.ToLower(side), idx),
	}
	if reduceOnly {
		ord.PostOnly = false
		ord.TimeInForce = "IOC"
	}
	res, usedPostOnly, err := r.submitOrderWithFallback(side, ord, reduceOnly)
	if err != nil {
		return err
	}
	if isBuy {
		if idx >= len(r.dynamicBids) {
			return nil
		}
		st := r.dynamicBids[idx]
		st.id = res.ID
		st.price = price
		st.placedAt = time.Now()
		metrics.IncrementOrdersPlaced("buy")
		metrics.IncrementDynamicOrderUpdate("buy")
		if usedPostOnly {
			r.clearPostOnlyCooldown("BUY")
		}
		r.dynamicBids[idx] = st
	} else {
		if idx >= len(r.dynamicAsks) {
			return nil
		}
		st := r.dynamicAsks[idx]
		st.id = res.ID
		st.price = price
		st.placedAt = time.Now()
		metrics.IncrementOrdersPlaced("sell")
		metrics.IncrementDynamicOrderUpdate("sell")
		if usedPostOnly {
			r.clearPostOnlyCooldown("SELL")
		}
		r.dynamicAsks[idx] = st
	}
	return nil
}

func (r *Runner) cancelDynamicLeg(isBuy bool, idx int) error {
	// 检查是否应抑制撤单（高频成交时）
	if r.shouldSuppressCancel() {
		return nil // 抑制撤单，保持现有订单
	}

	var st levelState
	if isBuy {
		if idx >= len(r.dynamicBids) {
			return nil
		}
		st = r.dynamicBids[idx]
	} else {
		if idx >= len(r.dynamicAsks) {
			return nil
		}
		st = r.dynamicAsks[idx]
	}
	if st.id == "" || r.OrderMgr == nil {
		return nil
	}
	_ = r.OrderMgr.Cancel(st.id)
	metrics.IncrementDynamicOrderCancel(map[bool]string{true: "buy", false: "sell"}[isBuy])
	st = levelState{}
	if isBuy {
		r.dynamicBids[idx] = st
	} else {
		r.dynamicAsks[idx] = st
	}
	return nil
}

func (r *Runner) cancelOutstanding(cancelBid, cancelAsk bool) {
	if r.OrderMgr == nil {
		return
	}
	if cancelBid && r.lastBidID != "" {
		_ = r.OrderMgr.Cancel(r.lastBidID)
		r.lastBidID = ""
		r.lastBidPrice = 0
		r.lastBidPlacedAt = time.Time{}
	}
	if cancelAsk && r.lastAskID != "" {
		_ = r.OrderMgr.Cancel(r.lastAskID)
		r.lastAskID = ""
		r.lastAskPrice = 0
		r.lastAskPlacedAt = time.Time{}
	}
	// 同步取消多档动态腿
	if cancelBid && len(r.dynamicBids) > 0 {
		for i := range r.dynamicBids {
			_ = r.cancelDynamicLeg(true, i)
		}
		r.dynamicBids = nil
	}
	if cancelAsk && len(r.dynamicAsks) > 0 {
		for i := range r.dynamicAsks {
			_ = r.cancelDynamicLeg(false, i)
		}
		r.dynamicAsks = nil
	}
}

func (r *Runner) triggerHalt(reason string) error {
	r.cancelOutstanding(true, true)
	r.haltUntil = time.Now().Add(r.haltDuration())
	r.setRiskState(RiskStateHalted, reason)
	if reason == "" {
		reason = "halted"
	}
	return errors.New(reason)
}

func (r *Runner) setRiskState(state RiskState, reason string) {
	if r.riskState == state {
		return
	}
	r.riskState = state
	if r.onRiskStateChange != nil {
		r.onRiskStateChange(state, reason)
	}
}

func (r *Runner) notifyStrategyAdjust(info StrategyAdjustInfo) {
	if r.onStrategyAdjust != nil {
		r.onStrategyAdjust(info)
	}
}

// SetRiskStateListener 在风险状态变化时回调。
func (r *Runner) SetRiskStateListener(fn func(RiskState, string)) {
	r.onRiskStateChange = fn
}

// SetStrategyAdjustListener 在策略参数更新时回调。
func (r *Runner) SetStrategyAdjustListener(fn func(StrategyAdjustInfo)) {
	r.onStrategyAdjust = fn
}

// SetAdaptiveRisk 设置自适应风控（包括 PostTrade Analyzer）
func (r *Runner) SetAdaptiveRisk(analyzer *posttrade.Analyzer, adaptiveRisk *risk.AdaptiveRiskManager) {
	r.postTradeAnalyzer = analyzer
	r.adaptiveRisk = adaptiveRisk
	r.lastAdaptiveUpdate = time.Now()

	// 将 adaptiveRisk 注入 ASMM 策略
	if r.ASMMStrategy != nil && adaptiveRisk != nil {
		r.ASMMStrategy.SetAdaptiveRisk(adaptiveRisk)
	}
}

// EnableCancelSuppression 启用高频成交时的撤单抑制
func (r *Runner) EnableCancelSuppression(fillRateThreshold float64, recentFillsThreshold int) {
	if r.fillTracker == nil {
		r.fillTracker = order.NewFillTracker(100, 5*time.Minute)
	}
	r.cancelSuppressionEnabled = true
	r.fillRateThreshold = fillRateThreshold
	r.recentFillsThreshold = recentFillsThreshold
}

// OnFill 处理成交事件，通知 PostTrade Analyzer 和 FillTracker
func (r *Runner) OnFill(orderID string, fillPrice float64, side string, quantity float64) {
	if r.postTradeAnalyzer != nil {
		r.postTradeAnalyzer.OnFill(orderID, fillPrice, side)
	}
	if r.fillTracker != nil {
		r.fillTracker.RecordFill(orderID, side, fillPrice, quantity)
	}
}

// shouldSuppressCancel 判断是否应抑制撤单（高频成交时）
func (r *Runner) shouldSuppressCancel() bool {
	if !r.cancelSuppressionEnabled || r.fillTracker == nil {
		return false
	}

	// 使用默认阈值如果未设置
	fillRateThreshold := r.fillRateThreshold
	if fillRateThreshold <= 0 {
		fillRateThreshold = 5.0 // 默认每分钟5次成交
	}
	recentFillsThreshold := r.recentFillsThreshold
	if recentFillsThreshold <= 0 {
		recentFillsThreshold = 3 // 默认1分钟内成交3次
	}

	return r.fillTracker.ShouldSuppressCancel(fillRateThreshold, recentFillsThreshold, 1*time.Minute)
}

// RiskStateUnsafe 返回当前风险状态（仅监控用途）。
func (r *Runner) RiskStateUnsafe() RiskState {
	return r.riskState
}

type invWrapper struct {
	tr *inventory.Tracker
}

func (i invWrapper) NetExposure() float64 { return i.tr.NetExposure() }

func alignQuote(c order.SymbolConstraints, bid, ask, qty float64) (float64, float64, float64, error) {
	if bid <= 0 || ask <= 0 {
		return 0, 0, 0, errors.New("invalid quote price")
	}
	if c.TickSize > 0 {
		bid = snapDown(bid, c.TickSize)
		ask = snapUp(ask, c.TickSize)
		if ask <= bid {
			ask = bid + c.TickSize
		}
	}
	if c.StepSize > 0 {
		qty = roundToStep(qty, c.StepSize)
	}
	if qty <= 0 {
		return 0, 0, 0, errors.New("qty <= 0 after rounding")
	}
	minQty := c.MinQty
	if minQty > 0 {
		minRequired := ceilToStep(minQty, c.StepSize)
		if minRequired > qty {
			qty = minRequired
		}
	}
	if c.MinNotional > 0 {
		buyReq := ceilToStep(c.MinNotional/bid, c.StepSize)
		sellReq := ceilToStep(c.MinNotional/ask, c.StepSize)
		if buyReq > qty {
			qty = buyReq
		}
		if sellReq > qty {
			qty = sellReq
		}
	}
	if c.MaxQty > 0 && qty > c.MaxQty {
		return 0, 0, 0, fmt.Errorf("qty %.8f > maxQty %.8f", qty, c.MaxQty)
	}
	return bid, ask, qty, nil
}

func snapDown(price, tick float64) float64 {
	if tick <= 0 {
		return price
	}
	return math.Floor(price/tick+1e-9) * tick
}

func snapUp(price, tick float64) float64 {
	if tick <= 0 {
		return price
	}
	return math.Ceil(price/tick-1e-9) * tick
}

func roundToStep(qty, step float64) float64 {
	if step <= 0 {
		return qty
	}
	return math.Round(qty/step) * step
}

func ceilToStep(val, step float64) float64 {
	if step <= 0 {
		return val
	}
	return math.Ceil(val/step-1e-9) * step
}
