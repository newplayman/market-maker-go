# TODO（优先级与具体任务）

按“立即/短期/中期”分层，对应代码模块与交付标准。

## 立即（P0）
1. Runner 接入 ASMM 多档报价 ✅
   - 已完成：接入 `ASMMStrategy.GenerateQuotes` 并选取首档买/卖；合并策略侧 Reduce-only；基于 `OrderBook` 的陈旧度守卫；多档差分下发与状态持有；PostOnly 协作与降级；多档指标观测。
   - 变更：`sim/runner.go` 的 `Runner.OnTick` 当 `r.ASMMStrategy != nil` 时调用 `ASMMStrategy.GenerateQuotes`，按档位差分下发。
   - 要点：维护每档 `lastOrderID/price/placedAt`；使用 `shouldReplacePassive`/`postOnlyReady` 阈值；减少闪撤。
   - 验收：本地仿真/回测下"撤单频率下降、订单留存率提升"，`go test ./...` 通过。

2. 补齐 Imbalance → Regime ✅
   - 已完成：`market.Snapshot.Imbalance` 字段；Runner 基于 OrderBook 计算前3档不平衡；`ASMMStrategy` 使用 Imbalance 驱动 Regime 检测与 spread 调整。
   - 变更：`market/imbalance.go` 计算前 N 档不平衡；`market.Snapshot.Imbalance`；`ASMMStrategy` 在 `DetectRegime` 中使用 Imbalance。
   - 验收：单测覆盖不同盘面（平稳/单边/剧烈），Regime 判定随 Imbalance/vol 波动。

3. 简版 VPIN/Toxic 联动 ✅
   - 已完成：`market/toxicity.go` 实现 VPIN 计算器；`market.Snapshot.VPIN` 字段；`ASMMStrategy` 在高 VPIN 时加宽价差与 Reduce-only 限制；指标 `mm_vpin_current`。
   - 变更：`market/toxicity.go` 维护 volume buckets，输出 `Snapshot.VPIN/Toxic`；`ASMMStrategy` 在 `AvoidToxic` 下保守化（只减仓或加宽价差/短暂冷却）。
   - 验收：仿真数据触发高 VPIN 时策略档数/价差调整，并打点 `mm_vpin_current`。

4. 引擎生命周期修复 ✅
   - 已完成：`Start` 检测 `StateStopped` 时重建 `stopChan/doneChan`；`Stop` 幂等（多次调用不 panic）；支持完整复启。
   - 变更：`internal/engine/trading_engine.go` 在 `Start` 检查 `StateStopped` 时重建 `stopChan/doneChan`；`Stop` 幂等。
   - 验收：引擎支持“启动→停止→再次启动”不 panic；单测覆盖。

5. 指标/事件名常量化 + 单位统一 ✅
   - 已完成：在 `metrics/prometheus.go` 定义所有指标名常量（`MetricVPINCurrent` 等）；统一 `MinSpreadBps/MinSpacingBps` 为 Bps 单位；所有配置与策略使用 Bps。
   - 变更：统一 `MinSpreadBps/MinSpacingBps` 的 Bps 单位；在 `metrics/` 定义指标名常量；日志事件统一字段（见 TECH_SPEC）。
   - 验收：Prometheus/日志面稳定，名称不再漂移。

## 短期（P1）
6. PostTrade Analyzer + Adaptive Risk 闭环 ✅
   - 已完成：`posttrade/analyzer.go` 收集1s/5s后价格统计逆选率；`risk/adaptive.go` 自适应调整 NetMax/BaseSize/MinSpreadBps；`ASMMStrategy.SetAdaptiveRisk` 注入；`Runner.SetAdaptiveRisk/OnFill` 接入；周期性 Update；指标上报 `mm_adverse_selection_rate`。
   - 变更：`posttrade/analyzer.go` 收集 1s/5s 后 mid，统计 `AdverseSelectionRate`；`risk/adaptive.go` 收敛 `NetMax/BaseSize/MinSpreadBps`。
   - 验收：在回测/仿真中高逆选期策略自动变保守，恢复期逐步回到原设。

7. FillTracker + 高频撤单抑制 ✅
   - 已完成：`order/fill_tracker.go` 跟踪成交历史（滑动窗口），统计每分钟成交率；`Runner.EnableCancelSuppression` 启用，在高频成交时抑制 `cancelDynamicLeg` 撤单；`Runner.OnFill` 同时通知 PostTrade 和 FillTracker；默认阈值：每分钟5次成交或1分钟内成交3次。
   - 变更：`order/fill_tracker.go` 维护满动窗口成交，计算 `recentFillRate`；`Runner.shouldSuppressCancel` 在高频期跳过撤单。
   - 验收：仿真高成交期开启抑制，撤单次数降低，成交率提升；监控指标 `mm_dynamic_order_cancels_total` 下降。

8. Runner 指标与日志完善 ✅
   - 已完成：新增 `mm_fill_rate_current/mm_recent_fills_count/mm_cancel_suppression_active` 指标；`UpdateFillTrackerMetrics` 定期上报；`mm_reservation_price/mm_inventory_skew_bps/mm_adaptive_netmax` 已在 `UpdateStrategyMetrics` 中使用。
   - 变更：补齐 `strategy_adjust` 的字段；新增 `mm_reservation_price/mm_inventory_skew_bps/mm_adaptive_netmax`；完善错误日志分流。
   - 验收：Grafana 报表可复现策略行为与风险状态变化。

## 中期（P2）
9. 网关健壮化（REST/WS）✅
   - 已完成：`gateway/rest_middleware.go` REST 中间件（签名/限流/重试/错误分类）；`gateway/ws_reconnect.go` WebSocket 重连管理器（心跳/断线重连/指数退避）；错误类型分类（Auth/RateLimit/Server/Client/PostOnlyReject/InsufficientBalance）；重试策略（418/429 自动重试，Retry-After 尊重）。
   - 变更：REST 签名/限流/重试中间件；WS 心跳/断线重连/快照+增量重放；combined stream 解复用。
   - 验收：VPS 干跑/小额联机稳定运行，429/418 有效处理，用户流回报与 `order.Manager` 对齐。

10. 发行与部署流程 ✅
   - 已完成：完整的部署脚本（`scripts/deploy.sh`）支持本地/远程部署、自动备份、Systemd 服务配置；GitHub Actions CI 配置（`.github/workflows/ci.yml`）自动执行测试/构建/Lint；健康检查脚本（`scripts/health_check.sh`）；一键启动/停止/回滚脚本。
   - 变更：打包 Runner，完善 `scripts/`（启动/备份/回滚/健康检查）；CI 执行 `go test ./...`+构建产物。
   - 验收：单命令部署，监控与日志自动生效。

---

# 任务映射（Module → TODO）
- `strategy/asmm`：`ASMMStrategy.GenerateQuotes` 多档/Reduce-only；`VolatilitySpreadAdjuster`；Regime/VPIN 联动。
- `market/`：`VolatilityCalculator`、`RegimeDetector`、`OrderBook`（已有）；新增 `imbalance.go/toxicity.go`。
- `sim/runner.go`：差分下发、闪撤抑制、降级逻辑（PostOnly→普通/IOC）。
- `risk/`：`MultiGuard`（已有）；新增 `adaptive.go`（闭环）。
- `posttrade/`：`analyzer.go`（事后学习）。
- `metrics/`：统一指标名常量、注册新指标。
- `internal/engine`：生命周期修复、陈旧度守卫。
