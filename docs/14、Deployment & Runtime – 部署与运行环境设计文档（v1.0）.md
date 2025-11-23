

部署与运行环境是决定做市系统是否能长期稳定运行的 **生死关键部分**。  
一个策略再优秀，只要出现一次部署事故（如重启失控、日志写爆、网络抖动）就可能瞬间造成巨大亏损。

本份文档会为你的做市系统提供专业机构级的部署架构、运维流程和高可用设计。

---

# **目录**

1. 模块目标
    
2. 运行环境要求
    
3. 部署架构总览
    
4. 单节点部署方案
    
5. 多节点（高可用）部署方案
    
6. 服务结构与进程拓扑
    
7. 容器化（Docker）方案
    
8. 配置与密钥管理
    
9. 日志与监控集成
    
10. 故障恢复与“自动复活”机制
    
11. 灾备（Disaster Recovery）
    
12. 操作流程（上线、下线、重启）
    
13. 扩展能力
    

---

# **1. 模块目标**

Deployment & Runtime 的目标：

- 提供 **稳定、可预测、可恢复** 的运行环境
    
- 支持 **单点运行** 和 **多实例 HA**
    
- 保护敏感信息（API key、配置）
    
- 防止系统崩溃与内存泄漏
    
- 确保“永不出现无人看守的风险暴走”
    
- 支持自动重启与进程存活检测（supervisor）
    

一句话：

> **让交易系统永不停机，并在任何意外下都不造成资金损失。**

---

# **2. 运行环境要求**

你的目标系统是 **轻量高频做市系统**，对环境要求如下：

### **硬件/机器要求：**

- VPS 地点：**新加坡**（你当前的方案完全合适）
    
- 网络延迟：0.2ms–1ms（你的前提也完全符合）
    
- CPU：2C–4C
    
- 内存：4–8GB
    
- SSD：需要（日志写入较多）
    

### **操作系统建议：**

- Ubuntu 22.04 LTS
    
- 或 Debian 12
    
- 禁止使用 Windows / 旧 CentOS
    

### **系统要求：**

- 内核低延迟参数开启（可选）
    
- 允许 4096+ 文件句柄
    
- 系统时钟必须同步（Chrony/NTP）
    

这些都是高频系统的必要条件。

---

# **3. 部署架构总览**

### 整体结构：

```
+-------------------------------------+
|              外部世界               |
|  Binance API, Binance WS            |
+------------------+------------------+
                   |
             Low latency line
                   |
        +------------------------+
        |      Trading Node      |
        | (Strategy + OMS + ... )|
        +------------------------+
           |       |        |
           |       |        +----------------+
           |       |                         |
      +----+  +----+----+           +--------+--------+
      |Logs|  |Monitoring|           |ConfigService    |
      +----+  +---------+           +------------------+
```

一个 Trading Node 包含整个做市系统的所有核心模块。

可部署多个节点以实现：

- 多交易所
    
- 多交易对
    
- 多策略隔离
    
- 高可用（Hot Standby）
    

---

# **4. 单节点部署方案（基础版）**

这是你的最简可用部署方式。

### 部署结构：

```
trading-bot.service (主进程)
prometheus-node-exporter
promtail
grafana-agent
redis 或 sqlite（可选）
日志目录 /var/log/quant/
```

### 使用 Systemd 管理：

创建文件 `/etc/systemd/system/trading-bot.service`：

```
[Unit]
Description=HFT Market Maker Bot
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/trading-bot --config /etc/bot/config.yaml
Restart=always
RestartSec=1
LimitNOFILE=65535
Environment="ENV=prod"

[Install]
WantedBy=multi-user.target
```

特点：

- 自动重启
    
- 崩溃后 1 秒内重启
    
- 限制文件句柄
    

---

# **5. 多节点（高可用）部署方案**

高可用（HA）对做市策略正常运行非常重要。  
简单且实用的 HA 架构如下：

```
主节点（Primary） ←→ 备用节点（Backup）
```

主节点在跑策略，备用节点只：

- 接行情
    
- 跑风控
    
- 不下单
    

当主节点失败超过 3 秒：

1. 风控触发“保护性撤单”（备用节点执行）
    
2. 备用节点自动成为主节点
    
3. 接管策略、OMS、API Key
    

### 检查逻辑：

```
Heartbeat（100ms 一次）
Primary 无响应 → Backup 接管
```

