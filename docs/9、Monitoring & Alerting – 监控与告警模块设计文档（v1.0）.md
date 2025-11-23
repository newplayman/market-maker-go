

做市系统是 **持续实时运行** 的复杂系统，一旦出现问题可能会在数秒内造成大额亏损。因此监控与告警体系必须：

- 全覆盖
    
- 低延迟
    
- 高可靠
    
- 可观测
    
- 易排查
    
- 可自动触发 panic stop
    

本设计文档提供一个接近专业做市商/量化机构的完整监控系统框架，你可以直接用于实盘部署。

---

# **目录**

1. 监控系统目标
    
2. 必须监控的核心领域（六大类）
    
3. 指标（Metrics）列表
    
4. 日志（Logs）体系
    
5. 事件（Events）体系
    
6. 告警（Alerts）体系
    
7. 技术架构
    
8. 监控面板（Dashboard）建议布局
    
9. 定时巡检流程
    
10. 故障场景与处理建议
    
11. 扩展能力
    

---

# **1. 监控系统目标**

Monitoring & Alerting 的最终目标：

- **监控系统健康**：WS、REST、策略、OMS、风控、库存、资金
    
- **监控交易行为**：挂单、撤单、成交、延迟、填单率
    
- **监控风险暴露**：inventory、PnL、回撤、波动率
    
- **监控外部系统**：交易所状态、网络延迟
    
- **发现异常 → 立刻告警 → 自动处理**
    

一句话：

> 让你的做市系统出现问题时，你能在 3 秒内知道。

---

# **2. 必须监控的六大类指标体系**

做市系统监控必须覆盖六大领域：

|类别|内容|
|---|---|
|**1）系统运行监控**|CPU、内存、goroutine、GC、进程重启|
|**2）行情监控**|WS 延迟、重连次数、depth 频率、trade 频率|
|**3）交易执行监控（OMS）**|下单耗时、撤单耗时、拒单率、重试次数|
|**4）策略行为监控**|挂单数量、撤单数量、网格层数、趋势判断|
|**5）风险控制监控**|inventory、回撤、uPnL、vol 超限触发|
|**6）账户 / 资金安全监控**|balance、equity、强平风险、资金费率|

整个系统必须保证这六类监控全部准确。

---

# **3. 指标（Metrics）列表**

以下指标采用 Prometheus 格式（推荐），能在 Grafana 展示。

---

## **3.1 系统级（System Metrics）**

|指标|说明|
|---|---|
|system_cpu_usage|CPU 百分比|
|system_memory_usage|内存占比|
|process_uptime_seconds|进程存活时间|
|goroutine_count|goroutine 数量|
|gc_pause_ms|Go GC 延迟|

---

## **3.2 行情监控（MarketData Service）**

|指标|说明|
|---|---|
|mds_depth_updates|每秒 depth 更新次数|
|mds_trade_updates|每秒 trade 更新次数|
|mds_depth_lag_ms|depth 延迟（最关键）|
|mds_ws_reconnect_total|WS 重连次数|
|mds_volatility|当前波动率|
|mds_imbalance|盘口不平衡|
|mds_trend|短期价格趋势|

**重点监控：**

- depth lag 超过 200ms 说明网络或系统卡顿
    
- ws_reconnect_total 上升说明行情质量下降，策略应保护性撤单
    

---

## **3.3 OMS/交易执行监控**

|指标|说明|
|---|---|
|oms_order_send_latency_ms|下单耗时（REST round trip）|
|oms_cancel_send_latency_ms|撤单耗时|
|oms_orders_per_second|每秒下单|
|oms_cancels_per_second|每秒撤单|
|oms_reject_total|订单拒绝次数|
|oms_api_error_total|API 错误次数（429、5xx）|
|oms_retry_total|重试次数|
|oms_local_order_count|当前挂单数量|

**这是做市系统最关键监控之一。**

---

## **3.4 策略行为监控（Strategy）**

|指标|说明|
|---|---|
|strategy_tick_rate|OnTick 每秒执行次数|
|strategy_intent_count|每次 tick 产生的 intents 数量|
|strategy_grid_levels|当前网格层数（phase2）|
|strategy_spread|计算的目标 spread|
|strategy_bid_price|挂单价格（可取平均）|
|strategy_ask_price|挂单价格|

---

## **3.5 风控监控（RiskControl）**

|指标|说明|
|---|---|
|risk_inventory|EffectiveInventory|
|risk_inventory_overlimit_total|超限次数|
|risk_daily_loss|当日损益|
|risk_drawdown|当前回撤|
|risk_panic_stop_total|PanicStop 触发次数|
|risk_cancel_rate|撤单比率|
|risk_volatility_panic|波动率触发 panic|

风控是系统安全的第二层（第一层是 OMS）。

---

## **3.6 账户监控（Account / Capital）**

|指标|说明|
|---|---|
|account_balance|账户余额|
|account_equity|账户权益|
|account_upnl|未实现盈亏|
|account_risk_rate|强平风险（维持保证金占比）|
|funding_rate|最新资金费率|

非常关键：  
account_risk_rate > 0.8 表示接近强平，系统应暂停策略。

---

# **4. 日志体系（Logs）**

