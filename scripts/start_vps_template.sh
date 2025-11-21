#!/usr/bin/env bash
set -euo pipefail

# VPS 启动模板：加载 .env（如果存在），然后运行 sim/backtest/或真实入口（后续可替换）

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [ -f .env ]; then
  set -a
  source .env
  set +a
fi

echo "[VPS] go test ./..."
go test ./...

echo "[VPS] run sim (mock gateway)..."
go run ./cmd/sim

echo "[VPS] backtest sample..."
if [ -f data/mids_sample.csv ]; then
  go run ./cmd/backtest data/mids_sample.csv
else
  echo "no sample data, skip backtest"
fi

echo "[VPS] done. 若要接入实盘，请替换入口为真实 WS/REST 客户端。"
