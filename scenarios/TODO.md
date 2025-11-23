# TODO Roadmap (离线/联网)

## 状态快照（VPS 自检 2025-02-14）
- [x] Go 1.25.4 + `go test ./...`、`cmd/sim`、`cmd/backtest` 全部通过，说明当前代码在 VPS 上可编译运行。
- [x] `configs/config.example.yaml` 可被 `config.Load` 解析，env override 依赖 `MM_GATEWAY_API_KEY/MM_GATEWAY_API_SECRET`。
- [ ] 尚未拉取真实行情 / 下单；Binance API Key、WS listenKey、监控/日志后端待接入。
- [x] `configs/config.yaml` 已写入 Binance 子账号 API Key（ETHUSDC，100 USDC 测试资金），具备进行 dry-run/实盘联调的配置基础。
- [x] 新增 `cmd/binance_balance`（基于真实 REST 客户端），已能读取 Binance USDC 永续账户余额（当前 100 USDC）。
- [x] 新增 `cmd/binance_position`（真实 REST），可实时查询 ETHUSDC 等合约的持仓/盈亏信息。
- [x] 新增 `cmd/binance_account`（真实 REST），输出账户可用余额及 ETHUSDC 杠杆档位，供下单前参数校验。
- [x] 新增 `cmd/binance_symbol`（真实 REST），可快速查看 ETHUSDC 的 tickSize、stepSize、最小名义等参数。
- [x] `symbols` 配置 + `order.SymbolConstraints` 校验：`sim.Runner` 会在下单前检查 tickSize/stepSize/minNotional。
- [x] 新增 `cmd/binance_userstream`，可自动创建/保活 listenKey 并订阅用户流 + depth 流（已成功打印 ETHUSDC depth push）。
- [x] 用户流事件解析：`gateway.ParseUserData` + `cmd/binance_userstream` 已可输出 ORDER/ACCOUNT 回报。
- [x] REST 限流占位：`gateway.BinanceRESTClient` 支持注入 `RateLimiter`（默认可用 `TokenBucketLimiter`）以避免 429/418。
- [x] 初版 orchestrator：`cmd/runner` 串联策略→下单→REST→用户流同步（默认 dry-run）。
- [x] 风控接入：`cmd/runner` 支持限额/延迟/PnL Guard，可切换 dry-run/实盘。
- [x] 监控：`cmd/runner` 暴露 Prometheus `/metrics`（REST 请求/错误/延迟、WS 重连、策略报价、风控拒单、仓位/PnL、mid 价格）。
- [x] 日志：`cmd/runner` 使用 JSON `logEvent`，输出 order/ws/风控/策略事件字段，便于集中采集。
- [x] WS 自动重连：`gateway.BinanceWSReal` 支持指数退避重连回调，`cmd/binance_userstream`/`cmd/runner` 均可利用。

## 已完成 ✔
- [x] 核心模块骨架：strategy/risk/order/market/inventory/sim/backtest，`go test ./...` 通过。
- [x] 风控组合：限额、VWAP、频率、PnL 守卫，可用 `risk.BuildGuards` 组合。
- [x] Binance 抽象：REST 签名骨架、listenKey 管理、WS 骨架与深度解析、demo 脚本。
- [x] 本地演示脚本：`./scripts/test_all.sh`、`cmd/sim`、`cmd/backtest`；示例数据 `data/mids_sample.csv`。
- [x] 里程碑/离线/在线说明：`scenarios/offline.md`、`scenarios/todo_online.md`、README。

## 待办（可离线推进） ☐
- [x] 细化策略/风控参数与配置映射：`configs.symbols.<symbol>.strategy/risk` 已驱动 `cmd/runner`，附示例与校验。
- [x] 扩展回测工具：`cmd/backtest` 支持多 symbol CSV、读取配置策略参数并输出统计/CSV。
- [x] 日志/监控接口占位：整理统一日志字段（`monitor/logschema`）、预埋 Prometheus 指标名 & mock exporter（`monitor/metrics/mock_exporter.go`），并补单测。
- [ ] 更多单测/场景：策略漂移/波动场景、风控边界、订单簿大批量增量+陈旧度检测。
- [ ] 交易员视角的 Grafana 面板：展示实时 PnL、成交/手续费、仓位、风险状态，并提供参数填写入口（与工程监控面板区分）。
- [x] 部署/运行脚本：`scripts/run_runner.sh` + `docs/systemd-runner.service` 可直接配置 ENV/systemd 运行。
- [x] `cmd/runner` dry-run 精度修复：对报价价位/数量做 tickSize/stepSize 对齐，并过滤深度流混入的非用户数据，消除 `quote_error` 与 JSON 解析报错。
- [x] Dry-run 联调回归：以 `SYMBOL=ETHUSDC DRY_RUN=1` 运行 `cmd/runner`（建议 `./scripts/run_runner.sh`，持续 ≥30 min），期间需记录 `logs/` JSON 与 `curl 127.0.0.1:9100/metrics` 指标，确认无 WS/REST 错误、无 `quote_error`、重连次数可接受，再进入实盘阶段。（最新一轮：WS 连续稳定 30 min，`risk_rejects_total=0`）
- [ ] 策略 V2（见 `docs/strategy_risk_enhancement.md`）：实现 inventory skew、动态 spread/刷新频率、盘口插单与快进快出逻辑。
  - [x] 策略子任务：动态 spread/quoteInterval 公式落地并回写配置。
  - [x] 策略子任务：inventory skew + reduce-only 状态下的“先撤后挂/优先减仓”调度。
  - [x] 策略子任务：盘口插单/快进快出（含 take-profit、shock 暂停）回归测试，dry-run 日志需看到 `strategy_adjust` 事件。
  - [x] 策略子任务：混合挂单（staticFraction/staticTicks），动态刷新 + 静态底仓挂单并纳入 reduce-only/暂停逻辑。
  - [x] 策略子任务：Post Only 降级/冷却机制，-5022 拒单后临时放宽到普通限价并自动恢复（含本周期 fallback 重试）。
  - [x] 策略子任务：REST Gateway 保留策略的 Post Only 设定，允许 fallback 真正发出 Taker 单。
  - [x] 策略子任务：reduce-only 报价缺深度时自动退化为 `mid ± slippage` 的激进行为，确保 IOC 单可成交。
