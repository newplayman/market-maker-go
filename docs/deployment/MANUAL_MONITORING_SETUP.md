# Grafana 监控手动配置指南（5分钟）

由于Prometheus可能通过Docker安装，这里提供手动配置步骤。

## 方法1: 仅配置Grafana（推荐，最简单）

### 步骤1: 访问Grafana（1分钟）

```bash
# 打开浏览器访问
http://localhost:3000

# 默认登录
用户名: admin
密码: admin
# 首次登录会要求修改密码，可以跳过
```

### 步骤2: 添加Prometheus数据源（2分钟）

1. 点击左侧菜单的 **齿轮图标** (Configuration)
2. 选择 **Data Sources**
3. 点击右上角蓝色按钮 **Add data source**
4. 选择 **Prometheus**
5. 填写配置：
   - Name: `Prometheus`
   - URL: `http://localhost:9090`
   - 其他保持默认
6. 滚动到底部，点击 **Save & Test**
7. 看到绿色的 ✓ "Data source is working" 表示成功

### 步骤3: 导入Dashboard（2分钟）

1. 点击左侧菜单的 **+** 号
2. 选择 **Import**
3. 点击 **Upload JSON file**
4. 选择文件：`/root/market-maker-go/deployments/grafana/dashboards/trading_overview.json`
5. 在下方 **Prometheus** 下拉菜单选择刚才创建的 `Prometheus` 数据源
6. 点击 **Import**

**完成！** 现在应该能看到交易监控Dashboard了。

---

## 方法2: 如果需要配置Prometheus

### 检查Prometheus运行方式

```bash
# 检查是否Docker运行
docker ps | grep prometheus

# 检查systemd服务
systemctl status prometheus 2>/dev/null

# 检查进程
ps aux | grep prometheus
```

### 如果是Docker运行

```bash
# 找到Prometheus容器
docker ps | grep prometheus

# 假设容器名为 prometheus
docker exec -it prometheus cat /etc/prometheus/prometheus.yml

# 需要添加market-maker job，创建新配置
cat > /tmp/prometheus.yml << 'EOF'
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']

  - job_name: 'market-maker'
    static_configs:
      - targets: ['host.docker.internal:9100']  # Docker访问主机
    scrape_interval: 5s
EOF

# 复制到容器（替换容器名）
docker cp /tmp/prometheus.yml prometheus:/etc/prometheus/prometheus.yml

# 重启容器
docker restart prometheus
```

### 如果是APT安装

```bash
# 查找配置文件
sudo find / -name "prometheus.yml" 2>/dev/null

# 编辑配置（假设找到了）
sudo vim /path/to/prometheus.yml

# 在 scrape_configs 部分添加：
  - job_name: 'market-maker'
    static_configs:
      - targets: ['localhost:9100']
    scrape_interval: 5s

# 重启
sudo systemctl restart prometheus
```

---

## 验证配置

### 1. 检查Prometheus能否访问

```bash
curl http://localhost:9090/-/ready
# 应该返回: Prometheus is Ready.
```

### 2. 启动market-maker后检查指标

```bash
# 启动程序
cd /root/market-maker-go
./build/trader -config configs/config.yaml

# 另一个终端检查指标端点
curl http://localhost:9100/metrics

# 应该看到类似输出：
# trading_pnl_total 0
# order_placed_total 0
# ...
```

### 3. 在Grafana中查看数据

访问 http://localhost:3000
- 打开刚导入的Dashboard
- 如果看到 "No data"，等待30秒让Prometheus抓取数据
- 刷新页面

---

## 如果遇到问题

### Q1: Grafana无法连接

```bash
# 检查Grafana状态
systemctl status grafana-server
# 或
docker ps | grep grafana

# 检查端口
curl http://localhost:3000
```

### Q2: Prometheus抓取不到market-maker指标

```bash
# 1. 确认market-maker在运行
ps aux | grep trader

# 2. 确认端口9100开放
netstat -tuln | grep 9100
# 或
curl http://localhost:9100/metrics

# 3. 在Prometheus中检查targets
# 浏览器访问: http://localhost:9090/targets
# 应该看到 market-maker job，状态为 UP
```

### Q3: Dashboard显示 "No data"

可能原因：
1. market-maker程序未启动 → 启动程序
2. Prometheus未配置job → 配置Prometheus
3. 数据还在收集中 → 等待30秒刷新

---

## 快速启动命令

```bash
# 1. 设置环境变量
export BINANCE_API_KEY="your_key"
export BINANCE_API_SECRET="your_secret"

# 2. 启动程序
cd /root/market-maker-go
./build/trader -config configs/config.yaml

# 3. 访问Grafana
# 浏览器: http://localhost:3000

# 4. 打开Dashboard查看实时监控
```

---

## 不配置Prometheus也可以运行

**重要**: 即使不配置Prometheus，market-maker程序也能正常运行交易！

Prometheus只是用于监控和可视化，不影响交易功能。

可以通过以下方式监控：
1. 查看终端日志输出
2. 查看日志文件: `/var/log/market-maker/app.log`
3. 使用健康检查脚本: `./scripts/health_check.sh`

---

**总结**: 最简单的方法就是只配置Grafana，3步骤5分钟搞定！
