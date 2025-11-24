package order

import (
	"fmt"
	"sync"
)

// 额外的状态定义（扩展现有的Status）
const (
	StatusPending   Status = "PENDING"   // 待提交
	StatusCanceling Status = "CANCELING" // 撤单中
)

// StateTransition 状态转换
type StateTransition struct {
	From Status
	To   Status
}

// StateMachine 订单状态机
type StateMachine struct {
	transitions map[StateTransition]bool
	mu          sync.RWMutex
}

// NewStateMachine 创建新的状态机
func NewStateMachine() *StateMachine {
	sm := &StateMachine{
		transitions: make(map[StateTransition]bool),
	}
	sm.initializeTransitions()
	return sm
}

// initializeTransitions 初始化所有合法的状态转换
func (sm *StateMachine) initializeTransitions() {
	// 定义所有合法的状态转换
	legalTransitions := []StateTransition{
		// 从PENDING可以转到
		{StatusPending, StatusNew},
		{StatusPending, StatusRejected},

		// 从NEW可以转到
		{StatusNew, StatusAck},
		{StatusNew, StatusPartial},
		{StatusNew, StatusFilled},
		{StatusNew, StatusCanceling},
		{StatusNew, StatusCanceled},
		{StatusNew, StatusRejected},
		{StatusNew, StatusExpired},

		// 从ACK可以转到
		{StatusAck, StatusPartial},
		{StatusAck, StatusFilled},
		{StatusAck, StatusCanceling},
		{StatusAck, StatusCanceled},
		{StatusAck, StatusExpired},

		// 从PARTIAL可以转到
		{StatusPartial, StatusPartial}, // 多次部分成交
		{StatusPartial, StatusFilled},
		{StatusPartial, StatusCanceling},
		{StatusPartial, StatusCanceled},
		{StatusPartial, StatusExpired},

		// 从CANCELING可以转到
		{StatusCanceling, StatusCanceled},
		{StatusCanceling, StatusFilled},  // 撤单时全部成交
		{StatusCanceling, StatusPartial}, // 撤单时部分成交

		// 终态不能转换（FILLED, CANCELED, REJECTED, EXPIRED）
	}

	for _, t := range legalTransitions {
		sm.transitions[t] = true
	}
}

// ValidateTransition 验证状态转换是否合法
func (sm *StateMachine) ValidateTransition(from, to Status) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// 相同状态允许（幂等性）
	if from == to {
		return nil
	}

	// 检查是否是合法转换
	transition := StateTransition{From: from, To: to}
	if !sm.transitions[transition] {
		return fmt.Errorf("illegal state transition: %s -> %s", from, to)
	}

	return nil
}

// AllowedTransitions 返回当前状态所有合法的目标状态
func (sm *StateMachine) AllowedTransitions(current Status) []Status {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	allowed := make([]Status, 0)
	for transition := range sm.transitions {
		if transition.From == current {
			allowed = append(allowed, transition.To)
		}
	}
	return allowed
}

// IsFinalState 判断是否是终态
func (sm *StateMachine) IsFinalState(status Status) bool {
	switch status {
	case StatusFilled, StatusCanceled, StatusRejected, StatusExpired:
		return true
	default:
		return false
	}
}

// IsActiveState 判断是否是活跃状态（可能产生成交）
func (sm *StateMachine) IsActiveState(status Status) bool {
	switch status {
	case StatusNew, StatusAck, StatusPartial:
		return true
	default:
		return false
	}
}

// CanCancel 判断当前状态下是否可以撤单
func (sm *StateMachine) CanCancel(status Status) bool {
	switch status {
	case StatusNew, StatusAck, StatusPartial:
		return true
	default:
		return false
	}
}

// GetStateDescription 获取状态描述
func (sm *StateMachine) GetStateDescription(status Status) string {
	descriptions := map[Status]string{
		StatusPending:   "订单待提交",
		StatusNew:       "订单已创建",
		StatusAck:       "订单已确认",
		StatusPartial:   "订单部分成交",
		StatusFilled:    "订单完全成交",
		StatusCanceling: "订单撤销中",
		StatusCanceled:  "订单已撤销",
		StatusRejected:  "订单被拒绝",
		StatusExpired:   "订单已过期",
	}

	if desc, ok := descriptions[status]; ok {
		return desc
	}
	return "未知状态"
}
