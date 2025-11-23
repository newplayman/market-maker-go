

这个模块用于控制 WebUI / 管理系统 / 内部 API 的 **访问权限与操作权限**。

虽然你的做市商系统主要是你自己或技术团队使用，但为了保证资金安全、策略稳定性、防止误操作或被攻击带来的巨大损失，**用户权限体系必须专业且严格**。

本份文档可直接作为后台权限系统开发蓝图。

---

# **目录**

1. 模块目标
    
2. 核心设计原则
    
3. 用户角色模型
    
4. 权限体系（粒度设计）
    
5. 功能访问矩阵（角色 → 权限）
    
6. 认证 Authentication
    
7. 授权 Authorization
    
8. Token / Session 管理
    
9. 审计日志（Audit Log）
    
10. 安全要求
    
11. 系统 API 设计
    
12. 扩展能力
    

---

# **1. 模块目标**

User & Permission 模块的目标：

- 管理系统用户
    
- 控制每个用户可以执行哪些操作
    
- 保护实盘资金
    
- 防止错误操作（例如误平仓）
    
- 对敏感行为进行二次确认
    
- 记录所有用户操作（审计日志）
    

一句话：

> **这是整个做市系统的“防火墙”，确保只有正确的人能做正确的事”。**

---

# **2. 核心设计原则**

你的系统涉及资金、策略、风控等高敏感内容，因此权限设计必须遵循：

### ① 最小权限原则（Least Privilege）

用户只能执行自己所需的功能。

### ② 分级权限（Critical / High / Normal）

例如：

- Critical：平仓、修改 API 密钥、调整风险参数
    
- High：修改策略参数、启动/停止策略
    
- Normal：查看监控、查看订单
    

### ③ 所有写操作必须可审计（100% traceable）

任何敏感操作必须记录：

- 谁
    
- 什么时候
    
- 做了什么
    
- 参数变更前后状态
    
- IP 地址
    

### ④ 管理员权限必须受保护

即使是你自己操作，也必须有严格的确认机制。

---

# **3. 用户角色模型**

推荐基础角色模型：

---

## **(1) Admin（平台管理员）**

拥有系统最高权限，可执行：

- 修改策略配置
    
- 修改风险参数
    
- 修改交易所 API key
    
- 热更新参数
    
- 手动平仓
    
- 停止策略
    
- 管理用户
    
- 查看所有日志、告警
    

Admin 应该 **极少使用**，仅用于关键动作。

---

## **(2) Trader / Operator（运营人员）**

可执行：

- 启动/停止策略
    
- 调整低风险参数
    
- 观察运行状态
    
- 查看订单与资金
    
- 查看风险指标
    
- 不允许修改高风险参数（如 API key、MaxDailyLoss）
    

适合日常运营。

---

## **(3) ReadOnly（观察员）**

只能查看：

- 监控面板
    
- 挂单
    
- 成交
    
- 边际 risk
    
- 账户资金
    

适合移动监控、外部合作伙伴、团队成员。

---

## **(4) Automation Account（自动化系统账户）**

专用于：

- 自动报表
    
- 自动巡检
    
- 自动调参（未来 AI）
    

权限仅限 API 层面的“读”和预定义操作。

---

## **(5) Future: Multi-Agent Strategy Roles（未来 AI 决策需要）**

允许多个 AI 或模型访问部分接口，例如：

- 审阅行情
    
- 获取库存
    
- 生成参数建议
    
- 但不能直接操作交易
    

你未来的“多模型辩论决策系统”会用到这个。

---

# **4. 权限体系（粒度设计）**

权限粒度建议分为 5 类：

|类别|示例权限|
|---|---|
|**系统配置权限**|修改配置、热更新、重启服务|
|**策略权限**|启动/停止策略、修改策略参数|
|**订单权限**|下单、撤单、紧急平仓|
|**账户权限**|查看余额、查看仓位|
|**访问权限**|查看监控页面、接口访问|

---

## 推荐权限列表（可扩展）：

```
view_dashboard
view_orders
view_trades
view_marketdata
view_logs
view_risk
view_config
edit_config
edit_strategy
edit_risk
open_position
close_position
cancel_all_orders
strategy_start
strategy_stop
admin_user_manage
admin_modify_api_key
```

系统内所有接口必须绑定权限点。

---

# **5. 功能访问矩阵（角色 → 权限）**

### **Admin**

