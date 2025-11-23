

告警系统是整个做市商系统的“神经反射层”。  
它必须在异常出现的**数秒内**通知操作人员，并在必要时**自动执行保护动作（panic-stop / 撤单 / 平仓）**。

此文档将提供：

- 告警体系结构
    
- 告警分类
    
- 告警优先级
    
- 告警合并策略
    
- 通知渠道
    
- 自动恢复机制
    
- 与监控、风控、日志联动机制
    
- 完整可运行的告警框架
    

---

# **目录**

1. 模块目标
    
2. 告警核心设计原则
    
3. 告警的三层结构
    
4. 告警类别
    
5. 告警优先级（P0/P1/P2）
    
6. 告警触发条件（完整清单）
    
7. 告警节流（Rate Limiting）与合并
    
8. 告警路由（Routing）
    
9. 告警渠道
    
10. 自动执行动作（Auto-Recover Actions）
    
11. 告警消息格式
    
12. 告警事件存储（Alert Log）
    
13. WebUI 显示与管理
    
14. 扩展能力
    

---

# **1. 模块目标**

Alerting 模块的核心目标：

- 在系统异常发生时**最短时间通知**
    
- 减少误报，避免“警报疲劳”
    
- 支持智能告警（合并、抑制、升级）
    
- 支持自动采取保护行动
    
- 支持 WebUI 告警管理面板
    

结合监控（Monitoring）与日志系统（Logging），形成完整的：

> **“发现问题 → 通知 → 自动处理 → 记录 → 人工介入”**闭环。

---

# **2. 告警核心设计原则**

1. **及时性**：关键告警必须在 1–3 秒通知
    
2. **准确性**：减少误报、重复报
    
3. **可读性**：告警必须简洁但包含关键信息
    
4. **可追踪性**：所有告警必须写入日志/数据库
    
5. **可恢复性**：关键告警必须联动自动风险动作
    
6. **可路由性**：不同告警 → 不同通知渠道
    

---

# **3. 告警体系的三层结构**

Alerting 系统分三层：

```
1. Source（数据来源）
   - Monitoring metrics
   - RiskControl
   - OMS events
   - MarketData
   - Account data

2. Evaluator（规则引擎）
   - 阈值判断
   - 时间窗口判断
   - 波动检测
   - 组合规则

3. Dispatcher（告警触发）
   - 合并/压缩
   - 路由到不同渠道
   - 自动执行动作
   - 写入审计日志
```

三层可以分开独立开发，也可以由 Alertmanager 实现。

---

# **4. 告警类别**

告警分类如下：

### A. **系统类告警（System Alerts）**

- CPU/Mem 超限
    
- goroutine 爆炸
    
- WS 掉线
    
- REST 不可用
    

### B. **市场数据类（MarketData Alerts）**

- 深度延迟
    
- trade lag
    
- WS 重连
    
- 波动率暴涨
    

### C. **策略行为类（Strategy Alerts）**

- OnTick 失效
    
- 策略停止执行
    
- 参数异常
    

### D. **OMS / 交易类告警（Execution Alerts）**

- 订单拒绝
    
- 撤单失败
    
- API 错误
    
- 下单延迟过高
    

### E. **风险类告警（Risk Alerts）**（最重要）

- inventory 超限
    
- 日内亏损过大
    
- 强平风险接近
    
- panicstop 触发
    

### F. **账户类告警（Account Alerts）**

- balance 异常
    
- funding rate 异常
    
- 权益骤降
    

### G. **系统管理类（Admin/Audit Alerts）**

- 参数修改
    
- 用户登录
    
- API key 更改
    
- 手动平仓
    

---

# **5. 告警优先级（P0 / P1 / P2）**

专业量化机构通常分三级：

---

## **P0 – 致命告警（必须 3 秒内通知）**

触发立即执行保护动作：

- PanicStop
    
- 强平风险 > 80%
    
- API 全面失败（REST 不可用）
    
- WS 中断（无行情）超过 3 秒
    
- inventory 超限 2 倍
    
- 策略停止运行
    
- OMS 下单失败率 > 30%
    

通知方式：  
**Telegram + 微信 + 声音报警**（如有本地终端）  
**必须发送审计日志记录**

---

## **P1 – 高优先级告警（1 分钟内通知）**

不会立即导致亏损，但需要关注：

- 波动率暴涨（> 3× 平均）
    
- cancel rate 过高
    
- API error rate 上升
    
- WS 重连过于频繁（> 5/min）
    
- 策略频繁对冲
    
- latency 上升超过阈值
    

通知方式：  
Telegram + WebUI 红色提示

---

## **P2 – 普通告警（仅记录 / 每小时合并一次）**

不影响交易安全，但需关注：

- balance 变化
    
- funding 费率变化
    
- 参数更新
    
- 程序重启
    
- 用户登录/退出
    

通知方式：  
WebUI / 日志

---

# **6. 告警触发条件（完整清单）**

下面为你提供完整可落地的“触发清单”。

---

## **A. 系统类**

|事件|条件|
|---|---|
|CPUHigh|CPU > 80% 持续 10 秒|
|MemHigh|内存占用 > 80%|
|GCRateHigh|GC pause > 20ms（HFT 关键）|
|ProcessRestart|进程重启|

---

## **B. MarketData 类**

|事件|条件|
|---|---|
|DepthLagHigh|depth_lag_ms > 200ms|
|WSReconnectSpike|1 分钟内重连 > 3 次|
|NoTickReceived|1 秒内无 trade/depth|

