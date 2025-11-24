# 部署指南

## 1. 环境准备

### 1.1 系统要求
- 操作系统: Ubuntu 20.04+ / CentOS 8+
- CPU: 4核+
- 内存: 8GB+
- 存储: 50GB+ SSD
- 网络: 低延迟接入（< 1ms到交易所）

### 1.2 依赖安装
```bash
# Go 1.21+
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Prometheus
sudo apt-get install prometheus

# Grafana
sudo apt-get install grafana
```

### 1.3 用户和权限
```bash
# 创建运行用户
sudo useradd -r -s /bin/false trader

# 创建目录
sudo mkdir -p /opt/market-maker/{bin,configs,logs,data,backups}
sudo chown -R trader:trader /opt/market-maker
```

## 2. 部署步骤

### 2.1 使用自动部署脚本
```bash
# 设置环境变量
export DEPLOY_USER=trader
export DEPLOY_HOST=your-vps-ip
export DEPLOY_DIR=/opt/market-maker

# 执行部署
./scripts/deploy.sh
```

### 2.2 手动部署
```bash
# 编译
GOOS=linux GOARCH=amd64 go build -o build/trader ./cmd/runner/main.go

# 上传
scp build/trader trader@your-vps:/opt/market-maker/bin/
scp configs/config.yaml trader@your-vps:/opt/market-maker/configs/

# 配置 systemd
sudo cp docs/systemd-runner.service /etc/systemd/system/market-maker.service
sudo systemctl daemon-reload
sudo systemctl enable market-maker
sudo systemctl start market-maker
```

## 3. 验证部署
```bash
# 检查服务状态
sudo systemctl status market-maker

# 查看日志
sudo journalctl -u market-maker -f

# 健康检查
/opt/market-maker/bin/health_check.sh
```

## 4. 常用命令
```bash
# 启动
./scripts/start.sh

# 停止
./scripts/stop.sh

# 重启
./scripts/restart.sh

# 备份
./scripts/backup.sh

# 回滚
./scripts/rollback.sh
```