建议日志分级：

- **INFO**：普通状态信息
    
- **WARN**：异常但可运行
    
- **ERROR**：错误（需要人工排查）
    
- **CRITICAL**：必须立即停止交易
    

Log 必须记录：

1. 下单/撤单事件
    
2. 订单状态变化
    
3. WS 断线/重连
    
4. 风控拒单原因
    
5. PanicStop 原因
    
6. 延迟超标
    
7. Account 信息变化
    
8. 策略决策（可选）
    

可以将关键日志保存到数据库用于复盘。

---

# **5. 事件体系（Events）**

Monitoring 系统应定义关键事件：

```
OrderRejected
OrderError
HighLatency
WSReconnected
PanicStopTriggered
InventoryOverLimit
RiskLimitTriggered
APIError
CancelRateHigh
BalanceLow
```

每个事件都应：

- 记录日志
    
- 推送告警（微信/短信/Telegram）
    

---

# **6. 告警体系（Alerting）**

告警必须分三级：

---

## **级别 1：Critical（立即报警）**

以下情况必须**在 3 秒内**告警：

- PanicStop 触发
    
- 强平风险 > 80%
    
- API error rate > 5%
    
- 持仓超过 MaxInventory 2 倍
    
- WS 延迟 > 500ms
    
- 连续 3 次订单拒绝
    
- 下单失败或撤单失败次数 > 阈值
    
- Equity 下跌 > 10%（日）
    

报警渠道：

- 微信企业号
    
- Telegram 机器人
    
- 邮件
    
- 声音提醒（若你有本地监控系统）
    

---

## **级别 2：Warning（1 分钟内通知）**

- CancelRate 高
    
- Volatility 超过 3 倍阈值
    
- API retry 频繁
    
- OMS 延迟升高
    
- 网络丢包
    
- WS 重连频率升高
    

---

## **级别 3：Info（只记录）**

- 程序重启
    
- 参数热更新
    
- 策略切换
    
- 账户余额更新
    

---

# **7. 技术架构**

推荐全套采用：

- **Prometheus（采集）**
    
- **Grafana（可视化）**
    
- **Alertmanager（告警）**
    

你的系统结构如下：

```
Strategy / OMS / Inventory / Risk
        ↓ expose /metrics
Prometheus <-- scrape
        ↓ alert rules
Alertmanager --> Telegram/Email/WeChat
Grafana <-- dashboards
```

延迟低，可靠性高，是专业量化团队标配。

---

# **8. Dashboard（监控面板）设计建议**

在 Grafana 新建“轻量做市商系统仪表盘”，内容：

---

## **页面 1：系统状态**

- CPU / 内存
    
- WS 延迟
    
- depth updates
    
- trade updates
    
- 进程存活时间
    

---

## **页面 2：订单执行**

- 下单延迟
    
- 撤单延迟
    
- 每秒下单/撤单
    
- API 错误率
    
- Reject 次数
    
- diff 行为统计（策略→OMS）
    

---

## **页面 3：策略状态**

- 当前网格层数
    
- spread / mid price
    
- imbalance / volatility
    
- bid/ask quote positions
    

---

## **页面 4：风险管理**

- EffectiveInventory
    
- MaxInventory
    
- uPnL（未实现）
    
- Daily PnL
    
- Drawdown
    
- PanicStop 触发记录
    

---

## **页面 5：账户资金**

- 余额
    
- equity
    
- risk rate（强平风险）
    
- funding 费率
    
- 仓位变化
    

---

# **9. 定时巡检流程（建议）**

每天自动巡检：

- 各模块 CPU > 50%?
    
- WS 延迟 > 200ms?
    
- API error > 1%?
    
- CancelRate > 20%?
    
- Inventory 超限次数 > 0?
    
- PnL 是否异常？
    

并发送巡检报告。

---

# **10. 故障场景与应对**

### 场景 1：WS 不断掉线

→ 系统应自动 panic stop（撤单）  
→ 告警  
→ 人工检查网络/代理/VPS

### 场景 2：策略疯狂下单触发风控

→ OMS + RiskControl 限流  
→ 告警  
→ 暂停策略

### 场景 3：API错误（429）

→ 降低下单频率  
→ 告警  
→ 若持续则停止交易

### 场景 4：inventory 激增

→ 自动 hedge  
→ 告警

### 场景 5：PnL 急剧下降

→ PanicStop  
→ 平仓  
→ 报警

---

# **11. 扩展能力**

此监控框架为未来扩展做好准备：

- 多交易所监控
    
- HFT 级延迟监控（p99/p999）
    
- AI 策略运行分析（模型输出）
    
- 高频订单簿质量分析
    
- 延迟敏感性分析
    
- 自动化交易健康检查（health-checker bot）
    

你的监控体系已经接近专业做市商系统水平。

---

# 下一份文档

按顺序，下一份是：

> **《WebUI / Dashboard – 前端监控与管理界面设计文档（v1.0）》**

它用于：

- 可视化策略状态
    
- 查看挂单/撤单/成交
    
- 显示资金与风险
    
- 修改参数（结合 ConfigService）
    
- 控制策略启动/停止
    

我可以继续撰写这一份吗？