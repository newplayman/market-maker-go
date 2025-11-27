package store

import (
	"sync"
	"testing"
	"time"

	"market-maker-go/gateway"
)

// TestStore_ConcurrentOrderUpdates 测试并发订单更新的安全性
func TestStore_ConcurrentOrderUpdates(t *testing.T) {
	st := New("BTCUSDT", 0.1, nil)

	var wg sync.WaitGroup
	operations := 100

	// 并发写入订单更新
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				orderID := int64(workerID*1000 + j)
				st.HandleOrderUpdate(gateway.OrderUpdate{
					Symbol:         "BTCUSDT",
					OrderID:        orderID,
					Side:           "BUY",
					Status:         "NEW",
					OrigQty:        1.0,
					AccumulatedQty: 0,
					UpdateTime:     time.Now().UnixMilli(),
				})
			}
		}(i)
	}

	// 并发读取
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				_ = st.PendingBuySize()
				_ = st.PendingSellSize()
				_ = st.Position()
				_ = st.MidPrice()
			}
		}()
	}

	wg.Wait()

	// 验证最终状态一致性
	if buy := st.PendingBuySize(); buy < 0 {
		t.Errorf("negative pending buy: %f", buy)
	}
}

// TestStore_ConcurrentPositionUpdates 测试并发仓位更新
func TestStore_ConcurrentPositionUpdates(t *testing.T) {
	st := New("ETHUSDT", 0.1, nil)

	var wg sync.WaitGroup
	operations := 50

	// 并发更新仓位
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				st.HandlePositionUpdate(gateway.AccountUpdate{
					Positions: []gateway.AccountPosition{{
						Symbol:      "ETHUSDT",
						PositionAmt: float64(workerID*10 + j),
						EntryPrice:  2000.0,
					}},
				})
			}
		}(i)
	}

	// 并发读取
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				_ = st.Position()
			}
		}()
	}

	wg.Wait()
}

// TestStore_ConcurrentDepthUpdates 测试并发深度更新
func TestStore_ConcurrentDepthUpdates(t *testing.T) {
	st := New("BTCUSDT", 0.1, nil)

	var wg sync.WaitGroup
	operations := 100

	// 并发更新深度
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				bid := 50000.0 + float64(workerID*10+j)
				ask := bid + 1.0
				st.UpdateDepth(bid, ask, time.Now())
			}
		}(i)
	}

	// 并发读取
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				_ = st.MidPrice()
				_ = st.PriceStdDev30m()
			}
		}()
	}

	wg.Wait()

	if mid := st.MidPrice(); mid <= 0 {
		t.Errorf("invalid mid price: %f", mid)
	}
}

// TestStore_ConcurrentFundingRateUpdates 测试并发资金费率更新
func TestStore_ConcurrentFundingRateUpdates(t *testing.T) {
	st := New("BTCUSDT", 0.1, nil)

	// 设置初始状态
	st.HandlePositionUpdate(gateway.AccountUpdate{
		Positions: []gateway.AccountPosition{{
			Symbol:      "BTCUSDT",
			PositionAmt: 1.0,
			EntryPrice:  50000.0,
		}},
	})
	st.UpdateDepth(50000.0, 50001.0, time.Now())

	var wg sync.WaitGroup
	operations := 50

	// 并发更新资金费率
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				rate := 0.0001 * float64(workerID*10+j) / 1000
				st.HandleFundingRate(rate)
			}
		}(i)
	}

	// 并发读取
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				_ = st.PredictedFundingRate()
			}
		}()
	}

	wg.Wait()
}

// TestStore_ConcurrentReplacePendingOrders 测试并发订单替换
func TestStore_ConcurrentReplacePendingOrders(t *testing.T) {
	st := New("BTCUSDT", 0.1, nil)

	var wg sync.WaitGroup

	// 并发替换订单
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			orders := make([]gateway.OrderUpdate, 10)
			for j := 0; j < 10; j++ {
				orders[j] = gateway.OrderUpdate{
					Symbol:         "BTCUSDT",
					OrderID:        int64(workerID*100 + j),
					Side:           "BUY",
					Status:         "NEW",
					OrigQty:        1.0,
					AccumulatedQty: 0,
					UpdateTime:     time.Now().UnixMilli(),
				}
			}
			st.ReplacePendingOrders(orders)
		}(i)
	}

	// 并发读取
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = st.PendingBuySize()
				_ = st.PendingSellSize()
			}
		}()
	}

	wg.Wait()

	// 验证最终状态
	if buy := st.PendingBuySize(); buy < 0 {
		t.Errorf("negative pending buy after replace: %f", buy)
	}
}

// TestStore_MixedConcurrentOperations 测试混合并发操作
func TestStore_MixedConcurrentOperations(t *testing.T) {
	st := New("BTCUSDT", 0.1, nil)

	var wg sync.WaitGroup
	operations := 50

	// 订单更新
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < operations; i++ {
			st.HandleOrderUpdate(gateway.OrderUpdate{
				Symbol:         "BTCUSDT",
				OrderID:        int64(i),
				Side:           "BUY",
				Status:         "NEW",
				OrigQty:        1.0,
				AccumulatedQty: 0,
				UpdateTime:     time.Now().UnixMilli(),
			})
		}
	}()

	// 仓位更新
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < operations; i++ {
			st.HandlePositionUpdate(gateway.AccountUpdate{
				Positions: []gateway.AccountPosition{{
					Symbol:      "BTCUSDT",
					PositionAmt: float64(i),
					EntryPrice:  50000.0,
				}},
			})
		}
	}()

	// 深度更新
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < operations; i++ {
			bid := 50000.0 + float64(i)
			st.UpdateDepth(bid, bid+1.0, time.Now())
		}
	}()

	// 资金费率更新
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < operations; i++ {
			st.HandleFundingRate(0.0001)
		}
	}()

	// 并发读取所有状态
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				_ = st.PendingBuySize()
				_ = st.PendingSellSize()
				_ = st.Position()
				_ = st.MidPrice()
				_ = st.PriceStdDev30m()
				_ = st.PredictedFundingRate()
			}
		}()
	}

	wg.Wait()
}