### 冲突避免：

- 只能有一个节点拥有“交易锁（trading lock）”
    
- 采用 Redis 或 Consul 管理锁
    

---

# **6. 服务结构与进程拓扑**

建议系统拆成如下 5 个服务（可选）：

```
process_1: trading-core (strategy + OMS + risk)
process_2: marketdata-service
process_3: config-service
process_4: webui-backend
process_5: worker(processor, alerting, logging)
```

可以全合并成一个 binary（你的系统偏轻量，推荐合并）。

---

# **7. 容器化（Docker）方案（可选）**

你可以采用 Docker 部署：

Docker 优点：

- 环境一致
    
- 部署方便
    
- 日志可统一收集
    

缺点：

- 增加 0.1–0.8ms 延迟
    
- 对超低延迟策略不利
    

你属于轻量高频（子毫秒级，不是纳秒级），因此 Docker 是可接受的。

### 推荐 docker-compose 结构：

```
trading-core
marketdata
config-service
grafana-agent
promtail
redis
webui
```

---

# **8. 配置与密钥管理**

密钥包括：

- Binance API Key / Secret
    
- Telegram Bot Key
    
- WebUI Admin Password
    
- Redis / DB 密码
    
- SSH 私钥
    

建议：

- 所有密钥存入 **.env 文件** 或 **环境变量**
    
- 系统内对 API key 加密存储
    
- 编辑 API key 操作必须写入审计日志
    
- Admin 密码必须复杂，定期轮换
    

---

# **9. 日志与监控集成**

部署环境必须整合：

### 1. Prometheus（export metrics）

暴露 `/metrics`  
采集：

- latency
    
- fill-rate
    
- API errors
    
- WS lag
    
- CPU / Memory
    

### 2. Loki（收集日志）

Promtail 上传结构化 JSON 日志。

### 3. Grafana（可视化）

强烈建议做一个：

**“做市系统总览 Dashboard”**  
包括：

- 行情
    
- 策略
    
- 风控
    
- OMS
    
- PnL
    
- 告警
    

---

# **10. 故障恢复与“自动复活”机制**

关键点：

### **A. 自动重启**

systemd 或 Docker 重启能力。

### **B. 崩溃恢复机制**

策略恢复流程：

```
1. 重新连接 WS
2. 同步账户状态
3. 获取未成交挂单
4. 更新 Inventory
5. 策略进入 warm-up 模式（不立刻挂单）
6. 风控检查
7. 恢复正常运行
```

### **C. PanicStop 之后自动恢复**

步骤：

1. 撤单
    
2. 平仓
    
3. 等待 30 秒
    
4. 恢复策略
    

（可配置）

---

# **11. 灾备（Disaster Recovery）**

灾备意味着：

- VPS 故障
    
- 磁盘损坏
    
- IP 被封
    
- 整个节点不可用
    

此时备用节点必须接管。

### 需要准备：

- 第二台新加坡 VPS
    
- 持续同步配置
    
- 部署同版本程序
    
- 可快速切换 API key（风控机制保护）
    

---

# **12. 标准运维操作流程**

### **上线新版本：**

```
git pull
go build
systemctl stop bot
systemctl start bot
检查日志
检查监控
```

### **下线策略：**

```
webUI → 停止策略
撤单确认
确认无持仓
```

### **重启节点：**

```
systemctl restart bot
```

### **修改参数：**

```
WebUI → edit config → hot reload → check effect → strategy resume
```

---

# **13. 扩展能力**

未来可以扩展：

- 多交易所（Binance + OKX + Bybit）
    
- 多节点统一调度（K8s）
    
- 自动金丝雀部署（灰度策略版本）
    
- A/B 测试策略版本
    
- 分布式市场数据服务
    
- 跨交换机低延迟部署（co-location）
    

最终可以接近：

> “量化机构级自动化集群部署系统”

---

# **下一份文档**

按顺序，下一份将是：

> **《Database & Storage – 数据存储与数据库模块设计文档（v1.0）》**

它将涵盖：

- 实盘数据如何存
    
- 历史 K 线、深度、成交如何组织
    
- 日志、告警、审计如何存档
    
- 回测数据仓库如何构建
    
- 参数与系统状态的数据模型
    
- 未来 AI 训练数据如何设计
    

我可以继续为你写下一份吗？