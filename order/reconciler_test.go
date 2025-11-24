package order

import (
	"context"
	"errors"
	"testing"
	"time"
)

// MockGateway 模拟交易所网关
type MockGateway struct {
	orders     map[string]*Order
	shouldFail bool
}

func NewMockGateway() *MockGateway {
	return &MockGateway{
		orders: make(map[string]*Order),
	}
}

func (m *MockGateway) Place(o Order) (string, error) {
	if m.shouldFail {
		return "", errors.New("mock error")
	}
	m.orders[o.ID] = &o
	return o.ID, nil
}

func (m *MockGateway) Cancel(orderID string) error {
	if m.shouldFail {
		return errors.New("mock error")
	}
	if o, ok := m.orders[orderID]; ok {
		o.Status = StatusCanceled
	}
	return nil
}

func (m *MockGateway) GetOrder(orderID string) (*Order, error) {
	if m.shouldFail {
		return nil, errors.New("mock error")
	}
	o, ok := m.orders[orderID]
	if !ok {
		return nil, errors.New("order not found")
	}
	return o, nil
}

func (m *MockGateway) GetOpenOrders(symbol string) ([]*Order, error) {
	if m.shouldFail {
		return nil, errors.New("mock error")
	}
	orders := make([]*Order, 0)
	for _, o := range m.orders {
		if o.Symbol == symbol && o.Status != StatusFilled && o.Status != StatusCanceled {
			orders = append(orders, o)
		}
	}
	return orders, nil
}

func (m *MockGateway) SetOrder(o *Order) {
	m.orders[o.ID] = o
}

func (m *MockGateway) SetShouldFail(fail bool) {
	m.shouldFail = fail
}

func TestNewReconciler(t *testing.T) {
	gw := NewMockGateway()
	mgr := NewManager(gw)

	config := ReconcilerConfig{
		Interval: 30 * time.Second,
	}
	rec := NewReconciler(gw, mgr, config)

	if rec == nil {
		t.Fatal("reconciler should not be nil")
	}
	if rec.interval != 30*time.Second {
		t.Errorf("interval = %v, want 30s", rec.interval)
	}
}

func TestNewReconcilerDefaultInterval(t *testing.T) {
	gw := NewMockGateway()
	mgr := NewManager(gw)

	rec := NewReconciler(gw, mgr, ReconcilerConfig{})

	if rec.interval != 30*time.Second {
		t.Errorf("default interval = %v, want 30s", rec.interval)
	}
}

func TestReconcileNoConflict(t *testing.T) {
	gw := NewMockGateway()
	mgr := NewManager(gw)

	// 提交订单
	order := Order{
		ID:       "test-order-1",
		Symbol:   "ETHUSDC",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    2000.0,
		Quantity: 0.1,
	}
	mgr.Submit(order)

	// 交易所也有相同状态的订单
	gw.SetOrder(&Order{
		ID:       "test-order-1",
		Symbol:   "ETHUSDC",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    2000.0,
		Quantity: 0.1,
		Status:   StatusAck,
	})

	rec := NewReconciler(gw, mgr, ReconcilerConfig{})
	err := rec.Reconcile()

	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	stats := rec.GetStatistics()
	if stats.TotalReconciliations != 1 {
		t.Errorf("total reconciliations = %d, want 1", stats.TotalReconciliations)
	}
	if stats.ConflictsResolved != 0 {
		t.Errorf("conflicts resolved = %d, want 0", stats.ConflictsResolved)
	}
}

