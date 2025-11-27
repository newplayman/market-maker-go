#!/bin/bash
set -e

# ===== 配置 =====
CONFIG_PATH="configs/round8_survival.yaml"
SYMBOL="ETHUSDC"
DURATION_SECONDS=1800  # 30分钟
LOG_FILE="logs/round8_30min_$(date +%Y%m%d_%H%M%S).log"
METRICS_ADDR=":9101"  # 避免与系统 node_exporter (9100) 冲突
PID_FILE="logs/runner.pid"

echo "========================================="
echo "Round8 生存版 - 30分钟实盘测试"
echo "========================================="
echo "配置文件: $CONFIG_PATH"
echo "交易对:   $SYMBOL"
echo "测试时长: 30分钟"
echo "日志文件: $LOG_FILE"
echo ""

# ===== 步骤 1: 清理旧进程和挂单 =====
echo "=== [1/7] 清理旧进程和挂单 ==="

# 强制终止所有 runner 相关进程
echo "  🔸 终止所有 runner 进程..."
pkill -9 -f 'go run.*runner' 2>/dev/null && echo "    - 已杀死 go run" || true
pkill -9 -f 'runner.*round8' 2>/dev/null && echo "    - 已杀死 runner" || true
pkill -9 -f '/tmp/go-build.*runner' 2>/dev/null && echo "    - 已杀死编译缓存" || true
pkill -9 -f '/root/.cache/go-build.*runner' 2>/dev/null && echo "    - 已杀死编译缓存2" || true
sleep 2

# 再次确认没有残留
if pgrep -f 'runner' > /dev/null; then
    echo "  ✗ 仍有 runner 进程残留："
    pgrep -af 'runner'
    echo "  强制清理..."
    pkill -9 -f 'runner'
    sleep 1
fi

pkill -f "sleep.*round" 2>/dev/null && echo "  ✓ 已终止旧定时器" || echo "  - 未发现旧定时器"
rm -f "$PID_FILE" 2>/dev/null && echo "  ✓ 已清理旧 PID 文件" || true

# 清理交易所挂单
echo "  🔸 清理交易所挂单..."
go run ./cmd/emergency_cleanup 2>&1 | grep -E "取消|仓位|✅" || true

# 检查端口占用
if lsof -i:9101 -sTCP:LISTEN -t >/dev/null 2>&1; then
    OLD_PID=$(lsof -i:9101 -sTCP:LISTEN -t)
    echo "  ⚠ 端口 9101 被占用（PID: $OLD_PID），强制终止..."
    kill -9 "$OLD_PID" 2>/dev/null || true
    sleep 1
fi

# 最终确认：没有任何 runner 进程
if pgrep -f 'runner' > /dev/null; then
    echo "  ✗ 错误：仍有 runner 进程在运行，退出"
    pgrep -af 'runner'
    exit 1
fi
echo "  ✓ 所有旧进程已清理"
echo ""

# ===== 步骤 2: 记录初始状态 =====
echo "=== [2/7] 记录初始账户状态 ==="
INIT_SNAPSHOT="logs/init_snapshot_$(date +%Y%m%d_%H%M%S).txt"
{
    echo "$(date '+%Y-%m-%d %H:%M:%S')"
    echo "=== Round8 30分钟测试初始状态 ==="
    go run ./cmd/binance_balance 2>/dev/null || echo "ERROR: 无法获取余额"
    go run ./cmd/binance_position -symbol "$SYMBOL" 2>/dev/null || echo "ERROR: 无法获取持仓"
    echo ""
} > "$INIT_SNAPSHOT"
echo "  ✓ 初始快照已保存: $INIT_SNAPSHOT"
echo ""

# ===== 步骤 3: 启动 runner =====
echo "=== [3/7] 启动 Round8 runner ==="
echo "  配置: $CONFIG_PATH"
echo "  日志: $LOG_FILE"
echo "  Metrics: $METRICS_ADDR"
echo ""

# 清空旧日志
> "$LOG_FILE"

# 后台启动
nohup go run ./cmd/runner -config "$CONFIG_PATH" >> "$LOG_FILE" 2>&1 &
RUNNER_PID=$!
echo "$RUNNER_PID" > "$PID_FILE"

echo "  ✓ Runner PID: $RUNNER_PID"
sleep 3

# 检查进程是否存活
if ! kill -0 "$RUNNER_PID" 2>/dev/null; then
    echo "  ✗ Runner 启动失败，检查日志："
    tail -n 30 "$LOG_FILE"
    exit 1