---

## **C. Strategy 类**

|事件|条件|
|---|---|
|StrategyTickStop|OnTick > 300ms 未执行|
|GridSpreadTooWide|spread 超过 99% 的历史值|
|HedgeLoop|10 秒内自动对冲多次|

---

## **D. OMS / Execution 类**

|事件|条件|
|---|---|
|OrderReject|拒单发生|
|CancelFail|撤单失败|
|LatencyHigh|下单延迟 > 10–20ms|
|APIRateLimit|429 错误频繁出现|
|OrderSendFail|订单发送失败|

---

## **E. Risk 类（最严重）**

|事件|条件|
|---|---|
|InventoryOverLimit|EffectiveInventory > MaxInventory|
|DailyLossHigh|当日亏损 > 阈值|
|PanicStopTriggered|panicstop 执行|
|RiskRateHigh|强平风险 rate > 80%|

---

## **F. Account 类**

|事件|条件|
|---|---|
|EquityDropFast|equity 短时间急剧下降|
|FundingSpike|funding > 0.1%（极端高）|

---

## **G. 管理类**

|事件|条件|
|---|---|
|UserLogin|用户登录|
|ConfigChange|配置修改|
|RiskConfigChange|风控参数修改|
|ApiKeyChange|修改交易所 API Key|
|ManualClose|用户手动平仓|

---

# **7. 告警节流（Rate Limiting）与合并**

防止短时间重复发通知（打爆手机）。

示例：

---

## 情形：订单拒绝多次

```
1 分钟内最多发 1 次告警
```

## 情形：WS 重连

```
合并成一条："WS reconnected 5 times in last minute"
```

## 情形：inventory 持续超限

```
首次触发立即告警  
随后 30 秒内不重复  
```

## 情形：大量 exception/log 触发

```
按类别每 10 秒合并一次
```

---

# **8. 告警路由（Routing）**

不同告警发送不同渠道：

|告警等级|渠道|
|---|---|
|P0|Telegram + 微信 + Email + WebUI + 声音报警|
|P1|Telegram + WebUI|
|P2|WebUI + 保存日志|

可按符号（BTC/ETH）区分渠道。

---

# **9. 告警渠道**

推荐渠道：

### 1. Telegram（最常用、最快速）

接近实时（< 1s）。

### 2. 企业微信 / 飞书

稳定可靠，适合团队使用。

### 3. Email

作为备份渠道。

### 4. WebUI 通知

可弹出红色提示框。

### 5. 声音警报

若你在本地运行，有助于立即注意到风险。

---

# **10. 自动执行动作（Auto-Recover Actions）**

某些告警必须触发自动处理程序：

---

## **PanicStopTriggered → 自动执行**

```
取消全部订单  
平掉持仓  
停止策略  
发送 P0 告警
```

---

## **InventoryOverLimit → 自动对冲**

```
根据 SuggestHedge 下市价单平衡持仓
```

---

## **APIFailure → 自动降频**

```
降低下单频率
减少撤单
```

---

## **WSDisconnect → 自动保护性撤单**

```
立即撤单  
暂停策略  
等待 WS 恢复
```

---

# **11. 告警消息格式**

统一 JSON 格式，便于审计记录：

```json
{
  "ts": "2025-11-19T13:02:11.992Z",
  "level": "P0",
  "module": "Risk",
  "event": "PanicStopTriggered",
  "symbol": "BTCUSDC",
  "value": 12345.67,
  "threshold": 5000,
  "action": "cancel_all + close_positions",
  "ip": "192.168.3.5"
}
```

也可同时生成美观的文本：

```
🔥 P0 Critical Alert: Panic Stop Triggered
Symbol: BTCUSDC
Reason: DailyLossExceeded
Action: All orders canceled, position closed
Time: 13:02:11
```

---

# **12. 告警事件存储（Alert Log）**

存储方式：

- 写入 audit 日志
    
- 存入 SQLite/MySQL
    
- 推入 Kafka/NATS（可选）
    

字段：

- id
    
- timestamp
    
- level
    
- module
    
- event
    
- message
    
- meta（JSON）
    
- action
    
- resolved（是否解决）
    

---

# **13. WebUI 中的告警功能**

WebUI 需包含：

### 1. 告警列表

按时间倒序排列。

### 2. 告警详情

查看 JSON 格式详情。

### 3. 告警过滤

按 symbol、event、level。

### 4. 历史告警分析

图表：

- 每小时告警数量
    
- 告警分类占比
    
- 风控触发统计
    

### 5. 告警确认机制

“我已处理”按钮。

---

# **14. 扩展能力**

未来可实现：

- AI 告警分类（智能区分误报）
    
- AI 告警原因解释
    
- 告警自愈（自动恢复系统）
    
- 全天候巡检机器人
    
- 性能异常预测
    
- 提前预警（early warning）
    

这些将使系统逐渐具备“智能运维（AIOps）”能力。

---

# 下一份文档

按顺序，下一份是做市系统的最后基础组件之一：

> **《Deployment & Runtime – 部署与运行环境设计文档（v1.0）》**

它将涵盖：

- 部署架构
    
- 多实例运行
    
- 高可用
    
- 灾备
    
- 进程管理
    
- 日志、监控、告警集成
    
- 如何安全运行你的做市系统
    

我可以继续写下一份吗？