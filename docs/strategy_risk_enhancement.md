# 市场中性做市策略改进方案（V2）

本文档用于约束本轮迭代的策略/风控改动，确定模块边界及实现顺序，确保后续编码按模块逐步完成。

---

## 1. 目标
- 维持零仓位/轻仓状态，通过“快进快出”提升资金周转率。
- 在价格快速波动或偏离时主动调节价差与频率，防止“休眠”或无限加仓。
- 在持仓达到预警值时，优先执行减仓，必要时触发止损/暂停。
- 充分利用现有 WSS + Go 框架的高频优势，按盘口动态决策。

---

## 2. 模块划分
| 模块 | 本轮改动 |
| --- | --- |
| 策略 Strategy | 动态 spread / 频率、库存驱动的报价、插单逻辑 |
| 风控 Risk | 仓位阈值、浮盈亏止损、波动暂停、Reduce-only 行为 |
| 订单管理 OrderMgr | 维持“先撤后挂”逻辑，新增条件下的减仓 | 
| 数据/行情 | 使用 websocket depth / user data；Prometheus 监控新增指标 |

---

## 3. 策略改进
1. **动态 Spread 公式**
   - Spread = `baseSpread × (1 + 波动率因子 + 库存因子)`
   - 波动率：使用近 N 秒 mid 变动计算 (`abs(mid_t - mid_{t-1})/mid_{t-1}` > shockPct)
   - 库存因子：`inventoryFactor = k × (currentNet / netMax)`，调整买/卖价偏移，平衡仓位。

2. **刷新频率**
   - `quoteInterval` = baseInterval / (1 + volatilityFactor)。
   - 波动低 → 更快；波动高 → 放缓，减少无效撤单。

3. **盘口插单**
   - 读取买一/卖一价差与深度：当 `spread_盘口 > 最低阈值`，尝试在中间插入自己的价位。
   - 插单失败（被吃掉/滑价）时退回常规 spread；成功则提高成交率。
   - 当 Binance 返回 `code -5022`（Post Only 被拒）时，记录方向性失败次数并动态拉宽该方向的报价（增加 tick 偏移），直至市场恢复为止，保证盘口始终有 Maker 挂单。

4. **获利平仓/止损**
   - 对持仓（或挂单）跟踪成本价，当 mid-avgCost > X（或 < -X）时，主动把对应方向的挂单移到 mid 附近并执行平仓（甚至市价）。
   - Reduce-only 阶段新增盘口深度评估：在提交减仓单前读取 orderbook，优先以当前 `bestBid/bestAsk`（或 EstimateFillPrice 给出的档位）作为限价，并设为 `IOC reduce-only`，如果超出 `reduceOnlyMaxSlippagePct`，则对限价做截断或拆单，避免滑点把盈转亏；相关信息通过 `strategy_adjust` 事件中的 `depthFillPrice/depthFillAvailable/depthSlippage` 字段上报。
   - 当 `mid` 已处于浮盈区间（多头：`mid > cost`，空头：`mid < cost`）时，优先保持已有 reduce-only 挂单，不再每个报价周期撤单重挂，仅在偏离超过容忍阈值或滑点超标时才替换，保证获利单尽快成交。
   - 当本地盘口数据缺失或多次 `IOC` 返回 EXPIRED 时，会直接退化为 `mid ± reduceOnlyMaxSlippagePct` 的激进价位（并在 fallback 阶段自动放大该百分比），确保减仓单真正击穿盘口。

5. **参数补充**
   - `risk.reduceOnlyMaxSlippagePct`：允许的最大减仓滑点（相对于 mid），默认 0.2%（0.002）。
   - 当盘口深度不足（`depthFillAvailable < orderSize`）或预估滑点过大时，策略会将限价限制在该百分比内，并在日志/Prometheus 中记录 `risk_event` 供运维干预。
   - `risk.reduceOnlyMarketTriggerPct`：当浮盈率 ≥ 该值时，可直接触发市价/IOC 击穿来快速平仓，防止长时间滞留盈利仓位；默认 0 表示关闭。