fi
echo "  ✓ Runner 运行正常"
echo ""

# ===== 步骤 4: 验证 WebSocket 连接 =====
echo "=== [4/7] 验证 WebSocket 连接 ==="
for i in {1..10}; do
    if grep -q "WebSocket UserStream connected" "$LOG_FILE"; then
        echo "  ✓ WebSocket 已连接"
        break
    fi
    if [ $i -eq 10 ]; then
        echo "  ✗ WebSocket 连接超时"
        tail -n 20 "$LOG_FILE"
    fi
    sleep 1
done
echo ""

# ===== 步骤 5: 验证 Prometheus 指标 =====
echo "=== [5/7] 验证 Prometheus 指标 ==="
sleep 2
if curl -s http://localhost:9101/metrics | grep -q "mm_ws_connected"; then
    echo "  ✓ Prometheus 指标可访问"
    curl -s http://localhost:9101/metrics | grep -E "mm_ws_connected|mm_worst_case" | head -n 5
else
    echo "  ✗ Prometheus 指标不可用"
fi
echo ""

# ===== 步骤 6: 验证无挂单限制错误 =====
echo "=== [6/7] 验证无挂单限制错误 ==="
sleep 5
if grep -q "Reach max open order limit" "$LOG_FILE"; then
    echo "  ✗ 检测到挂单限制错误，停止测试"
    kill -TERM "$RUNNER_PID" 2>/dev/null || true
    go run ./cmd/emergency_cleanup
    exit 1
fi
echo "  ✓ 挂单正常"
echo ""

# ===== 步骤 7: 启动定时收尾 =====
echo "=== [7/7] 启动定时收尾（$DURATION_SECONDS 秒后）==="
(
    sleep $DURATION_SECONDS
    echo ""
    echo "========================================="
    echo "测试时间到，开始收尾..."
    echo "========================================="
    
    # 停止 runner
    if [ -f "$PID_FILE" ]; then
        RUNNER_PID=$(cat "$PID_FILE")
        if kill -0 "$RUNNER_PID" 2>/dev/null; then
            echo "🔸 停止 Runner (PID: $RUNNER_PID)..."
            kill -TERM "$RUNNER_PID" 2>/dev/null || true
            sleep 5
            # 如果还在运行，强制停止
            if kill -0 "$RUNNER_PID" 2>/dev/null; then
                kill -9 "$RUNNER_PID" 2>/dev/null || true
            fi
            echo "✅ Runner 已停止"
        fi
        rm -f "$PID_FILE"
    fi
    
    # 等待优雅退出完成撤单平仓
    sleep 3
    
    # 记录最终状态
    FINAL_SNAPSHOT="logs/final_snapshot_$(date +%Y%m%d_%H%M%S).txt"
    {
        echo "$(date '+%Y-%m-%d %H:%M:%S')"
        echo "=== Round8 30分钟测试最终状态 ==="
        go run ./cmd/binance_balance 2>/dev/null || echo "ERROR: 无法获取余额"
        go run ./cmd/binance_position -symbol "$SYMBOL" 2>/dev/null || echo "ERROR: 无法获取持仓"
        echo ""
    } > "$FINAL_SNAPSHOT"
    echo "✅ 最终快照已保存: $FINAL_SNAPSHOT"
    
    # 生成测试报告
    REPORT_FILE="reports/round8_30min_test_$(date +%Y%m%d_%H%M%S).md"
    echo "🔸 生成测试报告..."
    bash scripts/generate_30min_report.sh "$LOG_FILE" "$INIT_SNAPSHOT" "$FINAL_SNAPSHOT" > "$REPORT_FILE"
    echo "✅ 测试报告已生成: $REPORT_FILE"
    
    echo ""
    echo "========================================="
    echo "测试完成！"
    echo "========================================="
) &

TIMER_PID=$!
echo "  ✓ 定时器 PID: $TIMER_PID"
echo ""

echo "========================================="
echo "✅ 测试已启动"
echo "========================================="
echo "Runner PID:  $RUNNER_PID"
echo "定时器 PID:  $TIMER_PID"
echo "日志文件:    $LOG_FILE"
echo "Metrics:     http://localhost:9101/metrics"
echo ""
echo "实时查看日志:"
echo "  tail -f $LOG_FILE"
echo ""
echo "手动停止:"
echo "  kill -TERM $RUNNER_PID"
echo ""
echo "30分钟后自动停止并生成报告..."
echo "========================================="
