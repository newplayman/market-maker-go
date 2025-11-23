

本模块是整个做市系统的 **核心基础设施**，用于：

- 记录系统运行状态
    
- 记录策略行为
    
- 跟踪订单生命周期
    
- 捕捉错误、告警、异常
    
- 存档敏感操作（审计日志）
    
- 支持实盘回溯
    
- 支持事故分析
    
- 支持模型调试（未来 AI 做市需要）
    

在真实的量化交易机构中，日志系统的设计质量直接决定：

- 故障能否快速定位
    
- 风控触发原因能否回溯
    
- 策略事故能否准确复盘
    
- 是否能为回测/模型提供真实数据
    

本设计文档提供一个完整且专业可落地的日志 & 审计体系。

---

# **目录**

1. 模块目标
    
2. 日志分类（五大类别）
    
3. 日志等级（Level）
    
4. 日志格式规范
    
5. 系统日志（System Logs）
    
6. 策略日志（Strategy Logs）
    
7. OMS / 交易日志（Order Logs）
    
8. 风控日志（Risk Logs）
    
9. 审计日志（Audit Logs）
    
10. 日志存储架构
    
11. 日志查询与可视化
    
12. 日志轮转与清理策略
    
13. 故障回溯方法
    
14. 扩展能力
    

---

# **1. 模块目标**

Logging 系统的最终目标是：

- **稳定、完整记录所有关键事件**
    
- **格式统一、易查询、可自动处理**
    
- **支持程序高频输出（每秒数百条）**
    
- **支持策略调试、事故回溯、风控验证**
    

Audit 系统的最终目标：

- **记录所有用户敏感操作**
    
- **记录参数变动前后的差异**
    
- **法律意义上的责任记录**（机构级体系）
    
- **不可篡改、可回溯**
    

一句话：

> 日志系统是做市商系统“神经系统”，审计日志是“黑匣子”。

---

# **2. 日志分类（五大类别）**

整个系统应将日志分为 5 类：

1. **System Log（系统日志）**
    
2. **MarketData Log（行情日志）**
    
3. **Strategy Log（策略日志）**
    
4. **OMS / Order Log（订单执行日志）**
    
5. **Risk Log（风险日志）**
    
6. **Audit Log（审计日志）** ← **必须单独管理**
    

每类日志必须独立输出、独立存储、独立级别控制。

---

# **3. 日志等级（Level）**

日志等级建议使用：

```
DEBUG
INFO
WARN
ERROR
CRITICAL
```

含义：

- **DEBUG**：调试用，高频输出（不用于生产）
    
- **INFO**：正常状态事件
    
- **WARN**：策略异常但可继续运行
    
- **ERROR**：需要人工介入
    
- **CRITICAL**：必须立即停止策略（panicstop）
    

在生产环境：

- Strategy / OMS 等模块偶尔会用 INFO
    
- MarketData 用 WARN/ERROR
    
- Risk 用 WARN/CRITICAL
    
- Audit 永远用 INFO
    

---

# **4. 日志格式规范**

所有日志必须使用 **结构化 JSON 日志**：

示例：

```json
{
  "ts": "2025-11-19T13:22:01.123Z",
  "module": "OMS",
  "level": "INFO",
  "event": "OrderSent",
  "symbol": "BTCUSDC",
  "orderId": "123456789",
  "side": "BUY",
  "price": 39880.1,
  "qty": 0.1,
  "latency_ms": 3.2
}
```

特点：

- 统一字段
    
- 统一大小写
    
- 时间戳统一使用 ISO8601
    
- 每条日志占一行
    

优点：  
可直接使用 Loki / ElasticSearch / Splunk 搜索。

---

# **5. System Logs（系统日志）**

记录：

- 系统启动/停止
    
- 服务加载
    
- 配置初始化
    
- API 启动
    
- goroutine 数量变化
    
- 内存警告
    
- CPU 过高警告
    
- 回测 / 仿真启动
    

示例：

```json
{
  "module": "System",
  "event": "ServiceStarted",
  "version": "1.2.0",
  "git": "a91bcf",
  "uptime_sec": 0
}
```

---

# **6. Strategy Log（策略日志）**

记录策略内部关键决策：

- onTick 触发
    
- 算出的 bid/ask
    
- 目标 spread
    
- 网格层分布
    
- imbalance / volatility / trend 值
    
- hedge 动作
    
- cancel/replace 触发逻辑
    

示例：

```json
{
  "module": "Strategy",
  "event": "NewQuote",
  "symbol": "BTCUSDC",
  "bid": 39879.8,
  "ask": 39880.2,
  "spread_ticks": 4,
  "imbalance": 0.32,
  "volatility": 0.81,
  "effective_inventory": -0.02
}
```

策略日志必须 **足够丰富** 才能帮助你定位异常。

