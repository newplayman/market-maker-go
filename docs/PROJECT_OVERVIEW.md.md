
下面我为你的做市系统生成两份可以直接放进项目仓库的文件：

- **`PROJECT_OVERVIEW.md`**（项目总览）
    
- **`MODULE_TASKS.md`**（模块任务清单 — 专门为多 Agent IDE/Agent Framework 使用优化过）
    

两份文档都遵循：

- **模块化结构**（符合你 Go 做市系统架构）
    
- **Agent 友好格式**（Cursor / Copilot Workspace / LangGraph / AutoGen 都能轻松解析）
    
- **完整但高度结构化**（便于 Agent 自动拆任务、生成代码和测试）
    

---

# **📄 PROJECT_OVERVIEW.md**

```markdown
# 项目总览：轻型高频做市系统（Lightweight HFT Market Maker）

本项目旨在构建一个 **模块化、高性能、可扩展** 的轻型高频做市系统，专注于以 USDC 永续合约市场（例如 Binance Futures）为核心，实现低成本挂单流动性提供与低延迟风控调度。

系统采用 **Go 语言** 实现，具备实时信号、策略执行、风控拦截、OMS、交易所适配器、行情聚合、回测框架等完整基础设施。

---

## 🚀 系统愿景

- 轻量级替代完整做市系统的 MVP 框架，但保留 **速度/安全/稳定性**。
- 支持 **多策略并行**、**低延迟下单**、**精简版持仓管理**。
- 为未来的 **模块 AI Agent 自动开发** 提供标准化接口 & 文档。

---

# 1. 系统架构（高层视图）

```

```
         ┌────────────┐
         │ Market Data │  ← Websocket / Rest
         └──────┬─────┘
                │
      ┌─────────▼─────────┐
      │   Data Aggregator  │
      └─────────┬─────────┘
                │
       ┌────────▼────────┐
       │   Signal Engine  │
       └────────┬────────┘
                │
        ┌───────▼────────┐
        │ Strategy Engine │
        └──────┬─────────┘
                │
       ┌────────▼────────┐
       │     Risk Engine  │
       └────────┬────────┘
                │
       ┌────────▼────────┐
       │       OMS       │
       └────────┬────────┘
                │
   ┌────────────▼─────────────┐
   │   Exchange Adapters (Binance…)  │
   └────────────┬─────────────┘
                │
       ┌────────▼────────┐
       │ Portfolio/PNL   │
       └─────────────────┘
```

```

---

# 2. 模块说明

## 2.1 Data Aggregator（行情聚合）
- 订阅深度、Ticker、Trades、Mark/Index Price。
- 可选数据源：Binance、OKX、其他。
- 输出统一格式的 `MarketSnapshot`.

## 2.2 Signal Engine（信号引擎）
- 读取 MarketSnapshot。
- 计算：微观结构信号、价格偏差、订单簿失衡（OBI）、成交量信号。
- 输出 `Signal` 对象。

## 2.3 Strategy Engine（策略引擎）
- 根据 Signal 决定：
  - Quote 方向
  - 挂单间距（tick offset）
  - 挂单数量
  - 是否撤单
- 输出 `Intent`（策略意图）。

## 2.4 Risk Engine（风险控制）
- 实时检查：
  - 仓位上限（notional limit）
  - 单笔下单上限
  - 连续亏损中止（drawdown）
  - 资金占用上限
  - 高波动熔断
- 输出 `ApprovedIntent` 或拦截。

## 2.5 OMS（订单管理系统）
- 统一管理订单生命周期：
  - 创建任务
  - 去重
  - track order 状态
  - 自动撤单
- 与 Adapter 解耦，保证可测试性。

## 2.6 Exchange Adapter（交易所适配器）
- 目前默认实现 Binance Futures：
  - REST 下单/撤单
  - WS 实时订单回报
  - 控制流（限速、签名、补偿机制）

## 2.7 Portfolio / PnL（账户与持仓）
- 持仓同步
- unrealized PnL / realized PnL
- 杠杆、余额、margin ratio

## 2.8 Backtest & Simulation（回测/模拟）
- 基于市场数据复现行情
- 支持：
  - 策略回测
  - 风控验证
  - OMS 行为模拟

---

# 3. Go 项目结构（推荐）

```

/cmd/maker/main.go

/internal/  
marketdata/  
signal/  
strategy/  
risk/  
oms/  
adapter/binance/  
portfolio/  
backtest/  
utils/  
config/

/configs/  
/docs/  
/scripts/  
/tests/

/web/ (仪表盘)  
/deploy/ (docker-compose, kubernetes)

```

---

# 4. 开发原则

- 所有模块均提供 **interface**。
- 所有关键逻辑必须具备 **mockable** 能力。
- 测试覆盖率要求：
  - Signal：80%+
  - Strategy：85%+
  - Risk：90%+
  - OMS：95%+
- 禁止在核心模块写入任意日志（使用 event bus）。

---

# 5. 未来扩展

- 多交易所套利
- 机器学习信号
- 多 Agent 决策
- 实盘监控中心
- 回测可视化 UI

---

# 6. 文档结构

- `PROJECT_OVERVIEW.md`
- `MODULE_TASKS.md`
- `API_SPEC.md`
- `RISK_RULES.md`
- `STRATEGY_SPEC.md`
- `OMS_SPEC.md`
- `EXCHANGE_ADAPTER_SPEC.md`

```

本文件作为整个项目的统一入口与架构说明。

---
