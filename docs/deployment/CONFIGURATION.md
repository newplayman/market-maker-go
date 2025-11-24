# 配置说明

## 配置文件结构
```yaml
# configs/config.yaml

exchange:
  name: "binance"
  api_key: "${BINANCE_API_KEY}"
  api_secret: "${BINANCE_API_SECRET}"
  testnet: false

strategy:
  name: "basic_mm"
  symbol: "ETHUSDC"
  base_spread: 0.001      # 0.1%
  base_size: 0.01
  max_inventory: 0.05
  skew_factor: 0.3
  tick_interval: 5s

risk:
  daily_loss_limit: 100.0
  max_drawdown_limit: 0.03
  max_position: 0.1
  circuit_breaker:
    threshold: 5
    timeout: 5m

monitoring:
  prometheus_port: 9090
  metrics_interval: 5s

logging:
  level: "info"
  format: "json"
  outputs: ["stdout", "file"]
  file_path: "/var/log/market-maker/app.log"
```

## 环境变量
```bash
export BINANCE_API_KEY=your_api_key
export BINANCE_API_SECRET=your_api_secret
```

## 参数说明

### 策略参数
- `base_spread`: 基础价差（0.1% = 10 bps）
- `base_size`: 基础订单大小
- `max_inventory`: 最大持仓限制
- `skew_factor`: 倾斜因子（0-1）

### 风控参数
- `daily_loss_limit`: 日亏损限制（USDC）
- `max_drawdown_limit`: 最大回撤限制（百分比）
- `circuit_breaker.threshold`: 熔断触发次数
- `circuit_breaker.timeout`: 熔断时长
