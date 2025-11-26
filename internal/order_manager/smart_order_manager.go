package order_manager

import (
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"market-maker-go/gateway"
	"market-maker-go/internal/strategy"
)

// OrderSnapshot 订单快照（记住上次下单状态）
type OrderSnapshot struct {
	Side     string // "BUY" or "SELL"
	Price    float64
	Size     float64
	OrderID  string // 币安订单ID（如已提交）
	PlacedAt time.Time
	Layer    int // 第几层
}

// SmartOrderManagerConfig 智能订单管理配置
type SmartOrderManagerConfig struct {
	Symbol string
	
	// 价格偏移阈值（相对mid）
	// 小波动：|理想价格 - 实际价格| / mid < PriceDeviationThreshold 则保持原单
	PriceDeviationThreshold float64 // 推荐 0.0005 (0.05%)
	
	// 重组阈值（相对mid）
	// 大波动：价格整体偏移超过此值时全量撤单重组
	ReorganizeThreshold float64 // 推荐 0.003 (0.3%)
	
	// 最小撤单间隔（避免频繁撤单触发速率限制）
	MinCancelInterval time.Duration // 推荐 2s
	
	// 订单老化阈值（超过此时间的订单即使价格未偏移也重新挂单）
	OrderMaxAge time.Duration // 推荐 60s
}

// 队列操作
type orderOp struct {
	kind    string // "place" or "cancel"
	side    string
	price   float64
	qty     float64
	orderID string
	layer   int
	result  chan opResult
}

// 队列操作结果
type opResult struct {
	orderID string
	err     error
}

// SmartOrderManager 智能订单管理器
type SmartOrderManager struct {
	cfg    SmartOrderManagerConfig
	client *gateway.BinanceRESTClient
	
	mu              sync.RWMutex
	buySnapshots    []OrderSnapshot  // 买单快照（按layer索引）
	sellSnapshots   []OrderSnapshot  // 卖单快照
	lastReorganize  time.Time        // 上次全量重组时间
	lastCancelTime  time.Time        // 上次撤单时间
	lastMidPrice    float64          // 上次中值价（用于检测大偏移）
	cancelCounter   int              // 撤单计数器（监控用）

	// 队列调度
	opsCh         chan orderOp
	stopCh        chan struct{}
	coalesceDelay time.Duration
}

func NewSmartOrderManager(cfg SmartOrderManagerConfig, client *gateway.BinanceRESTClient) *SmartOrderManager {
	m := &SmartOrderManager{
		cfg:           cfg,
		client:        client,
		buySnapshots:  make([]OrderSnapshot, 0, 32),
		sellSnapshots: make([]OrderSnapshot, 0, 32),
		opsCh:         make(chan orderOp, 1024),
		stopCh:        make(chan struct{}),
		coalesceDelay: 200 * time.Millisecond,
	}
	go m.runDispatcher()
	return m
}

// runDispatcher 队列调度器：撤单优先、近端优先、买卖交替
func (m *SmartOrderManager) runDispatcher() {
	for {
		batch := m.collectBatch()
		if len(batch) == 0 {
			continue
		}
		// 分类
		cancels := make([]orderOp, 0)
		buys := make([]orderOp, 0)
		sells := make([]orderOp, 0)
		for _, op := range batch {
			if op.kind == "cancel" {
				cancels = append(cancels, op)
			} else if op.kind == "place" {
				if op.side == "BUY" {
					buys = append(buys, op)
				} else {
					sells = append(sells, op)
				}
			}
		}
		// 近端优先（按layer升序）
		sort.Slice(cancels, func(i, j int) bool { return cancels[i].layer < cancels[j].layer })
		sort.Slice(buys, func(i, j int) bool { return buys[i].layer < buys[j].layer })
		sort.Slice(sells, func(i, j int) bool { return sells[i].layer < sells[j].layer })
		// 撤单优先
		for _, op := range cancels {
			var err error
			if op.orderID != "" {
				err = m.client.CancelOrder(m.cfg.Symbol, op.orderID)
			}
			if op.result != nil {
				op.result <- opResult{"", err}
			}
		}
		// 买卖交替下单
		bi, si := 0, 0
		for bi < len(buys) || si < len(sells) {
			if bi < len(buys) {
				op := buys[bi]
				bi++
				orderID, err := m.client.PlaceLimit(m.cfg.Symbol, "BUY", "GTC", op.price, op.qty, false, true, "")
				if op.result != nil {
					op.result <- opResult{orderID, err}
				}
			}
			if si < len(sells) {
				op := sells[si]
				si++
				orderID, err := m.client.PlaceLimit(m.cfg.Symbol, "SELL", "GTC", op.price, op.qty, false, true, "")
				if op.result != nil {
					op.result <- opResult{orderID, err}
				}
			}
		}
	}
}

