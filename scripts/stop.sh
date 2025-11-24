#!/usr/bin/env bash
#
# 停止 market-maker 服务
#

set -euo pipefail

APP_NAME="${APP_NAME:-market-maker}"

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}✓${NC} $1"
}

log_error() {
    echo -e "${RED}✗${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

echo "停止 ${APP_NAME} 服务..."

# 检查是否有 systemd
if command -v systemctl &> /dev/null; then
    # 检查服务是否在运行
    if ! systemctl is-active --quiet ${APP_NAME}.service; then
        log_warn "服务未在运行"
        exit 0
    fi
    
    # 停止服务
    sudo systemctl stop ${APP_NAME}.service
    
    # 等待服务停止
    sleep 2
    
    # 检查状态
    if ! systemctl is-active --quiet ${APP_NAME}.service; then
        log_info "服务已停止"
    else
        log_error "服务停止失败"
        systemctl status ${APP_NAME}.service --no-pager
        exit 1
    fi
else
    log_error "systemctl 不可用，无法停止服务"
    exit 1
fi
