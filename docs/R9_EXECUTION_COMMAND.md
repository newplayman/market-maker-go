# R9 参数优化测试 - 启动指令

**质检工程师**: 资深测试专家  
**生成时间**: 2025-11-24 13:37 UTC  
**目的**: 验证参数优化效果

---

## 快速启动

```bash
cd /root/market-maker-go && go run cmd/runner/main.go -config configs/config_r9_optimized.yaml
```

## 参数对比

| 参数 | R6 | R9 | 变化 |
|-----|----|----|------|
| baseSize | 0.008 | 0.012 | +50% |
| minSpread | 0.0003 | 0.00035 | +17% |
| singleMax | 0.024 | 0.036 | +50% |
| netMax | 0.024 | 0.036 | +50% |

## 预期效果

**R6基准**: 34笔成交, -0.34 USD收益

**R9目标**:
- 成交笔数: 40-50笔 (+18-47%)
- 收益状况: 0 USD 或 +值
- 成本改善: 继续优化

## 运行监控指标

监控命令:
```bash
tail -f logs/runner.log | grep -E "order_place|strategy_adjust|pnl"
```

关键指标:
1. **order_place** - 下单事件
2. **strategy_adjust** - 报价调整
3. **pnl** - 盈亏状态

## 停止指令

```bash
pkill -f "go run"
```

或按 `Ctrl+C` 停止

## 预期运行时间

- 快速验证: 10分钟
- 完整验证: 30-60分钟
- 深度验证: 2-4小时

## 执行步骤

1. **终端1** - 启动系统:
   ```bash
   cd /root/market-maker-go
   go run cmd/runner/main.go -config configs/config_r9_optimized.yaml
   ```

2. **终端2** - 监控日志:
   ```bash
   tail -f logs/runner.log | grep "order_place"
   ```

3. **终端3** - 检查进程:
   ```bash
   watch -n 1 'ps aux | grep go'
   ```

4. **观察指标**:
   - 成交频率（每5秒）
   - 累计成交数
   - 单笔盈损

## 预期结果评估标准

**成功标准** (3选2):
- ✓ 成交笔数 ≥ 40
- ✓ wallet_delta ≥ 0
- ✓ 无风控告警

**改进幅度**:
- vs R6: +15% ~ +50% 成交
- vs R1: +350% ~ +500% 成交

## 日志位置

```
logs/runner.log              # 实时日志
logs/net_value_ethusdc.csv   # 净值变化
```

## 故障排查

**问题**: 无挂单输出
- 原因: 风控卡住 → 检查日志中的 "risk_event"
- 原因: 参数格式错 → 验证 config_r9_optimized.yaml

**问题**: 频繁撤单
- 原因: 市场波动大 → 正常
- 原因: spread 太窄 → 观察后续优化

---

**质检工程师最后意见**: 该参数配置基于R6的+209%成交改善而来，有明确的量化支撑。预期能进一步验证收益趋势。
