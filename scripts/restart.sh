#!/usr/bin/env bash
#
# 重启 market-maker 服务
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

echo "重启 ${APP_NAME} 服务..."

# 检查是否有 systemd
if command -v systemctl &> /dev/null; then
    # 重启服务
    sudo systemctl restart ${APP_NAME}.service
    
    # 等待服务启动
    sleep 2
    
    # 检查状态
    if systemctl is-active --quiet ${APP_NAME}.service; then
        log_info "服务重启成功"
        systemctl status ${APP_NAME}.service --no-pager
    else
        log_error "服务重启失败"
        echo ""
        echo "查看详细日志："
        echo "  sudo journalctl -u ${APP_NAME} -n 50"
        exit 1
    fi
else
    log_error "systemctl 不可用，无法重启服务"
    exit 1
fi
