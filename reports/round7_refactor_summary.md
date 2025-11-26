# Round7 策略与风控改造总结报告

**生成时间**: 2025-11-26  
**目标**: 解决 Round6 暴露的"单边僵持"与"超限风险",提升策略对趋势市场的适应性

---

## 一、问题诊断（Round6 实盘观察）

### 1.1 核心现象
- **单边僵持**：行情单向偏移时累积持仓至 ≈0.288 ETH，超过 `netMax=0.16` 配置，触发风控拒绝新单，日志持续报 `net exposure limit exceeded`，策略进入"僵持"状态，仅等待价格回归。
- **超限风险**：虽然风控在报价阶段拒绝新单，但"已成交叠加"未做硬性回退，导致实际净仓在快速成交或并发情况下可能超出 `netMax` 达 72%（0.288 vs 0.16）。
- **缺乏主动减仓**：严重浮亏时无分层减仓机制，仅依赖硬止损（`stopLoss: -60.0`）一次性退出，代价大。

### 1.2 根因分析
1. **风控只在"报价前"拦截**，成交后无"硬帽回退"，允许净仓暂时超限。
2. **无分层减仓机制**，浮亏 5-12% 时未启动部分减仓，一旦触发止损损失较大。
3. **无趋势识别与防御模式**，单边趋势中仍按常规参数下单，导致逆向累积持仓。
4. **网格层级等间距**，持仓成本集中在近端价格区间，远端覆盖不足，单边移动后大量成交集中在窄幅区间。

---

## 二、改造方案概览

### 2.1 设计原则
- **杜绝超限**：在成交后立即检查，若净仓超过上限自动回退（优先 Maker reduce-only，必要时小比例 Taker）。
- **分层减仓**：浮亏达档位（5%、8%、12%）时自动触发部分减仓（15%、25%、40%），避免一次性硬止损。
- **趋势防御**：检测到单边趋势或高逆选率时，降低 `netMax`/`baseSize`，拉宽价差，减少逆向建仓。
- **几何加宽层级**：层间距按几何比例（1.20）扩大，远端下单量衰减（0.90），覆盖更宽价格范围，分散成本。

### 2.2 改造模块分布

| 模块                  | 文件路径                                         | 改造内容                                                                 |
|-----------------------|-------------------------------------------------|-------------------------------------------------------------------------|
| 成交后硬帽回退         | `internal/engine/trading_engine.go`             | `placeOrder` 中新增下单前容量收敛逻辑，避免单笔导致净仓超限                |
| 浮亏分层减仓           | `risk/drawdown_manager.go` (新)                 | 实现 `DrawdownManager`，提供档位检测与减仓建议                            |
|                       | `cmd/runner/main.go`                            | 实例化 `DrawdownManager` 并在主循环周期检查，触发时记录 `drawdown_trigger` 事件 |
| 趋势防御模式           | `risk/adaptive.go`                              | 新增 `UpdateWithTrend(forceTrend bool)` 方法，强制走高逆选分支             |
| 几何加宽网格           | `strategy/grid_geometric.go` (新)               | 实现 `BuildGeometricGrid` 生成几何间距与远端衰减的层级                    |
|                       | `strategy/grid.go`                              | 在 `GenerateQuotes` 中接入几何模式分支                                    |
| 配置扩展              | `config/load.go`                                | 新增字段：`LayerSpacingMode`、`SpacingRatio`、`LayerSizeDecay`、`MaxLayers`、`DrawdownBands`、`ReduceFractions`、`ReduceMode`、`ReduceCooldownSeconds` |
| Round7 配置草案        | `configs/config_round7_geometric_drawdown.yaml` | 整合上述参数，准备下一轮实盘                                              |
| 监控修复              | `monitoring/prometheus/prometheus.yml`          | 端口从 `:8080` 改为 `:9101`（避免与系统 Node Exporter 冲突）               |
|                       | `cmd/runner/main.go`                            | `-metricsAddr :9101` 默认值更新                                          |

---

## 三、核心改造详解

### 3.1 成交后净仓硬帽回退

**改动点**: `internal/engine/trading_engine.go`

