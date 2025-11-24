package order

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ExchangeGateway 交易所接口（用于对账）
type ExchangeGateway interface {
	GetOrder(orderID string) (*Order, error)
	GetOpenOrders(symbol string) ([]*Order, error)
}

// Reconciler 订单对账器
type Reconciler struct {
	gateway  ExchangeGateway
	manager  *Manager
	interval time.Duration

	stopChan chan struct{}
	doneChan chan struct{}
	mu       sync.RWMutex

	// 统计信息
	totalReconciliations int64
	conflictsResolved    int64
	lastReconcileTime    time.Time
}

// ReconcilerConfig 对账器配置
type ReconcilerConfig struct {
	Interval time.Duration // 对账间隔
}

// NewReconciler 创建订单对账器
func NewReconciler(gateway ExchangeGateway, manager *Manager, config ReconcilerConfig) *Reconciler {
	if config.Interval <= 0 {
		config.Interval = 30 * time.Second // 默认30秒
	}

	return &Reconciler{
		gateway:  gateway,
		manager:  manager,
		interval: config.Interval,
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
}

// Start 启动对账服务
func (r *Reconciler) Start(ctx context.Context) error {
	go r.reconcileLoop(ctx)
	return nil
}

// Stop 停止对账服务
func (r *Reconciler) Stop() error {
	close(r.stopChan)
	<-r.doneChan // 等待循环退出
	return nil
}

// reconcileLoop 对账循环
func (r *Reconciler) reconcileLoop(ctx context.Context) {
	defer close(r.doneChan)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopChan:
			return
		case <-ticker.C:
			if err := r.Reconcile(); err != nil {
				// 记录错误但继续运行
				// 实际使用时应该记录到日志
			}
		}
	}
}

// Reconcile 执行一次完整对账
func (r *Reconciler) Reconcile() error {
	r.mu.Lock()
	r.totalReconciliations++
	r.lastReconcileTime = time.Now()
	r.mu.Unlock()

	// 获取本地所有活跃订单
	localOrders := r.manager.GetActiveOrders()

	// 对每个订单进行对账
	var reconcileErr error
	for _, localOrder := range localOrders {
		if err := r.reconcileOrder(localOrder); err != nil {
			reconcileErr = err
			// 继续处理其他订单
		}
	}

	return reconcileErr
}

// reconcileOrder 对账单个订单
func (r *Reconciler) reconcileOrder(localOrder *Order) error {
	// 从交易所获取订单状态
	remoteOrder, err := r.gateway.GetOrder(localOrder.ID)
	if err != nil {
		return fmt.Errorf("get remote order failed: %w", err)
	}

	// 比较并解决冲突
	if err := r.resolveConflict(localOrder, remoteOrder); err != nil {
		return fmt.Errorf("resolve conflict failed: %w", err)
	}

	return nil
}

// resolveConflict 解决状态冲突
func (r *Reconciler) resolveConflict(local, remote *Order) error {
	hasConflict := false

	// 检查订单状态
	if local.Status != remote.Status {
		hasConflict = true
		// 以交易所状态为准
		if err := r.manager.UpdateStatus(local.ID, remote.Status); err != nil {
			return fmt.Errorf("update status failed: %w", err)
		}
	}

	// 可以在这里添加更多字段的对比
	// 例如：价格、数量等，但当前Order结构比较简单
	// 未来如需要可以扩展Order结构添加更多字段

	if hasConflict {
		r.mu.Lock()
		r.conflictsResolved++
		r.mu.Unlock()
	}

	return nil
}

// ReconcileBySymbol 对指定交易对进行对账
func (r *Reconciler) ReconcileBySymbol(symbol string) error {
	// 从交易所获取所有活跃订单
	remoteOrders, err := r.gateway.GetOpenOrders(symbol)
	if err != nil {
		return fmt.Errorf("get open orders failed: %w", err)
	}

	// 获取本地该交易对的活跃订单
	localOrders := r.manager.GetActiveOrdersBySymbol(symbol)

	// 创建本地订单ID映射
	localOrderMap := make(map[string]*Order)
	for _, order := range localOrders {
		localOrderMap[order.ID] = order
	}

	// 检查交易所订单是否在本地存在
	for _, remoteOrder := range remoteOrders {
		if localOrder, exists := localOrderMap[remoteOrder.ID]; exists {
			// 本地存在，进行对账
			if err := r.resolveConflict(localOrder, remoteOrder); err != nil {
				return err
			}
		} else {
			// 本地不存在但交易所存在，可能是遗漏的订单
			// 这里可以选择添加到本地或者记录告警
		}
	}

	return nil
}

// GetStatistics 获取对账统计信息
func (r *Reconciler) GetStatistics() ReconcilerStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return ReconcilerStats{
		TotalReconciliations: r.totalReconciliations,
		ConflictsResolved:    r.conflictsResolved,
		LastReconcileTime:    r.lastReconcileTime,
		Interval:             r.interval,
	}
}

// ReconcilerStats 对账统计信息
type ReconcilerStats struct {
	TotalReconciliations int64
	ConflictsResolved    int64
	LastReconcileTime    time.Time
	Interval             time.Duration
}

// ForceReconcile 立即执行一次对账（用于测试或紧急情况）
func (r *Reconciler) ForceReconcile() error {
	return r.Reconcile()
}

// UpdateInterval 更新对账间隔
func (r *Reconciler) UpdateInterval(interval time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if interval > 0 {
		r.interval = interval
	}
}
