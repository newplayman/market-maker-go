# Market Maker Go (Binance USDC-M Skeleton)

本仓库包含做市核心模块的骨架代码与模拟入口，当前重点聚焦 Binance USDC 永续、WebSocket 行情、零库存/网格策略与基础风控。

## 快速开始

```bash
cd /Users/bendu/market-maker-go
go test ./...    # 确认代码可编译并通过单测
go run ./cmd/sim # 本地随机行情模拟，不会连接交易所（支持参数调参，见下）
go run ./cmd/backtest mids.csv # 以 CSV mid 价格回放，生成报价
./scripts/test_all.sh # 依次跑单测、sim、backtest 示例
./scripts/start_local.sh # 同上，便捷本地回归
```

配置示例：`configs/config.example.yaml`（填入 API Key/Secret、风险参数等）。

## 模块概览
- `strategy/`：零库存报价、动态网格、动态价差，支持回测接口。
- `risk/`：单笔/日累计/净敞口限额，波动熔断，告警通知。
- `order/`：订单状态管理，可插拔 Gateway。
- `gateway/`：Binance REST/WS 抽象，签名、listenKey 管理，Stub/测试客户端。
- `market/`：深度/成交归一化，内存订单簿，中间价与陈旧度计算，发布订阅。
- `inventory/`：仓位跟踪、估值、快照。
- `sim/`：轻量 Runner 串联策略→风控→下单，用于本地演示。
- `cmd/sim/`：可执行程序入口，随机生成 mid 价格走一遍链路。
- `cmd/backtest/`：读取 CSV mid 价格序列，调用策略回测接口输出报价曲线。

## 环境变量（联网时）
- `BINANCE_API_KEY` / `BINANCE_API_SECRET`
- `BINANCE_REST_URL`（默认 https://fapi.binance.com）
- `BINANCE_WS_ENDPOINT`（默认 wss://fstream.binance.com）
- `LISTEN_KEY`（用户数据流，可选，供 WS demo 使用）
- 其余通用配置可参考 `.env` / `configs/config.example.yaml`

## 数据与回测
- 示例数据：`data/mids_sample.csv`（100 条 mid），用 `go run ./cmd/backtest data/mids_sample.csv` 回放。
- 可将自有历史 mid CSV 放入 `data/`，同样命令回放。

## 接入 Binance 的提示
- 实盘需替换 `gateway.BinanceRESTStub/BinanceRESTClient` 中的 HTTP 客户端配置，并实现真实 WS 客户端（深度、成交、用户数据流 listenKey；含心跳与断线重连）。
- 下单需符合 Binance USDC 永续参数（timeInForce/GTX、reduceOnly、精度），并处理 429/418 等限流。
- 建议先用 Stub 与 `sim.Runner` 完成端到端自测，再接入真实 API。

### WS demo（需要网络）
`scripts/run_ws_demo.sh` 可快速订阅深度流（和可选 listenKey）：
```bash
cd /Users/bendu/market-maker-go
BINANCE_WS_ENDPOINT="wss://fstream.binance.com" LISTEN_KEY="your_listen_key" ./scripts/run_ws_demo.sh BTCUSDT
```
只看公共深度时，可以不设置 `LISTEN_KEY`。

### sim 参数示例
```bash
go run ./cmd/sim -symbol BTCUSDT -ticks 10 -minSpread 0.0008 -baseSize 1 \
  -singleMax 3 -dailyMax 30 -netMax 8 -latencyMs 200
```
可调整策略（价差、基础量、目标仓位等）与风控（单笔/日累计/净敞口、下单最小间隔）。
