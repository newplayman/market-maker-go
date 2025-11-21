# 数据示例

- `mids_sample.csv`：脚本生成的 100 条随机 mid 价格，可用于 `go run ./cmd/backtest data/mids_sample.csv` 演示回测。
- 你可以将历史行情（只含 mid 列或第一列为 mid）的 CSV 放入此目录，用同样命令回放。

回测脚本入口：`cmd/backtest/main.go`，策略参数可在代码中调整（MinSpread、BaseSize 等）。
