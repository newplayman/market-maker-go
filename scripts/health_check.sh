#!/usr/bin/env bash
#
# 系统健康检查脚本
# 用于检查 market-maker 系统的运行状态
#

set -euo pipefail

# 环境变量兼容性
if [ -n "${BINANCE_API_KEY:-}" ] && [ -z "${MM_GATEWAY_API_KEY:-}" ]; then
    export MM_GATEWAY_API_KEY="$BINANCE_API_KEY"
fi
if [ -n "${BINANCE_API_SECRET:-}" ] && [ -z "${MM_GATEWAY_API_SECRET:-}" ]; then
    export MM_GATEWAY_API_SECRET="$BINANCE_API_SECRET"
fi

# ============================================================================
# 配置
# ============================================================================

APP_NAME="${APP_NAME:-market-maker}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PID_FILE="${PID_FILE:-/var/run/${APP_NAME}.pid}"
LOG_FILE="${LOG_FILE:-/var/log/${APP_NAME}/app.log}"
API_ENDPOINT="${API_ENDPOINT:-http://localhost:9100}"
PROMETHEUS_ENDPOINT="${PROMETHEUS_ENDPOINT:-http://localhost:9090}"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查结果统计
TOTAL_CHECKS=0
FAILED_CHECKS=0

# ============================================================================
# 辅助函数
# ============================================================================

log_info() {
    echo -e "${GREEN}✓${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

log_error() {
    echo -e "${RED}✗${NC} $1"
}

check_start() {
    TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
}

check_pass() {
    log_info "$1"
}

check_warn() {
    log_warn "$1"
}

check_fail() {
    log_error "$1"
    FAILED_CHECKS=$((FAILED_CHECKS + 1))
}

# ============================================================================
# 检查函数
# ============================================================================

# 1. 检查进程运行状态
check_process() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "检查 1: 进程运行状态"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    check_start
    
    # 检查 systemd 服务
    if command -v systemctl &> /dev/null; then
        if systemctl is-active --quiet ${APP_NAME}.service 2>/dev/null; then
            check_pass "Systemd 服务运行正常"
            return 0
        else
            check_warn "Systemd 服务未运行，检查进程..."
        fi
    fi
    
    # 检查 PID 文件
    if [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE")
        if ps -p "$PID" > /dev/null 2>&1; then
            check_pass "进程运行正常 (PID: $PID)"
            return 0
        else
            check_fail "PID 文件存在但进程不存在 (PID: $PID)"
            return 1
        fi
    fi
    
    # 直接检查进程
    if pgrep -f "cmd/runner" > /dev/null 2>&1; then
        local pid=$(pgrep -f "cmd/runner" | head -1)
        check_warn "进程通过 pgrep 找到 (PID: $pid)，但缺少 PID 文件"
        return 0
    fi
    
    check_fail "进程未运行"
    return 1
}

# 2. 检查端口监听
check_ports() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "检查 2: 端口监听状态"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    check_start
    
    # 检查 API 端口 (9100)
    if command -v netstat &> /dev/null; then
        if netstat -tuln 2>/dev/null | grep -q ":9100 "; then
            check_pass "API 端口 9100 监听正常"
        else
            check_fail "API 端口 9100 未监听"
        fi
    elif command -v ss &> /dev/null; then
        if ss -tuln 2>/dev/null | grep -q ":9100 "; then
            check_pass "API 端口 9100 监听正常"
        else
            check_fail "API 端口 9100 未监听"
        fi
    else
        check_warn "无法检查端口（netstat 和 ss 都不可用）"
    fi
    
    # 检查 Prometheus 端口 (9090)
    check_start
    if command -v netstat &> /dev/null; then
        if netstat -tuln 2>/dev/null | grep -q ":9090 "; then
            check_pass "Prometheus 端口 9090 监听正常"
        else
            check_warn "Prometheus 端口 9090 未监听（如果未启用监控可忽略）"
        fi
    elif command -v ss &> /dev/null; then
        if ss -tuln 2>/dev/null | grep -q ":9090 "; then
            check_pass "Prometheus 端口 9090 监听正常"
        else
            check_warn "Prometheus 端口 9090 未监听（如果未启用监控可忽略）"
        fi
    fi
}

# 3. 检查 API 健康端点
check_api_health() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "检查 3: API 健康端点"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    check_start
    
    if ! command -v curl &> /dev/null; then
        check_warn "curl 未安装，跳过 API 检查"
        return 0
    fi
    
    # 检查 /health 端点
    if curl -sf "${API_ENDPOINT}/health" > /dev/null 2>&1; then
        check_pass "API 健康端点响应正常"
    else
        check_fail "API 健康端点无响应"
    fi
    
    # 检查 /metrics 端点
    check_start
    if curl -sf "${PROMETHEUS_ENDPOINT}/metrics" > /dev/null 2>&1; then
        check_pass "Prometheus metrics 端点响应正常"
    else
        check_warn "Prometheus metrics 端点无响应（如果未启用监控可忽略）"
    fi
}

