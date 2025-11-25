# 第二轮测试执行计划 - 改进参数验证

> **执行日期**: 2025-11-24  
> **目标**: 验证参数改进方案的有效性  
> **预期结果**: 从负收益转为正收益

---

## 第二轮测试具体参数

### 关键改进（相对于Round 1）

```yaml
Round 1 配置:
  minSpreadPct: 0.03%        # 原始设置
  skewSensitivity: 1.0        # 原始设置
  reduceOnlyMaxSlippagePct: 0.5%  # 原始设置

Round 2 配置（改进）:
  minSpreadPct: 0.04%        # 提高1bp，覆盖手续费
  skewSensitivity: 1.5        # 提高50%，加快库存回归
  reduceOnlyMaxSlippagePct: 0.75%  # 放松50%，减少市价触发
```

### 预期改进幅度

```
收益指标:
  成交率: 28% → 40% (+42%)
  盈利笔: 9% → 50% (+456%)
  总P&L: -2.41 USD → +0.05 USD (+104%)

稳定性:
  无成交时长: 34分 → <5分 (-85%)
  系统稳定: 保持优秀
```

---

## 测试执行步骤

### 第一步：环境准备
```bash
# 1. 确保API密钥已设置
export BINANCE_API_KEY="your_key"
export BINANCE_SECRET_KEY="your_secret"

# 2. 确认账户状态
# - USDC余额充足（>100 USDC）
# - 无未平仓订单
# - 仓位已清零

# 3. 准备监控
# - 打开Prometheus (localhost:9090)
# - 打开Grafana (localhost:3000)
# - 准备tail日志命令
```

### 第二步：启动测试
```bash
# 使用改进配置运行
timeout 600 go run cmd/runner/main.go \
  -config configs/config_qa_test_improved_20251124.yaml 2>&1 \
  | tee logs/round2_test.log

# 说明：
# - timeout 600: 运行10分钟后自动停止
# -c onfig: 使用改进配置
# - tee: 同时输出到日志和终端
```

### 第三步：实时监控（并行执行）
```bash
# 终端2：监控关键指标
watch -n 1 'tail -20 logs/runner.log | grep -E "fill|pnl|spread|inventory"'

# 终端3：每30秒记录一次关键数据
while true; do
  echo "=== $(date) ===" >> logs/round2_metrics_log.txt
  tail -5 logs/runner.log >> logs/round2_metrics_log.txt
  sleep 30
done

# 终端4：监控CSV数据
tail -f logs/net_value_runs_ethusdc.csv | grep -v "^time"
```

### 第四步：测试完成
```bash
# 数据收集
- 查看 logs/net_value_runs_ethusdc.csv 最后N行数据
- 计算 Round 2 的关键指标
- 与 Round 1 对比
```

---

## 关键监控点（每分钟记录）

| 时间 | 检查项 | 预期 |
|------|--------|------|
| 0分 | 系统启动、行情连接 | ✓ |
| 1分 | 首笔报价、首笔成交 | ✓ |
| 3分 | 累积成交数（目标2+笔） | ✓ |
| 5分 | 中期评估、库存状态 | 无明显亏损 |
| 7分 | 成交频率、盈利比例 | >30% |
| 9分 | 最终数据、平稳退出 | ✓ |
| 10分 | 测试完成、数据分析 | ✓ |

---

## 预期问题处理

### 若第2分钟无成交
**原因**: 
- 新参数让spread更宽（0.04%），可能成交率下降
- 市场流动性不足

**处理**:
- 继续运行，不急于干预
- 观察到5分钟标记

### 若出现连续亏损（>3笔）
**原因**:
- 参数仍不适配市场
- 库存倾斜不够激进

**处理**:
- 允许继续运行记录完整数据
- 计划Round 3使用激进参数（spread 0.05%, skew 2.0）

### 若系统异常（无日志更新>30秒）
**原因**:
- 网络问题
- 系统hang

**处理**:
- 立即停止测试（Ctrl+C）
- 切回Round 1配置诊断问题

---

