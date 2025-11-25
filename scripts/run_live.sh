#!/usr/bin/env bash
set -euo pipefail

# 实盘交易运行脚本

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# 默认参数
CONFIG_PATH="${CONFIG_PATH:-configs/config_real_trading.yaml}"
SYMBOL="${SYMBOL:-ETHUSDC}"
DRY_RUN="false"
METRICS_ADDR="${METRICS_ADDR:-:9100}"

echo "🚨 实盘交易模式 🚨"
echo "=================="
echo "配置文件: $CONFIG_PATH"
echo "交易对: $SYMBOL"
echo "指标地址: $METRICS_ADDR"
echo ""

# 检查配置文件是否存在
if [ ! -f "$CONFIG_PATH" ]; then
    echo "错误: 配置文件不存在: $CONFIG_PATH"
    exit 1
fi

# 确认操作
read -p "确认要启动实盘交易吗？(输入 'LIVE' 确认): " -r
echo
if [[ ! $REPLY =~ ^LIVE$ ]]; then
    echo "操作已取消"
    exit 0
fi

echo "正在启动实盘交易..."
echo ""

# 创建日志目录
mkdir -p /var/log/market-maker 2>/dev/null || true

# 运行程序
exec go run ./cmd/runner \
    -config "$CONFIG_PATH" \
    -symbol "$SYMBOL" \
    -dryRun="$DRY_RUN" \
    -metricsAddr "$METRICS_ADDR"