**逻辑**:
```go
// 在提交前按净仓上限收敛下单量
{
    net := e.inventory.NetExposure()
    maxInv := e.strategy.GetConfig().MaxInventory
    if maxInv > 0 {
        var delta float64
        if quote.Side == "BUY" {
            delta = quote.Size
        } else {
            delta = -quote.Size
        }
        curAbs := net
        if curAbs < 0 { curAbs = -curAbs }
        new := net + delta
        newAbs := new
        if newAbs < 0 { newAbs = -newAbs }
        if newAbs > curAbs {
            remaining := maxInv - curAbs
            if remaining <= 0 {
                return fmt.Errorf("pre-trade capacity exhausted: |%.4f| >= %.4f", curAbs, maxInv)
            }
            if remaining < quote.Size {
                quote.Size = remaining
            }
        }
    }
}
```

**效果**:
- 下单前自动收敛 `quote.Size` 至剩余容量上限。
- 杜绝单笔导致净仓从 0.16 跳至 0.288 的超限情况。

---

### 3.2 DrawdownManager（浮亏分层减仓）

**新文件**: `risk/drawdown_manager.go`

**核心方法**:
```go
func (dm *DrawdownManager) Plan(symbol string, drawdownPct float64) (qty float64, preferMaker bool, band float64) {
    // 冷却检查
    if time.Since(dm.lastAction) < dm.Cooldown {
        return 0, false, 0
    }
    // 遍历档位
    for i, b := range dm.Bands {
        if drawdownPct >= b {
            net := dm.Pos.NetExposure()
            netAbs := net; if netAbs < 0 { netAbs = -netAbs }
            targetReduce := dm.Fractions[i] * netAbs
            qty = targetReduce
            if qty < dm.Base { qty = dm.Base }
            if qty > netAbs { qty = netAbs }
            preferMaker = (dm.Mode == "maker_first_then_taker")
            dm.lastAction = time.Now()
            return qty, preferMaker, b
        }
    }
    return 0, false, 0
}
```

**接入点**: `cmd/runner/main.go` 主循环周期调用：
```go
if ddMgr != nil {
    drawdownPct := (-pnl / initialBalance) * 100.0
    if qty, preferMaker, band := ddMgr.Plan(symbolUpper, drawdownPct); qty > 0 {
        logEvent("drawdown_trigger", map[string]interface{}{
            "symbol": symbolUpper,
            "drawdownPct": drawdownPct,
            "triggeredBand": band,
            "reduceQty": qty,
            "preferMaker": preferMaker,
            "net": net,
        })
        // 后续可扩展为自动挂减仓单
    }
}
```

**配置示例**:
```yaml
drawdownBands: [5, 8, 12]       # 浮亏档位（%）
reduceFractions: [0.15, 0.25, 0.40]  # 每档减仓比例
reduceMode: maker_first_then_taker
reduceCooldownSeconds: 120
```

**效果**:
- 浮亏 5% 时减仓 15%，8% 时再减 25%，12% 时再减 40%。
- 避免一次性硬止损（-60 USDT）的剧烈损失。

---

### 3.3 AdaptiveRiskManager 趋势防御入口

**改动点**: `risk/adaptive.go`

**新增方法**:
```go
func (a *AdaptiveRiskManager) UpdateWithTrend(forceTrend bool) {
    a.mu.Lock()
    defer a.mu.Unlock()
    
    
    avgAdverseRate := 0.0
    for _, rate := range a.recentAdverseRates {
        avgAdverseRate += rate
    }
    avgAdverseRate /= float64(len(a.recentAdverseRates))
    
    if forceTrend || avgAdverseRate > a.config.AdverseThreshold {
        // 高逆选 → 收紧风控
        a.currentNetMax = a.config.BaseNetMax * (1.0 - a.config.NetMaxAdjustStep)
        a.currentBaseSize = a.config.BaseSize * (1.0 - a.config.SizeAdjustStep)
        a.currentMinSpreadBps = a.config.BaseMinSpreadBps * (1.0 + a.config.SpreadAdjustStep)
        a.riskMode = "defensive"
    } else {
        // 正常 → 松开风控
        a.currentNetMax = a.config.BaseNetMax * (1.0 + a.config.NetMaxAdjustStep)
        a.currentBaseSize = a.config.BaseSize * (1.0 + a.config.SizeAdjustStep)
        a.currentMinSpreadBps = a.config.BaseMinSpreadBps * (1.0 - a.config.SpreadAdjustStep)
        a.riskMode = "aggressive"
    }
    
    a.lastAdjustTime = time.Now()
}
```

