#!/bin/bash
#############################################################################
# Runner 启动脚本（改进版）
# - 基于主机名的原子锁，防止多机共享 /tmp 时冲突
# - PID 管理 + 优雅退出脚本联动
# - 仅使用编译后的二进制，避免 go run 产生的僵尸进程
# - 自动清理锁文件，即使脚本被 Ctrl+C 中断
#############################################################################

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# === 配置参数 ===
CONFIG_PATH="${CONFIG_PATH:-./configs/round8_survival.yaml}"
METRICS_ADDR="${METRICS_ADDR:-:9101}"
DRY_RUN="${DRY_RUN:-false}"
PID_FILE="./logs/runner.pid"
LOG_FILE="./logs/runner_$(date +%Y%m%d_%H%M%S).log"
BIN_PATH="${BIN_PATH:-./build/runner}"
HOST_ID="$(hostname -s 2>/dev/null || hostname)"
LOCK_FILE="/var/run/market-maker-runner.${HOST_ID}.lock"

mkdir -p ./logs

if [ ! -w "/var/run" ]; then
    LOCK_FILE="$ROOT/logs/runner.${HOST_ID}.lock"
fi

LOCK_FD=""
cleanup() {
    local status=$1
    if [ -n "${LOCK_FD:-}" ]; then
        flock -u "$LOCK_FD" 2>/dev/null || true
        eval "exec ${LOCK_FD}>&-"
    fi
    if [ -n "${LOCK_FILE:-}" ] && [ -f "$LOCK_FILE" ]; then
        rm -f "$LOCK_FILE"
    fi
    return "$status"
}
trap 'status=$?; cleanup "$status"; exit "$status"' EXIT INT TERM

echo "========================================="
echo "  Market Maker Runner 启动"
echo "========================================="
echo "配置: $CONFIG_PATH"
echo "Metrics: $METRICS_ADDR"
echo "DryRun: $DRY_RUN"
echo "日志: $LOG_FILE"
echo ""

# === 1. 原子锁检查 ===
echo "[1/5] 检查原子锁 ($LOCK_FILE)..."
exec {LOCK_FD}> "$LOCK_FILE"
if ! flock -n "$LOCK_FD"; then
    echo "❌ 错误：已有 runner 实例在运行"
    echo "如果确认没有实例，请删除锁文件: $LOCK_FILE"
    exit 1
fi
echo "✅ 锁获取成功"

# === 2. 清理旧进程 ===
echo "[2/5] 清理旧进程..."
if [ -f "$PID_FILE" ]; then
    OLD_PID=$(cat "$PID_FILE")
    if ps -p "$OLD_PID" > /dev/null 2>&1; then
        echo "发现旧进程 (PID: $OLD_PID)，尝试停止..."
        kill -TERM "$OLD_PID" 2>/dev/null || true
        sleep 2
        if ps -p "$OLD_PID" > /dev/null 2>&1; then
            echo "旧进程未退出，发送 SIGKILL"
            kill -KILL "$OLD_PID" 2>/dev/null || true
        fi
    fi
    rm -f "$PID_FILE"
fi

# === 3. 编译程序 ===
echo "[3/5] 编译程序..."
mkdir -p "$(dirname "$BIN_PATH")"
if ! go build -o "$BIN_PATH" ./cmd/runner; then
    echo "❌ 编译失败"
    exit 1
fi
echo "✅ 编译完成: $BIN_PATH"

# === 4. 启动 runner ===
echo "[4/5] 启动 runner..."
export DRY_RUN
nohup "$BIN_PATH" \
    -config "$CONFIG_PATH" \
    -metricsAddr "$METRICS_ADDR" \
    >> "$LOG_FILE" 2>&1 &

RUNNER_PID=$!
echo "$RUNNER_PID" > "$PID_FILE"
echo "✅ Runner 已启动 (PID: $RUNNER_PID)"

# === 5. 验证进程启动 ===
echo "[5/5] 验证进程启动..."
sleep 2
if ! ps -p "$RUNNER_PID" > /dev/null 2>&1; then
    echo "❌ Runner 启动失败"
    echo "最后20行日志:"
    tail -n 20 "$LOG_FILE"
    exit 1
fi

echo "等待 WebSocket 连接..."
for i in {1..15}; do
    if grep -q "WebSocket UserStream connected" "$LOG_FILE" 2>/dev/null; then
        echo "✅ WebSocket 已连接"
        break
    fi
    if [ $i -eq 15 ]; then
        echo "⚠️ WebSocket 连接仍未确认，请检查日志"
    fi
    sleep 1
done

echo ""
echo "========================================="
echo "  ✅ Runner 启动成功"
echo "========================================="
echo "PID: $RUNNER_PID"
echo "日志: tail -f $LOG_FILE"
echo "指标: curl http://localhost${METRICS_ADDR}/metrics | grep mm_"
echo ""
echo "停止命令: ./scripts/graceful_shutdown.sh"
echo "========================================="
