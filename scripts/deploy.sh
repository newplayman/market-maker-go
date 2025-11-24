#!/usr/bin/env bash
#
# 部署脚本 - 自动化部署 market-maker 到生产环境
#

set -euo pipefail

# ============================================================================
# 配置
# ============================================================================

APP_NAME="market-maker"
BUILD_DIR="build"
DEPLOY_USER="${DEPLOY_USER:-trader}"
DEPLOY_HOST="${DEPLOY_HOST:-localhost}"
DEPLOY_DIR="${DEPLOY_DIR:-/opt/market-maker}"
GO_VERSION_REQUIRED="1.21"

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

# ============================================================================
# 检查函数
# ============================================================================

check_go_version() {
    log_step "检查 Go 版本..."
    
    if ! command -v go &> /dev/null; then
        error_exit "Go 未安装，请先安装 Go $GO_VERSION_REQUIRED 或更高版本"
    fi
    
    local go_version=$(go version | awk '{print $3}' | sed 's/go//')
    local major_minor=$(echo "$go_version" | cut -d. -f1,2)
    
    if [[ "$(printf '%s\n' "$GO_VERSION_REQUIRED" "$major_minor" | sort -V | head -n1)" != "$GO_VERSION_REQUIRED" ]]; then
        error_exit "Go 版本过低: $go_version，需要 $GO_VERSION_REQUIRED 或更高版本"
    fi
    
    log_success "Go 版本正常: $go_version"
}

