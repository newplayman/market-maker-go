# 2025年11月24日实盘迭代记录

本文档为每日实盘调参-测试-修复循环的快速接力参考，确保下个工程师可以迅速理解当前的测试方式、可用数据、发现的问题与对应的处理。

## 一、测试/数据链路

1. **实盘触发脚本**  
   - 使用 `scripts/quick_test.sh 60` 启动，默认运行 60 秒；脚本内部会在 `DRAIN_BEFORE_SEC` 设定的时间前发送 `SIGINT`，触发 Drain 流程，并在 12 秒超时后发 `SIGTERM`。
   - 脚本结束后自动运行 `scripts/emergency_stop.sh`，通过 Rest Gateway 强制撤单 & 市价平仓，确保每轮测试后净仓和委托都归零。
   - 脚本会统计 `logs/net_value_ethusdc.csv` 的时间区间净值，结果追加至 `logs/net_value_runs_ethusdc.csv`。

2. **关键日志/指标**  
   - `/tmp/test_run*.log`：策略运行全量事件（`strategy_adjust`、`order_place`、`drain_mode_*` 等），重点观察 Drain 前后是否还在新建挂单。
   - `logs/net_value_ethusdc.csv`：每次 `equityRecorder.Record` 记录 mid/net/pnl/wallet/equity/fees，用于计算 ΔEquity、手续费、Cash 变动。
   - `logs/net_value_runs_ethusdc.csv`： quick test 汇总；当前结构（增补后）包括 `wallet_delta` 与 `fees_delta`，便于区分手续费 vs 现金/浮盈影响。

3. **辅助指标（可用 Grafana/metrics）**  
   - `runner_equity`、`runner_position`、`runner_drain_mode` 等 metrics 已暴露，可配合 Grafana 或 Prometheus 进一步查看趋势（若面板不可用，可直接用 `curl http://localhost:9200/metrics`）。

## 二、测试期间发现的问题

1. **Drain 操作结束后继续挂单**  
   - 日志 `/tmp/test_run18.log` 显示在 `drain_mode_complete` 之后仍有 `order_place` 与 `strategy_adjust` 输出，说明 Drain 执行后没有完全停单重新开仓，导致净值继续因手续费下降。
2. **净值持续下滑，既不是行情主导也不是一次性事件**  
   - `logs/net_value_runs_ethusdc.csv` 最近 15 次全部出现负 ΔEquity，仅最近一次 +0.00896，表明策略一直在“回撤 + 手续费”阶段。
3. **快速测试统计缺少手续费与现金变动参考**  
   - 原 `quick_test.sh` 只输出 ΔEquity/起终点/极值，无法区分手续费 vs 现金 vs 市场方向影响；也未同步净值汇总表头更新，引导下一个工程师更难判断损耗来源。

## 三、采取的修复措施

1. **Drain 逻辑强化（`sim/runner.go:181-194`）**  
   - 只要处于 Drain 模式，除非 `drainSatisfied(net)` 达到阈值，否则只允许朝零头寸方向挂单；达到阈值后彻底禁止买卖，避免 Drain 完成后再次开新仓。
   - 这样可以确保 Drain 阶段的“静默等待”与“委托撤销”真正生效，防止 net=0 后又回到双边挂单状态。

2. **quick_test 统计增强（`scripts/quick_test.sh:83-160`）**  
   - 解析 `logs/net_value_ethusdc.csv` 时带上 `wallet` 与 `fees`，计算 `wallet_delta` 与 `fees_delta`，并在输出与 CSV 汇总中新增对应列，方便判断亏损是手续费/滑点还是现金变动。
   - 增加了 `ensure_summary_header` 逻辑，可自动修复历史文件缺失表头的问题；输出中也加入“现金Δ/手续费Δ”提示，协助快速排查亏损来源。

3. **测试后的验证步骤**  
   - `go test ./...` 全面通过，确保 runner 与脚本的修改不会影响现有单元测试。
   - 使用新的 quick test 流程确保 logs/net_value_runs_ethusdc.csv 包含 wallet/fees 字段，后续每条记录均可直接对比手续费是否在正向或负向贡献净值变化。

## 四、短期建议（供下个工程师接手后着手）

1. 持续保持“跑→停→分析”循环，每轮控制在 1~10 分钟，若 ΔEquity 连续下滑即刻查日志号停机；测试前确认没有历史净仓或挂单。
2. 依据新输出的 `wallet_delta` 与 `fees_delta` 判断亏损结构：若手续费接近 ΔEquity，就调 `feeBuffer`/`minSpread`；若手续费占比低且 net 方向错配，则优先考虑调整 inventory pressure 或 reduce-only 策略。
3. 观察 `/tmp/test_run*.log` 中 Drain 的时间点，验证 `drainSatisfied` 期间是否仍有 maker 订单未撤；若 necessary，可进一步增加 Drain 期间对 `order.Manager.GetActiveOrders` 的检查，并强制 cancel。
4. 后续若 Grafana 面板、Prometheus 可用，建议引入 `runner_equity` + `runner_drain_mode`  连续对比，结合 logs 计算出的 ΔEquity/手续费趋势，避免过早认为行情主导亏损。
5. 若出现 net 非常小仍继续挂单，可考虑引入 “净仓梯度门限” 或 “Drain 模式只允许 IOC” 等进一步保守规则。

## 五、联系方式与环境

- VPS IP 已列入币安白名单，可直接进行 HTTPS/WSS 访问。
- API key/secret 记录在 `configs/config.yaml`，请勿泄露；若需要按环境变量注入可参考 `config/load.go` 现有的 `LoadWithEnvOverrides`。
- 如果未来需要更多监控面，请先确认 `deployments/grafana` 中的 JSON 可用（当前用户只通过日志判断，因此不是必需）。

