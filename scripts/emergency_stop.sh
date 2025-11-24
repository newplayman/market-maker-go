#!/usr/bin/env bash
set -euo pipefail

# 环境变量兼容性：如果设置了BINANCE_*，自动转换为MM_GATEWAY_*
if [ -n "${BINANCE_API_KEY:-}" ] && [ -z "${MM_GATEWAY_API_KEY:-}" ]; then
    export MM_GATEWAY_API_KEY="$BINANCE_API_KEY"
fi
if [ -n "${BINANCE_API_SECRET:-}" ] && [ -z "${MM_GATEWAY_API_SECRET:-}" ]; then
    export MM_GATEWAY_API_SECRET="$BINANCE_API_SECRET"
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}" )/.." && pwd)"
CONFIG_PATH="${CONFIG_PATH:-$ROOT/configs/config.yaml}"
SYMBOL="${SYMBOL:-ETHUSDC}"
FLATTEN="${FLATTEN:-false}"

echo "[1/4] 停止本地 runner..."
if command -v systemctl >/dev/null 2>&1; then
	if systemctl list-units --all --no-legend | grep -q "runner.service"; then
		sudo systemctl stop runner.service >/dev/null 2>&1 || true
	fi
fi
pkill -f "cmd/runner" >/dev/null 2>&1 && echo "已终止本地 go run 进程" || echo "未发现本地 go run 进程"

cd "$ROOT"

echo "[2/4] 取消 Binance 上的全部挂单 ($SYMBOL)..."
MM_GATEWAY_API_KEY="$BINANCE_API_KEY" MM_GATEWAY_API_SECRET="$BINANCE_API_SECRET" go run ./cmd/binance_panic -config "$CONFIG_PATH" -symbol "$SYMBOL" -cancel

if [[ "$FLATTEN" == "true" ]]; then
	echo "[3/4] 市价 reduce-only 平掉当前仓位 ($SYMBOL)..."
	MM_GATEWAY_API_KEY="$BINANCE_API_KEY" MM_GATEWAY_API_SECRET="$BINANCE_API_SECRET" go run ./cmd/binance_panic -config "$CONFIG_PATH" -symbol "$SYMBOL" -close
else
	echo "[3/4] 跳过自动平仓，若需启用请设置 FLATTEN=true"
fi

echo "[4/4] 紧急刹车流程完成。"
