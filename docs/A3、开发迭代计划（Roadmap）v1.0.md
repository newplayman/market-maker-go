  
这个 Roadmap 将覆盖你整个做市系统未来 6 个月的核心工作内容，包含：

- 任务拆解
    
- 阶段目标
    
- 开发优先级
    
- 每阶段交付物
    
- 依赖关系
    
- 可并行与不可并行的任务
    
- 实盘上线流程
    

这是一个**可直接用于推进项目的“执行路线图”**。

---


## 时间跨度：**6 个月**

## 节奏：分为 **5 个主要版本**

## 风格：Scrum + Milestone（每阶段都有可验证的可运行成果）

---

# **目录**

1. 总体目标
    
2. 版本规划总览
    
3. 版本详细规划
    

- V1.0（核心基础）
    
- V1.1（稳定化与观测性）
    
- V1.5（数据层 + 仿真系统）
    
- V2.0（策略进化 + 多交易所）
    
- V3.0（AI & 智能策略阶段）
    

4. 任务依赖图
    
5. 每周/双周开发节奏
    
6. 上线策略
    
7. 风险与缓冲区
    
8. 未来扩展（Roadmap v2.0）
    

---

# **1. 总体目标**

在 6 个月内构建：

- 一个可稳定 24/7 运行
    
- 具备专业风控
    
- 多策略可扩展
    
- 多交易所可扩展
    
- 可自动恢复
    
- 可数据驱动
    
- 可监控
    
- 可接受 AI 插件
    

的完整**做市商交易系统（Market Maker Engine）**。

最终目标：

> 在安全、不爆仓的前提下，稳定获得持续的日收益。

---

# **2. 版本规划总览（Milestone Overview）**

|版本|时间|目标|状态|
|---|---|---|---|
|**V1.0**|0–4 周|做市系统核心框架（OMS + Risk + Gateway）|关键里程碑|
|**V1.1**|4–6 周|监控 + 用户系统 + WebUI 基础|稳定性提升|
|**V1.5**|6–10 周|数据基础设施 + 仿真系统 + 回测基础|数据驱动|
|**V2.0**|10–16 周|策略升级（Phase2/AS）+ 多交易所|功能增强|
|**V3.0**|16–24 周|AI 辅助策略 + 自动调参|智能化|

整个路线符合 “最小生产系统 → 稳定化 → 数据化 → 扩展 → 智能化” 的产品演进逻辑。

---

# **3. 版本详细规划**

---

# **📌 V1.0 – 核心基础（第 0–4 周）**

目标：**一个可运行的做市引擎 + 基础风控 + OMS + Gateway**

### **必须完成（关键路径）**

### ● API Gateway（REST + WS）

- WS 行情订阅（depth, trade, account）
    
- REST 下单 / 撤单 / 查询
    
- 限频控制
    
- 错误分类与重试
    
- listenKey 自动管理
    
- WS 自动重连
    
- Snapshot 恢复机制
    

### ● MarketData Processor

- 深度数据结构
    
- 逐笔成交
    
- 中间价 mid
    
- spread 计算
    
- imbalance / volatility（基础）
    
- Tick 流封装
    

### ● Order Management System（OMS v1）

- 下单队列
    
- 撤单队列
    
- 补单机制
    
- 状态机（active, pending, filled, canceled）
    
- 订单标签（maker/taker）
    

### ● Risk Engine（v1）

- max_inventory
    
- max_daily_loss
    
- panicstop
    
- error-rate 风险
    
- WS disconnect 风险
    

### ● Strategy Phase 1（轻量做市）

- 双边挂单
    
- Spread 调整（固定）
    
- 成交后自动置换
    
- 无 inventory 暴露
    

### ● State Cache

- 持仓快照
    
- 订单簿快照
    
- 未成交挂单快照
    
- 账户余额快照
    

### **可选任务（非关键路径）**

- 偏技术性优化
    
- 自动巡检（简版）
    

---

# **📌 V1.1 – 稳定化 + 可观测性（第 4–6 周）**

目标：**让系统可操控、可监控、可自恢复**

### ● Logging（结构化日志）

- 全模块 JSON Log
    
- 日志切割
    
- 日志等级控制
    

### ● Monitoring（Prometheus + Grafana）

- API 延迟
    
- 成交率 fill-rate
    
- 风控触发计数
    
- 余额与 equity 曲线
    
- WS 延迟
    
- OMS 内部延迟
    

### ● Alerting（P0/P1/P2）

- Telegram 通知
    
- 订单拒绝告警
    
- Panicstop 告警
    
- WS 断线告警
    

### ● ConfigService（热更新参数）

- 动态修改策略参数
    
- 动态修改风控参数
    
- 版本化管理
    

### ● WebUI（基础版）

- 状态展示
    
- 策略开启/暂停/停止
    
- 基础参数显示
    

### ● User & Permission（基础版）

- 管理员权限
    
- 登录系统
    
- 操作行为审计
    

---

# **📌 V1.5 – 数据系统 + 仿真系统（第 6–10 周）**

目标：**让系统可以靠数据驱动提升策略质量**

### ● Data Lake（本地 + S3/MinIO）

- 深度数据存储
    
- 逐笔成交存储
    
- PnL 数据日志化
    

### ● 结构化数据库（SQLite / Postgres）

- orders
    
- trades
    
- pnl
    
- audit
    
- inventory
    
- risk events
    

### ● 仿真系统 Simulator（关键）

- WS 模拟
    
- REST 模拟
    
- 可复现实盘行为
    
- 支持回放历史行情
    

### ● 回测引擎（Backtester v1）

- 使用 parquet 数据
    
- 策略回放
    
