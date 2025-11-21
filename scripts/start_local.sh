#!/usr/bin/env bash
set -euo pipefail

# 本地快速运行：单测 + sim + backtest
cd "$(dirname "$0")/.."

echo "[1/3] go test ./..."
go test ./...

echo "[2/3] run sim (mock gateway)..."
go run ./cmd/sim

echo "[3/3] backtest sample data..."
if [ -f data/mids_sample.csv ]; then
  go run ./cmd/backtest data/mids_sample.csv
else
  echo "no sample data, skip backtest"
fi