---

# **7. OMS / Order Log（订单执行日志）**

这是最重要日志之一。

必须记录：

- 下单
    
- 撤单
    
- 修改（替换）
    
- 部成
    
- 全成
    
- 订单被交易所拒绝
    
- 超过撤单上限
    
- 网络错误
    

示例：

```json
{
  "module": "OMS",
  "event": "OrderRejected",
  "orderId": "887299",
  "reason": "-2010 Insufficient margin",
  "retry": false
}
```

OMS 日志必须 **毫不遗漏** 地记录订单事件。

它是你未来“事故复盘”的核心信息源。

---

# **8. Risk Log（风险日志）**

风控日志必须记录：

- inventory 超限
    
- daily loss 超限
    
- panicstop
    
- vol 触发保护性撤单
    
- cancel rate 触发 action
    
- hedge 建议
    

示例：

```json
{
  "module": "Risk",
  "event": "InventoryOverLimit",
  "effective_inventory": 0.83,
  "max_inventory": 0.50,
  "action": "hedge",
  "hedge_side": "SELL",
  "hedge_qty": 0.33
}
```

风控日志要严格、可追溯。

---

# **9. Audit Log（审计日志）**

这是整个系统中最敏感、最严肃的日志。

必须记录：

- 用户登录
    
- 用户修改配置
    
- 参数的旧值与新值
    
- 启动/停止策略
    
- 手动平仓
    
- 修改 API key（加密后记录）
    
- 删除用户
    
- 动态调参操作
    

示例：

```json
{
  "ts": "2025-11-19T13:02:11.992Z",
  "module": "Audit",
  "user": "admin",
  "role": "admin",
  "action": "config_update",
  "symbol": "BTCUSDC",
  "before": {"GridLevels": 3},
  "after": {"GridLevels": 4},
  "ip": "192.168.3.20"
}
```

### **审计日志必须存入不可篡改存储（append-only）**：

- local log（append-only）
    
- 或 MySQL/SQLite（只写不删）
    
- 或 event-stream（Kafka / NATS）
    
- 或 MinIO（对象存储）
    

任何删除都必须报警。

审计日志是“事故责任”的核心依据。

---

# **10. 日志存储架构**

推荐结构：

```
Local raw logs (.log files)
       ↓
Promtail
       ↓
Loki (log index & search)
       ↓
Grafana (view)
       ↓
Archive storage (S3/MinIO)
```

特点：

- 多副本
    
- 快速搜索
    
- 自动索引
    
- 浏览器方便查看
    

---

# **11. 日志查询与可视化**

WebUI 必须提供：

- 日志搜索框
    
- 按 module、event、level 过滤
    
- 按时间范围查询
    
- 下载日志文件
    

Grafana/Loki 可提供：

- 正则搜索
    
- 聚类视图
    
- 图表化展示错误频率
    

常见分析：

- OMS 延迟异常图
    
- 风控触发次数图
    
- Reject 数量时间线
    
- 策略错单量统计
    

这对系统调优至关重要。

---

# **12. 日志轮转与清理策略**

推荐：

- 每天轮转一次
    
- 保留本地 7 天
    
- 压缩归档至对象存储（S3 / MinIO）
    
- 保留 180–360 天用于审计
    
- CRITICAL 和 Audit 日志永久保留（可选）
    

轮转策略示例：

```
logs/*.log
logs/*.log.1.gz
logs/*.log.2.gz
...
```

---

# **13. 故障回溯方法**

结合日志可以进行：

### ① 委托丢失排查

→ 看 OMS 日志是否发送成功  
→ 看 gateway 是否返回成功  
→ 看策略是否误操作

### ② inventory 爆炸排查

→ 看策略是否错判趋势  
→ 看风控是否失效  
→ 看订单是否没撤成功

### ③ API 错误排查

→ 使用 error 事件追踪  
→ 检查网络/交易所状态

### ④ 推断策略 bug

→ 查看 StrategyLog → 参数 → 挂单行为

你未来加入 AI 辅助分析后，这些日志就是训练数据。

---

# **14. 扩展能力**

未来可以扩展：

- **机器学习日志分析**：自动识别异常模式
    
- **关键事件可视化时间轴**
    
- **自动事故报告生成器**
    
- **实时异常检测（Anomaly Detection）**
    
- **Flink/Kafka 流式日志分析**
    
- **策略行为 Replay（复盘回放）**
    
- **自动调参建议（根据日志统计）**
    

这一部分会帮助你迈入“机构级做市系统”水平。

---

# 下一份文档

继续按顺序，下一份是：

> **《Alerting – 告警系统详细设计文档（v1.0）》**

（虽然监控文档中已有部分内容，但这个文档将提供完整的告警体系、渠道、通知机制、优先级策略、自动化恢复等。）

我继续？