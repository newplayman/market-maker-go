#!/usr/bin/env bash
set -euo pipefail

# 简化版运行脚本

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# 默认参数
CONFIG_PATH="${CONFIG_PATH:-configs/config.yaml}"
SYMBOL="${SYMBOL:-ETHUSDC}"
DRY_RUN="${DRY_RUN:-true}"
METRICS_ADDR="${METRICS_ADDR:-:9100}"

# 检查是否启用实盘模式
if [[ "${LIVE_TRADING:-false}" == "true" ]]; then
    DRY_RUN="false"
    CONFIG_PATH="${CONFIG_PATH:-configs/config_real.yaml}"
    echo "⚠️  WARNING: 实盘交易模式已启用"
    echo "========================"
    read -p "确认要启动实盘交易吗？(输入 'YES' 确认): " -r
    echo
    if [[ ! $REPLY =~ ^YES$ ]]; then
        echo "操作已取消"
        exit 0
    fi
fi

# 检查配置文件是否存在
if [ ! -f "$CONFIG_PATH" ]; then
    echo "错误: 配置文件不存在: $CONFIG_PATH"
    echo "请先创建配置文件或设置 CONFIG_PATH 环境变量"
    exit 1
fi

echo "启动 Market Maker..."
echo "配置文件: $CONFIG_PATH"
echo "交易对: $SYMBOL"
echo "Dry Run模式: $DRY_RUN"
echo "指标地址: $METRICS_ADDR"
echo ""

# 创建日志目录
mkdir -p /var/log/market-maker 2>/dev/null || true

# 运行程序
if [[ "$DRY_RUN" == "true" ]]; then
    exec go run ./cmd/runner \
        -config "$CONFIG_PATH" \
        -symbol "$SYMBOL" \
        -dryRun="$DRY_RUN" \
        -metricsAddr "$METRICS_ADDR"
else
    # 实盘模式需要API密钥
    if [[ -z "${MM_GATEWAY_API_KEY:-}" ]] || [[ -z "${MM_GATEWAY_API_SECRET:-}" ]]; then
        echo "错误: 实盘模式需要设置 MM_GATEWAY_API_KEY 和 MM_GATEWAY_API_SECRET 环境变量"
        exit 1
    fi
    
    exec go run ./cmd/runner \
        -config "$CONFIG_PATH" \
        -symbol "$SYMBOL" \
        -dryRun="$DRY_RUN" \
        -metricsAddr "$METRICS_ADDR"
fi