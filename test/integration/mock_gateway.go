package integration

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"market-maker-go/order"
)

// MockGateway 模拟交易所网关（用于集成测试）
type MockGateway struct {
	// 配置
	simulateLatency bool
	latency         time.Duration
	shouldFail      bool
	failureRate     float64

	// 订单存储
	orders map[string]*MockOrder
	mu     sync.RWMutex

	// 统计
	placeOrderCount  int
	cancelOrderCount int
	queryOrderCount  int

	// 回调
	onOrderUpdate func(order.Order)
}

// MockOrder 模拟订单
type MockOrder struct {
	ClientOrderID string
	Symbol        string
	Side          string
	Price         float64
	Quantity      float64
	Status        string
	FilledQty     float64
	UpdateTime    time.Time
}

// NewMockGateway 创建Mock Gateway
func NewMockGateway() *MockGateway {
	return &MockGateway{
		orders:          make(map[string]*MockOrder),
		simulateLatency: false,
		latency:         10 * time.Millisecond,
		shouldFail:      false,
		failureRate:     0.0,
	}
}

// SetSimulateLatency 设置是否模拟延迟
func (m *MockGateway) SetSimulateLatency(simulate bool, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simulateLatency = simulate
	m.latency = latency
}

// SetFailureRate 设置失败率（0.0-1.0）
func (m *MockGateway) SetFailureRate(rate float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureRate = rate
}

// SetOrderUpdateCallback 设置订单更新回调
func (m *MockGateway) SetOrderUpdateCallback(callback func(order.Order)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onOrderUpdate = callback
}

// PlaceOrder 下单（实现gateway接口）
func (m *MockGateway) PlaceOrder(symbol, side string, price, quantity float64, orderType string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.placeOrderCount++

	// 模拟延迟
	if m.simulateLatency {
		time.Sleep(m.latency)
	}

	// 模拟失败
	if m.failureRate > 0 && float64(m.placeOrderCount)/(float64(m.placeOrderCount)+1) < m.failureRate {
		return "", errors.New("simulated order placement failure")
	}

	// 生成订单ID
	clientOrderID := fmt.Sprintf("TEST_%d_%d", time.Now().Unix(), m.placeOrderCount)

	// 创建订单
	mockOrder := &MockOrder{
		ClientOrderID: clientOrderID,
		Symbol:        symbol,
		Side:          side,
		Price:         price,
		Quantity:      quantity,
		Status:        "NEW",
		FilledQty:     0,
		UpdateTime:    time.Now(),
	}

	m.orders[clientOrderID] = mockOrder

	// 触发回调
	if m.onOrderUpdate != nil {
		go m.onOrderUpdate(m.toOrderStruct(mockOrder))
	}

	return clientOrderID, nil
}

// Place 下单（实现order.Gateway接口）
func (m *MockGateway) Place(o order.Order) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.placeOrderCount++

	// 模拟延迟
	if m.simulateLatency {
		time.Sleep(m.latency)
	}

	// 模拟失败
	if m.failureRate > 0 && float64(m.placeOrderCount)/(float64(m.placeOrderCount)+1) < m.failureRate {
		return "", errors.New("simulated order placement failure")
	}

	// 使用传入的订单ID，而不是生成新的
	orderID := o.ID
	if orderID == "" {
		orderID = o.ClientID
	}
	if orderID == "" {
		orderID = fmt.Sprintf("TEST_%d_%d", time.Now().Unix(), m.placeOrderCount)
	}

	// 创建订单
	mockOrder := &MockOrder{
		ClientOrderID: orderID,
		Symbol:        o.Symbol,
		Side:          o.Side,
		Price:         o.Price,
		Quantity:      o.Quantity,
		Status:        "NEW",
		FilledQty:     0,
		UpdateTime:    time.Now(),
	}

	m.orders[orderID] = mockOrder

	// 触发回调
	if m.onOrderUpdate != nil {
		go m.onOrderUpdate(m.toOrderStruct(mockOrder))
	}

	return orderID, nil
}

// Cancel 撤单（实现order.Gateway接口）
func (m *MockGateway) Cancel(orderID string) error {
	return m.CancelOrder("", orderID)
}