// collectBatch 在合并窗口内收集操作
func (m *SmartOrderManager) collectBatch() []orderOp {
	batch := make([]orderOp, 0, 64)
	// 至少获取一个操作
	select {
	case <-m.stopCh:
		return nil
	case op := <-m.opsCh:
		batch = append(batch, op)
	}
	deadline := time.After(m.coalesceDelay)
	for {
		select {
		case op := <-m.opsCh:
			batch = append(batch, op)
			if len(batch) >= 256 {
				return batch
			}
		case <-deadline:
			return batch
		case <-m.stopCh:
			return batch
		}
	}
}

// ReconcileOrders 核心方法：智能对账并更新订单群组
// - targetBuys/targetSells：策略生成的理想报价
// - mid：当前中值价
// - dryRun：测试模式
func (m *SmartOrderManager) ReconcileOrders(targetBuys, targetSells []strategy.Quote, mid float64, dryRun bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	now := time.Now()
	
	// 1. 检测是否需要全量重组（大幅价格偏移）
	needReorganize := m.shouldReorganize(mid, now)
	
	if needReorganize {
		log.Printf("[SmartOrderMgr] 触发全量重组: mid偏移=%.4f%%",
			math.Abs(mid-m.lastMidPrice)/m.lastMidPrice*100)
		if err := m.cancelAllOrders(dryRun); err != nil {
			return err
		}
		m.lastReorganize = now
		m.lastMidPrice = mid
		m.buySnapshots = m.buySnapshots[:0]
		m.sellSnapshots = m.sellSnapshots[:0]
	}
	
	// 2. 差分更新买单
	if err := m.reconcileSide("BUY", targetBuys, &m.buySnapshots, mid, dryRun); err != nil {
		return err
	}
	
	// 3. 差分更新卖单
	if err := m.reconcileSide("SELL", targetSells, &m.sellSnapshots, mid, dryRun); err != nil {
		return err
	}
	
	return nil
}

// shouldReorganize 判断是否需要全量重组
func (m *SmartOrderManager) shouldReorganize(mid float64, now time.Time) bool {
	// 首次运行
	if m.lastMidPrice == 0 {
		m.lastMidPrice = mid
		return false
	}
	
	// 价格大幅偏移
	deviation := math.Abs(mid-m.lastMidPrice) / m.lastMidPrice
	if deviation > m.cfg.ReorganizeThreshold {
		return true
	}
	
	// 长时间未重组（避免订单老化）
	if m.cfg.OrderMaxAge > 0 && now.Sub(m.lastReorganize) > m.cfg.OrderMaxAge {
		return true
	}
	
	return false
}

// reconcileSide 对某一侧（BUY/SELL）进行差分更新
func (m *SmartOrderManager) reconcileSide(side string, targets []strategy.Quote, snapshots *[]OrderSnapshot, mid float64, dryRun bool) error {
	now := time.Now()
	
	// 扩展快照数组以匹配目标层数
	for len(*snapshots) < len(targets) {
		*snapshots = append(*snapshots, OrderSnapshot{Layer: len(*snapshots)})
	}
	
	// 遍历每一层，决定是否需要更新
	for i, target := range targets {
		snap := &(*snapshots)[i]
		
		// 决策：是否需要撤单重挂
		needUpdate := false
		reason := ""
		
		// 情况1：首次下单
		if snap.OrderID == "" {
			needUpdate = true
			reason = "首次"
		} else {
			// 情况2：价格偏离过大
			priceDelta := math.Abs(target.Price - snap.Price)
			priceDeviation := priceDelta / mid
			if priceDeviation > m.cfg.PriceDeviationThreshold {
				needUpdate = true
				reason = "价格偏离"
			}
			
			// 情况3：数量变化（可能被部分成交）
			if math.Abs(target.Size-snap.Size)/target.Size > 0.2 { // 20%变化
				needUpdate = true
				reason = "数量变化"
			}
			
			// 情况4：订单过老
			if m.cfg.OrderMaxAge > 0 && now.Sub(snap.PlacedAt) > m.cfg.OrderMaxAge {
				needUpdate = true
				reason = "订单老化"
			}
		}
		
		if needUpdate {
			// 先撤旧单
			if snap.OrderID != "" {
				if err := m.cancelOrder(snap.OrderID, dryRun); err != nil {
					log.Printf("[SmartOrderMgr] 撤单失败 layer=%d: %v", i, err)
					// 继续处理，不中断
				}
			}
			
			// 下新单
			if err := m.placeOrder(target, snap, mid, dryRun); err != nil {
				log.Printf("[SmartOrderMgr] 下单失败 %s layer=%d: %v", side, i, err)
				// 清空快照，下次重试
				snap.OrderID = ""
				continue
			}
			
			log.Printf("[SmartOrderMgr] 更新 %s layer=%d [%s]: %.4f@%.2f",
				side, i, reason, target.Size, target.Price)
		}
	}
	
	// 取消多余的旧单（目标层数减少时）
	for i := len(targets); i < len(*snapshots); i++ {
		snap := &(*snapshots)[i]
		if snap.OrderID != "" {
			if err := m.cancelOrder(snap.OrderID, dryRun); err != nil {
				log.Printf("[SmartOrderMgr] 清理多余订单失败 layer=%d: %v", i, err)
			}
			snap.OrderID = ""
		}
	}
	
	// 收缩快照数组
	if len(targets) < len(*snapshots) {
		*snapshots = (*snapshots)[:len(targets)]
	}
	
	return nil
}

