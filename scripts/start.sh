#!/usr/bin/env bash
#
# 一键启动脚本 - 启动 market-maker 服务
#

set -euo pipefail

# ============================================================================
# 配置
# ============================================================================

APP_NAME="${APP_NAME:-market-maker}"
DEPLOY_DIR="${DEPLOY_DIR:-/opt/market-maker}"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# ============================================================================
# 辅助函数
# ============================================================================

log_info() {
    echo -e "${GREEN}✓${NC} $1"
}

log_error() {
    echo -e "${RED}✗${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

# ============================================================================
# 启动函数
# ============================================================================

start_service() {
    echo "启动 ${APP_NAME} 服务..."
    
    # 检查是否有 systemd
    if command -v systemctl &> /dev/null; then
        # 检查服务是否存在
        if systemctl list-unit-files | grep -q "${APP_NAME}.service"; then
            # 启动服务
            sudo systemctl start ${APP_NAME}.service
            
            # 等待服务启动
            sleep 2
            
            # 检查状态
            if systemctl is-active --quiet ${APP_NAME}.service; then
                log_info "服务已启动"
                echo ""
                echo "查看日志命令:"
                echo "  sudo journalctl -u ${APP_NAME} -f"
                echo ""
                echo "查看状态命令:"
                echo "  sudo systemctl status ${APP_NAME}"
                return 0
            else
                log_error "服务启动失败"
                systemctl status ${APP_NAME}.service --no-pager
                return 1
            fi
        else
            log_error "服务文件不存在，请先运行部署脚本"
            return 1
        fi
    else
        log_error "systemctl 不可用，无法启动服务"
        return 1
    fi
}

# ============================================================================
# 主流程
# ============================================================================

main() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  Market Maker 启动脚本"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    # 检查环境变量
    if [ -z "${BINANCE_API_KEY:-}" ] || [ -z "${BINANCE_API_SECRET:-}" ]; then
        echo -e "${YELLOW}警告: 未设置API密钥${NC}"
        echo ""
        echo "请设置环境变量："
        echo "  export BINANCE_API_KEY=\"your_api_key_here\""
        echo "  export BINANCE_API_SECRET=\"your_secret_here\""
        echo ""
        if ! confirm "是否继续启动（无API密钥将无法连接交易所）？"; then
            echo "启动已取消"
            exit 0
        fi
        echo ""
    else
        log_info "API密钥已设置"
    fi
    
    # 启动服务
    start_service
}

confirm() {
    echo -e "${YELLOW}?${NC} $1 [y/N] "
    read -r response
    case "$response" in
        [yY][eE][sS]|[yY]) 
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

# 执行主流程
main "$@"