// CancelOrder 撤单（内部实现）
func (m *MockGateway) CancelOrder(symbol, clientOrderID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cancelOrderCount++

	// 模拟延迟
	if m.simulateLatency {
		time.Sleep(m.latency)
	}

	// 查找订单
	mockOrder, exists := m.orders[clientOrderID]
	if !exists {
		return fmt.Errorf("order not found: %s", clientOrderID)
	}

	// 检查状态
	if mockOrder.Status == "FILLED" || mockOrder.Status == "CANCELED" {
		return fmt.Errorf("cannot cancel order in %s state", mockOrder.Status)
	}

	// 更新状态
	mockOrder.Status = "CANCELED"
	mockOrder.UpdateTime = time.Now()

	// 触发回调
	if m.onOrderUpdate != nil {
		go m.onOrderUpdate(m.toOrderStruct(mockOrder))
	}

	return nil
}

// QueryOrder 查询订单（实现gateway接口）
func (m *MockGateway) QueryOrder(symbol, clientOrderID string) (order.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.queryOrderCount++

	mockOrder, exists := m.orders[clientOrderID]
	if !exists {
		return order.Order{}, fmt.Errorf("order not found: %s", clientOrderID)
	}

	return m.toOrderStruct(mockOrder), nil
}

// GetAllOrders 获取所有订单
func (m *MockGateway) GetAllOrders(symbol string) ([]order.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var orders []order.Order
	for _, mockOrder := range m.orders {
		if symbol == "" || mockOrder.Symbol == symbol {
			orders = append(orders, m.toOrderStruct(mockOrder))
		}
	}

	return orders, nil
}

// SimulateFill 模拟成交
func (m *MockGateway) SimulateFill(clientOrderID string, fillQty float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	mockOrder, exists := m.orders[clientOrderID]
	if !exists {
		return fmt.Errorf("order not found: %s", clientOrderID)
	}

	if mockOrder.Status != "NEW" && mockOrder.Status != "PARTIALLY_FILLED" {
		return fmt.Errorf("cannot fill order in %s state", mockOrder.Status)
	}

	// 更新成交数量
	mockOrder.FilledQty += fillQty
	if mockOrder.FilledQty >= mockOrder.Quantity {
		mockOrder.FilledQty = mockOrder.Quantity
		mockOrder.Status = "FILLED"
	} else {
		mockOrder.Status = "PARTIALLY_FILLED"
	}
	mockOrder.UpdateTime = time.Now()

	// 触发回调
	if m.onOrderUpdate != nil {
		go m.onOrderUpdate(m.toOrderStruct(mockOrder))
	}

	return nil
}

// SimulatePartialFill 模拟部分成交
func (m *MockGateway) SimulatePartialFill(clientOrderID string, fillRatio float64) error {
	m.mu.RLock()
	mockOrder := m.orders[clientOrderID]
	m.mu.RUnlock()

	if mockOrder == nil {
		return fmt.Errorf("order not found: %s", clientOrderID)
	}

	fillQty := mockOrder.Quantity * fillRatio
	return m.SimulateFill(clientOrderID, fillQty)
}

// SimulateFullFill 模拟全部成交
func (m *MockGateway) SimulateFullFill(clientOrderID string) error {
	return m.SimulatePartialFill(clientOrderID, 1.0)
}

// GetStatistics 获取统计信息
func (m *MockGateway) GetStatistics() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]int{
		"place_order_count":  m.placeOrderCount,
		"cancel_order_count": m.cancelOrderCount,
		"query_order_count":  m.queryOrderCount,
		"total_orders":       len(m.orders),
	}
}

// Reset 重置Mock Gateway
func (m *MockGateway) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.orders = make(map[string]*MockOrder)
	m.placeOrderCount = 0
	m.cancelOrderCount = 0
	m.queryOrderCount = 0
}

// toOrderStruct 转换为order.Order结构
func (m *MockGateway) toOrderStruct(mockOrder *MockOrder) order.Order {
	return order.Order{
		ID:       mockOrder.ClientOrderID,
		ClientID: mockOrder.ClientOrderID,
		Symbol:   mockOrder.Symbol,
		Side:     mockOrder.Side,
		Type:     "LIMIT",
		Price:    mockOrder.Price,
		Quantity: mockOrder.Quantity,
		Status:   order.Status(mockOrder.Status),
	}
}

// GetOrder 获取订单（内部使用）
func (m *MockGateway) GetOrder(clientOrderID string) (*MockOrder, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	order, exists := m.orders[clientOrderID]
	return order, exists
}
