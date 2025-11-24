package order

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Gateway 提供基础下单/撤单抽象；与 gateway.Client 对接。
type Gateway interface {
	Place(o Order) (string, error)
	Cancel(orderID string) error
}

// Manager 维护订单状态并通过 Gateway 下发。
type Manager struct {
	gw           Gateway
	stateMachine *StateMachine
	mu           sync.RWMutex
	orders       map[string]*Order
	constraints  map[string]SymbolConstraints
}

func NewManager(gw Gateway) *Manager {
	return &Manager{
		gw:           gw,
		stateMachine: NewStateMachine(),
		orders:       make(map[string]*Order),
	}
}

var ErrUnknownOrder = errors.New("unknown order")

// Submit 同步调用 Gateway 下单并登记状态。
func (m *Manager) Submit(o Order) (*Order, error) {
	if o.Type == "" {
		o.Type = "LIMIT"
	}
	if err := m.validateConstraint(o); err != nil {
		return nil, err
	}
	if o.ID == "" {
		o.ID = generateID(o.ClientID)
	}
	o.Status = StatusNew
	m.mu.Lock()
	m.orders[o.ID] = &o
	m.mu.Unlock()

	if m.gw != nil {
		if _, err := m.gw.Place(o); err != nil {
			m.updateStatus(o.ID, StatusRejected, err)
			return nil, err
		}
		m.updateStatus(o.ID, StatusAck, nil)
	}
	return &o, nil
}

// Update 收到回报后更新状态。
func (m *Manager) Update(id string, st Status) error {
	return m.updateStatus(id, st, nil)
}

// Cancel 调用 Gateway 撤单并标记状态。
func (m *Manager) Cancel(id string) error {
	m.mu.RLock()
	_, ok := m.orders[id]
	m.mu.RUnlock()
	if !ok {
		return ErrUnknownOrder
	}
	if m.gw != nil {
		if err := m.gw.Cancel(id); err != nil {
			return err
		}
	}
	return m.updateStatus(id, StatusCanceled, nil)
}

// Status 返回订单当前状态，如不存在则第二个返回值为 false。
func (m *Manager) Status(id string) (Status, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	o, ok := m.orders[id]
	if !ok {
		return "", false
	}
	return o.Status, true
}

func (m *Manager) updateStatus(id string, st Status, err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.orders[id]
	if !ok {
		return ErrUnknownOrder
	}

	// 验证状态转换
	if validErr := m.stateMachine.ValidateTransition(o.Status, st); validErr != nil {
		return fmt.Errorf("invalid state transition for order %s: %w", id, validErr)
	}

	o.Status = st
	if err != nil {
		o.LastError = err.Error()
	}
	return nil
}

// SetConstraints 设置各交易对的精度/名义限制。
func (m *Manager) SetConstraints(c map[string]SymbolConstraints) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.constraints = make(map[string]SymbolConstraints, len(c))
	for sym, sc := range c {
		m.constraints[sym] = sc
	}
}

// generateID 简单生成唯一 ID。生产环境应改为雪花/UUID。
func generateID(prefix string) string {
	if prefix == "" {
		prefix = "ord"
	}
	return prefix + "-" + time.Now().UTC().Format("20060102150405.000000000")
}

func (m *Manager) validateConstraint(o Order) error {
	m.mu.RLock()
	c, ok := m.constraints[o.Symbol]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	if o.Type != "" && (o.Type == "MARKET" || o.Type == "market") {
		return nil
	}
	return c.Validate(o.Price, o.Quantity)
}

// GetActiveOrders 获取所有活跃订单（用于对账）
func (m *Manager) GetActiveOrders() []*Order {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := make([]*Order, 0, len(m.orders))
	for _, o := range m.orders {
		// 只返回未完成的订单
		if o.Status != StatusFilled && o.Status != StatusCanceled &&
			o.Status != StatusRejected && o.Status != StatusExpired {
			active = append(active, o)
		}
	}
	return active
}

// GetActiveOrdersBySymbol 获取指定交易对的活跃订单
func (m *Manager) GetActiveOrdersBySymbol(symbol string) []*Order {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := make([]*Order, 0)
	for _, o := range m.orders {
		if o.Symbol == symbol && o.Status != StatusFilled &&
			o.Status != StatusCanceled && o.Status != StatusRejected &&
			o.Status != StatusExpired {
			active = append(active, o)
		}
	}
	return active
}

// GetOrder 获取订单详情
func (m *Manager) GetOrder(id string) (*Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	o, ok := m.orders[id]
	if !ok {
		return nil, ErrUnknownOrder
	}
	return o, nil
}

// UpdateStatus 公开的状态更新方法（用于对账）
func (m *Manager) UpdateStatus(id string, st Status) error {
	return m.updateStatus(id, st, nil)
}
