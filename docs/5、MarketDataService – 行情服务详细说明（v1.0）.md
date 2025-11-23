
这是做市商系统的 **“感知层”**，对做市策略非常重要。  
行情的准确性、稳定性和时效性决定：

- 策略的报价精度
    
- 订单撤单速度
    
- 风控的正确判断
    
- 网格距离、波动率、盘口不平衡的计算
    
- 高频交易的胜率


本设计文档保证程序员或模型执行者能按其实现一个高质量行情服务。

---

# **目录**

1. 模块目的
    
2. 主要输入/输出
    
3. 必要数据源（WebSocket & REST）
    
4. 架构设计
    
5. 行情快照结构
    
6. 数据更新流程
    
7. 关键指标计算
    
    - Spread
        
    - Mid Price
        
    - Volatility（短周期波动率）
        
    - Orderbook Imbalance
        
    - Short-term Trend
        
8. 断线与重连机制
    
9. 多线程/并发模型
    
10. 性能与延迟要求
    
11. 可观测性（监控指标）
    
12. 未来扩展
    

---

# **1. 模块目的**

MarketDataService 的目标是：

- 维持与 Binance Futures USDC-M 的稳定 WebSocket 连接
    
- 实时接收：
    
    - 深度更新（orderbook depth）
        
    - 成交（aggTrade）
        
    - ticker
        
- 清洗后生成 **本地行情快照 MarketSnapshot**
    
- 将快照推送给策略引擎（StrategyEngine）
    
- 提供短周期衍生指标（volatility、imbalance、trend）
    

最终输出是一个 **毫秒级更新的、干净且统一的行情结构体**。

---

# **2. 模块主要输入 / 输出**

## 输入（来自交易所）

- WebSocket Depth Stream
    
- WebSocket Trade Stream
    
- REST 补齐（启动阶段或 WS 断线）
    

## 输出（给策略）

- 最新 MarketSnapshot
    
- 指标：
    
    - Spread
        
    - Mid Price
        
    - Maker/Taker 强度
        
    - Volatility（1~5 秒）
        
    - Orderbook Imbalance
        
    - Short-term Trend
        
    - Trade Flow（买/卖成交比）
        

---

# **3. Binance 必要数据源**

## 必须订阅的 WS 流

### **① 深度更新 depth@100ms**

建议使用：

```
<symbol>@depth@100ms
```

为什么不用 `@depth@20ms`？因为：

- 币安 USDC 合约更新频繁
    
- 实测 100ms 足够做轻量做市
    
- 更低间隔容易造成数据风暴 > 增加延迟 & 掉包风险
    

### **② 成交流 aggTrade**

```
<symbol>@aggTrade
```

用于：

- 真实成交方向（maker/taker）
    
- 成交大小
    
- 微趋势
    
- 波动率补充
    

### **③ ticker（可选）**

```
<symbol>@ticker
```

部分快速行情可从 ticker 取得（但不是必须）。

---

# **4. 架构设计**

推荐使用多 goroutine + channel 的结构：

```
 +-------------------------+
 | Binance WebSocket Conn  |
 +-------------------------+
     | depth/trade events  
     v  
 +-------------------------+
 | Message Dispatcher      |
 +-------------------------+
     | event unpack        
     v  
 +-------------------------+
 | DepthHandler            |
 | TradeHandler            |
 +-------------------------+
     | updates MarketData  
     v
 +-------------------------+
 | MarketSnapshotBuilder   |
 +-------------------------+
     | snapshot per update 
     v
 +-------------------------+
 | StrategyEngine OnTick   |
 +-------------------------+
```

---

# **5. MarketSnapshot 结构（标准化）**

策略和风控依赖这一结构：

```go
type MarketSnapshot struct {
    Symbol         string
    BestBid        float64
    BestBidSize    float64
    BestAsk        float64
    BestAskSize    float64

    MidPrice       float64
    Spread         float64

    Timestamp      time.Time

    // 派生指标
    Volatility     float64
    Imbalance      float64
    ShortTrend     float64
    TradeFlowBuyRatio float64
}
```

---

# **6. 数据更新流程**

## **深度更新 → 更新本地 OrderBook → 推送快照**

流程：

```
WS Depth update → apply to local OrderBook → recompute top levels → compute snapshot → push to Strategy
```

本地 OrderBook 建议存：

- 前 N 档（例如 20–50 档）
    
- 使用双 map 或 skiplist
    

### 深度更新处理伪代码：

```go
func onDepthUpdate(u DepthEvent) {
    for _, bid := range u.Bids {
        if bid.qty == 0 {
            delete(orderBook.Bids[bid.price])
        } else {
            orderBook.Bids[bid.price] = bid.qty
        }
    }
    for _, ask := range u.Asks {
        if ask.qty == 0 {
            delete(orderBook.Asks[ask.price])
        } else {
            orderBook.Asks[ask.price] = ask.qty
        }
    }

    snapshot := BuildSnapshot()
    PushToStrategy(snapshot)
}
```

