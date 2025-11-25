# 技术规范（Tech Spec）

统一编码规范、接口契约、配置单位与监控约定，作为开发与联调依据。

## 1. 语言与版本
- Go 版本：`go 1.22+`（请在 `go.mod` 修正为实际环境版本）
- 构建与测试：`go build` / `go test ./...`

## 2. 代码风格与结构
- 命名统一：价差/间距使用 Bps（`MinSpreadBps`、`MinSpacingBps`）；百分比明确 `%` 含义。
- 目录边界：
  - 市场：`market/`（`VolatilityCalculator`、`RegimeDetector`、`OrderBook`、`Snapshot`）
  - 策略：`strategy/asmm/`（`ASMMStrategy.GenerateQuotes`、`VolatilitySpreadAdjuster`、配置）
  - 风控：`risk/`（`MultiGuard`、`Limits`、`LatencyGuard`、`PnLGuard`、`adaptive.go`）
  - 订单：`order/`（`Manager.Submit`、`Manager.Cancel`、`SymbolConstraints.Validate`）
  - Runner：`sim/runner.go`（差分下发/闪撤抑制/降级逻辑）
  - 网关：`gateway/`（REST/WS 客户端）

## 3. 策略接口契约（ASMM）
- 输入：`market.Snapshot{ Mid, BestBid/Ask, RealizedVol, Regime, Imbalance, VPIN/Toxic, StalenessMs }`；库存：`inventory.NetExposure`。
- 输出：`[]asmm.Quote{ Price, Size, Side(Bid/Ask), ReduceOnly }`。
- 行为：
  - `reservationPrice` = mid + inventory skew；`halfSpread` = `GetHalfSpread(vol, regime)`；生成 1..N 档价位（按 `MinSpacingBps`）。
  - `ReduceOnly`：处于软/硬库存限制或 `Toxic=true`，只生成减仓腿或冷却。

## 4. Runner 交互契约
- 价位差分：维护每档 `lastOrderID/price/placedAt`，使用 `shouldReplacePassive` 与 `DynamicRestDuration/DynamicThresholdTicks` 控制重挂。
- 降级：`PostOnly` 拒单 → 普通限价；在 `Reduce-only` 场景下可转 `IOC`。
- 限速：遵守交易所速率，避免“全撤全挂”。

## 5. 风控与事后学习
- 组合守卫：`risk.MultiGuard.PreOrder` 顺序执行，命中即拒单，返回原因码；
- 自适应：`risk/adaptive.go` 结合 `posttrade.Analyzer.Stats()` 与市场状态，动态收敛 `NetMax/BaseSize/MinSpreadBps`。

## 6. 配置约定（`configs/config.yaml`）
- `symbols.<sym>.strategy.type`：`grid` 或 `asmm`；ASMM 参数使用 Bps 单位；
- `symbols.<sym>.risk`：`singleMax/dailyMax/netMax/latencyMs/pnlMin/pnlMax/reduceOnlyThreshold/stopLoss/haltSeconds/shockPct`；
- 精度限制：`tickSize/stepSize/minQty/maxQty/minNotional`。

## 7. 指标名（Prometheus）
- Runner 核心：`mm_runner_mid_price`、`mm_runner_spread`、`mm_runner_quote_interval_seconds`、`mm_runner_risk_state`、`mm_runner_orders_placed_total`、`mm_runner_rest_requests_total/mm_runner_rest_errors_total/mm_runner_rest_latency_seconds`、`mm_runner_ws_connects_total/mm_runner_ws_failures_total`。
- 策略/市场：`mm_volatility_regime`、`mm_vpin_current`、`mm_inventory_net`、`mm_reservation_price`、`mm_inventory_skew_bps`、`mm_adaptive_netmax`、`mm_adverse_selection_rate`。

## 8. 日志事件（JSON）
- `strategy_adjust`：`symbol, mid, spread, spreadRatio, volFactor, inventoryFactor, intervalMs, net, reduceOnly, depthFillPrice, depthFillAvailable, depthSlippage`；
- `risk_state_change`：`symbol, state, reason?`；
- `order_update`：`symbol, status, clientOrderId, orderId, lastQty, lastPrice, realizedPnL`；
- `depth_snapshot`/`depth_refresh_error`、`listenkey_keepalive_*` 等。

## 9. 并发与生命周期
- 引擎：`internal/engine.TradingEngine` 通道在 `Stop` 后重建或禁止复启；
- Runner：以 WS 更新为主，REST 仅在陈旧时回退；按 `dynamicInterval` 控制频率。

## 10. 验收与测试
- 所有模块需具备表驱动单测；
- 回测/仿真用例覆盖平稳/单边/剧烈；
- 监控面板可复现策略行为与风险状态变化。
