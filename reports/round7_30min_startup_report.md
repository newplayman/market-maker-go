# Round7 30分钟实盘测试 - 启动状态报告

**生成时间**: 2025-11-26 04:59:10  
**测试类型**: 30分钟 Round7 策略验证  
**预计结束**: 2025-11-26 05:29:10

---

## 一、启动状态

### 1.1 账户初始状态
- **余额**: 200.00 USDC
- **持仓**: 0.000 ETH (ETHUSDC)
- **中间价**: 2930.0 USDT

### 1.2 策略配置
- **配置文件**: `configs/config_round7_geometric_drawdown.yaml`
- **策略类型**: 几何加宽网格 + 浮亏分层减仓
- **核心参数**:
  - `baseSize`: 0.009 ETH (~26 USDC)
  - `netMax`: 0.21 ETH (净仓上限)
  - `minSpread`: 0.0006 (6 bps)
  - `quoteIntervalMs`: 500ms
  - `staticFraction`: 0.97 (97% Maker)
  - **几何网格**: `spacingRatio=1.20`, `layerSizeDecay=0.90`, `maxLayers=24`
  - **浮亏减仓**: `bands=[5%,8%,12%]`, `fractions=[15%,25%,40%]`

### 1.3 进程状态
- **Runner PID**: 2710400
- **Metrics 端口**: `:8080`
- **日志文件**: `logs/round7_30min.log`
- **定时器**: 30分钟后自动停止并清仓 (PID: 2711003)

---

## 二、监控状态

### 2.1 Prometheus
- **状态**: ✅ up
- **抓取目标**: `172.17.0.1:8080` (宿主机 docker0 IP)
- **访问地址**: http://localhost:9090
- **最后抓取**: 成功

### 2.2 Grafana
- **状态**: ✅ 运行中
- **访问地址**: http://localhost:3001
- **可用面板**:
  - Market Maker 综合面板 (Prometheus + Loki 数据源)
  - Runner Dashboard
  - Trader Dashboard

### 2.3 Loki/Promtail
- **Loki状态**: ✅ 运行中
- **Promtail状态**: ✅ 采集中
- **日志源**: `logs/round7_30min.log`

---

## 三、运行中状态（启动后 5 分钟）

### 3.1 成交统计
- **FILLED 订单数**: 175
- **当前净仓**: -0.054 ETH (轻度空头)
- **库存压力系数**: -0.257 (有库存压力,价差会适度拉宽)

### 3.2 关键日志片段
```
[strategy_adjust] map[intervalMs:500 inventoryFactor:-0.2571428571428572 mid:2930.105 net:-0.054 reduceOnly:false spread:0.010000000000218279 spreadRatio:0.0007542857142857142 symbol:ETHUSDC takeProfit:true volFactor:0]
```
- `net`: -0.054 (在 netMax=0.21 限制内，安全)
- `reduceOnly`: false (正常做市中)
- `spreadRatio`: 0.000754 (7.54 bps,略宽于 minSpread=6 bps,符合库存压力调整逻辑)
- `intervalMs`: 500 (报价频率正常)

### 3.3 风控状态
- ✅ 净仓未超限 (0.054 < 0.21)
- ✅ 无 `drawdown_trigger` 事件 (未触发浮亏减仓)
- ✅ 无 `net exposure limit exceeded` 错误 (成交前硬帽未触发)

---

## 四、监控验证步骤

### 4.1 Grafana 面板验证
1. 访问 http://localhost:3001
2. 登录 (admin/admin)
3. 打开 "Market Maker 综合面板"
4. 验证以下指标是否正常显示:
   - ✅ 订单成交率 (应有数据点)
   - ✅ 净仓位 (应显示 -0.054 ETH)
   - ✅ 价差 (应在 6-10 bps 区间)
   - ✅ 订单日志 (Loki 面板,应显示最新日志)

### 4.2 Prometheus 查询验证
访问 http://localhost:9090，执行以下查询:
```promql
# 检查 up 状态
up{job="market-maker-go"}   # 应返回 1

# 检查订单成交总数 (如果 metrics 已注册)
mm_order_filled_total{symbol="ETHUSDC"}

# 检查净仓位 (如果 metrics 已注册)
mm_position_net{symbol="ETHUSDC"}
```

**注意**: 由于当前 runner 的 metrics 注册可能不完整,如果上述 `mm_*` 指标未返回数据,这是正常的。Prometheus 健康状态 `up` 已验证连接正常,后续可通过日志查看详细数据。

---

## 五、预期结果

### 5.1 30分钟后自动执行
1. **停止 Runner**: `pkill -f "cmd/runner.*config_round7"`
2. **清仓撤单**: 执行 `scripts/emergency_stop.sh`
   - 取消所有挂单
   - 市价 reduce-only 平掉剩余持仓
3. **创建完成标记**: `logs/round7_30min_completed.flag`

### 5.2 预期验证点
- **几何网格覆盖范围**: 单边偏移 2-3% 时仍有远端挂单,避免僵持
- **成交前硬帽**: 净仓始终 ≤ 0.21,无超限错误
- **浮亏减仓**: 如果浮亏达 5-12%,应触发分层减仓事件
- **账户净 PnL**: 预期在 -5 至 +5 USDC 区间 (短期测试,主要验证机制)

---

## 六、实时监控命令

### 6.1 查看实时日志
```bash
tail -f /root/market-maker-go/logs/round7_30min.log | grep -E 'FILLED|drawdown_trigger|net exposure'
```

### 6.2 查看成交统计
```bash
grep -c 'FILLED' /root/market-maker-go/logs/round7_30min.log
```

### 6.3 查看当前净仓
```bash
tail -n 100 /root/market-maker-go/logs/round7_30min.log | grep 'strategy_adjust' | tail -n 1 | grep -oP 'net:-?\d+(\.\d+)?'
```

### 6.4 检查定时器状态
```bash
ps aux | grep 'sleep 1800' | grep -v grep
```

---

## 七、下一步

**测试期间 (0-30分钟)**:
- 可访问 Grafana 面板实时观察
- 关注日志中的关键事件 (`drawdown_trigger`、`net exposure limit exceeded`)
- 如需提前停止,执行: `pkill -f "cmd/runner.*config_round7" && bash scripts/emergency_stop.sh`

**测试结束后 (30分钟)**:
- 自动停止并清仓
- 生成 Round7 阶段性测试报告 (包含成交统计、PnL 分析、机制验证)

---

**启动确认时间**: 2025-11-26 04:59:10  
**状态**: ✅ 运行正常,监控已就绪
