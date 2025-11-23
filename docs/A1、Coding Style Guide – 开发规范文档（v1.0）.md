  
这是一个成熟团队**必须具备**的基础文档，用来确保：

- 所有成员写出的代码风格一致
    
- 模块命名统一
    
- 项目结构清晰
    
- 避免未来维护困难
    
- 减少潜在 bug 与“隐性风险”
    
- 系统长期稳定可演进
    

---



本开发规范文档适用于你正在构建的**做市商交易系统**，以 Go（Golang）为主要语言，也适用于周边辅助代码（Python/JS 用于回测或工具脚本）。

目标：

- 创建一致、专业、可审阅、可维护的代码风格
    
- 避免重复造轮子
    
- 提高可读性与可测试性
    
- 降低运行时事故风险
    
- 为未来多开发者协作做准备
    

---

# **目录**

1. 语言规范（以 Go 为主）
    
2. 项目结构规范
    
3. 命名规范
    
4. 文件与目录规范
    
5. 接口与模块划分规范
    
6. 错误处理规范
    
7. 日志规范
    
8. 配置与参数规范
    
9. 依赖管理规范
    
10. 并发与通道（channel）规范
    
11. 时间与时区规范
    
12. 测试规范
    
13. 注释与文档规范
    
14. 安全规范
    
15. 版本控制规范（Git Flow）
    
16. 代码审查（Code Review）规则
    
17. 风险敏感模块的额外要求
    

---

# **1. 语言规范（Go 专项要求）**

### ✔ 使用 Go 1.20+

避免使用旧版本语言特性。

### ✔ 禁止使用 `panic()`

除非：

- 程序启动失败（初始化错误）
    
- 明确为不可恢复的错误
    

交易中的 panic → **你的资金会瞬间暴露风险**。

### ✔ 必须开启 race detector 测试

```
go test -race ./...
```

### ✔ 使用 go module，统一版本

禁止在 git 上提交 `vendor`。

### ✔ 严禁使用未检查错误

不得写：

```go
resp, _ := ...
```

这类代码在交易系统中属于严重事故源头。

---

# **2. 项目结构规范（非常重要）**

你的项目建议结构如下：

```
/cmd
    /bot              # 主程序
    /simulator        # 仿真执行器
    /admin            # 管理工具
/pkg
    /gateway          # API Gateway（WS + REST）
    /oms              # 下单与撤单管理
    /strategy         # 策略模块（可多策略）
    /risk             # 风控模块
    /marketdata       # 行情处理
    /state            # 账户、持仓、挂单缓存
    /store            # 数据库封装
    /config           # 配置系统
    /logx             # 日志系统
    /alert            # 告警系统
    /monitor          # metrics 报表
    /webapi           # WebUI 后端
    /common           # 公共工具（数学、时间、ID 等）
/internal             # 私有逻辑（不对外暴露）
/scripts              # 部署脚本
/testdata             # 单测样本
```

目的：

- 结构清晰
    
- 模块隔离
    
- 方便未来拆成微服务
    

---

# **3. 命名规范**

适用于变量、函数、目录、文件。

### ✔ 全局命名标准：

- 模块名：小写 + 无下划线（如 `gateway`, `oms`）
    
- 结构体：驼峰式（如 `OrderEvent`, `AccountSnapshot`）
    
- 接口：行为式名称，通常以 `er` 结尾（如 `Fetcher`, `Dispatcher`）
    
- 常量：全大写 + 下划线（如 `MaxOrderRetry`）
    
- 函数：驼峰式，且必须表达动作（如 `PlaceOrder`, `SyncAccount`）
    
- 变量：短名但可读（如 `pos`, `cfg`, `ts`）
    

### ✔ 订单相关命名统一：

```
order_id        string
client_order_id string
exchange_order_id string
```

### ✔ 账户与持仓统一命名：

```
balance
equity
position
avg_price
unrealized_pnl
realized_pnl
```

### ✔ 行情统一命名：

```
bid
ask
mid
spread
depth
volatility
imbalance
```

一致性非常重要。

---

# **4. 文件与目录规范**

## ✔ 每个模块必须有自己的 README.md

例如 `/oms/README.md` 包含：

- 模块目的
    
- 接口说明
    
- 状态机
    
- 示例
    

## ✔ 单文件不超过 800 行

策略文件建议保持在 300–600 行。

## ✔ 目录按业务而非技术拆分

正确示例：

```
/strategy/phase1_maker
```

错误示例：

```
/services
```

---

# **5. 接口与模块划分规范**

### ✔ 策略不得直接调用 Binance API

必须调用：

```
gateway.PlaceOrder()
gateway.Cancel()
state.GetPosition()
```

### ✔ 风控不能调用策略

风控是上层控制层。

### ✔ OMS 是唯一能发订单的模块

### ✔ 市场数据必须通过 marketdata 模块清洗后提供

### ✔ 风控不能依赖 WebUI

---

# **6. 错误处理规范**

交易系统最忌讳发生“默默失败”。  
错误分三类：

---

## **A. 可恢复错误（retryable）**

例如：

- 网络超时
    
- REST 429
    
- 系统繁忙
    

处理：

```
重试最多 3 次 → 仍失败 → 通知 OMS → 发告警
```

---

## **B. 业务错误（不可恢复）**

- 保证金不足
    
- 下单价格不合法
    
- 参数越界
    

处理：

```
错误日志 + 风控阻断
```

---

## **C. 严重错误（必须触发 PanicStop）**

- 下单后订单状态丢失
    
- 持仓与订单状态不一致
    
- 风控无法运行
    
- WS 深度无法恢复
    
- 关键缓存失效
    

处理：

```
触发风险保护：cancel_all + close_position
```

