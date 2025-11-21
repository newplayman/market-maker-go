# TODO Roadmap (离线/联网)

## 已完成 ✔
- [x] 核心模块骨架：strategy/risk/order/market/inventory/sim/backtest，`go test ./...` 通过。
- [x] 风控组合：限额、VWAP、频率、PnL 守卫，可用 `risk.BuildGuards` 组合。
- [x] Binance 抽象：REST 签名骨架、listenKey 管理、WS 骨架与深度解析、demo 脚本。
- [x] 本地演示脚本：`./scripts/test_all.sh`、`cmd/sim`、`cmd/backtest`；示例数据 `data/mids_sample.csv`。
- [x] 里程碑/离线/在线说明：`scenarios/offline.md`、`scenarios/todo_online.md`、README。

## 待办（可离线推进） ☐
- [ ] 细化策略参数映射到 config：最小价差、网格密度、目标仓位、频率等，增加校验与示例。
- [ ] 扩展回测工具：支持多符号、输出统计（最大回撤、PnL 估计）、表格/CSV 导出。
- [ ] 日志/监控接口占位：统一日志字段、预留 Prometheus 指标名，编写接口/测试（不需联网）。
- [ ] 更多单测/场景：策略（漂移/波动场景）、风控组合（边界条件）、订单簿（大批量增量）。
- [ ] 部署/运行脚本占位：本地/VPS 启动脚本模板，环境变量读取、日志目录准备。

## 待办（需联网/VPS） ☐
- [ ] Binance REST/WS 实盘联调：listenKey 生命周期、深度快照+增量校验、用户流回报解析、限流重试。
- [ ] 实盘下单参数与精度校验：timeInForce/GTX、reduceOnly、精度/步长限制，429/418 重试与退避。
- [ ] 实盘日志与监控：采集 WS/REST 错误、重连、延迟；Prometheus/日志收集。
- [ ] 实盘策略/风控调优：根据实盘行情调节策略参数、风控阈值，验证拒单/限流效果。
