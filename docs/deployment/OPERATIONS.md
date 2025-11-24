# 运维手册

## 1. 日常运维

### 服务管理
```bash
# 启动
sudo systemctl start market-maker

# 停止
sudo systemctl stop market-maker

# 重启
sudo systemctl restart market-maker

# 查看状态
sudo systemctl status market-maker
```

### 日志查看
```bash
# 实时日志
journalctl -u market-maker -f

# 错误日志
journalctl -u market-maker -p err

# 时间范围
journalctl -u market-maker --since "1 hour ago"
```

### 健康检查
```bash
# 执行健康检查
/opt/market-maker/bin/health_check.sh

# 查看指标
curl http://localhost:9090/metrics
```

## 2. 监控告警

### Grafana 使用
访问: http://your-server:3000
默认用户名: admin
默认密码: admin

### 关键指标监控
- 实时 PnL
- 订单成功率
- 系统延迟
- 风控状态

## 3. 备份恢复

### 备份
```bash
./scripts/backup.sh
```

### 恢复
```bash
./scripts/rollback.sh
```

## 4. 升级流程

### 灰度升级
1. 备份当前版本
2. 部署新版本到测试环境
3. 验证功能
4. 小流量灰度
5. 全量发布

### 回滚
```bash
./scripts/rollback.sh
```

## 5. 应急处理

### 紧急停止
```bash
./scripts/emergency_stop.sh
```

### 取消所有订单
```bash
SYMBOL=ETHUSDC ./scripts/emergency_stop.sh
```
