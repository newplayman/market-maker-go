# 故障排查指南

## 常见问题

### 问题1: 服务无法启动
**症状**: systemctl 启动失败

**诊断**:
```bash
journalctl -xe -u market-maker
```

**解决方案**:
1. 检查配置文件格式
2. 检查文件权限
3. 检查端口占用

### 问题2: 订单延迟过高
**症状**: Dashboard 显示延迟超过100ms

**诊断**:
```bash
ping api.binance.com
```

**解决方案**:
1. 检查网络连接
2. 分析CPU使用
3. 检查代码性能

### 问题3: 风控频繁触发
**症状**: 频繁熔断

**诊断**:
```bash
grep "RISK" /var/log/market-maker/app.log | tail -100
```

**解决方案**:
1. 检查市场波动
2. 调整风控参数
3. 分析策略表现

## 日志分析
```bash
# 查看错误日志
grep ERROR /var/log/market-maker/app.log

# 查看风控日志
grep RISK /var/log/market-maker/app.log

# 查看订单日志
grep order /var/log/market-maker/app.log
```

## 性能调优
1. 减少日志级别（生产环境用 INFO）
2. 优化数据库查询
3. 调整 goroutine 数量
4. 使用连接池
