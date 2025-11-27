# P1级问题 + Round8防闪烁修复计划

**日期**: 2025-11-27  
**目标**: 修复审计报告P1问题 + 实施Round8防闪烁改进

## 一、需求分析

### 1.1 审计报告P1问题
1. **改进错误处理和重试机制** - REST API调用失败后缺少统一重试策略
2. **添加配置参数验证** - 启动时验证所有配置参数
3. **优化价格标准差计算** - 添加缓存机制（TTL 10秒）

### 1.2 Round8防闪烁需求
1. **配置文件增强** - `configs/round8_survival.yaml` 新增quote_pinning段
2. **策略核心改动** - `internal/strategy/geometric_v2.go` 实现钉子模式
3. **磨成本小修** - `internal/risk/grinding.go` 使用固定大单
4. **新增监控指标** - 4个新指标

### 1.3 潜在冲突点
- ✅ **无冲突**: Round8改动针对geometric_v2，与已修复的ASMM策略独立
- ✅ **无冲突**: P1优化是基础设施改进，不影响策略逻辑
- ⚠️ **需注意**: grinding.go的改动需要考虑配置字段兼容性

## 二、实施计划

### Phase 1: 基础设施改进（P1问题）
**预计时间**: 2小时

#### 1.1 统一重试机制
- 文件: `gateway/rest_retry.go` (新建)
- 实现指数退避装饰器
- 集成到BinanceRESTClient

#### 1.2 配置参数验证
- 文件: `cmd/runner/main.go`
- 添加`validateConfig()`函数
- 启动时强制验证

#### 1.3 价格标准差缓存优化
- 文件: `internal/store/store.go`
- 添加缓存字段和TTL机制
- 保持并发安全

### Phase 2: Round8防闪烁实施
**预计时间**: 3小时

#### 2.1 配置文件扩展
- 文件: `configs/round8_survival.yaml`
- 添加quote_pinning配置段
- 更新配置结构体

#### 2.2 策略核心改造
- 文件: `internal/strategy/geometric_v2.go`
- 实现钉子模式逻辑
- 分段报价机制（近端+远端）
- 保持向后兼容

#### 2.3 磨成本引擎改进
- 文件: `internal/risk/grinding.go`
- 使用quote_pinning配置的固定size
- 保持配置灵活性

#### 2.4 监控指标增强
- 文件: `metrics/prometheus.go`
- 添加4个新指标
- 更新metrics使用点

### Phase 3: 测试验证
**预计时间**: 2小时

#### 3.1 单元测试
- 测试配置验证逻辑
- 测试标准差缓存
- 测试钉子模式触发

#### 3.2 集成测试
- 编译通过
- 竞态检测
- 配置加载测试

#### 3.3 Dry-run测试
- 使用新配置启动
- 验证钉子模式触发
- 监控指标采集

## 三、实施顺序

```
1. [P1-1] 实现统一重试机制 (30min)
   ├── 创建 gateway/rest_retry.go
   └── 测试重试逻辑

2. [P1-2] 添加配置参数验证 (30min)
   ├── 实现 validateConfig()
   └── 测试各种非法配置

3. [P1-3] 优化标准差计算 (30min)
   ├── 修改 store.go
   └── 测试缓存正确性

4. [R8-1] 扩展配置文件 (20min)
   ├── 更新 round8_survival.yaml
   └── 更新 Round8Config 结构体

5. [R8-2] 改造GeometricV2策略 (90min)
   ├── 实现钉子模式
   ├── 实现分段报价
   └── 单元测试

6. [R8-3] 改进Grinding引擎 (20min)
   ├── 读取quote_pinning配置
   └── 测试兼容性

7. [R8-4] 添加新监控指标 (20min)
   ├── 定义4个新指标
   └── 更新使用点

8. [TEST] 完整测试 (60min)
   ├── go test -race ./...
   ├── 编译验证
   └── Dry-run测试
```

## 四、关键代码设计

### 4.1 重试机制
```go
type RetryConfig struct {
    MaxRetries int
    BaseDelay  time.Duration
    MaxDelay   time.Duration
}

func WithRetry(fn func() error, cfg RetryConfig) error {
    for i := 0; i <= cfg.MaxRetries; i++ {
        err := fn()
        if err == nil {
            return nil
        }
        if !isRetryableError(err) {
            return err
        }
        if i < cfg.MaxRetries {
            delay := min(cfg.BaseDelay * time.Duration(1<<i), cfg.MaxDelay)
            time.Sleep(delay)
        }
    }
    return fmt.Errorf("max retries exceeded")
}
```

### 4.2 配置验证
```go
func validateConfig(cfg *Round8Config) error {
    if cfg.NetMax <= 0 {
        return fmt.Errorf("net_max must be positive")
    }
    if cfg.BaseSize <= 0 {
        return fmt.Errorf("base_size must be positive")
    }
    // ... 更多验证
    return nil
}
```

### 4.3 标准差缓存
```go
type Store struct {
    // ...
    cachedStdDev     float64
    cachedStdDevTime time.Time
    stdDevCacheTTL   time.Duration
}
```

### 4.4 钉子模式
```go
func (s *Strategy) GenerateQuotes() ([]Quote, []Quote) {
    if cfg.Enabled && math.Abs(pos)/netMax >= cfg.TriggerRatio {
        // 钉子模式
        return s.generatePinnedQuotes(pos, mid)
    }
    // 正常模式
    return s.generateNormalQuotes(pos, mid)
}
```

## 五、风险控制

### 5.1 向后兼容性
- ✅ quote_pinning.enabled默认false - 不影响现有配置
- ✅ 保留原有geometric_v2逻辑 - 通过enabled开关
- ✅ grinding引擎优雅降级 - 配置缺失时使用默认值

### 5.2 测试覆盖
- 单元测试覆盖新增逻辑
- 集成测试验证完整流程
- Dry-run测试验证实际效果

### 5.3 回滚计划
- 保留Git提交点
- 配置文件可快速切换
- 监控指标观察异常

## 六、验收标准

### 6.1 P1问题修复
- [ ] 所有REST调用有重试机制
- [ ] 配置验证100%覆盖
- [ ] 标准差计算性能提升>50%

### 6.2 Round8功能
- [ ] 钉子模式在70%仓位时触发
- [ ] 远端报价正确生成（4.8%-12%距离）
- [ ] 监控指标正常采集
- [ ] Dry-run测试无错误

### 6.3 质量保证
- [ ] go test -race无竞态
- [ ] 编译无警告
- [ ] 代码review通过
- [ ] 文档更新完整

## 七、时间表

- **Day 1 (今天)**: 完成Phase 1 + Phase 2.1-2.2
- **Day 2 (明天)**: 完成Phase 2.3-2.4 + Phase 3
- **Day 3 (后天)**: 24小时Dry-run测试 + 问题修复

**预计总工时**: 7小时开发 + 24小时测试
