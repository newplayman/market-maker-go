# 监控指标文档

本文档描述了market-maker-go系统中可用的Prometheus监控指标。

## 指标列表

### 市场相关指标

| 指标名称 | 类型 | 描述 |
|---------|------|------|
| mm_vpin_current | Gauge | 当前VPIN值 |
| mm_volatility_regime | Gauge | 市场状态 (0=Calm, 1=TrendUp, 2=TrendDown, 3=HighVol) |
| mm_inventory_net | Gauge | 净仓位 |

### 策略相关指标

| 指标名称 | 类型 | 描述 |
|---------|------|------|
| mm_reservation_price | Gauge | 策略计算的预订价格 |
| mm_inventory_skew_bps | Gauge | 仓位偏移基点 |
| mm_adaptive_netmax | Gauge | 自适应最大净仓位 |

### 事后交易指标

| 指标名称 | 类型 | 描述 |
|---------|------|------|
| mm_adverse_selection_rate | Gauge | 逆向选择率 |

### 计数器指标

| 指标名称 | 类型 | 标签 | 描述 |
|---------|------|------|------|
| mm_quotes_generated_total | Counter | side | 策略生成的报价总数 |
| mm_orders_placed_total | Counter | side | 策略下单总数 |
| mm_fills_total | Counter | side | 策略成交总数 |

## Grafana仪表板建议

### 面板1: 市场状态监控
- `mm_vpin_current` - 实时VPIN值
- `mm_volatility_regime` - 市场状态变化
- `mm_inventory_net` - 当前净仓位

### 面板2: 策略行为监控
- `mm_reservation_price` - 预订价格变化
- `mm_inventory_skew_bps` - 仓位偏移基点
- `mm_adaptive_netmax` - 自适应仓位限制

### 面板3: 交易性能监控
- `mm_adverse_selection_rate` - 逆向选择率
- `mm_quotes_generated_total` - 生成报价数量（按买卖方向）
- `mm_orders_placed_total` - 下单数量（按买卖方向）
- `mm_fills_total` - 成交数量（按买卖方向）

## 使用方法

默认情况下，指标服务器在端口8080上运行，可以通过`/metrics`路径访问。

启动runner时可以通过`-metricsAddr`参数指定不同的地址和端口：

```bash
go run cmd/runner/main.go -config configs/config.yaml -metricsAddr :8080
```

然后可以通过以下URL访问指标：
```
http://localhost:8080/metrics
```

或者在Grafana中配置Prometheus数据源指向该地址。

## 部署Prometheus和Grafana

项目提供了完整的Docker Compose部署方案，可以一键启动Prometheus和Grafana监控系统。

### 部署步骤

1. 进入监控目录：
   ```bash
   cd /root/market-maker-go/monitoring
   ```

2. 启动监控系统：
   ```bash
   ./start.sh
   ```

3. 访问监控界面：
   - Prometheus: http://localhost:9090
   - Grafana: http://localhost:3001

### Grafana登录信息

- 用户名: admin
- 密码: admin

首次登录后建议修改默认密码。

### 停止监控系统

```bash
cd /root/market-maker-go/monitoring
./stop.sh
```

### 验证部署

1. 启动market-maker-go并启用指标：
   ```bash
   go run cmd/runner/main.go -config configs/config.yaml -metricsAddr :8080 -dryRun
   ```

2. 在浏览器中访问 http://localhost:8080/metrics 确认指标正常输出

3. 在Grafana中查看Market Maker Dashboard仪表板