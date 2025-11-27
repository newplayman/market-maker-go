#!/usr/bin/env bash
###############################################################################
# 一键停止做市商 + 监控栈
# 会自动加载与 start 脚本相同的 .env.runner（或 ENV_FILE）
###############################################################################

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

ENV_FILE="${ENV_FILE:-.env.runner}"
if [ -f "$ENV_FILE" ]; then
  set -a
  source "$ENV_FILE"
  set +a
fi

echo "=============================="
echo " 优雅停止做市商 Runner"
echo "=============================="

./scripts/graceful_shutdown.sh

echo ""
echo "=============================="
echo " 停止监控栈"
echo "=============================="

(cd monitoring && ./stop.sh)

echo ""
echo "所有服务已停止。"