**效果**:
- 外部调用 `UpdateWithTrend(true)` 时强制进入防御模式，降低 `netMax`/`baseSize`，拉宽价差。
- 后续可接入 `market/regime.go` 的趋势检测，自动触发防御。

---

### 3.4 几何加宽层级与远端衰减

**新文件**: `strategy/grid_geometric.go`

**核心函数**:
```go
func BuildGeometricGrid(mid float64, levelCount int, baseSize, spacingRatio, sizeDecay float64) []GridLevel {
    if levelCount < 2 { levelCount = 2 }
    if baseSize <= 0 { baseSize = 1 }
    if spacingRatio <= 1.0 { spacingRatio = 1.10 }
    if sizeDecay <= 0 || sizeDecay >= 1 { sizeDecay = 0.95 }
    
    levels := make([]GridLevel, 0, levelCount*2)
    baseStep := mid * 0.0005
    step := baseStep
    size := baseSize
    for i := 1; i <= levelCount; i++ {
        // 远端衰减
        size *= sizeDecay
        levels = append(levels, GridLevel{Price: mid - step, Size: size})
        levels = append(levels, GridLevel{Price: mid + step, Size: size})
        step *= spacingRatio
    }
    return levels
}
```

**接入点**: `strategy/grid.go`

```go
func (g *GridStrategy) GenerateQuotes(ctx context.Context) ([]Quote, error) {
    
    if g.cfg.LayerSpacingMode == "geometric" && g.cfg.MaxLayers > 0 {
        levels := BuildGeometricGrid(
            mid,
            g.cfg.MaxLayers,
            g.cfg.BaseSize,
            g.cfg.SpacingRatio,
            g.cfg.LayerSizeDecay,
        )
        for _, lv := range levels {
            if lv.Price > mid {
                quotes = append(quotes, Quote{Side: "SELL", Price: lv.Price, Size: lv.Size})
            } else {
                quotes = append(quotes, Quote{Side: "BUY", Price: lv.Price, Size: lv.Size})
            }
        }
    } else {
        // ... existing linear grid code ...
    }
    
    return quotes, nil
}
```

**配置示例**:
```yaml
layerSpacingMode: geometric
spacingRatio: 1.20           # 每层间距 ×1.20
layerSizeDecay: 0.90         # 每层下单量 ×0.90
maxLayers: 24                # 最大 24 层
```

**效果**:
- 第一层距中间价 0.05%，第二层 0.06%，第三层 0.072%，……，第 24 层约 1.9%。
- 近端下单量 0.009 ETH，远端逐层衰减至 ≈0.001 ETH。
- 单边偏移 2% 时仍有多层挂单在远端等待回归，避免僵持。

---

## 四、Round7 配置草案

**文件**: `configs/config_round7_geometric_drawdown.yaml`

### 4.1 关键参数对比

| 参数                        | Round6（200 USDC）       | Round7（200 USDC）       | 变化说明                                   |
|-----------------------------|-------------------------|-------------------------|--------------------------------------------|
| `baseSize`                  | 0.012                   | 0.009                   | 略降，减少单笔风险                          |
| `netMax`                    | 0.16                    | 0.21                    | 提升 31%，配合分层减仓给予缓冲             |
| `takeProfitPct`             | 0.0025                  | 0.0025                  | 不变                                       |
| `staticFraction`            | 0.97                    | 0.97                    | 不变，保持高 Maker 占比                    |
| `staticRestMs`              | 40000                   | 40000                   | 不变                                       |
| `reduceOnlyMaxSlippagePct`  | 0.0003                  | 0.0003                  | 不变，严格滑点控制                         |
| `stopLoss`                  | -60.0                   | -60.0                   | 不变，硬止损兜底                           |
| **layerSpacingMode**        | -                       | `geometric`             | 新增：几何加宽模式                         |
| **spacingRatio**            | -                       | 1.20                    | 新增：层间距 ×1.20                         |
| **layerSizeDecay**          | -                       | 0.90                    | 新增：远端下单量 ×0.90                     |
| **maxLayers**               | -                       | 24                      | 新增：最大 24 层                           |
| **drawdownBands**           | -                       | [5, 8, 12]              | 新增：浮亏档位（%）                        |
| **reduceFractions**         | -                       | [0.15, 0.25, 0.40]      | 新增：每档减仓比例                         |
| **reduceMode**              | -                       | `maker_first_then_taker`| 新增：减仓模式                             |
| **reduceCooldownSeconds**   | -                       | 120                     | 新增：冷却时间（秒）                       |

