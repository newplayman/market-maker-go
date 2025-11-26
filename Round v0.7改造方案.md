Round8 生存版 + 磨成本核武器  
完整技术改造文档（2025-12）  
—— 给工程师的“防呆版”执行手册，照着敲不会死，改错就打手目标  
在你现有 Round v0.7 项目基础上，用最少改动（≤6 个文件）实现以下四个“绝不爆仓”能力：

1. 订单/仓位/资金费率实时（WebSocket UserStream）
2. 防止所有挂单被一次性扫完（Worst-Case Exposure + 指数衰减）
3. 资金费率真实计费 + 保留价偏移
4. 硬帽附近横盘期自动磨成本（Inventory Grinding）

改动范围（只允许动下面这些文件）

```text
configs/round8_survival.yaml                  # 主配置文件（新增大量参数）
cmd/runner/main.go                            # 启动 WebSocket + 启动磨成本循环
internal/exchange/binance_ws.go               # 新文件：WebSocket User Data Stream
internal/strategy/geometric_v2.go             # 替换原来的 asmm 策略文件
internal/risk/guard.go                        # 新增 worst-case、funding、grinding 触发
internal/risk/grinding.go                     # 新文件：磨成本核心逻辑
internal/store/store.go                       # 必须新增 5 个只读方法（给上面用）
metrics/prometheus.go                         # 新增 12 个关键指标
```

一、核心参数（configs/round8_survival.yaml 完整内容）  
工程师直接覆盖原文件，禁止删参数，只允许微调数值。

yaml

```yaml
symbol: ETHUSDC
quote_interval_ms: 220

# ===== 基础做市参数 =====
base_size: 0.007
net_max: 0.20                          # 硬帽，绝对不能改大
min_spread: 0.00065

# ===== 几何网格（保留你原来写法）=====
layer_spacing_mode: geometric
spacing_ratio: 1.185
layer_size_decay: 0.92
max_layers: 28

# ===== 防扫单核心（最致命）=====
worst_case:
  multiplier: 1.15                     # 允许超挂 15%（给波动留余量）
  size_decay_k: 3.8                    # 指数衰减系数，越高越激进（推荐 3.6~4.2）

# ===== 资金费率真实成本 =====
funding:
  sensitivity: 2.2                     # 保留价偏移强度
  predict_alpha: 0.25                  # EMA 预测下一期费率

# ===== 磨成本核武器 =====
grinding:
  enabled: true
  trigger_ratio: 0.87                  # 仓位 ≥87% net_max 触发
  range_std_threshold: 0.0038          # 30分钟价格标准差阈值（<0.38% 才磨）
  grind_size_pct: 0.075                # 每次反向 taker 7.5% 仓位
  reentry_spread_bps: 4.2              # 重新挂 maker 单的有利偏移
  max_grind_per_hour: 18
  min_interval_sec: 42
  funding_boost: true
  funding_favor_multiplier: 1.4

# ===== 分层减仓（放宽，减少误触发）=====
drawdown_bands: [6.0, 11.0, 17.0]
reduce_fractions: [0.25, 0.45, 0.80]
twap_slices: 4
twap_interval_ms: 1200
```

二、Store 必须新增的 5 个只读方法（internal/store/store.go）

go

```go
func (s *Store) PendingBuySize() float64      // 所有活跃买单数量之和
func (s *Store) PendingSellSize() float64     // 所有活跃卖单数量之和
func (s) MidPrice() float64                   // 当前中值价
func (s) PriceStdDev30m() float64             // 最近30分钟价格标准差
func (s) PredictedFundingRate() float64       // 当前预测的下一期资金费率（正=多头吃亏）
```

三、WebSocket UserStream 实现规范（internal/exchange/binance_ws.go）必须满足：

