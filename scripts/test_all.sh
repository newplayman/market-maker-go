#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
echo "Running go test ./..."
go test ./...

echo "Running sim (mock gateway)..."
go run ./cmd/sim

echo "Running backtest with sample data..."
if [ -f data/mids_sample.csv ]; then
  go run ./cmd/backtest data/mids_sample.csv
else
  echo "sample data not found, skip backtest"
fi
