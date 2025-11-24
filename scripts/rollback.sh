#!/usr/bin/env bash
#
# 回滚脚本 - 恢复到之前的版本
#

set -euo pipefail

APP_NAME="${APP_NAME:-market-maker}"
DEPLOY_DIR="${DEPLOY_DIR:-/opt/market-maker}"
BACKUP_DIR="${BACKUP_DIR:-${DEPLOY_DIR}/backups}"

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_step() {
    echo -e "${BLUE}==>${NC} $1"
}

log_info() {
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

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  ${APP_NAME} 版本回滚"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 检查备份目录
if [ ! -d "$BACKUP_DIR" ]; then
    error_exit "备份目录不存在: $BACKUP_DIR"
fi

# 列出可用的备份
log_step "可用的备份版本："
echo ""
BACKUPS=($(ls -1t "${BACKUP_DIR}"/backup_*.tar.gz 2>/dev/null || true))

if [ ${#BACKUPS[@]} -eq 0 ]; then
    error_exit "未找到任何备份文件"
fi

# 显示备份列表
for i in "${!BACKUPS[@]}"; do
    BACKUP_FILE="${BACKUPS[$i]}"
    BACKUP_NAME=$(basename "$BACKUP_FILE" .tar.gz)
    BACKUP_DATE=$(echo "$BACKUP_NAME" | sed 's/backup_//' | sed 's/_/ /')
    BACKUP_SIZE=$(du -h "$BACKUP_FILE" | cut -f1)
    echo "  [$((i+1))] $BACKUP_DATE (大小: $BACKUP_SIZE)"
done

echo ""

# 选择备份
if [ $# -eq 1 ]; then
    SELECTED_INDEX=$((${1} - 1))
else
    echo -n "请选择要回滚的版本 [1-${#BACKUPS[@]}]: "
    read SELECTED
    SELECTED_INDEX=$((SELECTED - 1))
fi

if [ $SELECTED_INDEX -lt 0 ] || [ $SELECTED_INDEX -ge ${#BACKUPS[@]} ]; then
    error_exit "无效的选择"
fi

BACKUP_FILE="${BACKUPS[$SELECTED_INDEX]}"
BACKUP_NAME=$(basename "$BACKUP_FILE" .tar.gz)

log_info "选择的备份: $BACKUP_NAME"
echo ""

# 确认回滚
echo -e "${YELLOW}⚠${NC} 此操作将："
echo "  1. 停止当前服务"
echo "  2. 备份当前版本"
echo "  3. 恢复到选择的版本"
echo "  4. 重启服务"
echo ""
echo -n "确认继续? [y/N] "
read -r CONFIRM
if [[ ! $CONFIRM =~ ^[Yy]$ ]]; then
    log_warn "回滚已取消"
    exit 0
fi
echo ""

# 停止服务
log_step "停止服务..."
if command -v systemctl &> /dev/null; then
    if systemctl is-active --quiet ${APP_NAME}.service; then
        sudo systemctl stop ${APP_NAME}.service
        log_info "服务已停止"
    else
        log_warn "服务未运行"
    fi
fi

# 备份当前版本
log_step "备份当前版本..."
CURRENT_BACKUP="rollback_before_$(date +%Y%m%d_%H%M%S)"
mkdir -p "${BACKUP_DIR}/${CURRENT_BACKUP}"
[ -f "${DEPLOY_DIR}/bin/trader" ] && cp "${DEPLOY_DIR}/bin/trader" "${BACKUP_DIR}/${CURRENT_BACKUP}/" || true
[ -f "${DEPLOY_DIR}/configs/config.yaml" ] && cp "${DEPLOY_DIR}/configs/config.yaml" "${BACKUP_DIR}/${CURRENT_BACKUP}/" || true
cd "${BACKUP_DIR}"
tar -czf "${CURRENT_BACKUP}.tar.gz" "${CURRENT_BACKUP}"
rm -rf "${CURRENT_BACKUP}"
log_info "当前版本已备份: ${CURRENT_BACKUP}.tar.gz"

# 解压备份
log_step "解压备份文件..."
TEMP_DIR=$(mktemp -d)
tar -xzf "$BACKUP_FILE" -C "$TEMP_DIR"
BACKUP_CONTENT_DIR="${TEMP_DIR}/${BACKUP_NAME}"

# 恢复二进制文件
log_step "恢复二进制文件..."
if [ -f "${BACKUP_CONTENT_DIR}/trader" ]; then
    cp "${BACKUP_CONTENT_DIR}/trader" "${DEPLOY_DIR}/bin/trader"
    chmod +x "${DEPLOY_DIR}/bin/trader"
    log_info "二进制文件已恢复"
else
    log_warn "备份中未找到二进制文件"
fi

# 恢复配置文件
log_step "恢复配置文件..."
if [ -f "${BACKUP_CONTENT_DIR}/config.yaml" ]; then
    log_warn "发现备份的配置文件，是否恢复? [y/N] "
    read -r RESTORE_CONFIG
    if [[ $RESTORE_CONFIG =~ ^[Yy]$ ]]; then
        cp "${BACKUP_CONTENT_DIR}/config.yaml" "${DEPLOY_DIR}/configs/config.yaml"
        log_info "配置文件已恢复"
    else
        log_warn "跳过配置文件恢复"
    fi
else
    log_warn "备份中未找到配置文件"
fi

# 清理临时目录
rm -rf "$TEMP_DIR"

# 启动服务
log_step "启动服务..."
if command -v systemctl &> /dev/null; then
    sudo systemctl start ${APP_NAME}.service
    sleep 2
    
    if systemctl is-active --quiet ${APP_NAME}.service; then
        log_info "服务已启动"
    else
        log_error "服务启动失败，请检查日志"
        exit 1
    fi
fi

# 验证回滚
log_step "验证回滚..."
if [ -f "${DEPLOY_DIR}/bin/trader" ]; then
    VERSION_INFO=$(${DEPLOY_DIR}/bin/trader -version 2>/dev/null || echo "unknown")
    log_info "当前版本: $VERSION_INFO"
else
    log_warn "无法获取版本信息"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  回滚完成！"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "当前版本已备份为: ${CURRENT_BACKUP}.tar.gz"
echo ""
