# 联网后待完成的事项（VPS/实盘联调）

1) Binance REST/WS 实测
   - 配置 `BINANCE_API_KEY/SECRET/BINANCE_REST_URL/BINANCE_WS_ENDPOINT`
   - listenKey 生命周期验证：创建、保活、关闭；打通用户数据流
   - 深度 combined stream：断线重连、快照校验、增量应用至 OrderBook
   - 下单接口：timeInForce/GTX、reduceOnly、精度、429/418 限流重试

2) 实盘日志与监控
   - 统一日志格式（字段：symbol, side, price, size, pnl, guard_hit 等）
   - 采集 WS/REST 错误、重连、延迟指标；预留 Prometheus 指标名

3) 策略/风控调优
   - 根据实盘行情调节 minSpread、网格密度、频率限制
   - 风控守卫（VWAP、PnL、频率）联动实盘数据，验证拒单/限流效果

4) 部署脚本
   - VPS 上的启动脚本（读取 .env/config.yaml、日志目录）
   - 备份/回滚脚本、运行状况检查

5) 回测/仿真数据对齐
   - 将实盘抓取的行情存入 data/，复盘与策略参数对比

以上事项需有网络环境和 API 白名单，届时在 VPS 上执行。当前本地离线逻辑与测试已就绪，可直接迁移。*** End Patch
