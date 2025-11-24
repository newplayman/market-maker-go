# 100 USDC 实盘快速启动指南

## ✅ 准备工作检查清单

- [ ] 已安装 Go 1.21+
- [ ] 已安装并运行 Prometheus（端口9090）
- [ ] 已安装并运行 Grafana（端口3000）
- [ ] 币安账户至少有 100 USDC
- [ ] 已获取币安API密钥和密钥对
- [ ] VPS IP已添加到币安API白名单

## 🚀 5分钟快速启动

### 步骤1: 设置环境变量（1分钟）

```bash
# 编辑 ~/.bashrc
vim ~/.bashrc

# 在文件末尾添加（替换为你的真实密钥）
export BINANCE_API_KEY="your_actual_api_key_here"
export BINANCE_API_SECRET="your_actual_secret_here"

# 保存后使生效
source ~/.bashrc

# 验证
echo $BINANCE_API_KEY  # 应该显示你的密钥
```

### 步骤2: 验证配置（1分钟）

```bash
cd /root/market-maker-go

# 查看配置文件
cat configs/config.yaml

# 确认配置正确：
# - testnet: false  ✅
# - base_size: 0.001  ✅
# - daily_loss_limit: 5.0  ✅
```

### 步骤3: 检查账户余额（1分钟）

```bash
# 查看账户余额
go run ./cmd/binance_balance -config configs/config.yaml

# 应该看到类似输出：
# USDC: 100.00
# ETH: 0.xxx

# 确认 USDC >= 100
```

### 步骤4: 编译程序（1分钟）

```bash
# 编译
go build -o build/trader ./cmd/runner/main.go

# 验证
ls -lh build/trader
```

### 步骤5: 启动交易系统（1分钟）

```bash
# 创建日志目录
sudo mkdir -p /var/log/market-maker
sudo chown $USER:$USER /var/log/market-maker

# 启动（前台运行，方便观察）
./build/trader -config configs/config.yaml
```

**看到以下输出表示启动成功：**
```
INFO  Starting market maker...
INFO  Exchange: binance
INFO  Symbol: ETHUSDC
INFO  Strategy: basic_mm
INFO  WebSocket connected
INFO  Order book initialized
INFO  Trading started
```

---

## 📊 配置Grafana监控（可选，5分钟）

### 快速配置

```bash
# 1. 访问 Grafana
浏览器打开: http://localhost:3000
用户名: admin
密码: admin（首次登录会要求修改）

# 2. 添加数据源
点击左侧齿轮图标 → Data Sources → Add data source
选择 Prometheus
URL: http://localhost:9090
点击 "Save & Test"（应该显示绿色✓）

# 3. 导入Dashboard
点击左侧 "+" → Import
点击 "Upload JSON file"
选择文件: /root/market-maker-go/deployments/grafana/dashboards/trading_overview.json
选择数据源: Prometheus
点击 "Import"

# 完成！现在可以看到实时监控了
```

---

## 🔍 实时监控

### 方法1: 查看终端日志
如果是前台运行，直接看终端输出

### 方法2: 使用journalctl（如果用systemd）
```bash
journalctl -u market-maker -f
```

### 方法3: Grafana Dashboard
访问: http://localhost:3000

### 方法4: 健康检查脚本
```bash
./scripts/health_check.sh
```

---

## ⚠️ 重要监控指标

### 必须关注的指标：

1. **PnL（盈亏）**
   - 目标：> 0
   - 警告：< -$2
   - 危险：< -$5

2. **订单成功率**
   - 目标：> 99%
   - 正常：> 95%
   - 异常：< 90%

3. **持仓**
   - 限制：< 0.01 ETH (约30 USDC)
   - 正常：接近0
   - 异常：超过限制

4. **系统延迟**
   - 目标：< 50ms
   - 正常：< 100ms
   - 异常：> 200ms

---

## 🛑 紧急停止

### 如果需要立即停止：

```bash
# 方法1: Ctrl+C（如果前台运行）
按 Ctrl+C

# 方法2: 紧急停止脚本
./scripts/emergency_stop.sh

# 方法3: 手动停止
pkill -f "cmd/runner"

# 方法4: 停止并取消所有订单
go run ./cmd/binance_panic -symbol ETHUSDC -cancel
```

---

## 📋 每小时检查清单

前24小时，每小时执行：

```bash
# 1. 健康检查
./scripts/health_check.sh

# 2. 查看PnL
# 在Grafana中查看，或查看日志中的PnL输出

# 3. 检查错误日志
journalctl -u market-maker -p err -n 20

# 4. 记录关键数据
# - 当前PnL
# - 订单数
# - 成交率
# - 是否有告警
```

---

## 🎯 成功标准（前72小时）

### 24小时目标
- ✅ 系统稳定运行24小时
- ✅ 没有崩溃
- ✅ PnL >= 0
- ✅ 订单成功率 > 99%

### 48小时目标  
- ✅ 继续稳定运行
- ✅ PnL > $0.2
- ✅ 无重大问题

### 72小时目标
- ✅ 累计PnL > $0.5
- ✅ 日均收益率 > 0.15%
- ✅ 准备加仓到200 USDC

---

## 🔧 常见问题快速解决

### Q1: "API key格式错误"
```bash
# 检查环境变量
echo $BINANCE_API_KEY
echo $BINANCE_API_SECRET

# 重新设置
export BINANCE_API_KEY="your_key"
export BINANCE_API_SECRET="your_secret"
```

### Q2: "余额不足"  
```bash
# 检查余额
go run ./cmd/binance_balance -config configs/config.yaml

# 确保 USDC >= 100
```

### Q3: "WebSocket连接失败"
```bash
# 检查网络
ping api.binance.com

# 检查防火墙
sudo ufw status
```

### Q4: "订单被拒绝"
可能原因：
- 数量太小（ETH最小0.001）
- 价格偏离市场价太多
- API权限不足

解决：检查config.yaml中的base_size和base_spread

---

## 📞 需要帮助？

1. 查看详细文档：
   - `docs/deployment/FAQ.md`
   - `docs/deployment/TROUBLESHOOTING.md`

2. 查看日志：
   ```bash
   tail -100 /var/log/market-maker/app.log
   journalctl -u market-maker -n 100
   ```

3. 运行诊断：
   ```bash
   ./scripts/health_check.sh
   ```

---

## ✨ 下一步

### 稳定运行72小时后：

1. **评估表现**
   - 累计PnL
   - 夏普比率
   - 最大回撤
   - 系统稳定性

2. **考虑加仓**
   ```bash
   # 如果表现良好，可以加到200 USDC
   vim configs/config.yaml
   # 调整 daily_loss_limit: 10.0
   # 调整 max_position: 0.02
   ```

3. **优化参数**
   - 根据实际表现调整spread
   - 优化订单大小
   - 调整刷新频率

---

**准备好了吗？开始你的做市之旅！** 🚀

记住：
- 💰 从小资金开始（100 USDC）
- 👀 密切监控前24小时
- 📊 使用Grafana实时监控
