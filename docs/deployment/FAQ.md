# 常见问题解答 (FAQ)

## Q1: Testnet 是否指 dry-run？

**不完全是**。两者有区别：

### Testnet（测试网）
- 使用 Binance 的测试网络 (testnet.binance.vision)
- **真实的API调用**，但使用模拟资金
- 完整的市场环境和订单撮合
- 可以验证完整的交易流程
- 需要在 Binance Testnet 申请测试账户

配置示例：
```yaml
exchange:
  name: "binance"
  api_key: "${BINANCE_TESTNET_API_KEY}"
  api_secret: "${BINANCE_TESTNET_API_SECRET}"
  testnet: true  # 启用测试网
```

### Dry-run（模拟模式）
- **不发送真实API请求**
- 完全本地模拟
- 用于开发调试
- 不需要网络连接

### 推荐使用顺序
1. **Dry-run** - 本地开发测试
2. **Testnet** - 接近真实环境测试（推荐 2天）
3. **小资金实盘** - 真实交易验证

## Q2: 小资金可以用 100 USDC 吗？

**可以！** 100 USDC 完全可行，而且更安全。

### 推荐方案（100 USDC 起步）

```yaml
# 阶段2配置（降低到 100 USDC）
环境: Binance Mainnet
资金: 100 USDC
交易对: ETHUSDC

strategy:
  base_size: 0.001        # 更小的订单（约3 USDC）
  max_inventory: 0.01     # 最大持仓 0.01 ETH（约30 USDC）

risk:
  daily_loss_limit: 5.0   # 日亏损限制 5 USDC（5%）
  max_drawdown_limit: 0.03 # 3% 回撤
  max_position: 0.01      # 最大持仓 0.01 ETH
```

### 加仓计划（从 100 USDC 开始）

```
Day 1-3:   100 USDC   (初始测试)
Day 4-5:   200 USDC   (翻倍)
Day 6-7:   500 USDC   (观察表现)
Day 8-10:  1000 USDC  (稳定后)
Day 11+:   根据收益决定
```

### 100 USDC 的优势
- ✅ 风险更小，更适合学习
- ✅ 心理压力小
- ✅ 足够验证策略有效性
- ✅ 低成本试错

## Q3: Grafana 监控面板在哪里？

Dashboard JSON 文件位于：
```
deployments/grafana/dashboards/trading_overview.json
```

### 导入方法

**最简单的方法**：
```bash
# 1. 复制文件路径
/root/market-maker-go/deployments/grafana/dashboards/trading_overview.json

# 2. 在 Grafana Web UI 中：
#    - 访问 http://your-server:3000
#    - 点击 "+" → "Import"
#    - 点击 "Upload JSON file"
#    - 选择上面的文件
#    - 点击 "Import"
```

详细步骤见：`docs/deployment/GRAFANA_SETUP.md`

## Q4: 如何获取可导入的 Grafana JSON？

直接查看或复制文件：

```bash
# 查看文件内容
cat deployments/grafana/dashboards/trading_overview.json

# 复制到剪贴板（如果支持）
cat deployments/grafana/dashboards/trading_overview.json | clip  # Windows
cat deployments/grafana/dashboards/trading_overview.json | pbcopy  # macOS
cat deployments/grafana/dashboards/trading_overview.json | xclip  # Linux
```

或直接用文本编辑器打开：
```bash
# 使用 nano
nano deployments/grafana/dashboards/trading_overview.json

# 使用 vim
vim deployments/grafana/dashboards/trading_overview.json

# 使用 VS Code
code deployments/grafana/dashboards/trading_overview.json
```

## Q5: 首次部署推荐配置？

### 最小风险配置（100 USDC）

```yaml
# configs/config.yaml

exchange:
  name: "binance"
  testnet: false  # 先在 testnet 测试，测试通过后改为 false

strategy:
  symbol: "ETHUSDC"
  base_spread: 0.002      # 0.2% (20 bps) - 保守
  base_size: 0.001        # 约 3 USDC/单
  max_inventory: 0.01     # 最大持仓 30 USDC
  tick_interval: 10s      # 10秒刷新（降低频率）

risk:
  daily_loss_limit: 5.0   # 5 USDC
  max_drawdown_limit: 0.05 # 5%
  max_position: 0.01
  circuit_breaker:
    threshold: 3
    timeout: 10m

monitoring:
  prometheus_port: 9090
  metrics_interval: 5s

logging:
  level: "info"
```

### 启动步骤

```bash
# 1. Testnet 验证（2天）
# 设置 testnet: true，运行48小时

# 2. 小资金实盘（3-5天）
# 设置 testnet: false，资金 100 USDC

# 3. 每日检查
./scripts/health_check.sh

# 4. 查看实时日志
journalctl -u market-maker -f
```

## Q6: 如何紧急停止？

```bash
# 方法1: 使用紧急停止脚本
./scripts/emergency_stop.sh

# 方法2: 停止服务
sudo systemctl stop market-maker

# 方法3: 手动取消订单
go run ./cmd/binance_panic -symbol ETHUSDC -cancel
```
