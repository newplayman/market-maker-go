# Grafana 监控面板设置指南

## Dashboard 文件位置

Grafana Dashboard JSON 文件位于：
```
deployments/grafana/dashboards/trading_overview.json
```

## 导入 Dashboard 到 Grafana

### 方法1: 通过 Web UI 导入

1. 访问 Grafana: `http://your-server:3000`
2. 默认登录: 
   - 用户名: `admin`
   - 密码: `admin`
3. 点击左侧菜单 "+" → "Import"
4. 点击 "Upload JSON file"
5. 选择文件: `deployments/grafana/dashboards/trading_overview.json`
6. 选择数据源: Prometheus
7. 点击 "Import"

### 方法2: 直接复制 JSON

1. 打开文件: `cat deployments/grafana/dashboards/trading_overview.json`
2. 复制全部内容
3. 在 Grafana 中点击 "+" → "Import"
4. 粘贴 JSON 到文本框
5. 点击 "Load"
6. 选择 Prometheus 数据源
7. 点击 "Import"

### 方法3: 自动配置（推荐）

使用 provisioning 自动加载：

```bash
# 1. 复制配置文件到 Grafana
sudo cp deployments/grafana/provisioning/datasources.yml /etc/grafana/provisioning/datasources/
sudo cp deployments/grafana/provisioning/dashboards.yml /etc/grafana/provisioning/dashboards/

# 2. 复制 Dashboard
sudo mkdir -p /etc/grafana/provisioning/dashboards
sudo cp deployments/grafana/dashboards/trading_overview.json /etc/grafana/provisioning/dashboards/

# 3. 重启 Grafana
sudo systemctl restart grafana-server
```

## Dashboard 包含的面板

1. **实时PnL** - 总盈亏显示
2. **持仓情况** - 当前持仓仪表盘
3. **订单统计** - 今日订单总数、成交数、撤单数、成交率
4. **PnL趋势图** - 累计PnL、实现PnL、未实现PnL
5. **订单成交分布** - 买单/卖单成交量
6. **当前报价** - 实时买卖价格和数量
7. **Spread分布** - 策略Spread vs 市场Spread
8. **成交量统计** - 每分钟成交量

## 配置 Prometheus 数据源

如果手动配置 Prometheus：

1. 点击 "Configuration" → "Data Sources"
2. 点击 "Add data source"
3. 选择 "Prometheus"
4. 配置：
   - Name: `Prometheus`
   - URL: `http://localhost:9090`
   - Access: `Server (default)`
5. 点击 "Save & Test"

## 告警配置

告警规则文件位于：
```
deployments/grafana/alerting/rules.yml
```

包含4个告警：
- 高亏损告警 (PnL < -100 USDC)
- 高延迟告警 (P95 > 100ms)
- 系统离线告警
- 风险等级告警 (risk_level >= 3)
