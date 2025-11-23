# Market Maker Go (Binance USDC-M Skeleton)

本仓库包含做市核心模块的骨架代码与模拟入口，当前重点聚焦 Binance USDC 永续、WebSocket 行情、零库存/网格策略与基础风控。

## 快速开始

```bash
cd /Users/bendu/market-maker-go
go test ./...    # 确认代码可编译并通过单测
go run ./cmd/sim # 本地随机行情模拟，不会连接交易所（支持参数调参，见下）
go run ./cmd/backtest mids.csv # 以 CSV mid 价格回放，生成报价
./scripts/test_all.sh # 依次跑单测、sim、backtest 示例
./scripts/start_local.sh # 同上，便捷本地回归
```

配置示例：`configs/config.example.yaml`（填入 API Key/Secret、风险参数等，`symbols` 段可写入从 `cmd/binance_symbol` 获取的 tickSize/stepSize/minNotional）。

### 策略 & 风控配置
每个 `symbols.<symbol>` 可定义策略与风控参数，`cmd/runner` 会自动加载：

```yaml
symbols:
  ETHUSDC:
    tickSize: 0.01
    stepSize: 0.001
    minQty: 0.001
    maxQty: 8000
    minNotional: 20
    strategy:
      minSpread: 0.0006
      baseSize: 0.01
      targetPosition: 0
      maxDrift: 1
      quoteIntervalMs: 800
      takeProfitPct: 0.001
    risk:
      singleMax: 1
      dailyMax: 10
      netMax: 5
      latencyMs: 500
      pnlMin: -5
      pnlMax: 10
      reduceOnlyThreshold: 3
      stopLoss: -20
      haltSeconds: 30
      shockPct: 0.02
```

命令行只需保留 `-symbol`、`-dryRun` 等基础开关，其余策略/风控参数由配置驱动，可为每个交易对设置独立策略、报价频率和风险阈值。

## 模块概览
- `strategy/`：零库存报价、动态网格、动态价差，支持回测接口。
- `risk/`：单笔/日累计/净敞口限额，波动熔断，告警通知。
- `order/`：订单状态管理，可插拔 Gateway。
- `gateway/`：Binance REST/WS 抽象，签名、listenKey 管理（支持 TokenBucket 限流），Stub/测试客户端。
- `market/`：深度/成交归一化，内存订单簿，中间价与陈旧度计算，发布订阅。
- `inventory/`：仓位跟踪、估值、快照。
- `sim/`：轻量 Runner 串联策略→风控→下单，用于本地演示。
- `cmd/sim/`：可执行程序入口，随机生成 mid 价格走一遍链路；可结合 `-tickSize/-stepSize/-minNotional` 等参数模拟实盘精度校验。
- `cmd/binance_userstream/`：listenKey + 用户流/深度流演示，会加载 `configs.symbols` 自动注入精度限制，使用 `TokenBucketLimiter` 控制 REST 请求，并把 ORDER/ACCOUNT 事件同步到 `order.Manager`/`inventory` 示例，WS 支持自动重连。
- `cmd/runner/`：基础 orchestrator，串联策略→风控→下单→REST gateway（带限流、参数校验、自动重试）→用户流同步，可通过 `-dryRun` 控制是否真实下单，默认暴露 `-metricsAddr :9100` 的 Prometheus 指标。
- `cmd/binance_router/`：示例如何把用户流事件同步到 `order.Manager` 与 `inventory`（需自备真实 API Key）。
- `cmd/backtest/`：读取 CSV mid 价格序列，调用策略回测接口输出报价曲线。

## 环境变量（联网时）
- `BINANCE_API_KEY` / `BINANCE_API_SECRET`
- `BINANCE_REST_URL`（默认 https://fapi.binance.com）
- `BINANCE_WS_ENDPOINT`（默认 wss://fstream.binance.com）
- `LISTEN_KEY`（用户数据流，可选，供 WS demo 使用）
- 其余通用配置可参考 `.env` / `configs/config.example.yaml`

