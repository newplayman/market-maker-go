

做市系统的数据库模块不仅是“存东西”，  
它必须做到：

- **高性能**（不能阻塞交易）
    
- **高可靠性**（数据不能丢）
    
- **可扩展性**（未来 AI / 回测系统会需要海量数据）
    
- **一致性**（策略恢复、故障恢复需要一致的数据状态）
    
- **结构化 + 流式数据并存**
    

在量化机构中，数据库的设计质量直接影响：

- **回测能否准确**
    
- **风控能否正确工作**
    
- **事故能否快速复盘**
    
- **模型训练是否有足够优质数据**
    

本设计文档将为你构建一个专业级的数据存储方案。

---

# **目录**

1. 模块目标
    
2. 为什么做市系统必须有高质量数据层
    
3. 数据分类（六大类）
    
4. 数据库选型建议
    
5. 存储架构（总体结构）
    
6. 关键数据表结构（schema）
    
7. 高频数据存储策略
    
8. 冷数据归档策略
    
9. 回测数据仓库（Backtest Data Lake）
    
10. 配置与元数据存储
    
11. 审计日志与告警存储
    
12. 数据访问层（DAL）设计
    
13. 扩展能力
    

---

# **1. 模块目标**

Database & Storage 模块目标：

- 保存所有对系统安全与策略复盘有价值的数据
    
- 提供低延迟访问
    
- 支持结构化查询（订单、策略、风险）
    
- 支持批量存储（行情、成交历史）
    
- 支持回测与 AI 模型训练需求
    
- 为未来的“策略比对、参数优化”提供历史数据基础
    

核心原则：

> **交易系统不能依赖数据库来执行策略，但数据库必须完整记录策略行为。**

---

# **2. 为什么做市系统必须有高质量数据层？**

做市策略不同于趋势策略（依赖几根 K 线）  
它需要：

- **OrderBook 深度数据**
    
- **逐笔成交数据**
    
- **PnL 与回撤数据**
    
- **每个订单的生命周期数据**
    
- **Inventory 变化轨迹**
    
- **风控触发记录**
    

这些数据不是“可选的”，而是：

> 构建专业做市系统的最基本基础。

---

# **3. 数据分类（六大类）**

按重要性与用途，分为 6 类：

1. **实时交易数据（订单、成交）**
    
2. **市场数据（OrderBook、Trade、Kline）**
    
3. **策略数据（决策、spread、网格层状态）**
    
4. **风险数据（inventory、loss、风控事件）**
    
5. **日志、审计、告警**
    
6. **配置与元数据**
    

每类数据存储方式不同。

---

# **4. 数据库选型建议**

根据你的系统规模与性能需求，推荐混合方案：

### **A) SQLite / PostgreSQL（结构化数据）**

用途：

- 订单生命周期
    
- 成交记录
    
- 风控事件
    
- 审计日志
    
- 配置快照
    

SQLite 特点：

- 单文件、稳定、无服务、可靠
    
- 适合单节点部署
    
- 性能足够支撑策略级写入（每秒几百条）
    

PostgreSQL 特点：

- 更适合你未来扩展到多节点、多交易所
    
- 支持复杂查询与数据分析
    

---

### **B) 对象存储（MinIO / S3）**

用途：

- 深度数据（大的 L2/L3）
    
- 逐笔成交（tick）
    
- 回测数据（data lake）
    
- 原始日志的冷存储
    

实时写入 → 分时压缩 → 保存为 parquet/csv/snappy。

---

### **C) Redis（可选）**

用途：

- 热状态缓存（非关键）
    
- 多节点锁（HA 模式）
    
- 最新行情快照缓存
    

Redis 不用于关键数据（不能丢）。

---

# **5. 存储架构（总体结构）**

推荐存储架构如下：

```
              +-------------------+
              | SQLite/PostgreSQL |
              | 结构化数据（订单）|
              +---------+---------+
                        |
                        |
     +-----------------------------------------+
     |              Data Lake (S3/MinIO)       |
     |   - depth.npy / depth.parquet           |
     |   - trades.parquet                      |
     |   - klines                              |
     |   - raw logs archive                    |
     +-----------------+-----------------------+
                       |
                       |
               +----------------+
               | Local Logs     |
               | JSON Lines     |
               +----------------+
```

三层结构：

1. **本地日志**（速度最快）
    
2. **结构化数据库**（可查询）
    
3. **对象存储**（海量历史数据）
    

---

# **6. 关键数据表结构（schema）**

下面给你的 schema 直接可用于 SQLite / PostgreSQL。

---

## **6.1 orders（订单生命周期表）**

记录一个订单全生命周期，是回溯核心。

```sql
CREATE TABLE orders (
    id TEXT PRIMARY KEY,
    symbol TEXT,
    side TEXT,
    price REAL,
    qty REAL,
    filled_qty REAL,
    status TEXT,
    created_at DATETIME,
    updated_at DATETIME,
    reason TEXT,
    raw JSON
);
```

