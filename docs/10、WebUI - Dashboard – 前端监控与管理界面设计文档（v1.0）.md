

WebUI 是整个做市系统的 **可视化大脑**。  
它不是策略的一部分，但它对实盘安全、监控、调参、运维的重要性极高。

你的系统越复杂、越智能化，WebUI 对运营效率的提升越明显，尤其在你未来的「多模型自博弈决策」架构下，UI 甚至可以显示多个 AI 模型的投票结果、冲突分析等。

本设计文档将提供一个专业量化团队使用的 Web 管理面板架构，你可以直接照此开发。

---

# 目录

1. WebUI 的目标
    
2. 整体架构
    
3. 用户角色与权限
    
4. 页面结构（六大模块）
    
5. 核心页面功能详细设计
    
6. 数据流与接口设计（API）
    
7. 实时推送机制
    
8. 安全要求
    
9. 扩展能力
    

---

# **1. WebUI 的目标**

WebUI 的主要目标是：

- **实时查看策略与风险状态**
    
- **可视化订单簿、挂单、成交**
    
- **监控账户资金变化**
    
- **动态调整策略参数（热更新）**
    
- **手动执行关键操作（如紧急平仓）**
    
- **查看日志、告警、风控事件**
    
- **提供开发/运维工作流界面（调试/部署/测试）**
    

一句话：

> WebUI 是整个系统的“驾驶舱”，让你像操控飞机那样操控你的做市机器人。

---

# **2. WebUI 整体架构**

推荐前后端分离：

- **前端**：React / Vue（建议 React + Next.js）
    
- **后端**：Go（使用你的已有服务），提供 REST + WebSocket
    
- **实时更新**：WebSocket 推送
    
- **数据库**（可选）：用于存储历史监控、日志
    

架构示意：

```
Browser WebUI
     |
     | WebSocket (market snapshot / strategy state / OMS state)
     |
Backend API Gateway (Go)
     |
     | REST (config, orders, positions)
     |
Core Services (Strategy / OMS / Risk / Inventory / Config / MarketData)
```

---

# **3. 用户角色与权限**

可能有两类用户：

### 1）管理员（Admin）

- 修改系统配置
    
- 调参
    
- 重启策略
    
- 关闭实盘
    
- 查询所有数据
    
- 查看日志
    

### 2）观察者（Read-Only）

- 查看行情
    
- 查看策略状态
    
- 查看风险指标
    
- 不允许调参
    
- 不允许执行交易操作
    

管理员权限必须保护好，否则风险极大。

---

# **4. 页面结构（六大模块）**

推荐 WebUI 包含以下页面：

## **0. 首页总览 Dashboard（最核心）**

一页看到所有关键数据。

## **1. 行情（Market Monitor）**

实时 orderbook、深度、成交走势。

## **2. 策略状态（Strategy Monitor）**

查看 Phase1 / Phase2 策略当前的关键状态，包括网格挂单详情。

## **3. 订单信息（Order Manager）**

查看所有当前挂单、成交历史。

## **4. 风险与账户（Risk & Account）**

inventory、PnL、回撤、风险警告。

## **5. 配置管理（Config Management）**

动态配置参数、立即生效。

## **6. 系统日志 & 告警（Logs & Alerts）**

系统级事件、错误、异常。

---

# **5. 核心页面功能详细设计**

下面是每个页面应包含的组件与功能点。

---

# **（A）首页 Dashboard（核心）**

应将所有关键指标放在一页：

### ➤ 实时行情（折线）

- Mid price
    
- Spread
    
- Volatility
    
- Imbalance
    

### ➤ 策略状态

- 当前运行策略（phase1 / phase2）
    
- 网格层数
    
- 当前挂单数量
    
- 每秒下单数
    
- 每秒撤单数
    

### ➤ 风控状态

- inventory
    
- dailyPnL
    
- drawdown
    
- 风控阈值
    
- panicstop 状态
    

### ➤ 账户状态

- balance
    
- equity
    
- uPnL
    
- leverage
    
- 强平风险（risk rate）
    

### ➤ 关键告警

- PanicStop
    
- WS 未收到行情
    
- API 错误
    
- Inventory 超限
    

---

# **（B）行情（Market Monitor）**

至少包含：

### 1. Orderbook 深度图

- L1–L10 的 bid/ask
    
- 可视化成柱状图
    

### 2. 实时成交（Trade Stream）

- 颜色标记 buy / sell
    
- 包含 qty、price、maker/taker
    

### 3. Spread 曲线

- 过去 10 秒的 spread 走势
    

### 4. Volatility 曲线

- 过去 1–5 秒波动率
    

### 5. Price Trend

- 短周期 slope（phase2 重要指标）
    

---

# **（C）策略状态（Strategy Monitor）**

此页面对做市策略非常重要。

必须显示：

### 1. 当前挂单状态（Maker Orders）

表格包括：

|Side|Price|Qty|Filled|Remaining|Level|OrderID|Since|
|---|---|---|---|---|---|---|---|