func TestReconcileStatusConflict(t *testing.T) {
	gw := NewMockGateway()
	mgr := NewManager(gw)

	// 本地订单状态为ACK
	order := Order{
		ID:       "test-order-2",
		Symbol:   "ETHUSDC",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    2000.0,
		Quantity: 0.1,
	}
	mgr.Submit(order)

	// 交易所订单已成交
	gw.SetOrder(&Order{
		ID:       "test-order-2",
		Symbol:   "ETHUSDC",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    2000.0,
		Quantity: 0.1,
		Status:   StatusFilled,
	})

	rec := NewReconciler(gw, mgr, ReconcilerConfig{})
	err := rec.Reconcile()

	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	// 检查本地状态是否更新
	status, ok := mgr.Status("test-order-2")
	if !ok {
		t.Fatal("order not found")
	}
	if status != StatusFilled {
		t.Errorf("status = %s, want FILLED", status)
	}

	stats := rec.GetStatistics()
	if stats.ConflictsResolved != 1 {
		t.Errorf("conflicts resolved = %d, want 1", stats.ConflictsResolved)
	}
}

func TestReconcileOrderNotFound(t *testing.T) {
	gw := NewMockGateway()
	mgr := NewManager(gw)

	// 本地有订单但交易所没有
	order := Order{
		ID:       "test-order-3",
		Symbol:   "ETHUSDC",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    2000.0,
		Quantity: 0.1,
	}
	mgr.Submit(order)

	rec := NewReconciler(gw, mgr, ReconcilerConfig{})
	err := rec.Reconcile()

	// 应该返回错误，因为交易所找不到订单
	if err == nil {
		t.Error("expected error when order not found on exchange")
	}
}

func TestReconcileBySymbol(t *testing.T) {
	gw := NewMockGateway()
	mgr := NewManager(gw)

	// 提交多个订单
	mgr.Submit(Order{ID: "ord-1", Symbol: "ETHUSDC", Side: "BUY", Price: 2000.0, Quantity: 0.1})
	mgr.Submit(Order{ID: "ord-2", Symbol: "ETHUSDC", Side: "SELL", Price: 2001.0, Quantity: 0.1})
	mgr.Submit(Order{ID: "ord-3", Symbol: "BTCUSDC", Side: "BUY", Price: 40000.0, Quantity: 0.01})

	// 交易所也有这些订单（都是活跃状态）
	gw.SetOrder(&Order{ID: "ord-1", Symbol: "ETHUSDC", Status: StatusAck})
	gw.SetOrder(&Order{ID: "ord-2", Symbol: "ETHUSDC", Status: StatusAck})
	gw.SetOrder(&Order{ID: "ord-3", Symbol: "BTCUSDC", Status: StatusAck})

	rec := NewReconciler(gw, mgr, ReconcilerConfig{})
	err := rec.ReconcileBySymbol("ETHUSDC")

	if err != nil {
		t.Fatalf("reconcile by symbol failed: %v", err)
	}

	// 验证只对ETHUSDC进行了对账（统计应该增加）
	stats := rec.GetStatistics()
	// ReconcileBySymbol不增加TotalReconciliations计数，所以我们只验证没有错误
	if stats.ConflictsResolved > 0 {
		t.Error("should not have conflicts when all status match")
	}
}

func TestStartStop(t *testing.T) {
	gw := NewMockGateway()
	mgr := NewManager(gw)

	rec := NewReconciler(gw, mgr, ReconcilerConfig{
		Interval: 100 * time.Millisecond,
	})

	ctx := context.Background()
	err := rec.Start(ctx)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// 等待一段时间让对账器运行
	time.Sleep(250 * time.Millisecond)

	err = rec.Stop()
	if err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	stats := rec.GetStatistics()
	// 应该至少执行了一次对账
	if stats.TotalReconciliations < 1 {
		t.Errorf("total reconciliations = %d, want >= 1", stats.TotalReconciliations)
	}
}

func TestForceReconcile(t *testing.T) {
	gw := NewMockGateway()
	mgr := NewManager(gw)

	rec := NewReconciler(gw, mgr, ReconcilerConfig{})

	err := rec.ForceReconcile()
	if err != nil {
		t.Fatalf("force reconcile failed: %v", err)
	}

	stats := rec.GetStatistics()
	if stats.TotalReconciliations != 1 {
		t.Errorf("total reconciliations = %d, want 1", stats.TotalReconciliations)
	}
}