6. **混合挂单（静态 + 动态）**
   - 动态挂单仍按上述公式每个报价周期刷新，保证对市场波动的敏捷响应。
   - 额外保留一部分“静态底仓”挂单，数量由 `strategy.staticFraction`（占 `baseSize` 的比例）决定，价位与当前 spread 同步，但不会在每个周期被强制撤单，仅当偏离超过 `strategy.staticTicks`（以 tick 为单位）或进入 reduce-only/暂停状态时撤销。
   - 静态挂单始终以 Post Only 方式提交，与动态挂单形成互补：动态部分保证策略在波动期拉开价差，静态部分提升成交概率，避免出现“频繁撤单但缺少真实成交”的情况。
   - 当币安返回 `code=-5022`（Post Only 被拒）时，策略会自动触发短暂的“降级模式”：该方向在若干毫秒内允许以普通限价（非 Post Only）挂单，同时 backoff maker 价位，成功成交后再切换回 maker 模式，避免因长时间拒单导致盘口无挂单。
   - 动态挂单在单次事件中也具备“即时回退”能力：若本次提交因 Post Only 被拒，Runner 会立刻以同一价格、非 Post Only 的形式再尝试一次，确保这一轮 tick 不会空仓。
   - REST Gateway 保留 Runner 传入的 Post Only 标志，不再强制改为 Maker，从而保证 fallback 真正能发出 Taker 订单。

---

## 4. 风控与应对策略
1. **净仓位阈值**
   - `reduceOnlyThreshold`: 当 `abs(net)` ≥ 阈值，策略转为 reduce-only：只允许挂减仓方向的单，并按 inventory skew 调整价位。
   - `netMax` 仍作为硬上限，触发后直接拒单 + 报警。

2. **浮盈亏止损**
   - `stopLoss`（负值）：当浮亏低于该值，立即：
     1. 撤掉所有挂单；
     2. 启动“紧急平仓”策略（可使用市价或逐步限价）；
     3. 暂停策略 `haltSeconds`，等待人工确认。

3. **波动暂停（Shock Halt）**
   - 当单次 mid 变动比上一时刻大于 `shockPct`，触发暂停：撤单 + `haltSeconds`，期间仅监控不下单。

4. **减仓流程**
   - 在 reduce-only 状态下：
     - 将买/卖价平移至 mid，或仅保留减仓方向的挂单。
     - 直到 `abs(net)` 回到阈值以下，再恢复正常策略。

---

## 5. 实施步骤（建议顺序）
1. **策略层**
   - 实现 inventory skew（将 buy/sell 价相对 mid 微调）。
   - 加入动态 spread / quoteInterval 计算。
   - 实现盘口插单 & 失败回退机制。

2. **风控层**
   - 扩展 Reduce-only 行为：达到阈值后（`reduceOnlyThreshold` 以“baseSize 的倍数”配置，例如 3=3×baseSize）只挂减仓单，必要时强制平仓。
   - 完善 stopLoss、shock halt 触发逻辑（撤单、暂停、报警）。

3. **监控与日志**
   - 在日志 & Prometheus 指标中增加 spread、volatility、risk state 等字段，便于追踪。

4. **Dry-run 验证 → 小额实盘**
   - 每一步调整先用 Dry-run 验证；通过后再小额实盘，监控日志/指标。
   - 确保日志中有 clear `risk_event`、`strategy_adjust` 等事件。

---

## 6. 开放问题
- 插单策略需要实时买一卖一数据，需确认现有 depth handler 的解析准确性。
- 止损/减仓可选用市价 or 限价 TWAP，需评估 Binance USDC-M 的 API 限制。
- 动态 spread / 频率公式需要根据真实波动调整系数，建议实盘数据积累后再 fine-tune。

以上方案若确认，可以按“策略 → 风控 → 监控”顺序逐项实现，并配合 Dry-run / 实盘回归验证。欢迎补充。 