- [ ] 风控 V2：扩展 reduce-only 行为、stopLoss/shock halt、自动减仓与平仓机制，并新增相关监控指标。
  - [x] 风控子任务：`reduceOnlyThreshold` 触发时仅允许减仓 + Prometheus `runner_risk_state`。
  - [x] 风控子任务：止损/暂停流程（撤单→紧急减仓→恢复）+ 操作说明。
  - [x] 风控子任务：监控指标/日志补充（inventory、spread、volatility、risk_event）。
  - [x] 风控子任务：reduce-only 深度&滑点保护（`reduceOnlyMaxSlippagePct` + `depthFill*` 指标）。
  - [x] 风控子任务：浮盈优先平仓（减仓单保持在盘口，仅在偏差/滑点超标时替换）。
- [x] 策略/风控 V2 设计文档：`docs/strategy_risk_enhancement.md` 已完成，作为以上子任务的需求依据。

- [ ] Binance REST/WS 联调：listenKey 生命周期、深度快照+增量校验、用户数据流回报解析、限流重试策略（`cmd/binance_userstream` 已示范如何同步订单/持仓，下一步需接入主进程并实现重连）。
  - [x] Runner 启动时通过 `/fapi/v1/depth` 拉取 snapshot 并回写 orderbook，日志/Prometheus 记录 `depth_snapshot`。
- [x] listenKey + 用户流 CLI 验证（`cmd/binance_userstream`），下一步把事件解析进 order/inventory 模块。
- [x] （REST 已通）补齐 /fapi/v2/account、/fapi/v1/leverageBracket 接口，输出杠杆与余额；下一步进入 listenKey / 下单精度校验。
- [x] 补齐 /fapi/v1/exchangeInfo 解析与 CLI，已拿到 ETHUSDC 的 tickSize/stepSize/minNotional；下一步可将其接入策略/风控配置。
- [x] 实盘下单参数校验：REST Gateway 校验 timeInForce/GTX/reduceOnly，并对 429/418 做指数退避重试。
- [x] 实盘日志与监控：WS/REST 错误与延迟上报、Prometheus 指标扩展、日志 Schema 统一（详见 `monitor/logging_monitoring.md` 和 `monitor/logschema`）。
- [ ] 实盘策略/风控调优：根据真实波动调节 spread、基础量、PnL 守卫；对拒单/限流做压测。
- [ ] PnL 评估：按手续费/滑点统计真实 PnL，输出报告评估策略是否具备剥头皮潜力。
- [ ] 信号层迭代规划：引入盘口速度、VWAP 偏移、资金费率权重等指标，形成下一阶段实施计划。
- [ ] 实盘联调（阶段 1）：关闭 dry-run，采用极小 `baseSize`（如 0.001 ETH）和更严 `risk.dailyMax`，在真实账户内完成 10~20 轮下单→撤单验证，并实时记录日志/指标。

## 待你配合/解锁的事项
- [x] 提供 Binance API Key/Secret（建议子账号+最小权限），并确认是否要打通 listenKey websocket。**操作提示：**复制 `configs/config.example.yaml` 为 `configs/config.yaml`，把 key/secret 写进 `gateway` 段；或在 shell export `MM_GATEWAY_API_KEY/MM_GATEWAY_API_SECRET`（`config.LoadWithEnvOverrides` 会覆盖）。
- [ ] 日志/监控采用 `journald + Promtail + Loki + Prometheus node_exporter + Alertmanager（Grafana Agent）` 的轻量方案；若希望改用其他栈请提前告知。
- [x] 指定第一批交易对与目标资金规模（ETH/USDC 永续，测试资金 100 USDC）；后续我会据此更新 risk/strategy 配置示例。
- [x] 确认 dry-run 阶段的观察指标（Prometheus/日志）与通过标准：例如连续 ≥30 min 内 `runner_ws_disconnect_total` ≤ 3、`runner_rest_errors_total=0`、日志无 `quote_error`，并在满足标准后由你拍板切换到真实下单。（已达标）
- [x] 确认实盘阶段的参数调节策略：实盘初期 `strategy.baseSize=0.001`、`risk.dailyMax=5`、`inventory.maxDrift=0.2`，确保 100 USDC 资金 100% 可承受；后续若需扩容再同步调整。