// placeOrder 入队并等待执行结果，然后更新快照
func (m *SmartOrderManager) placeOrder(q strategy.Quote, snap *OrderSnapshot, mid float64, dryRun bool) error {
	const tick = 0.01
	const step = 0.001
	
	// 精度对齐
	price := q.Price
	if q.Side == "BUY" {
		price = math.Floor(price/tick) * tick
	} else {
		price = math.Ceil(price/tick) * tick
	}
	qty := math.Floor(q.Size/step) * step
	// 最小名义值
	minQty := math.Ceil(20.0/price/step) * step
	if qty < minQty {
		qty = minQty
	}
	if qty < 0.001 {
		return nil
	}
	now := time.Now()
	if dryRun {
		log.Printf("DRY-RUN: Place %s %.4f @ %.2f", q.Side, qty, price)
		snap.OrderID = "dry-" + now.Format("150405.000")
		snap.Side = q.Side
		snap.Price = price
		snap.Size = qty
		snap.PlacedAt = now
		return nil
	}
	resCh := make(chan opResult, 1)
	m.opsCh <- orderOp{
		kind:   "place",
		side:   q.Side,
		price:  price,
		qty:    qty,
		layer:  snap.Layer,
		result: resCh,
	}
	res := <-resCh
	if res.err != nil {
		return res.err
	}
	// 更新快照
	snap.OrderID = res.orderID
	snap.Side = q.Side
	snap.Price = price
	snap.Size = qty
	snap.PlacedAt = time.Now()
	return nil
}

// cancelOrder 入队撤单并等待完成
func (m *SmartOrderManager) cancelOrder(orderID string, dryRun bool) error {
	now := time.Now()
	if dryRun {
		log.Printf("DRY-RUN: Cancel %s", orderID)
		m.lastCancelTime = now
		return nil
	}
	resCh := make(chan opResult, 1)
	m.opsCh <- orderOp{
		kind:    "cancel",
		orderID: orderID,
		result:  resCh,
	}
	res := <-resCh
	m.lastCancelTime = time.Now()
	m.cancelCounter++
	return res.err
}

// cancelAllOrders 全量撤单（仅在重组时使用）
func (m *SmartOrderManager) cancelAllOrders(dryRun bool) error {
	if dryRun {
		log.Println("DRY-RUN: CancelAll")
		return nil
	}
	if err := m.client.CancelAll(m.cfg.Symbol); err != nil {
		return err
	}
	m.lastCancelTime = time.Now()
	log.Printf("[SmartOrderMgr] 全量撤单完成 (累计撤单: %d次)", m.cancelCounter)
	return nil
}

// GetStatistics 获取统计信息
func (m *SmartOrderManager) GetStatistics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]interface{}{
		"total_cancels":      m.cancelCounter,
		"active_buy_orders":  len(m.buySnapshots),
		"active_sell_orders": len(m.sellSnapshots),
		"last_reorganize":    m.lastReorganize,
		"last_mid_price":     m.lastMidPrice,
	}
}

// ForceReorganize 强制触发重组（紧急使用）
func (m *SmartOrderManager) ForceReorganize() {
	m.mu.Lock()
	m.lastMidPrice = 0 // 强制下次触发
	m.mu.Unlock()
}
