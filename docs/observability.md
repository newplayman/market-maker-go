## Observability（监控与日志）

### 1. 事件日志（`logs/runner_events.log`）

Runner 在启动时会创建 JSONL 格式的事件日志，用于回放整个交易过程。每条记录包含 `ts`（UTC 纳秒级时间戳）、`event` 名称及若干字段：

| event                 | 说明 | 主要字段 |
|-----------------------|------|----------|
| `runner_start` / `runner_stop_signal` | 启停及配置快照 | `symbol`、`marginType` |
| `ws_connected` / `ws_disconnected`    | 用户数据流状态 | `listenKey` |
| `order_submit` / `order_submit_result`| REST 下单/结果 | `symbol`、`side`、`type` (`LIMIT`/`MARKET`)、`price`、`qty`、`duration_ms`、`error` |
| `order_update`                         | Binance 推送的订单状态 | `order_id`、`client_id`、`status`、`execution`、`realized_pnl`、`pending_buy/sell` |
| `position_update`                      | 账户仓位变化（ACCOUNT_UPDATE） | `position`、`entry_price`、`pnl`、`reason` |
| `funding_update`                       | 资金费率与累计资金成本 | `rate`、`predicted_rate`、`accum_pnl` |
| `reduce_only_force` / `_error`         | 硬减仓触发/失败 | `side`、`qty`、`error` |
| `runner_snapshot`                      | 每 60s 输出的仓位/挂单快照 | `mid`、`position`、`pending_buy/sell` |

这些日志可以直接导入 Loki/Grafana，或用 `jq`/`pandas` 做 PnL/风控回溯。未来如果需要额外字段，可在 `eventLogger.Log` 处扩展。

### 2. Prometheus 指标

Runner 默认在 `:9199` 暴露 Prometheus 指标，`monitoring/prometheus/prometheus.yml` 已配置抓取 `host.docker.internal:9199`。新增的关键指标包括：

| 指标 | 说明 |
|------|------|
| `mm_runner_risk_state{symbol}` | 风控状态机（0=正常，1=软减仓，2=硬减仓） |
| `mm_reduce_only_force_total{symbol}` | 强制减仓动作计数 |
| `mm_pending_exposure{symbol,side}` | 当前挂单敞口 |
| 其他 `mm_*`（mid、position、worst_case、funding…） | 同 README 中的指标 |

Grafana `monitoring/grafana/provisioning/dashboards/market-maker.json` 已更新，包含以下面板：

1. Mid/Bid/Ask 走势
2. 净仓位 + Pending Exposure
3. Worst-case long/short
4. 风控状态、WS 连接、报价抑制
5. 5 分钟内强制 reduce-only 次数
6. Grinding 触发计数
7. Funding & 30m StdDev

### 3. REST/WSS 取舍

当前版本仍以 REST 执行下单/撤单/磨成本，WebSocket 负责推送行情和订单状态。REST 请求会在事件日志中记录耗时和返回码，Prometheus 监控 WS 连接状态。后续会按照规划将下单切换到 WSS，并保留 REST 作为 fallback。

### 4. 日志使用建议

1. **实盘结束后**，先保存 `logs/runner_events.log` 及 Prometheus 数据（可用 Grafana 快照）。  
2. 使用 `jq` 快速统计，例如 `jq 'select(.event=="order_update" and .status=="FILLED")' logs/runner_events.log | wc -l`。  
3. 若需要 CSV，可将事件日志导入 pandas：`pd.read_json("runner_events.log", lines=True)`。  
4. 对比 Binance APP 的余额/成交，可直接依据 `order_update` 与 `position_update` 事件进行核对。
