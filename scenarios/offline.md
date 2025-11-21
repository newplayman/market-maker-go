# 离线演示与回归清单

1) 单测 + 演示脚本（已在 `scripts/test_all.sh` 封装）
   - `go test ./...`
   - `go run ./cmd/sim` （支持参数调参：spread/baseSize/风控限额/频率等）
   - `go run ./cmd/backtest data/mids_sample.csv`

2) 自定义 Runner 仿真
   - 在代码中使用 `sim.BuildRunner(sim.RunnerConfig{...})` 组合策略、风控（限额/VWAP/PnL/频率）和订单模块，驱动本地行情序列进行测试。

3) 数据回放
   - 将历史 mid 价格 CSV 放入 `data/`，使用 `go run ./cmd/backtest <csv>` 回放，观察报价曲线。

4) 风控组合验证
   - 使用 `risk.BuildGuards` 组合限额 + VWAP + PnL + 频率，结合 OrderBook/Mid 函数，编写表驱动物理测试或在 Runner 中调用。

以上流程不依赖外网，可在本地重复执行，确保核心逻辑与风控组合行为稳定。<Real部署> 待有网络后再在 VPS 上接入 Binance REST/WS 进行实盘联调。***
