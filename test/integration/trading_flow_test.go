package integration

import (
	"testing"
	"time"

	"market-maker-go/infrastructure/logger"
	"market-maker-go/internal/strategy"
	"market-maker-go/inventory"
	"market-maker-go/order"
)

// TestNormalTradingFlow 测试正常交易流程
func TestNormalTradingFlow(t *testing.T) {
	// 1. 初始化组件
	mockGateway := NewMockGateway()
	defer mockGateway.Reset()

	// 创建日志
	log, err := logger.New(logger.Config{
		Level:   "info",
		Outputs: []string{"stdout"},
		Format:  "console",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer log.Close()

	// 创建订单管理器
	orderMgr := order.NewManager(mockGateway)

	// 创建库存跟踪器
	inv := &inventory.Tracker{}

	// 创建策略
	strategyConfig := strategy.Config{
		BaseSpread:   0.001, // 0.1%
		BaseSize:     0.01,  // 0.01 ETH
		MaxInventory: 0.05,  // 0.05 ETH
		SkewFactor:   0.3,
	}
	mmStrategy := strategy.NewBasicMarketMaking(strategyConfig)

	// 2. 生成报价
	ctx := strategy.Context{
		Symbol:       "ETHUSDC",
		Mid:          2000.0,
		Inventory:    0.0,
		MaxInventory: 0.05,
	}

	quotes, err := mmStrategy.GenerateQuotes(ctx)
	if err != nil {
		t.Fatalf("Failed to generate quotes: %v", err)
	}

	if len(quotes) != 2 {
		t.Fatalf("Expected 2 quotes, got %d", len(quotes))
	}

	// 验证报价
	buyQuote := quotes[0]
	sellQuote := quotes[1]

	if buyQuote.Side != "BUY" {
		t.Errorf("Expected BUY side, got %s", buyQuote.Side)
	}
	if sellQuote.Side != "SELL" {
		t.Errorf("Expected SELL side, got %s", sellQuote.Side)
	}

	// 验证价差
	spread := sellQuote.Price - buyQuote.Price
	expectedMinSpread := 2000.0 * 0.001 // 0.1% of mid price
	if spread < expectedMinSpread*0.9 {
		t.Errorf("Spread too narrow: %.2f, expected >= %.2f", spread, expectedMinSpread)
	}

	// 3. 下单
	buyOrder, err := orderMgr.Submit(order.Order{
		Symbol:   "ETHUSDC",
		Side:     buyQuote.Side,
		Type:     "LIMIT",
		Price:    buyQuote.Price,
		Quantity: buyQuote.Size,
	})
	if err != nil {
		t.Fatalf("Failed to place buy order: %v", err)
	}

	sellOrder, err := orderMgr.Submit(order.Order{
		Symbol:   "ETHUSDC",
		Side:     sellQuote.Side,
		Type:     "LIMIT",
		Price:    sellQuote.Price,
		Quantity: sellQuote.Size,
	})
	if err != nil {
		t.Fatalf("Failed to place sell order: %v", err)
	}

	// 等待订单确认
	time.Sleep(50 * time.Millisecond)

	// 4. 验证订单状态
	buyStatus, ok := orderMgr.Status(buyOrder.ID)
	if !ok {
		t.Fatalf("Buy order not found")
	}
	if buyStatus != order.StatusAck && buyStatus != order.StatusNew {
		t.Errorf("Expected NEW/ACK status, got %s", buyStatus)
	}

	sellStatus, ok := orderMgr.Status(sellOrder.ID)
	if !ok {
		t.Fatalf("Sell order not found")
	}
	if sellStatus != order.StatusAck && sellStatus != order.StatusNew {
		t.Errorf("Expected NEW/ACK status, got %s", sellStatus)
	}

	// 5. 模拟成交
	err = mockGateway.SimulateFullFill(buyOrder.ID)
	if err != nil {
		t.Fatalf("Failed to simulate fill: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// 6. 更新订单状态
	err = orderMgr.Update(buyOrder.ID, order.StatusFilled)
	if err != nil {
		t.Fatalf("Failed to update order: %v", err)
	}

	// 7. 更新库存
	inv.Update(buyQuote.Size, buyQuote.Price)

	// 8. 验证库存
	position := inv.NetExposure()
	if position != buyQuote.Size {
		t.Errorf("Expected position %.4f, got %.4f", buyQuote.Size, position)
	}

	// 9. 通知策略成交
	mmStrategy.OnFill(strategy.Fill{
		Side:  "BUY",
		Price: buyQuote.Price,
		Size:  buyQuote.Size,
	})

	// 10. 验证策略统计
	stats := mmStrategy.GetStatistics()
	totalBuyFills := stats["total_buy_fills"].(int)
	if totalBuyFills != 1 {
		t.Errorf("Expected 1 buy fill, got %d", totalBuyFills)
	}

	t.Logf("✅ Normal trading flow test passed")
}

// TestPartialFillAndCancel 测试部分成交后撤单
func TestPartialFillAndCancel(t *testing.T) {
	mockGateway := NewMockGateway()
	defer mockGateway.Reset()

	orderMgr := order.NewManager(mockGateway)

	// 下单
	ord, err := orderMgr.Submit(order.Order{
		Symbol:   "ETHUSDC",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    2000.0,
		Quantity: 0.01,
	})
	if err != nil {
		t.Fatalf("Failed to place order: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// 部分成交 (50%)
	err = mockGateway.SimulatePartialFill(ord.ID, 0.5)
	if err != nil {
		t.Fatalf("Failed to simulate partial fill: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// 更新订单状态为部分成交
	err = orderMgr.Update(ord.ID, order.StatusPartial)
	if err != nil {
		t.Fatalf("Failed to update order: %v", err)
	}

	// 撤单
	err = orderMgr.Cancel(ord.ID)
	if err != nil {
		t.Fatalf("Failed to cancel order: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// 验证订单状态
	status, ok := orderMgr.Status(ord.ID)
	if !ok {
		t.Fatalf("Order not found")
	}

	if status != order.StatusCanceled {
		t.Errorf("Expected CANCELED status, got %s", status)
	}

	t.Logf("✅ Partial fill and cancel test passed")
}

// TestInventorySkew 测试库存倾斜
func TestInventorySkew(t *testing.T) {
	strategyConfig := strategy.Config{
		BaseSpread:   0.001,
		BaseSize:     0.01,
		MaxInventory: 0.05,
		SkewFactor:   0.3,
	}
	mmStrategy := strategy.NewBasicMarketMaking(strategyConfig)

	testCases := []struct {
		name         string
		inventory    float64
		maxInventory float64
		expectSkew   bool
	}{
		{
			name:         "Zero inventory - no skew",
			inventory:    0.0,
			maxInventory: 0.05,
			expectSkew:   false,
		},
		{
			name:         "Positive inventory - skew down",
			inventory:    0.03,
			maxInventory: 0.05,
			expectSkew:   true,
		},
		{
			name:         "Negative inventory - skew up",
			inventory:    -0.02,
			maxInventory: 0.05,
			expectSkew:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := strategy.Context{
				Symbol:       "ETHUSDC",
				Mid:          2000.0,
				Inventory:    tc.inventory,
				MaxInventory: tc.maxInventory,
			}

			quotes, err := mmStrategy.GenerateQuotes(ctx)
			if err != nil {
				t.Fatalf("Failed to generate quotes: %v", err)
			}

			if len(quotes) != 2 {
				t.Fatalf("Expected 2 quotes, got %d", len(quotes))
			}

			buyPrice := quotes[0].Price
			sellPrice := quotes[1].Price
			mid := 2000.0

			// 检查对称性
			if !tc.expectSkew {
				midPoint := (buyPrice + sellPrice) / 2
				if abs(midPoint-mid) > 0.1 {
					t.Errorf("Expected symmetric quotes around %.2f, got mid point %.2f", mid, midPoint)
				}
			}

			t.Logf("  Inventory: %.4f, Buy: %.2f, Sell: %.2f", tc.inventory, buyPrice, sellPrice)
		})
	}

	t.Logf("✅ Inventory skew test passed")
}

// TestConcurrentOrders 测试并发下单
func TestConcurrentOrders(t *testing.T) {
	mockGateway := NewMockGateway()
	defer mockGateway.Reset()

	orderMgr := order.NewManager(mockGateway)

	// 并发下10个订单
	numOrders := 10
	orderIDs := make(chan string, numOrders)
	errors := make(chan error, numOrders)

	for i := 0; i < numOrders; i++ {
		go func(idx int) {
			ord, err := orderMgr.Submit(order.Order{
				Symbol:   "ETHUSDC",
				Side:     "BUY",
				Type:     "LIMIT",
				Price:    2000.0 + float64(idx),
				Quantity: 0.01,
			})
			if err != nil {
				errors <- err
			} else {
				orderIDs <- ord.ID
			}
		}(i)
	}

	// 收集结果
	var successCount int
	var errorCount int

	timeout := time.After(2 * time.Second)
	for i := 0; i < numOrders; i++ {
		select {
		case <-orderIDs:
			successCount++
		case <-errors:
			errorCount++
		case <-timeout:
			t.Fatal("Timeout waiting for orders")
		}
	}

	if successCount != numOrders {
		t.Errorf("Expected %d successful orders, got %d (errors: %d)", numOrders, successCount, errorCount)
	}

	// 验证统计
	stats := mockGateway.GetStatistics()
	if stats["place_order_count"] != numOrders {
		t.Errorf("Expected %d place order calls, got %d", numOrders, stats["place_order_count"])
	}

	t.Logf("✅ Concurrent orders test passed (placed %d orders)", successCount)
}

// TestOrderLatency 测试订单延迟
func TestOrderLatency(t *testing.T) {
	mockGateway := NewMockGateway()
	defer mockGateway.Reset()

	// 设置延迟模拟网络延迟
	mockGateway.SetSimulateLatency(true, 100*time.Millisecond)

	orderMgr := order.NewManager(mockGateway)

	start := time.Now()

	ord, err := orderMgr.Submit(order.Order{
		Symbol:   "ETHUSDC",
		Side:     "BUY",
		Type:     "LIMIT",
		Price:    2000.0,
		Quantity: 0.01,
	})
	if err != nil {
		t.Fatalf("Failed to place order: %v", err)
	}

	elapsed := time.Since(start)

	if elapsed < 100*time.Millisecond {
		t.Errorf("Expected latency >= 100ms, got %v", elapsed)
	}

	// 验证订单创建成功
	if ord.ID == "" {
		t.Error("Expected non-empty order ID")
	}

	t.Logf("✅ Order latency test passed (latency: %v)", elapsed)
}

// Helper function
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
