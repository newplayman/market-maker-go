# 主入口指南（Main Guide）

本指南是后续工作的统一入口，明确近期目标、运行方式与关键参考。

- 最新权威蓝图：`docs/项目改造方案+Todo.md`
- 监控与运营：`docs/monitoring.md`
- 策略说明：`docs/strategy_asmm.md`, `docs/strategy_risk_enhancement.md`
- Runner 概览：`docs/runner_overview.md`
- 路线图：`docs/roadmap.md`

## 一、近期工作重心（两周）
1) 打通 `ASMM` 多档报价到 `sim.Runner.OnTick`，实现差分下发与减少闪撤；
2) 补齐 `market.Imbalance` 接入 `ASMM` 的 Regime 判定；
3) 简版 `VPIN/Toxic` 标记与策略保守化联动；
4) 修复 `internal/engine.TradingEngine` 停止后通道重建/幂等；
5) 统一配置单位（Bps/百分比），固化指标与日志事件名。

详见 `docs/TODO.md` 的优先级与具体任务。

## 二、运行与测试
- 单元测试：`go test ./...`
- 本地仿真：`go run ./cmd/sim -symbol ETHUSDC -ticks 10 -minSpread 0.0008 -baseSize 1`
- 回测：`go run ./cmd/backtest -config configs/config.yaml -symbols ETHUSDC:data/mids_sample.csv`
- Runner（干跑/联机）：
  - 干跑：`./scripts/run_runner.sh` 并设置 `DRY_RUN=true`
  - 联机：配置 `BINANCE_API_KEY/SECRET`、`BINANCE_REST_URL/BINANCE_WS_ENDPOINT`，参考 `docs/runner_overview.md`

## 三、配置与单位
- 统一价差为 Bps（1 bps = 0.01%）；`ASMM.MinSpreadBps/MinSpacingBps` 使用 Bps 表示；
- `risk` 阈值按“名义数量/净仓位/PnL 比例”统一；
- `symbols.<sym>.tickSize/stepSize/minNotional` 来源于交易所精度限制。

## 四、指标与日志（Prometheus + JSON 日志）
- 核心指标：`mm_runner_mid_price`、`mm_runner_spread`、`mm_runner_quote_interval_seconds`、`mm_runner_risk_state`、`mm_runner_orders_placed_total` 等；
- 新增指标：`mm_vpin_current`、`mm_volatility_regime`、`mm_adverse_selection_rate`、`mm_inventory_net`、`mm_reservation_price`、`mm_inventory_skew_bps`、`mm_adaptive_netmax`；
- 日志事件：`strategy_adjust`、`risk_state_change`、`order_update`、`depth_snapshot`、`listenkey_keepalive_*`。

## 五、代码入口与模块
- 市场：`market/`（`VolatilityCalculator`、`RegimeDetector`、`OrderBook`）
- 策略：`strategy/asmm/`（`ASMMStrategy.GenerateQuotes`、`VolatilitySpreadAdjuster`）
- 风控：`risk/`（`MultiGuard`、`Limits`、`LatencyGuard`、`PnLGuard`）
- 订单：`order/`（`Manager.Submit`、`Manager.Cancel`、`SymbolConstraints`）
- Runner：`sim/runner.go`（`Runner.OnTick` 差分下发/Reduce-only/降级）
- 网关：`gateway/`（REST/WS/ListenKey 客户端骨架）

## 六、协作约定
- 优先修复阻塞路径的“正确性与稳定性”问题；
- 每个阶段合并时需保证 `go test ./...` 通过；
- 指标名与事件名使用常量统一管理，避免文档与代码偏移；

更多细节见 `docs/TECH_SPEC.md` 与 `docs/ARCHITECTURE.md`。