---

# **7. 日志规范**

### ✔ 每条日志必须是 JSON（结构化）

禁止！：

```
fmt.Println("Order failed!!!")
```

### ✔ 日志必须包含以下字段：

- ts（时间戳）
    
- module
    
- level
    
- event
    
- meta（json）
    

### ✔ 策略日志必须可控制 granularity（粒度）

### ✔ 日志必须分 module 存储（前文已定义）

---

# **8. 配置与参数规范**

### ✔ 所有参数必须通过 configService 管理

禁止 hardcode。

### 配置文件必须遵循：

```
YAML 格式
分模块配置
每个字段必须有注释
支持热更新
支持版本控制
```

示例：

```yaml
maker:
  spread_ticks: 3     # 基础挂单间距
  grid_levels: 5      # 网格深度
risk:
  max_inventory: 2.0
  max_daily_loss: 0.03
```

---

# **9. 依赖管理规范**

### ✔ 必须使用 Go module

### ✔ 每个依赖必须 pin 版本

### ✔ 不使用奇怪的小众库

### ✔ 不使用不稳定的“magic 库”

你主要使用：

- websocket
    
- REST HTTP
    
- zap/logger
    
- prometheus client
    
- sqlite/postgres driver
    

---

# **10. 并发与 channel 规范**

### ✔ 禁止 goroutine 泄漏

必须有退出机制。

### ✔ channel 必须明确方向和缓冲区大小

正确：

```go
orderEvents := make(chan OrderEvent, 500)
```

错误：

```go
orderEvents := make(chan OrderEvent)
```

### ✔ 所有长循环必须有 `ctx.Done()`

例如：

```go
select {
case msg := <-ch:
case <-ctx.Done():
    return
}
```

---

# **11. 时间与时区规范**

### ✔ 全系统统一使用 UTC

### ✔ 所有 timestamp 必须是 `time.Time`

### ✔ 不允许手写时间戳 int（易造成事故）

### ✔ 与 Binance 交互必须做时间校准（前文 API Gateway 已定义）

---

# **12. 测试规范**

### ✔ 单元测试覆盖：

- strategy module（最重要）
    
- risk engine
    
- oms
    
- gateway mock
    
- state cache
    

### ✔ 不对 Binance 实盘接口做 CI 测试

必须用 mock。

### ✔ 所有 panic 都必须有测试覆盖（确保不会触发）

### 测试风格：

- table-driven
    
- 快速
    
- 幂等
    
- 无网络依赖
    

---

# **13. 注释与文档规范**

- 每个模块必须有 README
    
- 每个导出方法必须有注释
    
- 每个策略必须有策略说明
    
- 每段复杂逻辑必须注释“为什么这么做”（not what）
    

---

# **14. 安全规范**

### ✔ API Key 绝对不能写入代码库

必须使用环境变量或加密文件。

### ✔ 禁止在日志中打印 key/secret

必须 mask：

```
AK-******************
```

### ✔ 所有写操作必须记录审计日志

来源：WebUI / opskit / 管理 API

### ✔ WebUI 必须有权限控制

参考你之前的《User & Permission》文档。

---

# **15. 版本控制规范（Git Flow）**

强烈建议使用如下流程：

```
main —— 稳定可上线分支
dev —— 开发主分支
feature/* —— 新功能
hotfix/* —— 线上紧急修复
release/* —— 上线准备
```

规则：

- 所有代码必须通过 Pull Request
    
- PR 必须由他人或你自己第二次审阅（至少 1 次 review）
    
- main 分支禁止直接 push
    

---

# **16. 代码审查（Code Review）规则**

重点检查：

- 有没有隐藏 panic
    
- 是否有未处理 error
    
- 是否有 goroutine 泄漏
    
- 是否有不合理的 channel 操作
    
- 是否所有模块遵循依赖方向（不能倒依赖）
    
- 风控边界是否安全
    
- OMS 是否有幂等保障
    
- 日志是否结构化
    
- 是否遵守命名规范
    

每个 PR 必须：

- 测试通过
    
- linter 通过（使用 golangci-lint）
    
- 风控逻辑复核
    
- 策略逻辑复核
    
- 性能敏感模块必须标注时间复杂度/延迟
    

---

# **17. 风险敏感模块的额外要求**

下列模块属于“高风险模块”，必须额外审查：

### ✔ Risk Engine

- 不能混入策略逻辑
    
- 出错必须触发告警
    

### ✔ OMS

- 必须保证订单幂等
    
- 不允许未跟踪的订单存在
    
- WS 状态与本地状态必须一致
    

### ✔ API Gateway

- 下单/撤单必须支持重试
    
- 断线必须自动恢复
    
- 必须有全量同步机制
    

### ✔ Strategy

- 不得直接管理订单
    
- 不得直接调用 Binance API
    
- 必须保证 CPU / 内存稳定
    
- 参数必须可审计
    

---

# **结语**

此《Coding Style Guide》文档是你整个做市商系统的工程基础，  
配合之前的：

- 风控
    
- OMS
    
- MarketData
    
- API Gateway
    
- 系统架构
    
- 开发流程
    
- 配置系统
    

你的系统现已具备：

> **专业量化交易团队的完整工程体系。**

---

# 下一步（可选方向）

如果你愿意，我可以继续帮助你写：

### ✔ 《产品规划手册（Product Planning Manual）》

用于：路线图、功能范围、优先级、版本计划。

### ✔ 《开发迭代计划（Roadmap）》

用于：规划未来 3 个月 / 6 个月的开发阶段。

### ✔ 《Go 项目骨架（项目目录 + 初始化代码）》

直接一键生成你的做市商系统基础代码结构。

你想继续哪一部分？