## 数据分析框架

###对比指标计算

```python
# Round 2 vs Round 1

def calculate_metrics(csv_file):
    data = read_csv(csv_file)
    
    total_fills = len(data)
    profitable_fills = len(data[data['pnl'] > 0])
    profit_rate = profitable_fills / total_fills
    
    total_pnl = data['pnl'].sum()
    avg_pnl_per_fill = data['pnl'].mean()
    max_loss = data['pnl'].min()
    
    return {
        'fills': total_fills,
        'profit_rate': profit_rate,
        'total_pnl': total_pnl,
        'avg_pnl': avg_pnl_per_fill,
        'max_loss': max_loss
    }

round2 = calculate_metrics('logs/net_value_runs_ethusdc.csv')
round1 = {...}  # 从第一轮数据中获取

# 对比结果
improvement = {
    'fills_delta': round2['fills'] - round1['fills'],
    'profitability_improvement': round2['profit_rate'] - round1['profit_rate'],
    'pnl_improvement': round2['total_pnl'] - round1['total_pnl'],
}
```

---

## 成功标准（3选2原则）

### 标准A：收益指标 ✓/✗
- [ ] 总P&L 为正（> 0）
- [ ] 盈利笔数比例 > 50%
- [ ] 平均单笔盈利 > 1 USD bps

**判定**: 如果2项以上满足 → 收益标准通过

### 标准B：稳定性指标 ✓/✗
- [ ] 无系统crash/panic
- [ ] 订单管理100%准确
- [ ] 强制市价触发 ≤ 2次

**判定**: 如果2项以上满足 → 稳定性标准通过

### 标准C：操作指标 ✓/✗
- [ ] 成交率 ≥ 40%
- [ ] 无长时间(>5分)无成交
- [ ] 成交频率稳定

**判定**: 如果2项以上满足 → 操作标准通过

### 最终判定
```
若 3个标准中 ≥ 2个通过:
  ✓ Round 2 成功！进入 Round 3（24小时完整测试）

若 3个标准中 < 2个通过:
  × Round 2 未通过，触发应急方案
  - 分析失败原因
  - 调整参数进行 Round 3（激进方案）
```

---

## 应急方案

### 应急方案A：激进参数调整
若Round 2失败，尝试更激进的参数：
```yaml
激进方案:
  minSpreadPct: 0.05%        # 继续提高
  skewSensitivity: 2.0        # 更激进倾斜
  reduceOnlyMaxSlippagePct: 1.0%  # 进一步放松
```

### 应急方案B：保守回退
若激进方案也失败，回退到：
```yaml
保守方案:
  minSpreadPct: 0.035%       # 微调
  skewSensitivity: 1.2        # 微调
  reduceOnlyMaxSlippagePct: 0.6%  # 微调
```

### 应急方案C：代码优化
若参数调优都无效，需要考虑：
- 是否需要增加盘口深度分析
- 是否需要改进报价策略（引入波动率调整）
- 是否需要优化订单填充逻辑

---

## 测试执行时间表

| 时间 | 任务 |
|------|------|
| 10:40 | 所有准备完成，启动Round 2 |
| 10:50 | 5分钟中期检查 |
| 11:00 | 测试完成，数据收集 |
| 11:05 | 初步分析，判定是否通过 |
| 11:10 | 决定是否继续Round 3或寻找调优方向 |

---

## 预期收益曲线（如果改进有效）

```
Round 1 结果:
  收益曲线: ↘  (持续亏损)
  最终P&L: -2.41 USD
  
Round 2 预期（改进有效）:
  收益曲线: ↗  (持续盈利)
  最终P&L: +0.05 ~ +0.20 USD
  
目标（Round 3+）:
  收益曲线: ↗↗ (稳定正向)
  日化收益: > 0.1% (年化36%+)
```

---

**执行开始时间**: 待用户确认  
**测试配置**: configs/config_qa_test_improved_20251124.yaml  
**数据输出**: logs/net_value_runs_ethusdc.csv  
**预期完成**: 11:00 UTC