check_dependencies() {
    log_step "检查系统依赖..."
    
    local missing_deps=()
    
    # 检查必需的命令
    for cmd in git ssh scp; do
        if ! command -v $cmd &> /dev/null; then
            missing_deps+=("$cmd")
        fi
    done
    
    if [ ${#missing_deps[@]} -ne 0 ]; then
        error_exit "缺少依赖: ${missing_deps[*]}"
    fi
    
    log_success "系统依赖检查通过"
}

check_config() {
    log_step "检查配置文件..."
    
    if [ ! -f "configs/config.yaml" ]; then
        error_exit "配置文件不存在: configs/config.yaml"
    fi
    
    # 简单验证配置文件格式
    if ! grep -q "exchange:" configs/config.yaml; then
        error_exit "配置文件格式错误"
    fi
    
    log_success "配置文件检查通过"
}

check_remote_connection() {
    log_step "检查远程服务器连接..."
    
    if [ "$DEPLOY_HOST" = "localhost" ]; then
        log_warn "部署到本地，跳过远程连接检查"
        return 0
    fi
    
    if ! ssh -o ConnectTimeout=5 -o BatchMode=yes ${DEPLOY_USER}@${DEPLOY_HOST} "echo 'Connection OK'" &> /dev/null; then
        error_exit "无法连接到远程服务器 ${DEPLOY_USER}@${DEPLOY_HOST}"
    fi
    
    log_success "远程服务器连接正常"
}

# ============================================================================
# 构建函数
# ============================================================================

clean_build() {
    log_step "清理构建目录..."
    rm -rf "$BUILD_DIR"
    mkdir -p "$BUILD_DIR"
    log_success "构建目录已清理"
}

run_tests() {
    log_step "运行测试..."
    
    if ! go test -v ./... -timeout 5m; then
        error_exit "测试失败，请修复后重新部署"
    fi
    
    log_success "所有测试通过"
}

build_binary() {
    log_step "编译程序..."
    
    # 编译主程序
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
        -ldflags "-s -w -X main.Version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev') -X main.BuildTime=$(date -u '+%Y-%m-%d_%H:%M:%S')" \
        -o ${BUILD_DIR}/trader \
        ./cmd/runner/main.go
    
    if [ ! -f "${BUILD_DIR}/trader" ]; then
        error_exit "编译失败"
    fi
    
    # 编译其他工具
    log_step "编译管理工具..."
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ${BUILD_DIR}/binance_panic ./cmd/binance_panic/main.go || log_warn "binance_panic 编译失败（非必需）"
    
    log_success "程序编译完成"
    
    # 显示二进制文件信息
    ls -lh ${BUILD_DIR}/
}

# ============================================================================
# 部署函数
# ============================================================================

prepare_remote_directories() {
    log_step "准备远程目录结构..."
    
    if [ "$DEPLOY_HOST" = "localhost" ]; then
        # 本地部署
        mkdir -p ${DEPLOY_DIR}/{bin,configs,logs,data,backups}
    else
        # 远程部署
        ssh ${DEPLOY_USER}@${DEPLOY_HOST} "mkdir -p ${DEPLOY_DIR}/{bin,configs,logs,data,backups}"
    fi
    
    log_success "远程目录已准备"
}

backup_current_version() {
    log_step "备份当前版本..."
    
    local backup_name="backup_$(date +%Y%m%d_%H%M%S)"
    
    if [ "$DEPLOY_HOST" = "localhost" ]; then
        if [ -f "${DEPLOY_DIR}/bin/trader" ]; then
            cp ${DEPLOY_DIR}/bin/trader ${DEPLOY_DIR}/backups/${backup_name}
            log_success "当前版本已备份: ${backup_name}"
        else
            log_warn "未发现旧版本，跳过备份"
        fi
    else
        ssh ${DEPLOY_USER}@${DEPLOY_HOST} "
            if [ -f ${DEPLOY_DIR}/bin/trader ]; then
                cp ${DEPLOY_DIR}/bin/trader ${DEPLOY_DIR}/backups/${backup_name}
                echo 'Backup created: ${backup_name}'
            fi
        " || log_warn "备份失败（可能是首次部署）"
    fi
}

upload_files() {
    log_step "上传文件到服务器..."
    
    if [ "$DEPLOY_HOST" = "localhost" ]; then
        # 本地部署
        cp ${BUILD_DIR}/trader ${DEPLOY_DIR}/bin/
        cp configs/config.yaml ${DEPLOY_DIR}/configs/
        cp scripts/*.sh ${DEPLOY_DIR}/bin/ 2>/dev/null || true
        chmod +x ${DEPLOY_DIR}/bin/*
    else
        # 上传二进制文件
        scp ${BUILD_DIR}/trader ${DEPLOY_USER}@${DEPLOY_HOST}:${DEPLOY_DIR}/bin/
        
        # 上传配置文件（如果不存在）
        ssh ${DEPLOY_USER}@${DEPLOY_HOST} "test -f ${DEPLOY_DIR}/configs/config.yaml" || \
            scp configs/config.yaml ${DEPLOY_USER}@${DEPLOY_HOST}:${DEPLOY_DIR}/configs/
        
        # 上传脚本
        scp scripts/*.sh ${DEPLOY_USER}@${DEPLOY_HOST}:${DEPLOY_DIR}/bin/ 2>/dev/null || true
        
        # 设置执行权限
        ssh ${DEPLOY_USER}@${DEPLOY_HOST} "chmod +x ${DEPLOY_DIR}/bin/*"
    fi
    
    log_success "文件上传完成"
}

create_systemd_service() {
    log_step "配置 Systemd 服务..."
    
    local service_file="/tmp/${APP_NAME}.service"
    
    cat > "$service_file" <<EOF
[Unit]
Description=Market Maker Trading Bot
After=network.target

[Service]
Type=simple
User=${DEPLOY_USER}
WorkingDirectory=${DEPLOY_DIR}
ExecStart=${DEPLOY_DIR}/bin/trader -config ${DEPLOY_DIR}/configs/config.yaml
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${APP_NAME}

# 资源限制
LimitNOFILE=65536
MemoryLimit=2G

# 安全设置
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
    
    if [ "$DEPLOY_HOST" = "localhost" ]; then
        sudo cp "$service_file" /etc/systemd/system/${APP_NAME}.service
        sudo systemctl daemon-reload
    else
        scp "$service_file" ${DEPLOY_USER}@${DEPLOY_HOST}:/tmp/
        ssh ${DEPLOY_USER}@${DEPLOY_HOST} "
            sudo cp /tmp/${APP_NAME}.service /etc/systemd/system/
            sudo systemctl daemon-reload
        "
    fi
    
    rm -f "$service_file"
    log_success "Systemd 服务已配置"
}

setup_logging() {
    log_step "配置日志..."
    
    if [ "$DEPLOY_HOST" = "localhost" ]; then
        sudo mkdir -p /var/log/${APP_NAME}
        sudo chown ${DEPLOY_USER}:${DEPLOY_USER} /var/log/${APP_NAME}
    else
        ssh ${DEPLOY_USER}@${DEPLOY_HOST} "
            sudo mkdir -p /var/log/${APP_NAME}
            sudo chown ${DEPLOY_USER}:${DEPLOY_USER} /var/log/${APP_NAME}
        "
    fi
    
    log_success "日志目录已配置"
}

# ============================================================================
# 启动和验证
# ============================================================================

start_service() {
    log_step "启动服务..."
    
    if [ "$DEPLOY_HOST" = "localhost" ]; then
        if sudo systemctl is-active --quiet ${APP_NAME}.service; then
            log_warn "服务已在运行，正在重启..."
            sudo systemctl restart ${APP_NAME}.service
        else
            sudo systemctl start ${APP_NAME}.service
        fi
        sudo systemctl enable ${APP_NAME}.service
    else
        ssh ${DEPLOY_USER}@${DEPLOY_HOST} "
            if sudo systemctl is-active --quiet ${APP_NAME}.service; then
                echo 'Restarting service...'
                sudo systemctl restart ${APP_NAME}.service
            else
                sudo systemctl start ${APP_NAME}.service
            fi
            sudo systemctl enable ${APP_NAME}.service
        "
    fi
    
    log_success "服务已启动"
}

verify_deployment() {
    log_step "验证部署..."
    
    # 等待服务启动
    sleep 3
    
    if [ "$DEPLOY_HOST" = "localhost" ]; then
        if sudo systemctl is-active --quiet ${APP_NAME}.service; then
            log_success "服务运行正常"
        else
            error_exit "服务启动失败，请检查日志: sudo journalctl -u ${APP_NAME} -n 50"
        fi
    else
        if ssh ${DEPLOY_USER}@${DEPLOY_HOST} "sudo systemctl is-active --quiet ${APP_NAME}.service"; then
            log_success "服务运行正常"
        else
            error_exit "服务启动失败，请检查远程日志"
        fi
    fi
}

# ============================================================================
# 主流程
# ============================================================================

show_banner() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  Market Maker 部署脚本"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  目标主机: ${DEPLOY_HOST}"
    echo "  部署用户: ${DEPLOY_USER}"
    echo "  部署目录: ${DEPLOY_DIR}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}

show_summary() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  部署完成！"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "后续操作命令："
    echo ""
    if [ "$DEPLOY_HOST" = "localhost" ]; then
        echo "  查看状态: sudo systemctl status ${APP_NAME}"
        echo "  查看日志: sudo journalctl -u ${APP_NAME} -f"
        echo "  停止服务: sudo systemctl stop ${APP_NAME}"
        echo "  重启服务: sudo systemctl restart ${APP_NAME}"
        echo "  健康检查: ${DEPLOY_DIR}/bin/health_check.sh"
    else
        echo "  查看状态: ssh ${DEPLOY_USER}@${DEPLOY_HOST} 'sudo systemctl status ${APP_NAME}'"
        echo "  查看日志: ssh ${DEPLOY_USER}@${DEPLOY_HOST} 'sudo journalctl -u ${APP_NAME} -f'"
        echo "  健康检查: ssh ${DEPLOY_USER}@${DEPLOY_HOST} '${DEPLOY_DIR}/bin/health_check.sh'"
    fi
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}

main() {
    show_banner
    
    # 环境检查
    check_go_version
    check_dependencies
    check_config
    check_remote_connection
    
    # 确认部署
    echo ""
    if ! confirm "确认要部署到 ${DEPLOY_USER}@${DEPLOY_HOST}:${DEPLOY_DIR} 吗？"; then
        log_warn "部署已取消"
        exit 0
    fi
    echo ""
    
    # 构建
    clean_build
    
    # 可选：运行测试
    if confirm "是否运行测试？（推荐）"; then
        run_tests
    else
        log_warn "跳过测试"
    fi
    
    build_binary
    
    # 部署
    prepare_remote_directories
    backup_current_version
    upload_files
    create_systemd_service
    setup_logging
    
    # 启动
    echo ""
    if confirm "是否立即启动服务？"; then
        start_service
        verify_deployment
    else
        log_warn "请手动启动服务"
    fi
    
    show_summary
}

# 执行主流程
main "$@"
