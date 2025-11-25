#!/usr/bin/env bash
#
# 一键部署脚本 - 部署 market-maker 到实盘测试服务器
#

set -euo pipefail

# ============================================================================
# 配置
# ============================================================================

# 实盘测试服务器配置
DEPLOY_USER="${DEPLOY_USER:-trader}"
DEPLOY_HOST="${DEPLOY_HOST:-your-server-ip}"  # 请修改为您的服务器IP
DEPLOY_DIR="${DEPLOY_DIR:-/opt/market-maker}"
APP_NAME="market-maker"

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
# 部署函数
# ============================================================================

check_environment() {
    log_step "检查环境..."
    
    # 检查必需命令
    for cmd in git ssh scp go; do
        if ! command -v $cmd &> /dev/null; then
            error_exit "缺少必需命令: $cmd"
        fi
    done
    
    # 检查SSH连接
    if ! ssh -o ConnectTimeout=5 -o BatchMode=yes ${DEPLOY_USER}@${DEPLOY_HOST} "echo 'Connection OK'" &> /dev/null; then
        error_exit "无法连接到服务器 ${DEPLOY_USER}@${DEPLOY_HOST}，请检查SSH配置"
    fi
    
    log_success "环境检查通过"
}

clone_repository() {
    log_step "克隆最新代码..."
    
    # 在服务器上克隆仓库
    ssh ${DEPLOY_USER}@${DEPLOY_HOST} "
        if [ -d '${DEPLOY_DIR}' ]; then
            cd '${DEPLOY_DIR}' && git pull origin main
        else
            git clone https://github.com/newplayman/market-maker-go.git '${DEPLOY_DIR}'
        fi
    " || error_exit "克隆/更新代码失败"
    
    log_success "代码已更新到最新版本"
}

build_binary() {
    log_step "编译程序..."
    
    # 在服务器上编译
    ssh ${DEPLOY_USER}@${DEPLOY_HOST} "
        cd '${DEPLOY_DIR}' && 
        GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
            -ldflags '-s -w -X main.Version=$(date +%Y%m%d-%H%M%S) -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)' \
            -o build/trader \
            ./cmd/runner/main.go
    " || error_exit "编译失败"
    
    log_success "程序编译完成"
}

setup_directories() {
    log_step "设置目录结构..."
    
    ssh ${DEPLOY_USER}@${DEPLOY_HOST} "
        mkdir -p ${DEPLOY_DIR}/{bin,configs,logs,data,backups} &&
        mkdir -p /var/log/${APP_NAME} 2>/dev/null || true
    " || error_exit "创建目录失败"
    
    log_success "目录结构已设置"
}

upload_config() {
    log_step "上传配置文件..."
    
    # 检查本地配置文件
    if [ ! -f "configs/config.yaml" ]; then
        log_warn "本地配置文件不存在，将使用服务器上的配置文件"
        return 0
    fi
    
    # 上传配置文件
    scp configs/config.yaml ${DEPLOY_USER}@${DEPLOY_HOST}:${DEPLOY_DIR}/configs/ || error_exit "上传配置文件失败"
    
    log_success "配置文件已上传"
}

create_systemd_service() {
    log_step "创建 Systemd 服务..."
    
    local service_content="[Unit]
Description=Market Maker Trading Bot
After=network.target

[Service]
Type=simple
User=${DEPLOY_USER}
WorkingDirectory=${DEPLOY_DIR}
ExecStart=${DEPLOY_DIR}/build/trader -config ${DEPLOY_DIR}/configs/config.yaml
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
WantedBy=multi-user.target"

    echo "$service_content" | ssh ${DEPLOY_USER}@${DEPLOY_HOST} "sudo tee /etc/systemd/system/${APP_NAME}.service" > /dev/null || error_exit "创建服务文件失败"
    
    ssh ${DEPLOY_USER}@${DEPLOY_HOST} "
        sudo systemctl daemon-reload
    " || error_exit "重载 systemd 失败"
    
    log_success "Systemd 服务已创建"
}

backup_current_version() {
    log_step "备份当前版本..."
    
    local backup_name="backup_$(date +%Y%m%d_%H%M%S)"
    
    ssh ${DEPLOY_USER}@${DEPLOY_HOST} "
        if [ -f ${DEPLOY_DIR}/build/trader ]; then
            cp ${DEPLOY_DIR}/build/trader ${DEPLOY_DIR}/backups/${backup_name}
            echo 'Backup created: ${backup_name}'
        fi
    " || log_warn "备份失败（可能是首次部署）"
    
    log_success "备份完成"
}

deploy_files() {
    log_step "部署文件..."
    
    # 上传编译好的二进制文件
    ssh ${DEPLOY_USER}@${DEPLOY_HOST} "
        cd '${DEPLOY_DIR}' && 
        cp build/trader bin/ 2>/dev/null || true
    " || error_exit "部署文件失败"
    
    log_success "文件部署完成"
}

verify_deployment() {
    log_step "验证部署..."
    
    ssh ${DEPLOY_USER}@${DEPLOY_HOST} "
        cd '${DEPLOY_DIR}' && 
        if [ -f build/trader ]; then
            chmod +x build/trader
            echo 'Binary exists and is executable'
        else
            echo 'Binary not found'
            exit 1
        fi
    " || error_exit "部署验证失败"
    
    log_success "部署验证通过"
}

# ============================================================================
# 主流程
# ============================================================================

show_banner() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  Market Maker 一键部署脚本"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  目标主机: ${DEPLOY_HOST}"
    echo "  部署用户: ${DEPLOY_USER}"
    echo "  部署目录: ${DEPLOY_DIR}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}

main() {
    show_banner
    
    # 确认部署
    if ! confirm "确认要部署到 ${DEPLOY_USER}@${DEPLOY_HOST}:${DEPLOY_DIR} 吗？"; then
        log_warn "部署已取消"
        exit 0
    fi
    echo ""
    
    # 执行部署步骤
    check_environment
    clone_repository
    build_binary
    setup_directories
    backup_current_version
    deploy_files
    upload_config
    create_systemd_service
    verify_deployment
    
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  部署完成！"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "后续操作命令："
    echo "  启动服务: ./start.sh"
    echo "  停止服务: ./stop.sh"
    echo "  紧急停车: ./emergency_stop.sh"
    echo "  健康检查: ./health_check.sh"
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}

# 执行主流程
main "$@"