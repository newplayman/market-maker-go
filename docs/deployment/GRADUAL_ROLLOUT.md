# 灰度发布计划（100 USDC 起步版）

## 阶段1: Testnet验证 (2天)

### 目标
- 验证系统完整功能
- 识别潜在问题
- 建立监控基线

### 环境配置
```yaml
环境: Binance Testnet
资金: 模拟资金（无限）
交易对: ETHUSDC
运行模式: 完全自动化
监控: 24小时监控
```

### 配置参数
```yaml
exchange:
  testnet: true  # 重要！

strategy:
  base_spread: 0.002
  base_size: 0.01
  max_inventory: 0.05
```

### 验收标准
- [ ] 0崩溃运行48小时
- [ ] 所有功能正常工作
- [ ] 订单准确率 100%
- [ ] 风控触发准确
- [ ] 监控告警正常

### 操作步骤
```bash
# 1. 配置 testnet
vim configs/config.yaml  # 设置 testnet: true

# 2. 启动
./scripts/start.sh

# 3. 监控
./scripts/health_check.sh  # 每小时检查
journalctl -u market-maker -f  # 实时日志

# 4. 48小时后评估
```

---

## 阶段2: 小资金实盘 (3-5天)

### 目标
- 真实环境验证
- 收益能力验证
- 风险控制验证

### 环境配置  
```yaml
环境: Binance Mainnet
资金: 100 USDC
交易对: ETHUSDC
运行时长: 72-120小时
监控: 实时监控 + 人工值守
```

### 风控设置（100 USDC 专用）
```yaml
exchange:
  testnet: false  # 切换到主网

strategy:
  base_spread: 0.002      # 0.2% (较保守)
  base_size: 0.001        # 约 3 USDC/单
  max_inventory: 0.01     # 最大持仓 0.01 ETH (约30 USDC)
  tick_interval: 10s

risk:
  daily_loss_limit: 5.0   # 5 USDC (5%资金)
  max_drawdown_limit: 0.05 # 5%
  max_position: 0.01      # 30 USDC
  circuit_breaker:
    threshold: 3
    timeout: 10m
```

### 每日复盘（22:00）
```markdown
1. 交易统计
   - 总订单数、成交订单数、撤单数
   - 平均spread、成交率
  
2. 收益分析
   - 每日PnL（目标: > $0.1）
   - 累计PnL
   - 收益率（目标: > 0.1%/天）
  
3. 风险评估
   - 最大回撤（容忍: < 5%）
   - 风控触发情况
   - 最大单笔亏损
  
4. 系统性能
   - 平均延迟、P95、P99
   - 系统稳定性
   - 资源使用情况
  
5. 问题记录
   - 发现的问题
   - 改进建议  
   - 待优化项
```

### 成功标准（100 USDC）
- [ ] 运行72小时无重大问题
- [ ] 累计PnL > 0（不亏即可）
- [ ] 日均收益 > $0.1（0.1%）
- [ ] 最大回撤 < 5%
- [ ] 订单准确率 > 99.5%
- [ ] 系统稳定性 > 99.9%

### 失败处理（100 USDC）
立即停止条件：
1. 累计亏损 > $10（10%）
2. 单日亏损 > $5（5%）
3. 系统崩溃 > 3次
4. 订单异常 > 10%
5. 数据不一致

---

## 阶段3: 逐步加仓 (1-2周)

### 加仓计划（从 100 USDC 开始）
```
Day 1-3:   100 USDC   (保持，观察稳定性)
Day 4-5:   200 USDC   (翻倍，验证可扩展性)
Day 6-7:   500 USDC   (5倍，观察表现)
Day 8-10:  1000 USDC  (10倍，正常规模)
Day 11-14: 2000 USDC  (20倍，根据表现)
Day 15+:   根据收益和风险决定
```

### 每次加仓前检查清单
- [ ] 前期收益稳定（累计PnL > 0）
- [ ] 无重大故障
- [ ] 系统资源充足（CPU < 50%, 内存 < 70%）
- [ ] 风控参数相应调整
- [ ] 监控告警正常
- [ ] 夏普比率 > 1（如有足够数据）

### 加仓时风控调整示例

**200 USDC 配置**：
```yaml
risk:
  daily_loss_limit: 10.0   # 5%
  max_position: 0.02       # 60 USDC
```

