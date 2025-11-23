# Runner 运行机制概述（交付给后来者的操作手册）

本文汇总当前版本 Runner 的代码逻辑与关键参数，帮助接手者在不阅读全部源码的情况下理解“行情→策略→风控→下单”链路。

---

## 1. 模块拓扑

1. **cmd/runner**：进程入口  
   - 读取 `configs/config.yaml`，将策略参数与风控阈值灌入 Runner。  
   - 建立 Binance REST/WS 连接，持续刷新 OrderBook 与用户数据。  
   - 通过 Prometheus 暴露指标，日志采用 JSON（`logs/` 与 `/var/log/market-maker`）。

2. **sim/runner.Runner**：策略核心  
   - 接收 mid/OrderBook/Inventory 状态 → 生成报价。  
   - 管理静态/动态挂单、reduce-only、stop loss、shock halt。  
   - 经 `order.Manager` 转化为 REST 下单请求。

3. **risk** 组合  
   - LimitGuard：单笔 / 日累计 / 净仓限制。  
   - PnLGuard：浮盈亏超阈值时拒单。  
   - LatencyGuard：限制下单频率。  
   - Reduce-only 触发后，Runner 会持续向风控模块上报 `risk_event`。

4. **monitor**  
   - Prometheus 端口 9100，采集 `runner_*` 指标。  
   - Grafana 仪表盘在 `monitor/runner_dashboard.json`、`monitor/trader_dashboard.json`。

---

## 2. 参数映射关系

| 配置字段 | Runner 行为 | 备注 |
| --- | --- | --- |
| `strategy.minSpread` | 设定 `BaseSpread` | spread 越大，单笔预期收益越高但成交概率下降。 |
| `strategy.baseSize` | 影响订单数量、reduce-only 阈值、静态/动态拆分 | 建议结合账户保证金与 Binance 最小下单量。 |
| `strategy.staticFraction/staticTicks/staticRestMs` | 控制静态挂单保留 | 静态挂单永远以 PostOnly 提交，仅当偏离过大或休眠到期才撤单。 |
| `strategy.dynamicRestMs/dynamicRestTicks` | 控制动态腿刷新频率 | 解决“每 tick 即撤单”的问题，提升 maker 成交占比。 |
| `strategy.takeProfitPct` | 触发锁盈 | 当浮盈达到该比例时，将 reduce-only 挂单拉近 mid。 |
| `risk.reduceOnlyThreshold` | 进入 reduce-only 状态 | 单位为 `baseSize` 的倍数，例如 8 表示净仓达到 8×baseSize。 |
| `risk.reduceOnlyMaxSlippagePct` / `reduceOnlyMarketTriggerPct` | 减仓 aggressiveness | 超阈值后可直接以 IOC/市价击穿盘口。 |
| `risk.stopLoss/haltSeconds/shockPct` | 整体风控 | 达到阈值后 Runner 会撤单、触发暂停并记录 `risk_event`。 |

---

## 3. 报价生命周期

1. **数据采集**  
   - Websocket 深度 → `market.OrderBook`。  
   - Binance 用户流 → 成交/撤单回报，驱动 `inventory.Tracker`。

2. **OnTick 流程**  
   1. 计算动态 spread、inventory skew、take-profit 调整。  
   2. 将挂单拆分为“动态腿 + 静态腿”：  
      - 静态腿调用 `manageStaticOrders`，遵循 ticks/rest 约束。  
      - 动态腿在 `shouldReplacePassive` 判定下，只有偏差或休眠到期才撤单。  
   3. risk guard 检查 → submit order。  
   4. 若进入 reduce-only，则只留减仓方向的挂单，必要时触发 `tryMarketReduce`。

3. **订单回报**  
   - `order.Manager` 维护最后一次提交的 `lastBid/lastAsk`，用于决定是否需要取消上一轮。  
   - Binance 反馈 `-5022`（Post Only 被拒）时，Runner 会 fallback 为普通限价并记录 cooldown。

4. **日志/监控**  
   - `strategy_adjust`：记录每轮报价的 mid、spread、inventoryFactor、reduceOnly 状态。  
   - `risk_event`：风控状态迁移（normal/reduce_only/halted）。  
   - Prometheus: `runner_spread`, `runner_quote_interval_seconds`, `runner_risk_state` 等。

---

## 4. 新增注释汇总

为便于阅读，关键函数/结构已在 `sim/runner.go`、`config/load.go`、`cmd/runner/main.go` 注释：

| 文件 | 说明 |
| --- | --- |
| `sim/runner.go` | `Runner` 结构、`OnTick`、`manageStaticOrders`、`ensureStaticOrder`、`shouldReplacePassive` 等新增中文注释，解释静态/动态腿如何互补、何时撤单。 |
| `config/load.go` | `StrategyParams` 字段逐一附加注释，说明各参数作用与取值建议。 |
| `cmd/runner/main.go` | 描述 `staticRest/dynamicRest` 如何转化为 Runner 的 `time.Duration`。 |

---

## 5. 接手者 checklist

1. **调参顺序**  
   1. 核对账户保证金 & Binance 限制 → 设定 `baseSize` / `reduceOnlyThreshold`。  
   2. 根据目标盈利空间设置 `minSpread` 与 `takeProfitPct`。  
   3. 视波动性调整 `dynamicRestMs/dynamicRestTicks`，必要时配合提升 `quoteInterval`。

2. **上线步骤**  
   - Dry-run ≥ 30 分钟（监控 `runner_rest_errors_total=0`、`risk_rejects_total=0`）。  
   - 小额实盘：密切关注 `/var/log/market-maker/runner.log` 中的 `strategy_adjust` 与 `risk_event`。  
   - 有任何 `quote_error` 或 `risk_event` 异常时，使用 `scripts/emergency_stop.sh` 停止 → `scripts/cancel_all.sh` 清挂单。

3. **调试定位**  
   - **挂单缺失**：查看 `reduce_only net=...` 日志是否卡在风控状态。  
   - **频繁 taker**：检查 `shouldReplacePassive` 是否被触发，可增大 `dynamicRestMs`。  
   - **浮盈不平仓**：确认 `takeProfitPct` 与 `reduceOnlyMarketTriggerPct`，以及 `runner_errors.log` 是否记录 `pnl too ...`。

---

若需要更细的流程图，可在 `docs/strategy_risk_enhancement.md` 基础上继续扩展。本文件将随着策略/风控迭代持续更新。欢迎在改动后同步补充。 
