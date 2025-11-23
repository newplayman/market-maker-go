#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_PATH="${CONFIG_PATH:-$ROOT/configs/config.yaml}"
SYMBOL="${SYMBOL:-ETHUSDC}"
DRY_RUN="${DRY_RUN:-false}"
METRICS_ADDR="${METRICS_ADDR:-:9100}"
REST_RATE="${REST_RATE:-5}"
REST_BURST="${REST_BURST:-10}"

cd "$ROOT"
LOG_PATH="${LOG_PATH:-/var/log/market-maker/runner.log}"
mkdir -p "$(dirname "$LOG_PATH")"
touch "$LOG_PATH"
chmod 664 "$LOG_PATH"
if command -v chown >/dev/null 2>&1; then
  chown monitor:monitor "$LOG_PATH" >/dev/null 2>&1 || true
fi
go run ./cmd/runner \
  -config "$CONFIG_PATH" \
  -symbol "$SYMBOL" \
  "-dryRun=$DRY_RUN" \
  -metricsAddr "$METRICS_ADDR" \
  -restRate "$REST_RATE" \
  -restBurst "$REST_BURST" 2>&1 | tee -a "$LOG_PATH"