## 部署脚本
- `scripts/run_runner.sh`：读取 `CONFIG_PATH`/`SYMBOL`/`DRY_RUN` 等环境变量后启动 `cmd/runner`，默认启用 dry-run，可配合 `REST_RATE/REST_BURST/METRICS_ADDR` 控制限流与监控。
- `docs/systemd-runner.service`：提供 systemd 单元示例，将仓库放在 `/opt/market-maker` 并设定环境变量即可 `systemctl enable --now runner`。
- `scripts/install_prometheus_stack.sh`：一键安装 Prometheus + node_exporter + Alertmanager，并自动生成抓取 runner (`:9100`) 的配置与示例告警。
- `scripts/install_loki_stack.sh`：安装 Loki + Promtail + Grafana，默认把 `/var/log/market-maker/*.log` 推送至 Loki，Grafana 监听 3000 端口。
  - 建议运行 runner 时 `./scripts/run_runner.sh 2>&1 | tee -a /var/log/market-maker/runner.log`（配合 `logrotate`），Promtail 会自动采集。
  - 以上两个安装脚本需 root 权限，默认安装路径 `/opt/market-monitoring`。
- Grafana 仪表盘：`monitor/runner_dashboard.json`（在 Grafana → Dashboards → Import 中选择该文件即可）。
- Alertmanager 路由示例：`monitor/alertmanager_route_example.yml`（可直接拷贝到 `/opt/market-monitoring/alertmanager.yml` 并替换 webhook/email）。

## 数据与回测
- 示例数据：`data/mids_sample.csv`（100 条 mid），可配置多个 symbol 同时回放。
- `cmd/backtest` 支持根据配置加载策略/风控参数及多 symbol CSV：  
  ```bash
  go run ./cmd/backtest -config configs/config.yaml \
    -symbols ETHUSDC:data/mids_sample.csv,BTCUSDC:data/btc_mids.csv \
    -out summary.csv
  ```
  输出会包含每个 symbol 的笔数、均值、最大回撤、首个报价等统计（可选写入 CSV）。

## 接入 Binance 的提示
- 实盘需替换 `gateway.BinanceRESTStub/BinanceRESTClient` 中的 HTTP 客户端配置，并实现真实 WS 客户端（深度、成交、用户数据流 listenKey；含心跳与断线重连）。
- 下单需符合 Binance USDC 永续参数（timeInForce/GTX、reduceOnly、精度），并处理 429/418 等限流。
- 建议先用 Stub 与 `sim.Runner` 完成端到端自测，再接入真实 API。

### WS demo（需要网络）
`scripts/run_ws_demo.sh` 可快速订阅深度流（和可选 listenKey）：
```bash
cd /Users/bendu/market-maker-go
BINANCE_WS_ENDPOINT="wss://fstream.binance.com" LISTEN_KEY="your_listen_key" ./scripts/run_ws_demo.sh BTCUSDT
```
只看公共深度时，可以不设置 `LISTEN_KEY`。

### sim 参数示例
```bash
go run ./cmd/sim -symbol BTCUSDT -ticks 10 -minSpread 0.0008 -baseSize 1 \
  -singleMax 3 -dailyMax 30 -netMax 8 -latencyMs 200
```
可调整策略（价差、基础量、目标仓位等）与风控（单笔/日累计/净敞口、下单最小间隔）；若想验证精度限制，可再加 `-tickSize 0.01 -stepSize 0.001 -minNotional 20` 等参数（参数可从 `configs.symbols` 或 `cmd/binance_symbol` 获取）。

### 运营监控
`cmd/runner` 默认在 `-metricsAddr :9100` 暴露 Prometheus 指标（REST 请求/错误/延迟、WS 重连次数、策略报价次数、风控拒单、仓位/PnL、mid 价格等），可直接用 `curl http://localhost:9100/metrics` 检查；结合 Prometheus + Grafana 即可实现实时监控。

关键日志通过 `logEvent` 以 JSON 格式输出（包含 event、orderId、symbol、error 等字段），方便接入 ELK/Vector 等日志系统。
