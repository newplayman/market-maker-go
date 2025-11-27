package risk

import (
	"market-maker-go/internal/store"
)

// Guard 负责统一的风控检查（Worst-case, Hard limit等）
type Guard struct {
	netMax        float64
	worstCaseMult float64
	store         *store.Store
}

func NewGuard(netMax, worstCaseMult float64, st *store.Store) *Guard {
	return &Guard{
		netMax:        netMax,
		worstCaseMult: worstCaseMult,
		store:         st,
	}
}

// CheckWorstCase 检查是否超过最坏敞口限制
func (g *Guard) CheckWorstCase(side string, currentPos float64) bool {
	pendingBuy := g.store.PendingBuySize()
	pendingSell := g.store.PendingSellSize()

	if side == "BUY" {
		worstLong := currentPos + pendingBuy
		if worstLong >= g.netMax*g.worstCaseMult {
			return false // 禁止开仓
		}
	} else {
		worstShort := currentPos - pendingSell
		if worstShort <= -g.netMax*g.worstCaseMult {
			return false // 禁止开仓
		}
	}
	return true
}