## **Trade Update → 更新成交方向、波动率、趋势指标**

成交结构：

```go
type TradeEvent struct {
    Price float64
    Qty float64
    IsBuyerMaker bool // maker = true
    Ts time.Time
}
```

---

# **7. 指标计算（核心）**

## **7.1 Spread**

```
spread = best_ask - best_bid
```

单位：价格  
同时可转 tick：

```
spread_ticks = spread / tickSize
```

用于判断“是否挂单”。

---

## **7.2 Mid Price**

```
mid = (best_bid + best_ask) / 2
```

用于：

- 动态 spread
    
- 波动率基准价
    
- 微趋势
    

---

## **7.3 Volatility（过去 N 秒 mid price 标准差）**

推荐：

- 使用过去 1~3 秒
    
- 计算 mid price 序列的标准差
    

公式：

```
vol = StdDev(mid_price[t], t ∈ last X ms)
```

可用滚动窗口队列实现：

```go
window = sliding_window(last_30_mid_prices)
vol = stddev(window)
```

---

## **7.4 Orderbook Imbalance（盘口不平衡）**

计算前 3–5 档：

```
bidDepth = sum(bid_qty[0~5])
askDepth = sum(ask_qty[0~5])

imbalance = (bidDepth - askDepth) / (bidDepth + askDepth)
```

用例：

- bid 强则上移 mid、上移挂单基准
    

---

## **7.5 Short-term Trend（短期趋势）**

通过 mid price 变化率：

```
trend = mid_now - mid_200ms_ago
```

也可用 return：

```
trend = (mid_now / mid_200ms_ago) - 1
```

用来避免逆势挂单。

---

## **7.6 Trade Flow（taker 主导方向）**

定义：

```
buy_taker_volume = sum(qty for IsBuyerMaker==false)
sell_taker_volume = sum(qty for IsBuyerMaker==true)

ratio = buy_taker_volume / (buy + sell)
```

用于判断：

- 当前多 vs 空力量
    
- 是否偏向挂 ask 或 bid
    

---

# **8. 断线与重连机制**

行情服务必须 100% 自动重连，不允许等待人工干预。

### **断线检测**

若：

- N 秒无 depth 事件
    
- 心跳监测不到 ws ping/pong
    

则判定“连接已失效”。

### **重连流程**

```
close old connection  
sleep 300~500ms  
connect new ws  
subscribe depth & trade  
REST 拉取一次全量 orderbook  
应用初始 depth snapshot  
开始增量推送  
```

### **对账逻辑**

每次 WS 重连：

- 立即 REST 拉取全量深度
    
- 和本地订单簿对齐
    
- 再开始消费 WS depth 更新
    

这是“无序增量包”导致错乱的关键防护。

---

# **9. 多线程/并发模型（推荐 Go 版本）**

推荐结构：

```
goroutine WSReader → messageChan
goroutine Dispatcher → depthChan / tradeChan
goroutine OrderBookUpdater → snapshotChan
goroutine StrategyRunner ← snapshotChan
```

每类处理放到独立 goroutine，互不阻塞。

### **禁止策略直接操作 socket 或 orderbook！**

策略必须只读 MarketSnapshot。

---

# **10. 性能要求**

|指标|目标|
|---|---|
|WS 深度更新延迟|< 5ms 处理完成|
|snapshot 构建延迟|< 2ms|
|推送到策略延迟|< 1ms|
|最大 CPU 占用|< 20%（单 symbol）|

可支持每秒 10–20 次 snapshot 推送。

---

# **11. 可观测性（监控指标）**

推荐输出 Prometheus 指标：

|指标|含义|
|---|---|
|mds_ws_reconnect_total|重连次数|
|mds_depth_updates|depth 更新速率|
|mds_trade_updates|trade 更新速率|
|mds_snapshot_build_latency|构建 snapshot 耗时|
|mds_lag_ms|深度事件延迟|
|mds_volatility|波动率时间序列|
|mds_imbalance|盘口不平衡|
|mds_trend|短期趋势指标|

关键监控：

- 若 WS 重连频率过高 → 延迟问题
    
- 若 depth lag > 200ms → 网络问题
    
- 若波动率突然飙升 → 风控应介入
    

---

# **12. 未来扩展能力**

整个框架已为未来准备好：

- 多交易所数据对齐（Okx/Bybit）
    
- Depth sequence ID 去重
    
- 价差检测（跨交易所套利）
    
- K 线生成服务
    
- 资金费率预测
    
- 高频市场冲击模型（AIM）
    

---

# **下一份文档**

如果你愿意，我可以继续写：

> **《InventoryManager（库存管理）模块详细说明文档）》**

这是做市商中除了策略外最重要的风险控制模块之一，  
直接影响 inventory 风险、对冲行为、利润结构。

你需要继续吗？