- 订单簿模拟（L1/L2）
    
- 简易 PnL 输出
    

### ● WebUI 扩展

- 历史 PnL
    
- 订单历史
    
- Risk 日志展示
    

---

# **📌 V2.0 – 策略进化 + 多交易所（第 10–16 周）**

目标：**提升收益能力 + 扩展可用性**

### ● Strategy Phase 2（动态做市 + 网格仿射）

- 动态 spread（波动率驱动）
    
- 动态 grid size
    
- 盘口偏移决策（imbalance）
    
- “稳定利润区间”机制
    
- inventory-based quoting（轻量）
    

### ● Avellaneda-Stoikov（简化版）

- 风险厌恶参数 γ
    
- 市场深度参数 k
    
- 最优 bid/ask 算法
    
- 在高波动区执行更聪明的报价
    

### ● 多交易所支持（OKX / Bybit）

- Gateway Adapter
    
- 统一接口
    
- 订单状态差异适配
    

### ● 多 Symbol 支持

- 并行策略调度
    
- 多 symbol 状态隔离
    

### ● 交易所级风控（per-exchange）

- 使用不同 MaxInventory
    
- 使用不同价差策略
    

---

# **📌 V3.0 – AI & 智能策略阶段（第 16–24 周）**

目标：**实现 AI 辅助决策、AI 参数调整、策略自动进化**

### ● AI 模型（Phase A）

- 基础微观结构预测模型（direction / volatility）
    
- 特征生成器（Feature Engine）
    
- imbalance + trend 模型
    
- orderflow 分类模型
    

### ● 参数自动调节系统（Auto Tuning）

- 用历史数据寻找最优参数
    
- 自动生成新参数组合
    
- 灰度实验
    

### ● 多模型协同决策（Model Voting）

- rule-based + ML 模型融合
    
- LLM 解释器（解释行为）
    

### ● 策略评价系统（Benchmark System）

- 每日策略得分
    
- 每周策略表现报告
    
- 风险收益比排行
    

---

# **4. 任务依赖图（Critical Dependency Graph）**

非常重要，用于规划开发顺序：

```
Gateway → MarketData → StateCache → OMS → Strategy Phase1 → RiskEngine → Deployment
                                                          ↓
                                                   Strategy Phase2
                                                          ↓
                                                   Backtest + Data Lake
                                                          ↓
                                                   多交易所支持
                                                          ↓
                                                   AI 模型
```

关键路径（必须按顺序完成）：

```
Gateway → OMS → Risk Engine → Strategy → Deployment → Logging → Monitoring
```

可并行任务：

- WebUI
    
- ConfigService
    
- Data Lake
    
- 回测系统
    
- AI（后期）
    

---

# **5. 每周/双周开发节奏（按 6 周示例）**

以下是推荐开发节奏（你可根据团队人数调整）：

### **Sprint 1（第 1–2 周）**

- Gateway WS + REST
    
- MarketData 基础
    
- OMS 下单 + 撤单
    

### **Sprint 2（第 3–4 周）**

- RiskEngine
    
- Strategy Phase1
    
- StateCache
    
- Panicstop
    
- 基础监控
    

### **Sprint 3（第 5–6 周）**

- Logging
    
- Alerting
    
- ConfigService
    
- WebUI v1
    
- 实盘灰度（超低资金）
    

之后可以按需求继续制定版本节奏。

---

# **6. 上线策略（非常关键）**

上线必须遵守：

### ✔ Step 1：小资金灰度

资金：$50–$500  
时间：1–3 天

### ✔ Step 2：观察三指标

- Reject rate
    
- inventory 波动
    
- latency 稳定度
    

### ✔ Step 3：全量上线

- 进入自动模式
    
- 启用巡检报告
    
- 每天记录表现
    

### ✔ Step 4：每周性能评审

分析：

- spread 采集
    
- fill-rate
    
- pnl 曲线
    
- 风控触发频率
    

### ✔ Step 5：参数微调

使用 phase2/AS 的参数优化策略。

---

# **7. 风险与缓冲区（Risk & Buffer）**

开发中可能遇到：

|风险|对策|
|---|---|
|交易所 API 改动|Gateway modular 层可迅速适配|
|极端行情|RiskEngine 强制保护|
|策略 bug|仿真系统可以重现|
|超期任务累积|严格每周 Review|
|技术债累积|采用 CI + Linter + Review|
|数据膨胀|定期压缩与归档|

Roadmap 中已预留 15–20% 的“浮动空间”用于不可预期问题。

---

# **8. 未来扩展（Roadmap v2.0 草案）**

未来可以加入：

- 多账户资产管理系统（Portfolio）
    
- 做市 + 套利融合策略
    
- L3 ultra depth 数据引擎
    
- Onchain/DEX 做市策略
    
- 多模型 ensemble 架构
    
- 全自动“做市即服务”平台
    

最终实现：

> **“全自动、多模型、跨市场、智能化”的做市交易系统。**

---

# 🎉 Roadmap 文档完成！

到此为止，你已经具备：

- 技术架构
    
- 策略体系
    
- 风控体系
    
- 系统设计
    
- 产品规划
    
- 开发规范
    
- 开发路线图（当前这份）
    

整个体系已经达到“专业量化团队”标准。

---

# 下一步选项：

### ✔ A) 我为你生成 **完整 Go 项目目录 + 模板代码**

（立即开始工程落地）

### ✔ B) 为你产出 **Phase1 策略 Go 代码（可运行示例）**

### ✔ C) 为你设计 **WebUI 前端 + API 结构文档**

### ✔ D) 为你写 **仿真系统（Simulator）架构文档**

你希望下一步进入哪部分？