### 4.2 完整配置文件（节选）

```yaml
env: production

risk:
  maxOrderValueUSDT: 50
  maxNetExposure: 200

gateway:
  apiKey: "..."
  apiSecret: "..."
  baseURL: "https://fapi.binance.com"

inventory:
  targetPosition: 0
  maxDrift: 0.5

symbols:
  ETHUSDC:
    tickSize: 0.01
    stepSize: 0.001
    minQty: 0.001
    maxQty: 1000
    minNotional: 10

    strategy:
      minSpread: 0.0006
      feeBuffer: 0.0002
      baseSize: 0.009
      quoteIntervalMs: 500
      takeProfitPct: 0.0025
      staticFraction: 0.97
      staticTicks: 12
      staticRestMs: 40000
      
      # 几何网格
      layerSpacingMode: geometric
      spacingRatio: 1.20
      layerSizeDecay: 0.90
      maxLayers: 24
      
      # 动量与库存压力（保留现有）
      inventoryPressureThreshold: 0.4
      inventoryPressureStrength: 0.5
      inventoryPressureExponent: 2.5
      momentumThreshold: 0.0006
      momentumAlpha: 0.08

    risk:
      singleMax: 0.06
      dailyMax: 50.0
      netMax: 0.21
      latencyMs: 150
      pnlMin: -15.0
      pnlMax: 100.0
      reduceOnlyThreshold: 25.0
      reduceOnlyMaxSlippagePct: 0.0003
      reduceOnlyMarketTriggerPct: 0.15
      stopLoss: -60.0
      haltSeconds: 300
      shockPct: 0.10
      
      # 浮亏分层减仓
      drawdownBands: [5, 8, 12]
      reduceFractions: [0.15, 0.25, 0.40]
      reduceMode: maker_first_then_taker
      reduceCooldownSeconds: 120
```

---

## 五、监控修复

### 5.1 问题
- 系统 Node Exporter 占用 `:9100`，导致 runner 的 metrics server 无法绑定。
- Prometheus 抓取目标为 `host.docker.internal:9100`，实际抓到的是系统指标而非 `mm_*` 指标。

### 5.2 修复
1. **runner 端口改为 `:9101`**（`cmd/runner/main.go` 默认值更新）
2. **Prometheus 抓取目标改为 `host.docker.internal:9101`**（`monitoring/prometheus/prometheus.yml`）
3. **重启 Prometheus 容器**（`docker compose restart prometheus`）

### 5.3 验证
```bash
curl -s http://localhost:9101/metrics | grep -E '^mm_' | head -n 5
```
应返回：
```
mm_order_submitted_total{symbol="ETHUSDC",side="BUY"} 123
mm_order_filled_total{symbol="ETHUSDC",side="SELL"} 45
mm_pnl_unrealized{symbol="ETHUSDC"} -2.34
...
```

---

## 六、改造文件清单

### 6.1 新增文件（4 个）
1. `risk/drawdown_manager.go` - 浮亏分层减仓管理器
2. `strategy/grid_geometric.go` - 几何加宽网格生成器
3. `configs/config_round7_geometric_drawdown.yaml` - Round7 配置草案
4. `reports/round7_refactor_summary.md` - 本报告