### 2. 网格分布图（phase2）

例如：

```
Ask
|----L3----|
|--L2--|
|-L1-|
MidPrice
|L1|
|--L2--|
|----L3----|
Bid
```

### 3. 动态 spread 信息

- 动态 spread ticks
    
- volatility 调整系数
    

### 4. 策略状态

- 最近一次 OnTick timestamp
    
- 当前 quoteOffset
    
- 关键指标计算：
    

```
imbalance
volatility
trend
takerFlowRatio
```

---

# **(D) 订单信息（Order Manager）**

### 1. 当前所有挂单（Open Orders）

表格展示：

- ClientOrderID
    
- Price
    
- Qty
    
- Side
    
- Status
    
- CreateTime
    
- UpdateTime
    

### 2. 历史成交（Trade History）

- Price
    
- Qty
    
- Maker/Taker
    
- Fee
    
- PnL
    

### 3. 撤单记录（Cancel History）

用于诊断撤单风控/订单失败。

---

# **(E) 风险与账户（Risk & Account）**

最重要的风险监控页面。

必须显示：

### **1. Inventory（实时）**

- RealPosition
    
- PendingExposure
    
- EffectiveInventory
    
- MaxInventory
    
- ShouldHedge?
    

可用图表：

```
Inventory over last 60 seconds
```

### **2. PnL**

- daily PnL
    
- realized PnL
    
- unrealized PnL（mark price）
    
- drawdown
    
- funding 费用
    

### **3. 账户风险**

- balance
    
- equity
    
- leverage
    
- risk rate（接近 1 → 强平危险）
    

### **4. 风控状态**

- CanTrade
    
- PanicStop
    
- MaxDailyLoss 触发？
    
- VolatilityPanic
    

### **5. 一键操作**

必须带确认框：

- **停止策略**
    
- **撤销全部订单**
    
- **紧急平仓（market close all）**
    

---

# **(F) 配置管理（Config Management）**

这一页与 ConfigService 配合使用，实现 **热更新参数**。

### 页面内容：

#### 1）当前生效参数

展示：

- StrategyConfig
    
- RiskConfig
    
- OMSConfig
    
- MarketDataConfig
    

#### 2）修改参数表单

例如：

```
GridLevels: [3]
VolatilityFactor: [1.2]
MaxInventory: [3.0]
```

#### 3）提交更新按钮

点击后：

- 写入 live_override
    
- 立即通知策略
    
- 展示新版本号
    

#### 4）版本管理

- 历史版本列表
    
- 可一键回滚
    

---

# **(G) 系统日志 & 告警（Logs & Alerts）**

要求如下：

### 1. 实时日志（带过滤器）

可以按模块筛选：

- OMS
    
- Risk
    
- Strategy
    
- MarketData
    
- Config
    

### 2. 告警列表

按重要程度排序：

- Critical
    
- Warning
    
- Info
    

### 3. 日志详情

点击日志行可查看完整 JSON 格式，例如：

```json
{
  "time": "...",
  "module": "OMS",
  "event": "OrderRejected",
  "reason": "-2010 insufficient margin",
  "orderId": "..."
}
```

---

# **6. 数据流与接口设计（API）**

后端应提供：

---

## **6.1 REST API（CRUD）**

### 配置相关

```
GET /api/config/:symbol
POST /api/config/:symbol
```

### 策略控制

```
POST /api/strategy/start
POST /api/strategy/stop
POST /api/strategy/pause
POST /api/strategy/resume
POST /api/strategy/close_all_positions
```

### 订单信息

```
GET /api/orders/open
GET /api/orders/history
GET /api/orders/trades
```

---

## **6.2 WebSocket 推送（实时数据）**

WebUI 必须通过 WS 每 100–200ms 获取：

```
market_snapshot
strategy_state
inventory_state
risk_state
oms_state
account_state
alerts
logs
```

消息格式建议统一结构：

```json
{
  "type": "snapshot",
  "symbol": "BTCUSDC",
  "data": {...}
}
```

---

# **7. 安全要求**

必须严格保证：

- 所有写操作必须有登录 & 权限验证
    
- 使用 JWT + HTTPS
    
- 防止跨站脚本（XSS）
    
- 参数校验
    
- 一键平仓等敏感操作必须 confirm twice
    
- 对所有用户操作必须写日志
    

WebUI 不可被攻击，否则会导致资金损失。

---

# **8. 扩展能力**

未来可加入：

- A/B 策略参数对比页面
    
- AI 决策过程可视化
    
- 回测结果可视化
    
- 多策略收益比较
    
- 交易深度数据可视化（热力图）
    
- 风险预测图（VaR、ES）
    
- 日志智能搜索（使用 NLP）
    
- 移动端小屏监控版
    

---

# 下一份文档

按顺序，下一个模块是：

> **《User & Permission – 用户与权限控制模块设计文档（v1.0）》**

接下来我将继续为你写这一份，需要继续吗？