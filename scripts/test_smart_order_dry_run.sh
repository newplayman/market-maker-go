#!/bin/bash
# 智能订单管理器 DRY-RUN 测试脚本

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT"

echo "======================================="
echo "  智能订单管理器 DRY-RUN 测试"
echo "======================================="
echo ""
echo "测试配置:"
echo "  - 模式: DRY-RUN (不发送真实订单)"
echo "  - 时长: 60秒"
echo "  - 指标端口: 9102"
echo "  - 价格偏移阈值: 0.08%"
echo "  - 重组阈值: 0.35%"
echo ""

# 清理旧进程
echo "🔸 清理旧进程..."
pkill -9 -f './build/runner' 2>/dev/null || true
sleep 1

# 确保可执行文件存在
if [ ! -f "./build/runner" ]; then
    echo "🔸 编译程序..."
    go build -o ./build/runner ./cmd/runner
fi

# 启动程序（后台）
echo "🔸 启动 runner (DRY-RUN)..."
LOG_FILE="/tmp/smart_order_test_$(date +%s).log"
DRY_RUN=1 ./build/runner \
    -config configs/round8_survival.yaml \
    -metricsAddr :9102 \
    > "$LOG_FILE" 2>&1 &

RUNNER_PID=$!
echo "  - PID: $RUNNER_PID"
echo "  - 日志: $LOG_FILE"
echo ""

# 等待启动
echo "等待5秒启动..."
sleep 5

# 检查进程是否还在运行
if ! kill -0 $RUNNER_PID 2>/dev/null; then
    echo "❌ 进程启动失败"
    echo "最后20行日志:"
    tail -20 "$LOG_FILE"
    exit 1
fi

echo "✅ 进程已启动"
echo ""

# 监控60秒
echo "📊 监控60秒订单行为..."
echo "------------------------------------"

for i in {1..12}; do
    sleep 5
    
    # 统计最近5秒的行为
    echo "[$((i*5))秒] 订单活动:"
    
    # 统计各种操作
    FIRST_COUNT=$(tail -100 "$LOG_FILE" | grep -c "\[首次\]" || true)
    UPDATE_COUNT=$(tail -100 "$LOG_FILE" | grep -c "价格偏离\|数量变化\|订单老化" || true)
    REORG_COUNT=$(tail -100 "$LOG_FILE" | grep -c "触发全量重组" || true)
    DRY_RUN_COUNT=$(tail -100 "$LOG_FILE" | grep -c "DRY-RUN: Place" || true)
    
    echo "  首次下单: $FIRST_COUNT"
    echo "  更新订单: $UPDATE_COUNT"
    echo "  全量重组: $REORG_COUNT"
    echo "  DRY-RUN下单: $DRY_RUN_COUNT"
    echo ""
done

# 停止程序
echo "🔸 停止程序..."
kill -TERM $RUNNER_PID 2>/dev/null || true
sleep 2
kill -KILL $RUNNER_PID 2>/dev/null || true

echo "✅ 测试完成"
echo ""

# 分析结果
echo "======================================="
echo "  测试结果分析"
echo "======================================="

echo ""
echo "📈 订单操作统计:"
TOTAL_FIRST=$(grep -c "\[首次\]" "$LOG_FILE" || true)
TOTAL_UPDATE=$(grep -c "价格偏离\|数量变化\|订单老化" "$LOG_FILE" || true)
TOTAL_REORG=$(grep -c "触发全量重组" "$LOG_FILE" || true)
TOTAL_DRY_RUN=$(grep -c "DRY-RUN: Place" "$LOG_FILE" || true)
TOTAL_CANCEL=$(grep -c "DRY-RUN: Cancel" "$LOG_FILE" || true)

echo "  - 首次下单次数: $TOTAL_FIRST"
echo "  - 更新订单次数: $TOTAL_UPDATE"
echo "  - 全量重组次数: $TOTAL_REORG"
echo "  - 总下单次数: $TOTAL_DRY_RUN"
echo "  - 总撤单次数: $TOTAL_CANCEL"
echo ""

# 检查是否有真实订单（不应该有）
REAL_PLACE=$(grep -c "Place BUY\|Place SELL" "$LOG_FILE" | grep -v "DRY-RUN" || true)
if [ "$REAL_PLACE" -gt 0 ]; then
    echo "⚠️  警告: 检测到 $REAL_PLACE 次真实下单（不应该发生）"
else
    echo "✅ 确认: 无真实订单发送"
fi

echo ""
echo "💡 关键观察:"

# 计算撤单频率
DURATION=60
AVG_CANCEL_PER_MIN=$((TOTAL_CANCEL * 60 / DURATION))
echo "  - 平均撤单频率: $AVG_CANCEL_PER_MIN 次/分钟"

if [ "$AVG_CANCEL_PER_MIN" -lt 50 ]; then
    echo "    ✅ 撤单频率良好 (< 50次/分钟)"
elif [ "$AVG_CANCEL_PER_MIN" -lt 100 ]; then
    echo "    ⚠️  撤单频率偏高 (50-100次/分钟)"
else
    echo "    ❌ 撤单频率过高 (> 100次/分钟)"
fi

echo ""
echo "📄 完整日志: $LOG_FILE"
echo ""

# 显示最后20行日志
echo "最后20行日志:"
echo "------------------------------------"
tail -20 "$LOG_FILE"
echo "------------------------------------"
