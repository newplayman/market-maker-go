

运维（Operations）与维护（Maintenance）是一个做市商系统能够 **长期稳定、安全运行** 的关键。  
好的策略如果没有好的运维体系，依然会因为事故、疏忽、参数错误、版本回退不当而导致巨大损失。

本篇文档将提供一个专业量化团队使用的运维体系，包括：

- 日常巡检
    
- 日常维护
    
- 故障处理
    
- 版本发布流程
    
- 参数修改流程
    
- 紧急响应流程
    
- 事故复盘流程
    
- 持续监控和健康检查
    

这些内容将把你整个系统的完整度提升到 “可真正实盘运行 24/7” 的专业级别。

---

# **目录**

1. 模块目标
    
2. 运维体系设计理念
    
3. 日常巡检流程（Daily Routine）
    
4. 每小时轻量巡检（Hourly Checks）
    
5. 日常维护内容
    
6. 系统更新与上线流程（Release Pipeline）
    
7. 热更新参数流程（Param Rollout）
    
8. 紧急操作流程（Emergency Procedures）
    
9. 故障处理流程（Incident Handling）
    
10. 事故复盘流程（Postmortem）
    
11. 执行保障机制（Health Checks）
    
12. 配套工具（OpsKit）
    
13. 扩展能力
    

---

# **1. 模块目标**

Ops & Maintenance 的目标：

- 确保系统长期安全、稳定、可控
    
- 让策略“可靠运行”，而不是“希望它没出事”
    
- 错误发生时自动保护资金
    
- 提供清晰、可重复的运维流程
    
- 让事故可复盘、可修复、可避免再次发生
    

一句话：

> **让你的做市系统具备专业量化机构的运维能力：可靠、可控、不出事。**

---

# **2. 运维体系设计理念**

核心理念：

### ✔ 最小化手动操作

所有可自动完成的任务都自动完成。

### ✔ 降低出错概率

包括：

- 热更新参数限制
    
- 策略一致性检查
    
- 更新后自动风控验证
    
- 双确认（手动平仓等行为）
    

### ✔ 避免复杂

越简单越安全。  
系统越稳定，策略越能长期赚钱。

### ✔ 算法错误比人工错误少

所有关键操作必须走系统，而不是 SSH 命令行随意操作。

---

# **3. 日常巡检流程（Daily Routine）**

每天固定时间（例如早上 9:00）进行系统巡检：

你可以使用机器人自动生成巡检报告（推荐）。

巡检内容：

### **(1) 行情健康**

- WS 重连次数（早上打开系统的第一指标）
    
- depth lag 最大值（过去 12 小时）
    
- trade stream 中断情况
    

### **(2) 策略运行状态**

- OnTick 平均延迟
    
- 策略暂停次数
    
- 自动对冲次数
    
- 最大 spread
    
- 最大网格层数（phase2）
    

### **(3) 风控状态**

- inventory 轨迹（过去 12 小时）
    
- MaxInventory 是否触发
    
- 是否有 PanicStop
    
- 风险限额是否被触及
    

### **(4) OMS 状态**

- 下单延迟分布
    
- 撤单延迟分布
    
- 拒单率
    
- API 错误率
    

### **(5) 账户状态**

- balance
    
- equity
    
- funding 费用
    
- risk rate
    

### **(6) WebUI 日志与告警**

- 异常告警
    
- 操作记录
    
- 参数更改记录（从 audit log 获取）
    

所有字段自动生成一份巡检报告，比如：

```
=== Daily Quant Ops Report ===
Date: 2025-11-20

Market:
 - WS reconnects: 1
 - Depth lag: p99=85ms

Strategy:
 - OnTick average: 2.8ms
 - Hedge count: 2
 - Max spread: 4 ticks

Risk:
 - Inventory peaks: +0.32/-0.28
 - PanicStops: 0

OMS:
 - Rejects: 0
 - Avg order latency: 3.1ms

Account:
 - Balance: 12892.41
 - Equity: 12912.32
```

可自动推送到 Telegram。

---

# **4. 每小时轻量巡检（Hourly Checks）**

由后台自动执行，每小时一次：

- WS 静默检查（trade/depth 是否正常）
    
- API 联通检查（ping）
    
- inventory 是否超限
    
- account equity 是否异常
    
- OMS 失败率
    
- latency 报告
    

如发现异常 → 自动发送 P1 告警。

---

# **5. 日常维护内容**

每天建议维护：

- 清理旧日志（超出 7 天自动归档）
    
- 压缩深度数据（parquet）
    
- 备份 SQLite（或触发自动冷备份）
    
- 检查磁盘空间（避免写爆）
    
- 检查 CPU/GPU（如有）
    
- 检查系统更新
    

自动维护脚本（cron）示例：

```
0 3 * * * /usr/local/bin/bot-maintenance --archive-logs
30 3 * * * /usr/local/bin/sync-data-to-s3
0 */1 * * * /usr/local/bin/check-system-health
```

---

# **6. 系统更新与上线流程（Release Pipeline）**

做市系统更新必须谨慎，此处定义标准流程：

## ✔ A. 开发阶段

- 新策略 → feature 分支
    
- 单元测试完整
    
- 仿真环境跑至少 12 小时
    

## ✔ B. 回测阶段

- 回测池至少使用近 30 天数据
    