如需继续测试，可立刻执行 `scripts/quick_test.sh 60`，在记录中新增加的 columns 可以帮助您区分手续费与现金变动。欢迎继续“跑→查→修→跑”的循环，直至出现排除行情影响的持续盈利。  

## 六、2025-11-24 09:30-09:38 新增调参记录

1. **Reduce-only 下单尺寸修复**  
   - `sim/runner.go` 现用 `computeReduceQty` 根据当前净仓位 + `SingleMax` 计算减仓数量，并对齐到 stepSize，避免仓位不足时继续提交 0.008 ETH 减仓单导致 -2022 拒绝。  
   - 当净仓接近 0 时，减少 `buyReduceOnly/sellReduceOnly` 标记，确保 Drain 阶段不会再触发新的减仓单。
2. **配置同步**  
   - 三份配置 (`configs/config.yaml`, `config_fixed.yaml`, `config_real_trading.yaml`) 的 `risk.singleMax` 提升至 0.024，允许一次性平掉全部净仓；`reduceOnlyMaxSlippagePct` 收紧至 0.00025，将 taker 减仓成本控制在 2.5 bps。
3. **测试结果**  
   - 新二进制 + 新配置后连续运行 3 轮 `./scripts/quick_test.sh 120`：  
     - Run#21 (09:26) ΔEquity -0.00852，钱包 -0.01672。  
     - Run#22 (09:29) ΔEquity -0.02944，钱包 -0.02944（主要亏损来自 0.016 ETH 减仓 IOC 失败后 fallback）。  
     - Run#23 (09:33) ΔEquity -0.00144，基本持平。  
     - Run#24 (09:36) ΔEquity -0.08126，因 reduce-only 多次 EXPIRED 后触发 0.016 ETH 市价减仓，手续费+滑点合计约 -0.062。  
4. **遗留问题 / 下一步**  
   - 虽然数量修复已生效，但仍可在 Drain 或净仓归零时看到 -2022 拒绝（net≈0 时 reduce 逻辑仍会尝试下单）；需要在 reduce block 中显式跳过 `abs(net) < stepSize` 的场景。  
   - 减仓 IOC 经常 `EXPIRED`，需要排查是否因为 `ReduceOnlyMaxSlippagePct` 仍高估、或 orderMgr 未及时刷新盘口（建议在 `planReduceOnlyPrice` 中落地深度快照以评估真实 slippage）。  
   - 建议下一轮聚焦于：  
     1. 在 reduce-only 未成交连续 2 次时直接走 `tryMarketReduce`（当前 `ReduceOnlyMarketTriggerPct=1%`，可以调至 0 或 0.1%）。  
     2. 增加日志输出（side/qty/price/net）定位 `EXPIRED` 原因，确认是否由仓位不足 / 深度空档引起。  
     3. 根据上述日志再评估 `reduceOnlyThreshold` 是否需要进一步上调，以减少 -0.016 ETH 级别的强减仓。

### 09:51-09:56 迭代补充

1. **逻辑优化**
   - `sim/runner.go`：当 Drain 模式已满足净仓、或 `abs(net)` < 0.5*stepSize 时不再触发 reduce-only，并输出 `reduce_skip_small_net`，避免 `net≈0` 却仍遭 -2022 拒单。
   - `reduceOnlyThreshold` 提升至 2.5（≈0.02 ETH），降低“刚成交两笔即强减仓”的频率，减少 taker 手续费。
2. **实盘验证**
   - Run#25 (09:51) ΔEquity -0.05687（旧阈值，作为对照）。
   - Run#26 (09:54) ΔEquity **+0.01004**，钱包 +0.00472，手续费 0。全程未再出现 drain 阶段的 reduce-only 报错。
3. **后续观察**
   - 新阈值下 net 在 ±0.02 ETH 内波动，可明显减少减仓 IOC；若再出现 net 长时间贴边，可考虑将 inventory skew 强度调高。
   - `logs/net_value_runs_ethusdc.csv` 已追加 Run#26 记录，可持续监控盈亏趋势。

### 10:01-10:11 强化 reduce-only fallback

1. **强制市价兜底**
   - `sim/runner.go` 的 `tryMarketReduce` 新增 `force` 逻辑：当 `reduceFailCount` 连续≥3 次触发 fallback 时，即使未达到 `ReduceOnlyMarketTriggerPct` 也会直接提交 taker，以免在行情连续单边时反复挂 IOC 却不成交。
   - `planReduceOnlyPrice` 现在透传 fallback 标记，确保同一 tick 内的所有 reduce-only 行为使用一致的滑点/侵略性参数，避免多次调用 `shouldFallbackReduceOnly()` 造成状态抖动。
2. **实测**
   - Run#27（10:01~10:04，修改前）Δ=-0.00609，手续费 0.01233，亏损主要来自多次 IOC taker。
   - Run#28（10:08~10:11，修改后）Δ=-0.01188，手续费=0，亏损来自行情在测试尾段冲高回落；drain 阶段仅触发 1 次强制市价，未再出现 “ReduceOnly Order is rejected”。
3. **下一步**
   - 若未来仍出现 fallback 频繁触发，可考虑提高触发阈值（如失败≥4次）或动态放宽 `reduceOnlyMaxSlippagePct`。
   - 建议在更长（≥10min）测试中观察 `wallet_delta`/`fees_delta` 占比，必要时配合 inventory skew 或 take-profit 参数继续减小均值回撤。
