#!/usr/bin/env bash
###############################################################################
# 一键启动做市商 + 监控栈
# 需求：
#   1. 在仓库根目录准备一个环境文件（默认 .env.runner），内容形如：
#        BINANCE_API_KEY=xxx
#        BINANCE_API_SECRET=yyy
#        CONFIG_PATH=./configs/round8_survival.yaml
#        METRICS_ADDR=:9101
#   2. 已安装 Docker，并允许 docker compose 运行
###############################################################################

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

ENV_FILE="${ENV_FILE:-.env.runner}"
if [ ! -f "$ENV_FILE" ]; then
  cat <<EOF
❌ 未找到环境文件: ${ENV_FILE}
请创建文件并写入 BINANCE_API_KEY / BINANCE_API_SECRET 等变量，例如:

BINANCE_API_KEY=your_key
BINANCE_API_SECRET=your_secret
CONFIG_PATH=./configs/round8_survival.yaml
METRICS_ADDR=:9101
EOF
  exit 1
fi

set -a
source "$ENV_FILE"
set +a

CONFIG_PATH="${CONFIG_PATH:-./configs/round8_survival.yaml}"
METRICS_ADDR="${METRICS_ADDR:-:9101}"

if [ -z "${BINANCE_API_KEY:-}" ] || [ -z "${BINANCE_API_SECRET:-}" ]; then
  echo "❌ BINANCE_API_KEY / BINANCE_API_SECRET 尚未配置，请更新 ${ENV_FILE}"
  exit 1
fi

echo "=============================="
echo " 启动监控栈 (Prometheus/Loki/Grafana/Promtail)"
echo "=============================="

(cd monitoring && ./start.sh)

echo ""
echo "=============================="
echo " 启动做市商 Runner"
echo "=============================="

CONFIG_PATH="$CONFIG_PATH" METRICS_ADDR="$METRICS_ADDR" ./scripts/start_runner.sh

cat <<EOF

========================================
监控访问:
  Prometheus: http://localhost:9090
  Grafana:    http://localhost:3001 (admin/admin)
  Loki API:   http://localhost:3100

Runner 指标查看:
  curl -s http://localhost${METRICS_ADDR}/metrics | head

停止脚本:
  ./scripts/stop_bot_with_monitoring.sh (会自动优雅停机并关闭监控)
========================================
EOF
