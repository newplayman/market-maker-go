#!/bin/bash
set -e

CONFIG="configs/round8_survival.yaml"
LOG_FILE="logs/round8_30min_$(date +%Y%m%d_%H%M%S).log"
DURATION=1800  # 30分钟

echo "=== Round8 30分钟实盘测试 ==="
echo "日志: $LOG_FILE"
echo ""

# 清理
pkill -9 -f 'runner' 2>/dev/null || true
pkill -9 -f 'go run.*runner' 2>/dev/null || true
sleep 2

# 清理挂单
go run ./cmd/emergency_cleanup

# 记录初始状态
echo "初始状态:" > "$LOG_FILE.init"
go run ./cmd/binance_balance >> "$LOG_FILE.init"
go run ./cmd/binance_position -symbol ETHUSDC >> "$LOG_FILE.init"

# 启动 runner
echo "启动 runner..."
nohup go run ./cmd/runner -config "$CONFIG" -metricsAddr :9101 > "$LOG_FILE" 2>&1 &
RUNNER_PID=$!
echo "Runner PID: $RUNNER_PID"
echo $RUNNER_PID > logs/runner.pid

# 等待启动
sleep 5

# 验证
if ! kill -0 $RUNNER_PID 2>/dev/null; then
    echo "错误: Runner 启动失败"
    tail -n 30 "$LOG_FILE"
    exit 1
fi

echo "✅ Runner 运行中"
echo "日志: tail -f $LOG_FILE"
echo "指标: curl http://localhost:9101/metrics | grep mm_"
echo ""
echo "$DURATION 秒后执行以下命令停止:"
echo "  kill -TERM $RUNNER_PID && sleep 5"
echo ""

# 后台定时器
(
    sleep $DURATION
    echo "时间到，停止 runner..."
    kill -TERM $RUNNER_PID 2>/dev/null || true
    sleep 10
    
    # 记录最终状态
    echo "最终状态:" > "$LOG_FILE.final"
    go run ./cmd/binance_balance >> "$LOG_FILE.final"
    go run ./cmd/binance_position -symbol ETHUSDC >> "$LOG_FILE.final"
    
    echo "测试完成，日志: $LOG_FILE"
) &

TIMER_PID=$!
echo "定时器 PID: $TIMER_PID"
echo "========================================="