1. 启动即 POST /fapi/v1/listenKey 获取 listenKey
2. 连接 wss://fstream.binance.com/ws/{listenKey}
3. 每 25 分钟自动 PUT keepalive
4. 断线自动重连（最多 5 次，间隔 3s、6s、12s…）
5. 三类消息必须实时回调：
    - ORDER_TRADE_UPDATE → store.HandleOrderUpdate
    - ACCOUNT_UPDATE → store.HandlePositionUpdate
    - FUNDING_RATE → risk.HandleFundingRate

四、策略层必须实现的防扫单逻辑（internal/strategy/geometric_v2.go）每生成一层报价前必须执行：

go

```go
// 计算最坏敞口
worstLong  := position + pendingBuySize
worstShort := position - pendingSellSize

// 买单方向检查
if side == "BUY" && worstLong >= cfg.NetMax * cfg.WorstCaseMultiplier {
    return 0, 0
}
// 卖单方向检查
if side == "SELL" && worstShort <= -cfg.NetMax * cfg.WorstCaseMultiplier {
    return 0, 0
}

// 指数衰减 size
exposure := worstLong if side=="BUY" else -worstShort
decay := math.Exp(-math.Abs(exposure)/cfg.NetMax * cfg.SizeDecayK)
size := cfg.BaseSize * decay * math.Pow(0.92, float64(layer))
```

五、磨成本模块完整实现要求（internal/risk/grinding.go）必须 100% 按以下逻辑实现（82 行核心代码，禁止改逻辑）：

1. 每 55 秒调用一次 MaybeGrind()
2. 检查是否达到 trigger_ratio + 横盘条件
3. 资金费率有利时自动 ×1.4
4. 多头磨：先市价卖 → 立即挂比当前价低 4.2bps 的买单（size×2.1）
5. 空头磨：先市价买 → 立即挂比当前价高 4.2bps 的卖单（size×2.1）
6. 严格频率限制（每小时 ≤18 次，两次间隔 ≥42 秒）

六、必加的 12 个 Prometheus 指标（metrics/prometheus.go）

go

```go
mm_worst_case_long            // 最坏多头敞口
mm_worst_case_short           // 最坏空头敞口
mm_dynamic_decay_factor       // 当前 size 衰减倍率
mm_funding_pnl_acc            // 累计资金费率盈亏
mm_predicted_funding_rate
mm_grind_count_total
mm_grind_active               // Gauge 1=正在磨
mm_grind_cost_saved           // 估算节省的持仓成本
mm_price_stddev_30m
mm_quote_suppressed           // 是否因 worst-case 暂停报价
mm_ws_connected               // WebSocket 是否在线
mm_rest_fallback_count        // 降级使用 REST 次数（>10 就报警）
```

七、测试验收标准（必须全部通过才算完成）

1. 启动后 10 秒内日志出现 "WebSocket UserStream connected"
2. Grafana 首屏能看到 mm_ws_connected=1  
    在 Grafana 首屏可以看到 mm_ws_connected=1
3. 人为挂 30 层买单，mm_worst_case_long 不会超过 0.23
4. 手动触发资金费率事件，mm_funding_pnl_acc 正确变化
5. 仓位到 0.18+，价格横盘 30 分钟，日志出现 "Grinding triggered"，且能看到市价单+追单
6. 一小时内 grind 次数 ≤18

八、调参顺序（只允许按这个顺序调）

1. 先跑 48 小时看 mm_worst_case_long 最高值  
    → 经常 >0.22 → 把 size_decay_k 从 3.8 改到 4.1
2. 利润太低 → base_size 0.007 → 0.008
3. 资金费率一天亏 >5 USDC → funding.sensitivity 2.2 → 2.6
4. 磨成本太猛手续费吃死 → grind_size_pct 0.075 → 0.055

九、禁止事项（写了就打手）

- 禁止关闭 grinding
- 禁止把 net_max 改到 0.25 以上
- 禁止用 REST 轮询代替 WebSocket
- 禁止把磨成本间隔改到 <30 秒
- 禁止删除任何一条新增指标

