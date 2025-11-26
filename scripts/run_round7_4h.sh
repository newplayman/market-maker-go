#!/usr/bin/env bash
set -euo pipefail

#############################################################################
# Round7 4小时单实例实盘脚本
#
# 功能：
# 1. 清场（杀掉所有旧 runner/监控脚本）
# 2. 记录账户初始快照
# 3. 启动单个 Round7 runner，记录 PID
# 4. 启动定时器（4小时后自动停止、清仓、撤单、生成报告）
#############################################################################

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# === 配置参数 ===
CONFIG_PATH="./configs/config_round7_geometric_drawdown.yaml"
SYMBOL="ETHUSDC"
METRICS_ADDR=":8080"
LOG_FILE="./logs/round7_4h.log"
PID_FILE="./logs/round7_4h_runner.pid"
TIMER_PID_FILE="./logs/round7_4h_timer.pid"
INIT_SNAPSHOT="./logs/round7_4h_init.txt"
DURATION_SECONDS=$((4 * 3600))  # 4小时

# === 步骤 1: 清场（防止影子程序） ===
echo "=== [1/5] 清场：停止所有旧 runner 和监控脚本 ==="

# 杀掉所有使用 Round7 配置的 runner
pkill -f "runner -config ./configs/config_round7_geometric_drawdown.yaml" 2>/dev/null && echo "  ✓ 已终止旧 Round7 runner" || echo "  - 未发现旧 Round7 runner"

# 杀掉所有 continuous_monitor.sh
pkill -f "continuous_monitor.sh" 2>/dev/null && echo "  ✓ 已终止 continuous_monitor 脚本" || echo "  - 未发现 continuous_monitor 脚本"

# 杀掉所有旧的定时器（包括 30min/24h 等）
pkill -f "sleep.*round" 2>/dev/null && echo "  ✓ 已终止旧定时器" || echo "  - 未发现旧定时器"

# 再次检查端口占用（确保 8080 空闲）
if lsof -i:8080 -sTCP:LISTEN -t >/dev/null 2>&1; then
    OLD_PID=$(lsof -i:8080 -sTCP:LISTEN -t)
    echo "  ⚠ 端口 8080 仍被占用（PID: $OLD_PID），强制终止..."
    kill -9 "$OLD_PID" 2>/dev/null || true
    sleep 1
fi

echo ""

# === 步骤 2: 记录初始账户快照 ===
echo "=== [2/5] 记录账户初始状态 ==="
mkdir -p logs

{
    echo "$(date '+%Y-%m-%d %H:%M:%S')"
    echo "=== Round7 4小时测试初始状态 ==="
    go run ./cmd/binance_balance -config "$CONFIG_PATH" 2>/dev/null || echo "ERROR: 无法获取余额"
    go run ./cmd/binance_position -config "$CONFIG_PATH" -symbol "$SYMBOL" 2>/dev/null || echo "ERROR: 无法获取持仓"
    echo ""
} > "$INIT_SNAPSHOT"

echo "  ✓ 初始快照已保存: $INIT_SNAPSHOT"
echo ""

# === 步骤 3: 启动 Round7 runner（单实例） ===
echo "=== [3/5] 启动 Round7 runner ==="
echo "  配置: $CONFIG_PATH"
echo "  日志: $LOG_FILE"
echo "  Metrics: $METRICS_ADDR"
echo ""

# 清空旧日志
> "$LOG_FILE"

# 后台启动 runner
nohup go run ./cmd/runner \
    -config "$CONFIG_PATH" \
    -dryRun=false \
    -metricsAddr "$METRICS_ADDR" \
    >> "$LOG_FILE" 2>&1 &

RUNNER_PID=$!
echo "$RUNNER_PID" > "$PID_FILE"

echo "  ✓ Runner PID: $RUNNER_PID (已保存至 $PID_FILE)"
sleep 3

# 验证进程是否存活
if ! kill -0 "$RUNNER_PID" 2>/dev/null; then
    echo "  ✗ Runner 启动失败，请检查日志: $LOG_FILE"
    tail -n 20 "$LOG_FILE"
    exit 1
fi

echo "  ✓ Runner 运行正常"
echo ""

# === 步骤 4: 启动4小时定时器（后台） ===
echo "=== [4/5] 启动4小时定时器（自动收尾） ==="
echo "  结束时间: $(date -d "+${DURATION_SECONDS} seconds" '+%Y-%m-%d %H:%M:%S')"
echo ""

(
    sleep "$DURATION_SECONDS"
    
    echo "[定时器触发] $(date '+%Y-%m-%d %H:%M:%S') - 开始执行收尾流程..." | tee -a "$LOG_FILE"
    
    # 调用收尾脚本
    bash "$ROOT/scripts/finish_round7_4h.sh"
    
) &

TIMER_PID=$!
echo "$TIMER_PID" > "$TIMER_PID_FILE"

echo "  ✓ 定时器 PID: $TIMER_PID (已保存至 $TIMER_PID_FILE)"
echo ""

# === 步骤 5: 输出运行状态 ===
echo "=== [5/5] 4小时实盘已启动 ==="
echo ""
echo "  📊 监控面板:"
echo "     - Grafana: http://localhost:3001"
echo "     - Prometheus: http://localhost:9090"
echo "     - Metrics: http://localhost:8080/metrics"
echo ""
echo "  📁 关键文件:"
echo "     - 日志: $LOG_FILE"
echo "     - Runner PID: $PID_FILE"
echo "     - 定时器 PID: $TIMER_PID_FILE"
echo ""
echo "  ⏱️  预计结束时间: $(date -d "+${DURATION_SECONDS} seconds" '+%Y-%m-%d %H:%M:%S')"
echo ""
echo "  🔍 实时监控命令:"
echo "     tail -f $LOG_FILE | grep -E 'FILLED|drawdown_trigger|net exposure|strategy_adjust'"
echo ""
echo "  🛑 手动停止命令:"
echo "     bash scripts/finish_round7_4h.sh"
echo ""
echo "=== 启动完成 ==="