- 参数自检
    
- 风控自检
    

## ✔ C. 仿真实盘（Simulator）

- 使用实时行情
    
- 不真正下单
    
- 运行 1–3 小时确认无误
    

## ✔ D. 生产环境灰度（可选）

- 小资金运行或只挂一半网格
    

## ✔ E. 正式上线

步骤：

```
停止策略
确认无持仓
停止进程
部署新版本
启动进程
启动策略
检查日志 & 告警状态
```

## ✔ F. 上线后监控

前 30 分钟必须密切观察：

- Reject
    
- inventory
    
- latency
    
- WS 切换
    
- 偏移行为
    

如异常 → 回滚上一版本。

---

# **7. 热更新参数流程（Param Rollout）**

ConfigService 已支持热更新，但必须有流程：

## ✔ A. 禁止直接修改敏感参数

如：

- MaxInventory
    
- MaxDailyLossPercent
    
- Leverage
    
- RiskRateLimit
    

这些必须由 Admin 修改。

## ✔ B. 参数变更必须双确认

流程：

```
Operator 提交变更（WebUI）
  ↓
Admin 审批
  ↓
ConfigService 热更新
  ↓
记录审计日志
  ↓
策略调用新参数
```

## ✔ C. 参数变更影响检查

系统应自动检查：

- 新 spread 是否合理？
    
- 网格层数是否过高？
    
- volatility factor 是否过大？
    
- MaxInventory 是否足够？
    

若检测失败 → 阻止参数应用。

---

# **8. 紧急操作流程（Emergency Procedures）**

紧急情况下，必须有标准化操作保证资金安全。

## ❗ 1. 紧急撤单（Cancel All）

触发：

- WS 断线
    
- API 错误
    
- latency 激增
    
- inventory 超限
    

流程：

```
暂停策略
撤销全部订单
sync account/position
等待系统恢复
```

## ❗ 2. 紧急平仓（Close All）

触发：

- 风险率 > 80%
    
- 策略无法继续运行
    
- API 某方向完全失效
    

流程：

```
市价平仓
撤销全部挂单
停止策略
通知运维
```

## ❗ 3. PanicStop

自动流程：

```
cancel_all
market_close
disable strategy
alert P0
```

---

# **9. 故障处理流程（Incident Handling）**

系统出现异常 → 进入“事故处理模式”

## Step 1：检测异常

通过告警模块自动触发。

## Step 2：保护资金

执行：

- 撤单
    
- 平仓（如需要）
    
- 停止策略
    

## Step 3：收集上下文信息

自动：

- recent logs
    
- recent OMS 状态
    
- market snapshots
    
- risk state
    

## Step 4：人工排查原因

常见原因：

- 数据质量问题（WS 停更）
    
- API 错误
    
- 策略 bug
    
- 机器故障
    
- 网络抖动
    
- 磁盘写满
    

## Step 5：修复并验证

通过 Simulator 模式验证修复是否有效。

---

# **10. 事故复盘流程（Postmortem）**

真实量化团队会对每次事故写复盘报告。  
建议结构：

```
=== Incident Report ===
Incident ID: xxxx
Time: xxx
Detection: panicstop
Root Cause: WS reconnect 失败
Impact: 亏损 xxx
Mitigation: cancel_all, close, restart system
Prevention: WS reconnect fallback, heartbeat
```

复盘要补充：

- 系统应该如何更快检测
    
- 是否可以自动避免该事故
    
- 风控是否可以更早介入
    

---

# **11. 执行保障机制（Health Checks）**

建议实现以下 Health Check 机制：

## ✔ 内部健康检查

- StrategyTick
    
- WSHeartbeat
    
- APIPing
    
- Redis Ping（如使用）
    
- OMSLatency
    
- InventoryHealth
    
- Risk Health
    

## ✔ 外部健康检查（对 WebUI）

`GET /health` 返回：

```
{
  "system": "ok",
  "strategy": "running",
  "ws": "connected",
  "api": "ok",
  "oms": "ok"
}
```

你可以用 uptime 机器人持续检查。

---

# **12. 配套工具（OpsKit）**

建议你为运维构建一个小工具集 opskit，包括：

- `cancel_all`
    
- `close_all`
    
- `sync_account`
    
- `sync_orders`
    
- `backup_db`
    
- `archive_logs`
    
- `rotate_logs`
    
- `status`（打印系统状态）
    

所有 opskit 操作必须写入审计日志。

---

# **13. 扩展能力**

未来可扩展：

- 自适应参数调整（基于长期运行数据）
    
- 自动化版本回滚
    
- 在线策略 A/B 测试控制面板
    
- 自动巡检机器人（ChatGPT Agent）
    
- AI 主动预测系统风险
    
- 事故自动解释模型
    
- 自动化 capacity scaling（多节点自动扩展）
    

最终会形成：

> **一个自维护、自监控、自恢复的智能做市系统。**

---

# **下一份文档**

你的系统还有最后两个模块：

### **1）API Gateway & External Integrations – 外部接口与网关设计文档（v1.0）**

（统一 Binance、OKX、Bybit 的 API，作为策略与交易所之间的“单一入口”）

### **2）System Architecture Summary – 系统架构总览与依赖关系图（v1.0）**

（将所有模块整合为最终完整图谱）

你希望继续按顺序输出下一份吗？