|权限|状态|
|---|---|
|view_*|✔|
|edit_*|✔|
|strategy_start/stop|✔|
|cancel_all_orders|✔|
|close_position|✔|
|admin_user_manage|✔|
|admin_modify_api_key|✔|

---

### **Trader / Operator**

|权限|状态|
|---|---|
|view_*|✔|
|edit_strategy|✔|
|edit_config（低风险项）|✔ 限制版|
|strategy_start/stop|✔|
|cancel_all_orders|✔|
|close_position|✔|
|admin_modify_api_key|❌|

---

### **ReadOnly**

|权限|状态|
|---|---|
|view_*|✔|
|edit_*|❌|
|strategy_start/stop|❌|
|cancel_all_orders|❌|
|close_position|❌|

---

### **Automation Account**

自定义最少权限。

---

# **6. 认证 Authentication**

推荐使用：

- JWT（短期 token）
    
- Refresh Token（用于保持登录状态）
    
- HTTPS（必须）
    
- IP 白名单（对 Admin 可开启）
    
- 可选：2FA（二步验证）
    

JWT payload 示例：

```json
{
  "user_id": 1,
  "role": "admin",
  "permissions": ["view_dashboard", "edit_config", ...],
  "exp": 1700000000
}
```

---

# **7. 授权 Authorization**

授权过程：

1. 用户访问 API
    
2. 后端解析 JWT
    
3. 提取角色与权限
    
4. 检查该 API 所需权限
    
5. 决定允许或拒绝
    

伪代码：

```go
if !user.HasPermission("strategy_stop") {
    return HTTP 403
}
```

策略、风控、OMS 的关键操作必须绑定权限。

---

# **8. Token / Session 管理**

必须包括：

- token 黑名单（支持注销）
    
- token 过期策略（如 8 小时）
    
- refresh token 具备更长寿命（3 天）
    
- 每次敏感操作要求重新验证 token（可选）
    

---

# **9. 审计日志（Audit Log）**

所有敏感操作必须写入审计日志：

- 登录、退出
    
- 参数修改
    
- 风控修改
    
- 手动平仓
    
- 撤单
    
- 启动/停止策略
    
- 修改 API key
    
- 调整 MaxInventory
    

Audit Log 结构：

```json
{
  "time": "...",
  "user": "admin",
  "role": "admin",
  "action": "edit_config",
  "target": "BTCUSDC",
  "before": {...},
  "after": {...},
  "ip": "123.45.67.89"
}
```

不可删除（只能追加）。

---

# **10. 安全要求**

由于系统与资金直接相关，必须重视安全性：

### 必须的：

- 全站 HTTPS
    
- JWT + 强密码策略
    
- 随机盐
    
- 密码哈希（bcrypt）
    
- 防暴力登录（rate limit）
    
- 管理员登录报警
    
- 参数修改报警
    
- 二次确认（critical 操作）
    
- IP 白名单（管理员）
    
- API key 加密存储
    

### 推荐的：

- YubiKey / 2FA
    
- 行为分析（异常登录检测）
    
- 操作时间限制（夜晚禁止高风险操作）
    

---

# **11. 系统 API 设计**

关键接口：

---

## 用户管理

```
POST /api/user/login
POST /api/user/logout
GET  /api/user/info
GET  /api/user/list   (admin)
POST /api/user/create (admin)
POST /api/user/update (admin)
POST /api/user/delete (admin)
```

---

## 权限列表

```
GET /api/user/permissions
```

---

## 角色管理（可选）

```
GET /api/role/list
POST /api/role/update
```

---

## 审计日志

```
GET /api/audit
GET /api/audit/:id
```

---

# **12. 扩展能力**

未来可扩展：

- RBAC → ABAC（基于属性的权限控制）
    
- 每个 symbol 单独授权（例如 BTC 只给某个 Trader）
    
- 为 AI 多策略提供虚拟账户权限
    
- 单操作限额（例如每次只能修改参数 5%）
    
- 冲突检测（多人同时修改）
    
- 操作审批流程（需要另一个管理员批准）
    

你的系统将逐渐接近 **机构级量化交易系统**。

---

# 下一份文档

按顺序，下一份模块是：

> **《Logging & Audit – 日志与审计系统设计文档（v1.0）》**

这是用于策略回溯、错误定位、风险追踪、事后分析的重要模块，  
也是做市商系统中不可或缺的一部分。

我继续写下一份吗？