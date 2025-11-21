# 测试计划（离线可执行）

## 回归套件
- 脚本：`./scripts/test_all.sh` 或 `./scripts/start_local.sh`
- 覆盖：`go test ./...` + sim + backtest (sample data)

## 风控组合用例
- 限额：single/daily/net 正常与超限
- VWAP 守卫：价差阈值触发拒单
- 频率限制：minInterval 触发拒单
- PnL 守卫：亏损/盈利过阈值触发拒单
- MultiGuard：组合限额 + VWAP + 频率 + PnL

## 行情与订单簿
- OrderBook 大批量增量更新（价格删除、更新）
- KlineAggregator 跨周期生成
- Publisher/Service 订阅与中间价/陈旧度

## 策略
- QuoteZeroInventory 漂移/波动调价
- DynamicGrid 构建对称网格
- 回测：CSV mid 回放，统计 min/max/mean/drawdown

## WS/REST 骨架
- Binance 签名/ListenKey/WS 解析单元测试（已在 go test）
- WS handler 解析 depth 原始消息（离线 JSON）

## 扩展/占位
- 可加入表驱动测试覆盖更多边界（零价格、负 drift 等）
- 实盘联调后补充集成测试，收集回归样例
