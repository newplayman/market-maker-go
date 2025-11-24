#!/usr/bin/env bash
#
# 备份脚本 - 备份配置和数据
#

set -euo pipefail

APP_NAME="${APP_NAME:-market-maker}"
DEPLOY_DIR="${DEPLOY_DIR:-/opt/market-maker}"
BACKUP_DIR="${BACKUP_DIR:-${DEPLOY_DIR}/backups}"
BACKUP_NAME="backup_$(date +%Y%m%d_%H%M%S)"

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

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  备份 ${APP_NAME}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 创建备份目录
log_step "创建备份目录..."
mkdir -p "${BACKUP_DIR}/${BACKUP_NAME}"

# 备份二进制文件
log_step "备份二进制文件..."
if [ -f "${DEPLOY_DIR}/bin/trader" ]; then
    cp "${DEPLOY_DIR}/bin/trader" "${BACKUP_DIR}/${BACKUP_NAME}/trader"
    log_info "二进制文件已备份"
else
    log_warn "未找到二进制文件"
fi

# 备份配置文件
log_step "备份配置文件..."
if [ -f "${DEPLOY_DIR}/configs/config.yaml" ]; then
    cp "${DEPLOY_DIR}/configs/config.yaml" "${BACKUP_DIR}/${BACKUP_NAME}/config.yaml"
    log_info "配置文件已备份"
else
    log_warn "未找到配置文件"
fi

# 备份日志（最近的）
log_step "备份最近的日志..."
if [ -d "/var/log/${APP_NAME}" ]; then
    mkdir -p "${BACKUP_DIR}/${BACKUP_NAME}/logs"
    # 只备份最近 7 天的日志
    find /var/log/${APP_NAME} -name "*.log" -mtime -7 -exec cp {} "${BACKUP_DIR}/${BACKUP_NAME}/logs/" \; 2>/dev/null || true
    log_info "日志已备份"
else
    log_warn "未找到日志目录"
fi

# 备份数据文件（如果有）
log_step "备份数据文件..."
if [ -d "${DEPLOY_DIR}/data" ] && [ "$(ls -A ${DEPLOY_DIR}/data 2>/dev/null)" ]; then
    cp -r "${DEPLOY_DIR}/data" "${BACKUP_DIR}/${BACKUP_NAME}/"
    log_info "数据文件已备份"
else
    log_warn "未找到数据文件"
fi

# 创建备份清单
log_step "创建备份清单..."
cat > "${BACKUP_DIR}/${BACKUP_NAME}/manifest.txt" << MANIFEST
备份信息
========================================
备份时间: $(date '+%Y-%m-%d %H:%M:%S')
主机名: $(hostname)
应用名: ${APP_NAME}
备份内容:
$(ls -lh "${BACKUP_DIR}/${BACKUP_NAME}")
========================================
MANIFEST

# 压缩备份
log_step "压缩备份..."
cd "${BACKUP_DIR}"
tar -czf "${BACKUP_NAME}.tar.gz" "${BACKUP_NAME}"
rm -rf "${BACKUP_NAME}"

# 显示备份信息
BACKUP_SIZE=$(du -h "${BACKUP_DIR}/${BACKUP_NAME}.tar.gz" | cut -f1)
log_info "备份完成: ${BACKUP_NAME}.tar.gz (大小: ${BACKUP_SIZE})"

# 清理旧备份（保留最近 30 个）
log_step "清理旧备份..."
BACKUP_COUNT=$(ls -1 "${BACKUP_DIR}"/backup_*.tar.gz 2>/dev/null | wc -l)
if [ "$BACKUP_COUNT" -gt 30 ]; then
    REMOVE_COUNT=$((BACKUP_COUNT - 30))
    ls -1t "${BACKUP_DIR}"/backup_*.tar.gz | tail -${REMOVE_COUNT} | xargs rm -f
    log_info "已删除 ${REMOVE_COUNT} 个旧备份"
else
    log_info "当前备份数: ${BACKUP_COUNT}"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "备份文件: ${BACKUP_DIR}/${BACKUP_NAME}.tar.gz"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
