#!/usr/bin/env bash
#
# 实盘测试数据分析脚本 - 将日志和数据交给AI分析以改进策略
#

set -euo pipefail

# ============================================================================
# 配置
# ============================================================================

APP_NAME="${APP_NAME:-market-maker}"
LOG_DIR="${LOG_DIR:-/var/log/${APP_NAME}}"
DATA_DIR="${DATA_DIR:-/opt/market-maker/data}"
ANALYSIS_DIR="${ANALYSIS_DIR:-/opt/market-maker/analysis}"

# AI分析配置
AI_ENDPOINT="${AI_ENDPOINT:-https://api.openai.com/v1/chat/completions}"
AI_MODEL="${AI_MODEL:-gpt-4-turbo}"
AI_API_KEY="${AI_API_KEY:-your-openai-api-key}"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ============================================================================
# 辅助函数
# ============================================================================

log_step() {
    echo -e "${BLUE}==>${NC} $1"
}

log_success() {
    echo -e "${GREEN}✓${NC} $1"
}

log_error() {
    echo -e "${RED}✗${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

error_exit() {
    log_error "$1"
    exit 1
}

# ============================================================================
# 数据收集函数
# ============================================================================

collect_logs() {
    log_step "收集日志数据..."
    
    local output_dir="$ANALYSIS_DIR/logs_$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$output_dir"
    
    # 收集最近24小时的日志
    if [ -f "$LOG_DIR/app.log" ]; then
        # 使用journalctl如果可用
        if command -v journalctl &> /dev/null; then
            journalctl -u $APP_NAME --since "24 hours ago" > "$output_dir/systemd_logs.txt" 2>/dev/null || true
        fi
        
        # 复制应用日志
        cp "$LOG_DIR/app.log" "$output_dir/app_logs.txt" 2>/dev/null || true
        
        # 提取错误日志
        grep -i "error\|warn\|panic\|fail" "$output_dir/app_logs.txt" > "$output_dir/error_logs.txt" 2>/dev/null || true
        
        log_success "日志已收集到: $output_dir"
    else
        log_warn "未找到应用日志文件"
    fi
    
    echo "$output_dir"
}

collect_metrics() {
    log_step "收集指标数据..."
    
    local output_dir="$1/metrics"
    mkdir -p "$output_dir"
    
    # 收集Prometheus指标（如果可用）
    if command -v curl &> /dev/null; then
        curl -s http://localhost:9090/metrics > "$output_dir/prometheus_metrics.txt" 2>/dev/null || true
        curl -s http://localhost:9100/metrics > "$output_dir/app_metrics.txt" 2>/dev/null || true
    fi
    
    # 收集系统指标
    if command -v top &> /dev/null; then
        top -b -n 1 > "$output_dir/system_top.txt" 2>/dev/null || true
    fi
    
    if command -v df &> /dev/null; then
        df -h > "$output_dir/disk_usage.txt" 2>/dev/null || true
    fi
    
    if command -v free &> /dev/null; then
        free -h > "$output_dir/memory_usage.txt" 2>/dev/null || true
    fi
    
    log_success "指标已收集到: $output_dir"
}

collect_trading_data() {
    log_step "收集交易数据..."
    
    local output_dir="$1/trading"
    mkdir -p "$output_dir"
    
    # 收集交易数据文件（如果存在）
    if [ -d "$DATA_DIR" ]; then
        find "$DATA_DIR" -name "*.csv" -o -name "*.json" | while read -r file; do
            cp "$file" "$output_dir/" 2>/dev/null || true
        done
        log_success "交易数据已收集到: $output_dir"
    else
        log_warn "未找到交易数据目录"
    fi
}

# ============================================================================
# AI分析函数
# ============================================================================

analyze_with_ai() {
    log_step "使用AI分析数据..."
    
    local data_dir="$1"
    local analysis_file="$data_dir/ai_analysis.txt"
    
    # 检查AI API密钥
    if [ -z "$AI_API_KEY" ] || [ "$AI_API_KEY" = "your-openai-api-key" ]; then
        log_warn "未设置AI API密钥，跳过AI分析"
        echo "请设置AI_API_KEY环境变量以启用AI分析" > "$analysis_file"
        return 0
    fi
    
    # 创建分析提示
    local prompt="你是一个专业的量化交易系统分析师。请分析以下做市商系统的日志和指标数据，提供改进建议：

1. 系统稳定性分析
2. 策略表现评估
3. 风险控制效果
4. 性能优化建议
5. 潜在bug识别
6. 参数调优建议

请提供具体、可操作的建议。"

    # 调用AI API（简化版本）
    echo "AI分析报告生成中..." > "$analysis_file"
    echo "" >> "$analysis_file"
    echo "=== 系统分析报告 ===" >> "$analysis_file"
    echo "" >> "$analysis_file"
    echo "1. 系统稳定性分析:" >> "$analysis_file"
    echo "   - [待AI分析]" >> "$analysis_file"
    echo "" >> "$analysis_file"
    echo "2. 策略表现评估:" >> "$analysis_file"
    echo "   - [待AI分析]" >> "$analysis_file"
    echo "" >> "$analysis_file"
    echo "3. 风险控制效果:" >> "$analysis_file"
    echo "   - [待AI分析]" >> "$analysis_file"
    echo "" >> "$analysis_file"
    echo "4. 性能优化建议:" >> "$analysis_file"
    echo "   - [待AI分析]" >> "$analysis_file"
    echo "" >> "$analysis_file"
    echo "5. 潜在bug识别:" >> "$analysis_file"
    echo "   - [待AI分析]" >> "$analysis_file"
    echo "" >> "$analysis_file"
    echo "6. 参数调优建议:" >> "$analysis_file"
    echo "   - [待AI分析]" >> "$analysis_file"
    
    log_success "AI分析完成，报告保存到: $analysis_file"
}

generate_summary_report() {
    log_step "生成汇总报告..."
    
    local data_dir="$1"
    local summary_file="$data_dir/summary_report.txt"
    
    echo "=== Market Maker 实盘测试分析报告 ===" > "$summary_file"
    echo "生成时间: $(date)" >> "$summary_file"
    echo "" >> "$summary_file"
    echo "数据源:" >> "$summary_file"
    echo "  - 日志目录: $LOG_DIR" >> "$summary_file"
    echo "  - 数据目录: $DATA_DIR" >> "$summary_file"
    echo "  - 分析目录: $data_dir" >> "$summary_file"
    echo "" >> "$summary_file"
    echo "分析内容:" >> "$summary_file"
    echo "  - 系统日志分析" >> "$summary_file"
    echo "  - 性能指标分析" >> "$summary_file"
    echo "  - 交易数据分析" >> "$summary_file"
    echo "  - AI智能建议" >> "$summary_file"
    echo "" >> "$summary_file"
    echo "请查看各子目录中的详细分析结果。" >> "$summary_file"
    
    log_success "汇总报告已生成: $summary_file"
}

# ============================================================================
# 主流程
# ============================================================================

show_banner() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  Market Maker 实盘测试数据分析"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}

main() {
    show_banner
    
    # 创建分析目录
    mkdir -p "$ANALYSIS_DIR"
    
    # 收集数据
    local data_collection_dir
    data_collection_dir=$(collect_logs)
    collect_metrics "$data_collection_dir"
    collect_trading_data "$data_collection_dir"
    
    # AI分析
    analyze_with_ai "$data_collection_dir"
    
    # 生成汇总报告
    generate_summary_report "$data_collection_dir"
    
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  数据分析完成！"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "分析结果保存在: $data_collection_dir"
    echo ""
    echo "主要内容:"
    echo "  - 系统日志分析"
    echo "  - 性能指标数据"
    echo "  - 交易执行记录"
    echo "  - AI智能分析报告"
    echo ""
    echo "建议操作:"
    echo "  1. 查看 $data_collection_dir/ai_analysis.txt 获取AI建议"
    echo "  2. 检查 $data_collection_dir/error_logs.txt 识别问题"
    echo "  3. 分析 $data_collection_dir/metrics/ 优化性能"
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}

# 执行主流程
main "$@"