func TestUpdateInterval(t *testing.T) {
	gw := NewMockGateway()
	mgr := NewManager(gw)

	rec := NewReconciler(gw, mgr, ReconcilerConfig{
		Interval: 30 * time.Second,
	})

	rec.UpdateInterval(60 * time.Second)

	stats := rec.GetStatistics()
	if stats.Interval != 60*time.Second {
		t.Errorf("interval = %v, want 60s", stats.Interval)
	}

	// 无效值应该被忽略
	rec.UpdateInterval(0)
	stats = rec.GetStatistics()
	if stats.Interval != 60*time.Second {
		t.Errorf("interval should not change with invalid value, got %v", stats.Interval)
	}
}

func TestGetStatistics(t *testing.T) {
	gw := NewMockGateway()
	mgr := NewManager(gw)

	rec := NewReconciler(gw, mgr, ReconcilerConfig{
		Interval: 30 * time.Second,
	})

	stats := rec.GetStatistics()
	if stats.TotalReconciliations != 0 {
		t.Errorf("initial total = %d, want 0", stats.TotalReconciliations)
	}
	if stats.ConflictsResolved != 0 {
		t.Errorf("initial conflicts = %d, want 0", stats.ConflictsResolved)
	}
	if stats.Interval != 30*time.Second {
		t.Errorf("interval = %v, want 30s", stats.Interval)
	}
	if !stats.LastReconcileTime.IsZero() {
		t.Error("last reconcile time should be zero initially")
	}

	// 执行一次对账
	rec.ForceReconcile()

	stats = rec.GetStatistics()
	if stats.TotalReconciliations != 1 {
		t.Errorf("total after reconcile = %d, want 1", stats.TotalReconciliations)
	}
	if stats.LastReconcileTime.IsZero() {
		t.Error("last reconcile time should be set")
	}
}

func TestReconcileWithGatewayError(t *testing.T) {
	gw := NewMockGateway()
	mgr := NewManager(gw)

	// 提交订单
	mgr.Submit(Order{ID: "ord-1", Symbol: "ETHUSDC", Side: "BUY", Price: 2000.0, Quantity: 0.1})

	// 设置网关返回错误
	gw.SetShouldFail(true)

	rec := NewReconciler(gw, mgr, ReconcilerConfig{})
	err := rec.Reconcile()

	// 应该返回错误
	if err == nil {
		t.Error("expected error when gateway fails")
	}
}

func TestMultipleConflicts(t *testing.T) {
	gw := NewMockGateway()
	mgr := NewManager(gw)

	// 提交多个订单
	for i := 1; i <= 5; i++ {
		id := time.Now().Format("20060102150405") + string(rune(i))
		mgr.Submit(Order{
			ID:       id,
			Symbol:   "ETHUSDC",
			Side:     "BUY",
			Price:    2000.0,
			Quantity: 0.1,
		})

		// 交易所状态都是FILLED
		gw.SetOrder(&Order{
			ID:     id,
			Symbol: "ETHUSDC",
			Status: StatusFilled,
		})
	}

	rec := NewReconciler(gw, mgr, ReconcilerConfig{})
	err := rec.Reconcile()

	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	stats := rec.GetStatistics()
	if stats.ConflictsResolved != 5 {
		t.Errorf("conflicts resolved = %d, want 5", stats.ConflictsResolved)
	}
}

// 基准测试
func BenchmarkReconcile(b *testing.B) {
	gw := NewMockGateway()
	mgr := NewManager(gw)

	// 创建100个订单
	for i := 0; i < 100; i++ {
		id := time.Now().Format("20060102150405") + string(rune(i))
		mgr.Submit(Order{
			ID:       id,
			Symbol:   "ETHUSDC",
			Side:     "BUY",
			Price:    2000.0,
			Quantity: 0.1,
		})
		gw.SetOrder(&Order{
			ID:     id,
			Symbol: "ETHUSDC",
			Status: StatusAck,
		})
	}

	rec := NewReconciler(gw, mgr, ReconcilerConfig{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Reconcile()
	}
}
