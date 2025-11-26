#!/bin/bash
#############################################################################
# Runner 启动脚本（带原子锁机制）
# 功能：
# 1. 原子锁检查（防止重复启动）
# 2. 清理旧进程（防止僵尸进程）
# 3. 使用编译后的二进制（不要用go run）
# 4. 等待进程确认启动
#############################################################################

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# === 配置参数 ===
CONFIG_PATH="${CONFIG_PATH:-./configs/round8_survival.yaml}"
METRICS_ADDR="${METRICS_ADDR:-:9101}"
DRY_RUN="${DRY_RUN:-false}"
LOCK_FILE="/var/run/market-maker-runner.lock"
PID_FILE="./logs/runner.pid"
LOG_FILE="./logs/runner_$(date +%Y%m%d_%H%M%S).log"
BIN_PATH="./build/runner"

echo "========================================="
echo "  Market Maker Runner 启动"
echo "========================================="
echo "配置: $CONFIG_PATH"
echo "Metrics: $METRICS_ADDR"
echo "DryRun: $DRY_RUN"
echo "日志: $LOG_FILE"
echo ""

# === 1. 原子锁检查 ===
echo "[1/5] 检查原子锁..."
if [ ! -d "/var/run" ]; then
    # 如果没有 /var/run 权限，使用本地锁
    LOCK_FILE="$ROOT/logs/runner.lock"
fi

# 尝试获取锁
exec 200>"$LOCK_FILE"
if ! flock -n 200; then
    echo "❌ 错误：已有runner实例在运行"
    echo "如果确认没有实例，请删除锁文件: $LOCK_FILE"
    exit 1
fi
echo "✅ 锁获取成功"

# === 2. 清理旧进程 ===
echo "[2/5] 清理旧进程..."
if [ -f "$PID_FILE" ]; then
    OLD_PID=$(cat "$PID_FILE")
    if ps -p "$OLD_PID" > /dev/null 2>&1; then
        echo "发现旧进程 (PID: $OLD_PID)，停止中..."
        kill -TERM "$OLD_PID" 2>/dev/null || true
        sleep 2
        if ps -p "$OLD_PID" > /dev/null 2>&1; then
            kill -KILL "$OLD_PID" 2>/dev/null || true
        fi
        echo "✅ 旧进程已终止"
    fi
    rm -f "$PID_FILE"
fi

# === 3. 编译程序 ===
echo "[3/5] 编译程序..."
mkdir -p "$(dirname "$BIN_PATH")"
mkdir -p "$(dirname "$LOG_FILE")"

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

# 检查 WebSocket 连接（等待最多10秒）
echo "等待 WebSocket 连接..."
for i in {1..10}; do
    if grep -q "WebSocket UserStream connected" "$LOG_FILE" 2>/dev/null; then
        echo "✅ WebSocket 已连接"
        break
    fi
    if [ $i -eq 10 ]; then
        echo "⚠️ WebSocket 连接超时（但进程仍在运行）"
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
#!/bin/bash
#############################################################################
# Runner 启动脚本（带原子锁机制）
# 功能：
# 1. 原子锁检查（防止重复启动）
# 2. 清理旧进程（防止僵尸进程）
# 3. 使用编译后的二进制（不要用go run）
# 4. 等待进程确认启动
#############################################################################

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# === 配置参数 ===
CONFIG_PATH="${CONFIG_PATH:-./configs/round8_survival.yaml}"
METRICS_ADDR="${METRICS_ADDR:-:9101}"
DRY_RUN="${DRY_RUN:-false}"
LOCK_FILE="/var/run/market-maker-runner.lock"
PID_FILE="./logs/runner.pid"
LOG_FILE="./logs/runner_$(date +%Y%m%d_%H%M%S).log"
BIN_PATH="./build/runner"

echo "========================================="
echo "  Market Maker Runner 启动"
echo "========================================="
echo "配置: $CONFIG_PATH"
echo "Metrics: $METRICS_ADDR"
echo "DryRun: $DRY_RUN"
echo "日志: $LOG_FILE"
echo ""

# === 1. 原子锁检查 ===
echo "[1/5] 检查原子锁..."
if [ ! -d "/var/run" ]; then
    # 如果没有 /var/run 权限，使用本地锁
    LOCK_FILE="$ROOT/logs/runner.lock"
fi

# 尝试获取锁
exec 200>"$LOCK_FILE"
if ! flock -n 200; then
    echo "❌ 错误：已有runner实例在运行"
    echo "如果确认没有实例，请删除锁文件: $LOCK_FILE"
    exit 1
fi
echo "✅ 锁获取成功"

# === 2. 清理旧进程 ===
echo "[2/5] 清理旧进程..."
if [ -f "$PID_FILE" ]; then
    OLD_PID=$(cat "$PID_FILE")
    if ps -p "$OLD_PID" > /dev/null 2>&1; then
        echo "发现旧进程 (PID: $OLD_PID)，停止中..."
        kill -TERM "$OLD_PID" 2>/dev/null || true
        sleep 2
        if ps -p "$OLD_PID" > /dev/null 2>&1; then
            kill -KILL "$OLD_PID" 2>/dev/null || true
        fi
        echo "✅ 旧进程已终止"
    fi
    rm -f "$PID_FILE"
fi

# === 3. 编译程序 ===
echo "[3/5] 编译程序..."
mkdir -p "$(dirname "$BIN_PATH")"
mkdir -p "$(dirname "$LOG_FILE")"

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

# 检查 WebSocket 连接（等待最多10秒）
echo "等待 WebSocket 连接..."
for i in {1..10}; do
    if grep -q "WebSocket UserStream connected" "$LOG_FILE" 2>/dev/null; then
        echo "✅ WebSocket 已连接"
        break
    fi
    if [ $i -eq 10 ]; then
        echo "⚠️ WebSocket 连接超时（但进程仍在运行）"
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