# 4. 检查日志错误
check_log_errors() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "检查 4: 日志错误分析"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    check_start
    
    if [ ! -f "$LOG_FILE" ]; then
        check_warn "日志文件不存在: $LOG_FILE"
        return 0
    fi
    
    # 检查最近 1000 行日志中的错误
    local error_count=$(tail -1000 "$LOG_FILE" 2>/dev/null | grep -c "ERROR" || true)
    local fatal_count=$(tail -1000 "$LOG_FILE" 2>/dev/null | grep -c "FATAL" || true)
    local panic_count=$(tail -1000 "$LOG_FILE" 2>/dev/null | grep -c "PANIC" || true)
    
    if [ "$fatal_count" -gt 0 ] || [ "$panic_count" -gt 0 ]; then
        check_fail "发现严重错误 (FATAL: $fatal_count, PANIC: $panic_count)"
    elif [ "$error_count" -gt 10 ]; then
        check_warn "最近 1000 行日志中有 $error_count 个错误（超过阈值 10）"
    elif [ "$error_count" -gt 0 ]; then
        check_pass "最近 1000 行日志中有 $error_count 个错误（在正常范围内）"
    else
        check_pass "最近 1000 行日志中无错误"
    fi
}

# 5. 检查系统资源
check_system_resources() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "检查 5: 系统资源使用"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    # 检查 CPU 使用率
    check_start
    if command -v top &> /dev/null; then
        # 获取 CPU 使用率（idle 百分比）
        local cpu_idle=$(top -bn1 | grep "Cpu(s)" | sed "s/.*, *\([0-9.]*\)%* id.*/\1/" | awk '{print int($1)}')
        local cpu_usage=$((100 - cpu_idle))
        
        if [ "$cpu_usage" -gt 80 ]; then
            check_warn "CPU 使用率较高: ${cpu_usage}%"
        else
            check_pass "CPU 使用率正常: ${cpu_usage}%"
        fi
    else
        check_warn "top 命令不可用，无法检查 CPU"
    fi
    
    # 检查内存使用率
    check_start
    if command -v free &> /dev/null; then
        local mem_usage=$(free | grep Mem | awk '{printf "%.0f", $3/$2 * 100.0}')
        
        if [ "$mem_usage" -gt 90 ]; then
            check_fail "内存使用率过高: ${mem_usage}%"
        elif [ "$mem_usage" -gt 80 ]; then
            check_warn "内存使用率较高: ${mem_usage}%"
        else
            check_pass "内存使用率正常: ${mem_usage}%"
        fi
    else
        check_warn "free 命令不可用，无法检查内存"
    fi
    
    # 检查磁盘空间
    check_start
    local disk_usage=$(df -h "$ROOT" | awk 'NR==2 {print $5}' | sed 's/%//')
    
    if [ "$disk_usage" -gt 90 ]; then
        check_fail "磁盘使用率过高: ${disk_usage}%"
    elif [ "$disk_usage" -gt 80 ]; then
        check_warn "磁盘使用率较高: ${disk_usage}%"
    else
        check_pass "磁盘使用率正常: ${disk_usage}%"
    fi
}

# 6. 检查 WebSocket 连接（如果可以访问进程信息）
check_websocket() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "检查 6: WebSocket 连接"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    check_start
    
    # 检查是否有到 Binance 的 WebSocket 连接
    if command -v netstat &> /dev/null; then
        local ws_count=$(netstat -an 2>/dev/null | grep -c "stream.binance.com" || true)
        if [ "$ws_count" -gt 0 ]; then
            check_pass "检测到 $ws_count 个 Binance WebSocket 连接"
        else
            check_warn "未检测到 Binance WebSocket 连接"
        fi
    elif command -v ss &> /dev/null; then
        local ws_count=$(ss -an 2>/dev/null | grep -c "stream.binance.com" || true)
        if [ "$ws_count" -gt 0 ]; then
            check_pass "检测到 $ws_count 个 Binance WebSocket 连接"
        else
            check_warn "未检测到 Binance WebSocket 连接"
        fi
    else
        check_warn "无法检查 WebSocket 连接（netstat 和 ss 都不可用）"
    fi
}

# 7. 检查最近的订单活动
check_order_activity() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "检查 7: 订单活动"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    check_start
    
    if [ ! -f "$LOG_FILE" ]; then
        check_warn "日志文件不存在，无法检查订单活动"
        return 0
    fi
    
    # 检查最近 5 分钟的订单日志
    local recent_orders=$(tail -1000 "$LOG_FILE" 2>/dev/null | grep -c "order_placed\|order_filled\|order_cancelled" || true)
    
    if [ "$recent_orders" -gt 0 ]; then
        check_pass "检测到最近的订单活动 (最近 1000 行日志中 $recent_orders 条记录)"
    else
        check_warn "最近无订单活动（可能正常，取决于市场条件）"
    fi
}

# ============================================================================
# 主函数
# ============================================================================

main() {
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  市场做市商系统健康检查"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "主机: $(hostname)"
    echo ""
    
    # 执行所有检查
    check_process
    check_ports
    check_api_health
    check_log_errors
    check_system_resources
    check_websocket
    check_order_activity
    
    # 输出总结
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  检查总结"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "总检查项: $TOTAL_CHECKS"
    echo "失败项数: $FAILED_CHECKS"
    echo ""
    
    if [ "$FAILED_CHECKS" -eq 0 ]; then
        log_info "所有检查通过！系统运行正常。"
        echo ""
        exit 0
    else
        log_error "$FAILED_CHECKS 个检查失败！请查看上述详情。"
        echo ""
        exit 1
    fi
}

# 执行主函数
main "$@"