**500 USDC 配置**：
```yaml
risk:
  daily_loss_limit: 25.0   # 5%
  max_position: 0.05       # 150 USDC
```

**1000 USDC 配置**：
```yaml
risk:
  daily_loss_limit: 50.0   # 5%
  max_position: 0.1        # 300 USDC
```

---

## 阶段4: 稳定运行 (长期)

### 日常运维
```markdown
每日检查（自动化）:
  - 08:00 系统健康检查
  - 12:00 中午检查
  - 18:00 下午检查
  - 22:00 每日复盘

每周检查:
  - 周日: 全面性能分析
  - 参数优化建议
  - 策略调整评估
  - 安全审计

每月检查:
  - 全面回测验证
  - 收益分析报告
  - 风险评估报告
  - 系统升级计划
```

### 持续优化方向
```markdown
1. 策略优化
   - 参数调优（A/B测试）
   - 信号增强（多指标）
   - 多品种支持（BTC, SOL等）
  
2. 性能优化
   - 降低延迟（目标 P95 < 50ms）
   - 提高吞吐（支持更多订单）
   - 减少资源占用
  
3. 风控加强
   - 多维度监控（相关性、波动率）
   - 智能熔断（机器学习）
   - 异常检测（异常模式识别）
  
4. 功能扩展
   - 跨交易所套利
   - API接口开发
   - 更多策略类型（网格、趋势跟踪）
```

---

## 监控检查清单

### 启动前检查
- [ ] 配置文件验证（testnet 设置）
- [ ] API密钥测试（testnet key vs mainnet key）
- [ ] 网络连接检查（ping api.binance.com < 10ms）
- [ ] 系统资源检查（磁盘空间 > 10GB）
- [ ] 日志目录权限

### 运行中监控（100 USDC）
- [ ] 每30分钟检查系统状态
- [ ] 每小时审查交易记录
- [ ] 实时监控PnL变化
  - 警告: PnL < -$2
  - 告警: PnL < -$5
- [ ] 关注风控告警（邮件/钉钉）

### 每日复盘清单
- [ ] 总订单数统计
- [ ] 成交数和成交率
- [ ] 日PnL和累计PnL
- [ ] 最大回撤
- [ ] 风控触发次数
- [ ] 系统延迟统计
- [ ] 异常事件记录
- [ ] 改进建议

---

## 应急预案

### Level 1: 警告 (黄色)
- **触发**: PnL下降 > $2（2%）
- **处理**: 
  - 加强监控（每15分钟检查）
  - 准备干预
  - 分析原因

### Level 2: 减仓 (橙色)
- **触发**: PnL下降 > $5（5%）
- **处理**: 
  - 减半仓位（改为50 USDC）
  - 调整参数（增加spread）
  - 暂停自动交易，人工审查

### Level 3: 停止 (红色)
- **触发**: PnL下降 > $10（10%）
- **处理**: 
  - 立即停止: `./scripts/emergency_stop.sh`
  - 全面检查代码
  - 问题修复后重新测试
  - 回到 Testnet 验证

### 紧急联系
```
技术负责人: [您的姓名]
电话: [您的电话]
邮箱: [您的邮箱]

备用联系: [备用人员]
电话: [备用电话]
```

---

## 成功指标（100 USDC）

### 第1周目标
- ✅ 系统稳定运行
- ✅ 累计PnL ≥ 0
- ✅ 无重大故障

### 第2周目标
- ✅ 累计PnL > $1
- ✅ 日均收益率 > 0.1%
- ✅ 准备加仓到200 USDC

### 第1个月目标
- ✅ 累计PnL > $10
- ✅ 月收益率 > 3%
- ✅ 夏普比率 > 1
- ✅ 系统稳定性 > 99.5%

---

## 附录：快速命令参考

```bash
# 启动（Testnet）
vim configs/config.yaml  # 设置 testnet: true
./scripts/start.sh

# 切换到主网（100 USDC）
vim configs/config.yaml  # 设置 testnet: false
./scripts/restart.sh

# 健康检查
./scripts/health_check.sh

# 查看日志
journalctl -u market-maker -f

# 紧急停止
./scripts/emergency_stop.sh

# 备份
./scripts/backup.sh

# 回滚
./scripts/rollback.sh
```
