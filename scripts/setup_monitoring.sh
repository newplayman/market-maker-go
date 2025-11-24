#!/usr/bin/env bash
#
# Grafana & Prometheus 自动配置脚本
#

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

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

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Grafana & Prometheus 自动配置"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# ============================================================================
# 1. 配置 Prometheus
# ============================================================================

log_step "步骤 1/5: 配置 Prometheus"

# 查找 Prometheus 配置文件
PROM_CONFIG=""
for path in /etc/prometheus/prometheus.yml /etc/prometheus.yml; do
    if [ -f "$path" ]; then
        PROM_CONFIG="$path"
        break
    fi
done

if [ -z "$PROM_CONFIG" ]; then
    error_exit "未找到 Prometheus 配置文件"
fi

log_success "找到配置文件: $PROM_CONFIG"

# 备份配置
sudo cp "$PROM_CONFIG" "${PROM_CONFIG}.backup.$(date +%Y%m%d_%H%M%S)"
log_success "已备份原配置"

# 检查是否已配置 market-maker
if grep -q "job_name.*market-maker" "$PROM_CONFIG"; then
    log_warn "Prometheus 已配置 market-maker，跳过"
else
    # 添加配置
    sudo tee -a "$PROM_CONFIG" > /dev/null << 'EOF'

  # Market Maker 监控
  - job_name: 'market-maker'
    static_configs:
      - targets: ['localhost:9100']
        labels:
          service: 'market-maker'
          env: 'production'
    scrape_interval: 5s
    scrape_timeout: 4s
EOF
    log_success "已添加 market-maker 配置"
    
    # 重启 Prometheus
    log_step "重启 Prometheus..."
    sudo systemctl restart prometheus
    sleep 2
    
    if systemctl is-active --quiet prometheus; then
        log_success "Prometheus 重启成功"
    else
        error_exit "Prometheus 重启失败"
    fi
fi

# ============================================================================
# 2. 配置 Grafana 数据源
# ============================================================================

log_step "步骤 2/5: 配置 Grafana 数据源"

# 等待 Grafana 启动
sleep 2

# 使用 Grafana API 添加数据源
GRAFANA_URL="http://localhost:3000"
GRAFANA_USER="admin"
GRAFANA_PASS="admin"

# 检查数据源是否存在
DS_EXISTS=$(curl -s -u "$GRAFANA_USER:$GRAFANA_PASS" \
    "$GRAFANA_URL/api/datasources/name/Prometheus" \
    -H "Content-Type: application/json" 2>/dev/null | grep -c "Prometheus" || true)

if [ "$DS_EXISTS" -gt 0 ]; then
    log_warn "Prometheus 数据源已存在，跳过"
else
    # 创建数据源
    curl -s -X POST -u "$GRAFANA_USER:$GRAFANA_PASS" \
        "$GRAFANA_URL/api/datasources" \
        -H "Content-Type: application/json" \
        -d '{
            "name": "Prometheus",
            "type": "prometheus",
            "url": "http://localhost:9090",
            "access": "proxy",
            "isDefault": true,
            "jsonData": {
                "timeInterval": "5s"
            }
        }' > /dev/null
    
    log_success "已创建 Prometheus 数据源"
fi

# ============================================================================
# 3. 导入 Dashboard
# ============================================================================

log_step "步骤 3/5: 导入 Grafana Dashboard"

DASHBOARD_FILE="$ROOT/deployments/grafana/dashboards/trading_overview.json"

if [ ! -f "$DASHBOARD_FILE" ]; then
    error_exit "Dashboard 文件不存在: $DASHBOARD_FILE"
fi

# 准备 Dashboard JSON（添加必要的包装）
DASHBOARD_JSON=$(cat "$DASHBOARD_FILE")
IMPORT_JSON=$(cat << EOF
{
  "dashboard": $DASHBOARD_JSON,
  "overwrite": true,
  "inputs": [{
    "name": "DS_PROMETHEUS",
    "type": "datasource",
    "pluginId": "prometheus",
    "value": "Prometheus"
  }]
}
EOF
)

# 导入 Dashboard
IMPORT_RESULT=$(curl -s -X POST -u "$GRAFANA_USER:$GRAFANA_PASS" \
    "$GRAFANA_URL/api/dashboards/db" \
    -H "Content-Type: application/json" \
    -d "$IMPORT_JSON")

if echo "$IMPORT_RESULT" | grep -q "success"; then
    DASHBOARD_UID=$(echo "$IMPORT_RESULT" | grep -o '"uid":"[^"]*"' | cut -d'"' -f4)
    log_success "Dashboard 导入成功！"
    echo "    访问: $GRAFANA_URL/d/$DASHBOARD_UID"
else
    log_warn "Dashboard 导入可能失败，请手动导入"
fi

# ============================================================================
# 4. 验证配置
# ============================================================================

log_step "步骤 4/5: 验证配置"

# 检查 Prometheus
if curl -s http://localhost:9090/-/ready | grep -q "Prometheus"; then
    log_success "Prometheus 运行正常"
else
    log_error "Prometheus 可能未正常运行"
fi

# 检查 Prometheus 能否抓取 market-maker 指标
TARGETS=$(curl -s http://localhost:9090/api/v1/targets | grep -c "market-maker" || true)
if [ "$TARGETS" -gt 0 ]; then
    log_success "Prometheus 已配置 market-maker 目标"
else
    log_warn "Prometheus 尚未发现 market-maker 目标（正常，需启动程序后才会出现）"
fi

# 检查 Grafana
if curl -s "$GRAFANA_URL/api/health" | grep -q "ok"; then
    log_success "Grafana 运行正常"
else
    log_error "Grafana 可能未正常运行"
fi

# ============================================================================
# 5. 输出访问信息
# ============================================================================

log_step "步骤 5/5: 配置完成"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  配置完成！"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Prometheus:"
echo "  访问地址: http://localhost:9090"
echo "  配置文件: $PROM_CONFIG"
echo "  备份文件: ${PROM_CONFIG}.backup.*"
echo ""
echo "Grafana:"
echo "  访问地址: http://localhost:3000"
echo "  默认用户: admin"
echo "  默认密码: admin"
echo ""
echo "下一步："
echo "  1. 启动 market-maker 程序"
echo "  2. 访问 Grafana 查看监控"
echo "  3. 如果看不到数据，等待几分钟让指标收集"
