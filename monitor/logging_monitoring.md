# Runner 日志与监控说明

本文件总结 `cmd/runner` 在实盘/干跑过程中的日志格式、关键指标以及 Promtail/Grafana 的配置指引，便于运维快速接入监控。

## 1. 日志事件 Schema

日志输出全部采用 JSON，并由 `monitor/logschema` 在运行时校验字段完整性（缺少字段会写入 `_schema_error`）。目前关注的事件如下：

| 事件名 | 必备字段 | 含义 |
| --- | --- | --- |
| `strategy_adjust` | `symbol`,`mid`,`spread`,`spreadRatio`,`intervalMs` | 每轮报价时记录最终 mid、spread、动态刷新间隔，可用于还原策略行为 |
| `risk_event` | `symbol`,`state`,`reason`(可选) | 风控状态机切换（`normal` / `reduce_only` / `halted`） |
| `order_update` | `symbol`,`status`,`clientOrderId`,`orderId` | Binance 用户流回报；`status` 用于统计成交/撤单 |
| `depth_snapshot` | `symbol`,`bid`,`ask` | Runner 启动时的 REST Depth 初始化结果 |
| `listenkey_keepalive_*` | `listenKey`,`error`(可选) | ListenKey keepalive 成功/失败/重试情况 |
| `metrics_listen`/`metrics_error` | `addr`/`error` | Prometheus 监听状态 |

> **提示**：在 Loki 中按 `logfmt`/`json` 解析后，可以直接使用 `event=strategy_adjust` 过滤，若 `_schema_error` 被设定则触发告警。

## 2. Prometheus 指标

`cmd/runner` 暴露的主要指标：

- `runner_mid_price`：策略使用的 mid。
- `runner_spread`：实时 spread（结合日志可分析 spread 调整原因）。
- `runner_quote_interval_seconds`：动态报价刷新间隔，观察频率调节。
- `runner_risk_state`：0=normal，1=reduce_only，2=halted。
- `runner_orders_placed_total` / `runner_risk_rejects_total`：下单及风控拒单次数。
- `runner_rest_requests_total`/`runner_rest_errors_total`：REST 请求与错误。
- `runner_ws_connects_total` / `runner_ws_failures_total`：WebSocket 重连与失败。

Grafana 可以针对 `runner_risk_state` 设置阈值（>0 告警），对 `runner_spread`、`runner_quote_interval_seconds` 画曲线，配合日志还原策略行为。

## 3. Promtail 配置示例

```yaml
scrape_configs:
  - job_name: runner
    static_configs:
      - targets: [localhost]
        labels:
          job: runner
          __path__: /var/log/market-maker/runner.log
    pipeline_stages:
      - json:
          expressions:
            event: event
            ts: ts
            symbol: symbol
            state: state
            listenKey: listenKey
            _schema_error: _schema_error
            spread: spread
      - labels:
          event:
          symbol:
          state:
      - timestamp:
          source: ts
          format: RFC3339Nano
```

上述配置会将 `event`/`symbol` 作为 label；若存在 `_schema_error` 可在 Loki 中 `| _schema_error!=\"\"` 过滤查看异常日志。

## 4. Grafana 仪表盘建议

1. **日志面板**：基于 `event` 展示：
   - `strategy_adjust`：表格显示 `mid`、`spread`、`intervalMs`。
   - `listenkey_keepalive_*`：统计 30 分钟内成功/失败次数。
2. **Prometheus 面板**：
   - `runner_spread` + `runner_quote_interval_seconds` 组合折线。
   - `runner_risk_state` 与 `runner_position` 双纵轴，观察在 Reduce-only/ Halted 状态下的持仓。
   - `runner_rest_errors_total`、`runner_ws_failures_total` 作为单值 + 告警。

## 5. Alertmanager 建议

- **ListenKey keepalive 失败**：Loki 查询 `event="listenkey_keepalive_error"`，若 10 分钟内 >0 则告警。
- **风险状态**：Prometheus 规则 `runner_risk_state > 0` 持续 1 分钟触发，通知运维关注。
- **REST 错误率**：`increase(runner_rest_errors_total[5m]) / increase(runner_rest_requests_total[5m]) > 0.05` 告警。

通过上述约定，即可实现日志/指标统一监控，且后续若新增事件只需在 `monitor/logschema` 登记并更新本文即可。 
