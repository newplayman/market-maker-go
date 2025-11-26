#!/bin/bash
#############################################################################
# 优雅退出脚本
# 功能：
# 1. 发送SIGTERM触发优雅退出
# 2. 等待进程自行退出（最多20秒）
# 3. 强制杀死并清理订单
#############################################################################

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PID_FILE="${PID_FILE:-$ROOT/logs/runner.pid}"
CONFIG_PATH="${CONFIG_PATH:-$ROOT/configs/round8_survival.yaml}"
SYMBOL="${SYMBOL:-ETHUSDC}"

echo "=== 优雅退出流程 ==="
echo "PID文件: $PID_FILE"
echo "配置: $CONFIG_PATH"
echo "交易对: $SYMBOL"
echo ""

if [ ! -f "$PID_FILE" ]; then
    echo "⚠️ PID文件不存在，尝试查找进程..."
    # 尝试通过进程名查找
    if pgrep -f "cmd/runner.*$CONFIG_PATH" > /dev/null; then
        PID=$(pgrep -f "cmd/runner.*$CONFIG_PATH" | head -n1)
        echo "找到进程: $PID"
    else
        echo "未找到运行中的runner进程"
        exit 0
    fi
else
    PID=$(cat "$PID_FILE")
fi

# 1. 发送SIGTERM触发优雅退出
echo "[1/3] 发送SIGTERM信号 (PID: $PID)..."
if ! kill -0 "$PID" 2>/dev/null; then
    echo "进程已不存在"
    rm -f "$PID_FILE"
    exit 0
fi

kill -TERM "$PID" || true

# 2. 等待进程自行退出（最多20秒）
echo "[2/3] 等待进程自行退出（最多20秒）..."
for i in {1..20}; do
    if ! ps -p "$PID" > /dev/null 2>&1; then
        echo "✅ Runner 已优雅退出"
        rm -f "$PID_FILE"
        exit 0
    fi
    sleep 1
    echo -n "."
done
echo ""

# 3. 强制杀死并清理订单
echo "[3/3] 强制终止并清理订单..."
kill -KILL "$PID" 2>/dev/null || true
rm -f "$PID_FILE"

# 紧急清理交易所订单
cd "$ROOT"
if [ -f "./cmd/emergency_cleanup/main.go" ]; then
    echo "执行emergency_cleanup..."
    go run ./cmd/emergency_cleanup || true
else
    echo "⚠️ emergency_cleanup不存在，跳过"
fi

echo "✅ 优雅退出流程完成"
