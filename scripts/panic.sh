#!/usr/bin/env bash
set -euo pipefail

# 简化版紧急停止脚本

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# 默认参数
CONFIG_PATH="${CONFIG_PATH:-configs/config.yaml}"
SYMBOL="${SYMBOL:-ETHUSDC}"

# 如果配置了实盘，则使用实盘配置
if [[ -f "configs/config_real.yaml" ]]; then
    CONFIG_PATH="${CONFIG_PATH:-configs/config_real.yaml}"
fi

echo "🚨 紧急停止 Market Maker 🚨"
echo "========================="
echo ""

# 确认操作
read -p "确定要紧急停止并平仓吗？(输入 'yes' 确认): " -r
echo
if [[ ! $REPLY =~ ^yes$ ]]; then
    echo "操作已取消"
    exit 0
fi

echo "正在执行紧急停止..."
echo ""

# 停止本地进程
echo "1. 停止本地 runner 进程..."
pkill -f "cmd/runner" >/dev/null 2>&1 && echo "   ✓ 已终止本地进程" || echo "   - 未发现运行中的进程"

# 取消所有订单
echo "2. 取消所有挂单..."
if go run ./cmd/binance_panic -config "$CONFIG_PATH" -symbol "$SYMBOL" -cancel; then
    echo "   ✓ 所有挂单已取消"
else
    echo "   ✗ 取消挂单失败"
fi

# 平仓
echo "3. 市价平仓..."
if go run ./cmd/binance_panic -config "$CONFIG_PATH" -symbol "$SYMBOL" -close; then
    echo "   ✓ 平仓指令已发送"
else
    echo "   ✗ 平仓指令发送失败"
fi

echo ""
echo "✅ 紧急停止流程完成"
echo "请检查 Binance 账户确认所有订单已取消且仓位已平仓"