---

## **6.2 trades（成交记录）**

记录策略所有成交：

```sql
CREATE TABLE trades (
    trade_id TEXT PRIMARY KEY,
    order_id TEXT,
    symbol TEXT,
    side TEXT,
    price REAL,
    qty REAL,
    fee REAL,
    is_maker BOOLEAN,
    ts DATETIME
);
```

---

## **6.3 inventory_log（库存变化）**

管理：

```sql
CREATE TABLE inventory_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ts DATETIME,
    symbol TEXT,
    real_position REAL,
    pending_buy REAL,
    pending_sell REAL,
    effective REAL
);
```

---

## **6.4 pnl_log（损益记录）**

```sql
CREATE TABLE pnl_log (
    ts DATETIME,
    symbol TEXT,
    realized REAL,
    unrealized REAL,
    equity REAL
);
```

---

## **6.5 risk_events（风控事件）**

```sql
CREATE TABLE risk_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ts DATETIME,
    symbol TEXT,
    type TEXT,
    value REAL,
    threshold REAL,
    action TEXT,
    details JSON
);
```

---

## **6.6 config_snapshots（配置快照）**

```sql
CREATE TABLE config_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ts DATETIME,
    symbol TEXT,
    config JSON
);
```

---

## **6.7 audit_log（审计日志）**

```sql
CREATE TABLE audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ts DATETIME,
    user TEXT,
    role TEXT,
    action TEXT,
    target TEXT,
    before JSON,
    after JSON,
    ip TEXT
);
```

---

# **7. 高频数据存储策略**

深度数据（OrderBook L1–L10）和逐笔成交是高频数据。  
每秒几十次，总量极大。

### **不建议直接写数据库**

会压爆 I/O。

### 推荐方案：

```
内存 → 缓冲区 → 每秒/每 100ms 批量写 → parquet 文件
```

示例文件结构：

```
/data/lake/depth/2025-11-19/14/05.parquet
/data/lake/trades/2025-11-19/14/05.parquet
```

补充：

- parquet 压缩率高
    
- 适合回测
    
- 适合 AI 训练（支持 Arrow 格式）
    

---

# **8. 冷数据归档策略**

日志、深度数据等存入：

- MinIO（本地对象存储）
    
- 或 AWS S3
    

定期任务：

- 7 天本地
    
- 30 天热存储
    
- 冷存储保存半年或一年
    

---

# **9. 回测数据仓库（Backtest Data Lake）**

后续你需要大量历史数据来：

- 回测策略行为
    
- 训练 AI 决策模型
    
- 分析市场 micro-structure
    

推荐采用：

```
Symbol / YYYY / MM / DD / HH
```

例如：

```
BTCUSDC/2025/11/19/14/depth.parquet
BTCUSDC/2025/11/19/14/trade.parquet
```

也可以按 1 分钟切片。

---

# **10. 配置与元数据存储**

所有参数更新必须写入配置快照表：  
（ConfigService 支持）

```
config_snapshots
```

每个 symbol 单独存一份：

```
symbol: "BTCUSDC"
config: { "GridLevels": 3, "MaxInventory": 2.0 ...}
```

方便：

- 回溯策略行为
    
- 分析参数调整与表现关系
    

---

# **11. 审计日志与告警存储**

审计日志必须：

- 可追加
    
- 不可篡改
    
- 不可删除
    
- 可按时间检索
    

建议：

- audit_log 存 SQLite
    
- 同时写入对象存储作为备份
    

告警存入：

```
alert_log
```

---

# **12. 数据访问层（DAL）设计**

数据库必须异步写入，不阻塞交易：

建议采用 Go 的 channel + worker pool：

```
dbWriteChan ← append events
worker 读取 → 批量写入数据库
```

示例：

```go
type DBEvent struct {
    Type string
    Data interface{}
}
```

该结构可用于：

- orders
    
- trades
    
- pnl
    
- audit
    
- risk events
    

异步写入保证高性能。

---

# **13. 扩展能力**

未来数据库层可扩展：

- Kafka 作为事件流
    
- Flink/Spark 进行流式分析
    
- AI 数据准备（深度 → embedding）
    
- 策略强化学习训练数据管线
    
- 多交易所统一数据仓库
    
- L3 数据保存（撮合引擎级别）
    
- 超低延迟行情采集系统（co-location）
    

这一步将让你迈向：

> **“自研高性能专业做市系统 + AI 决策平台”**  
> 的完整体系。

---

# 下一份文档

按顺序，下一份将是整个系统的**最后基础组件之一**：

> **《Ops & Maintenance – 运维与维护流程设计文档（v1.0）》**  
> （包含巡检、日志归档、版本管理、参数管理流程、事故处理流程等）

我可以继续输出下一份吗？