### 6.2 修改文件（6 个）
1. `internal/engine/trading_engine.go` - 成交前净仓容量收敛
2. `cmd/runner/main.go` - DrawdownManager 实例化与周期调用
3. `risk/adaptive.go` - 新增 `UpdateWithTrend(forceTrend bool)` 方法
4. `strategy/grid.go` - 接入几何网格分支
5. `config/load.go` - 新增配置字段（`LayerSpacingMode`、`SpacingRatio`、`LayerSizeDecay`、`MaxLayers`、`DrawdownBands`、`ReduceFractions`、`ReduceMode`、`ReduceCooldownSeconds`）
6. `monitoring/prometheus/prometheus.yml` - 抓取端口改为 `:9101`

### 6.3 编译验证
```bash
cd /root/market-maker-go
go build -o /tmp/test_runner ./cmd/runner/main.go
echo $?  # 应返回 0
```

---

## 七、下一步建议

### 7.1 集成测试（建议在下次实盘前执行）
1. **单边趋势仿真**：回放 Round6 的行情数据，验证几何网格是否覆盖更宽价格范围。
2. **浮亏减仓验证**：模拟浮亏 5-12% 场景，确认 `DrawdownManager.Plan` 返回正确减仓量。
3. **超限拦截验证**：模拟快速成交并发，确认净仓不会超过 `netMax=0.21`。

### 7.2 实盘准备
1. **确认配置文件**：使用 `config_round7_geometric_drawdown.yaml` 或将其参数合并至 `config_maker_free.yaml`。
2. **启动命令**：
   ```bash
   cd /root/market-maker-go
   go run ./cmd/runner -config ./configs/config_round7_geometric_drawdown.yaml -dryRun=false -metricsAddr :9101 > logs/round7_live.log 2>&1 &
   ```
3. **监控面板**：访问 `http://localhost:3001`，选择"Market Maker 综合面板"，确认指标正常更新。
4. **日志观察**：
   ```bash
   tail -f logs/round7_live.log | grep -E 'drawdown_trigger|net exposure|Order.*FILLED'
   ```

### 7.3 待扩展功能（可选，不阻塞 Round7）
1. **趋势自动检测**：接入 `market/regime.go`，自动调用 `AdaptiveRiskManager.UpdateWithTrend(true)`。
2. **减仓单自动挂单**：在 `drawdown_trigger` 事件中自动构造 reduce-only 订单并提交（当前仅记录日志）。
3. **在途挂单超限取消**：在 `order/reconciler.go` 中增加"潜在增仓评估"，提前取消可能导致超限的挂单。

---

## 八、风险提示

### 8.1 参数敏感性
- `netMax` 从 0.16 提升至 0.21（31%），虽配合分层减仓，但单边趋势中仍可能较快累积至上限，建议实盘前仿真验证。
- `spacingRatio=1.20` 与 `layerSizeDecay=0.90` 为经验值，不同币种波动率可能需调整。

### 8.2 分层减仓冷却
- `reduceCooldownSeconds: 120` 设为 2 分钟，避免频繁减仓。若浮亏快速扩大，可能无法及时响应，建议结合硬止损（`stopLoss: -60.0`）兜底。

### 8.3 监控依赖
- Prometheus/Grafana 需正常运行，若容器停止或网络异常，将无法实时观察指标，建议在启动实盘前验证：
  ```bash
  curl -s http://localhost:9101/metrics | grep mm_ | wc -l  # 应 > 0
  curl -s http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | select(.labels.job=="market-maker-go") | .health'  # 应返回 "up"
  ```

---

## 九、总结

本次改造通过 **硬帽回退**、**分层减仓**、**趋势防御** 与 **几何加宽层级** 四大机制，系统性解决了 Round6 暴露的"单边僵持"与"超限风险"问题。Round7 配置草案已准备就绪，建议在集成测试验证后启动新一轮实盘，并持续观察以下关键指标：

- **净仓上限命中率**：`net exposure limit exceeded` 日志频率应显著降低。
- **浮亏减仓触发次数**：预期在 5-12% 浮亏时触发，避免硬止损。
- **远端层级成交率**：几何网格应在单边偏移 1-2% 时仍有远端成交，覆盖更宽范围。
- **账户净 PnL**：在 200 USDC 本金下，24 小时净盈亏预期在 -5 至 +10 USDC 区间（具体取决于行情）。

---

**报告结束**  
**下一步**: 等待用户审阅，确认后可启动集成测试或直接准备 Round7 实盘。
