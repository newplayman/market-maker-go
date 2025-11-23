

> **说明：**  
> 这份任务清单专为 AI Agent（Cursor、Copilot Workspace、AutoGen、LangGraph）设计，所有模块均拆分为「实现目标 → 细分任务 → 交付产物」。

---

```markdown
# 模块任务清单（Module Task Breakdown）

本文件提供整个做市系统的「任务树」，可供多 Agent 按模块并行开发。

---

# 任务结构说明

每个模块包含：

- **Goal**：模块目标  
- **Tasks**：细分任务（适合 Agent 拆分与执行）  
- **Deliverables**：最终产物（文件、测试、接口等）  
- **Dependencies**：依赖关系  

---

# 1. Market Data Aggregator

## Goal
实现统一的深度、交易、Ticker、指数价格行情聚合器。

## Tasks
- [ ] 定义 `MarketSnapshot` 数据结构
- [ ] 实现 Binance Websocket 行情订阅
- [ ] 处理深度增量合并
- [ ] 处理 trade, ticker, index/mark price
- [ ] 输出统一 snapshot（频率：20ms~50ms）
- [ ] 开发 mock 数据源用于测试

## Deliverables
- `/internal/marketdata/aggregator.go`
- `/internal/marketdata/binance_ws.go`
- `/tests/marketdata_test.go`

## Dependencies
无（基础模块）

---

# 2. Signal Engine

## Goal
基于市场数据生成做市信号。

## Tasks
- [ ] 定义 `Signal` 结构体
- [ ] 实现 Orderbook Imbalance (OBI)
- [ ] 实现微观结构趋势信号
- [ ] 实现成交量加权信号
- [ ] 实现 mark/index 偏移检测
- [ ] 单测覆盖率 80%+

## Deliverables
- `/internal/signal/engine.go`
- `/tests/signal_test.go`

## Dependencies
- Market Data

---

# 3. Strategy Engine

## Goal
根据信号产生做市挂单意图（Intent）。

## Tasks
- [ ] 定义 `Intent`
- [ ] 指定挂单距离（tick offset）
- [ ] 指定挂单数量
- [ ] 实现撤单条件
- [ ] 支持简单 inventory 风险修正
- [ ] 单测覆盖率 85%+

## Deliverables
- `/internal/strategy/engine.go`
- `/tests/strategy_test.go`

## Dependencies
- Signal Engine
- Portfolio（库存修正）

---

# 4. Risk Engine

## Goal
风控拦截非法、过度风险、越界的 Intent。

## Tasks
- [ ] 定义 `RiskCheckResult`
- [ ] 仓位上限检查
- [ ] 单笔下单上限检查
- [ ] 高波动熔断
- [ ] 连续亏损中止逻辑
- [ ] Notional 限制
- [ ] 单测覆盖率 90%+

## Deliverables
- `/internal/risk/risk_engine.go`
- `/tests/risk_test.go`

## Dependencies
- Portfolio
- Config

---

# 5. OMS（订单管理系统）

## Goal
提供稳定、安全、可追踪的订单生命周期管理。

## Tasks
- [ ] 定义 `OrderRequest`, `OrderState`
- [ ] 实现下单请求队列
- [ ] 实现撤单逻辑
- [ ] 跟踪订单状态（使用 Adapter 回调）
- [ ] 支持“无重复下单”保护（idempotency）
- [ ] 写全套单元测试（覆盖率 95%）

## Deliverables
- `/internal/oms/oms.go`
- `/tests/oms_test.go`

## Dependencies
- Risk Engine
- Strategy Engine
- Adapter

---

# 6. Exchange Adapter（Binance）

## Goal
提供 Binance Futures 的 REST/WS 封装，实现稳定的下单、撤单、回报机制。

## Tasks
- [ ] 定义下单/撤单 API
- [ ] REST 发送带签名请求
- [ ] Websocket 订单回报
- [ ] 限速器（RateLimiter）
- [ ] 自动补偿机制（REST fallback）
- [ ] 心跳与重连策略

## Deliverables
- `/internal/adapter/binance/rest.go`
- `/internal/adapter/binance/ws.go`
- `/tests/adapter_binance_test.go`

## Dependencies
无（外部系统）

---

# 7. Portfolio / PnL

## Goal
提供实时持仓、余额、杠杆、未实现/已实现盈亏信息。

## Tasks
- [ ] 同步账户余额
- [ ] 维护本地仓位状态
- [ ] 计算 unrealized pnl
- [ ] 计算 realized pnl
- [ ] 保存手续费数据

## Deliverables
- `/internal/portfolio/portfolio.go`
- `/tests/portfolio_test.go`

## Dependencies
- Adapter

---

# 8. Backtest & Simulation

## Goal
提供回测框架用于策略验证和风控验证。

## Tasks
- [ ] 构建 tick-by-tick 撮合引擎
- [ ] 实现撮合逻辑（maker/taker 封装）
- [ ] 读取历史行情进行信号→策略→风控路径回放
- [ ] 输出回测报告

## Deliverables
- `/internal/backtest/backtest.go`
- `/tests/backtest_test.go`

## Dependencies
- Strategy
- Risk
- OMS（模拟版）

---

# 9. Config

## Goal
提供统一的配置加载与热更新支持。

## Tasks
- [ ] 初始化全局配置
- [ ] yaml & env 组合加载
- [ ] 提供专门结构体用于 risk、strategy、adapter

## Deliverables
- `/internal/config/config.go`

---

# 10. Web Dashboard（可选）

## Goal
展示实时行情、订单、持仓、PnL、风险指标。

---

# 附：Agent 使用建议

- 每个模块可以由不同 Agent 负责，互不覆盖。
- 所有模块均基于 interface + mockable 设计。
- 优先完成 MarketData → Signal → Strategy → Risk → OMS → Adapter。

---

```
