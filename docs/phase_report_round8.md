# 阶段性报告：Round8 几何网格与智能订单管理改造

## 概述
- 目标：在不触发交易所速率限制的前提下，降低挂单密度与仓位占用，并强化进程与退出清场的可控性。
- 本阶段完成：几何网格参数收敛（格子减半但覆盖不变）、智能订单管理器队列化与组合限速、指标端口致命退出、退出清场规矩、DRY-RUN 验证。

## 与原“改造方案”的差异点
1) 智能订单管理器（Smart Order Manager）
- 原方案：以价格偏移/数量变化/老化为触发条件进行差分更新；逻辑以“逐层直接下单/撤单”为主。
- 现方案：在保持差分触发规则不变的前提下，引入“队列+调度+限速”三件套。
  - 队列化：所有下单/撤单操作进入统一队列，按合并窗口处理，避免爆量；接口改为入队并等待结果。
  - 调度策略：撤单优先、近端优先（按 layer 升序）、买卖交替（避免一侧拥挤）。
  - 组合限速：令牌桶基础速率 + 双窗硬上限（300/10s，2400/min）；命中阈值时阻塞等待，降低触顶概率。
  - 影响：在初次挂单和剧烈波动场景下，REST 请求节奏更加平滑；429/418 风险显著降低。
  - 相关代码：`SmartOrderManager` 的 `placeOrder`/`cancelOrder` 改为入队；新增 `runDispatcher` 与 `collectBatch`。

2) 指标端口占用处理
- 原方案：指标端口占用仅打印错误，主程序可能继续运行，存在“影子进程”风险。
- 现方案：`StartMetricsServer` 遇到端口占用直接致命退出（`log.Fatalf`），避免影子进程继续下单。

3) 退出清场规矩（Stop 钩子）
- 原方案：无统一的退出清场保证。
- 现方案：在容器 `Stop()` 中强制执行“撤单 + reduce-only 市价平仓”，并逐符号汇报结果；确保退出时不留挂单与敞口。

4) 几何网格参数收敛
- 原设：每边 28 层、`spacing_ratio: 1.185`，覆盖广但密度高。
- 现设：每边 14 层、`spacing_ratio: 1.35`，保持近端距离与最远覆盖接近原值，中间层数减半，降低仓位与订单压力。
- 实盘生效逻辑：`GeometricV2` 在 `MaxLayers=14`、`SpacingRatio=1.35` 下生成（BUY/SELL）阶梯；
- 配置改动文件：`configs/round8_survival.yaml`（`max_layers: 28 -> 14`，`spacing_ratio: 1.185 -> 1.35`）。

## 代码改动总览（关键文件）
- `internal/order_manager/smart_order_manager.go`
  - 新增队列类型与调度：`orderOp`/`opResult`、`runDispatcher`、`collectBatch`
  - `placeOrder`/`cancelOrder` 改为入队等待执行结果；维持快照更新与差分触发逻辑不变
  - 调度策略：撤单优先 + 近端优先 + 买卖交替
- `gateway/limiter.go`
  - 新增 `CompositeLimiter`：令牌桶 + 双窗硬限（10s/60s）
- `internal/container/container.go`
  - 网关限速：`Limiter: NewCompositeLimiter(25.0, 50, 280, 2200)`
  - 退出清场：`Stop()` 中循环符号执行撤单与 reduce-only 市价平仓
- `metrics/metrics.go`
  - 端口占用：`ListenAndServe` 错误使用 `log.Fatalf`，直接退出
- `configs/round8_survival.yaml`
  - `spacing_ratio: 1.35`、`max_layers: 14`

## 测试与验证
- 编译：`go build` 通过
- DRY-RUN（60秒）：
  - 首次挂单→一次全量重组→持续差分；总撤单 85 次
  - 无 429/418 错误；存在 -5022（Post-Only 拒单）为预期行为，不影响队列与限速验证
  - 队列合并窗口（200ms）与双窗限速均生效；请求速率未触顶
- 紧急清场工具：`cmd/emergency_cleanup` 验证“取消所有挂单 + reduce-only 平仓”
- 影子进程处置：通过致命退出与主动进程探测/终止，避免持久后台订单干扰

## 风险与后续计划
- 极端波动压力测试：建议 10 分钟 DRY-RUN 压测统计每 10s/60s 请求分布，确认在高波动场景下的安全边界
- 队列优化：增加“近似价格合并/去重”（tick×N 阈值），进一步减少冗余
- 实盘流程管控：统一前台/超时模式、启动前进程探测、退出信号时清场，杜绝多实例干扰

## 附：符号与文件（便于检索）
- 主要符号/方法：`SmartOrderManager`、`placeOrder`、`cancelOrder`、`runDispatcher`、`collectBatch`、`CompositeLimiter`、`StartMetricsServer`、`Container.Stop`
- 主要文件：`internal/order_manager/smart_order_manager.go`、`gateway/limiter.go`、`internal/container/container.go`、`metrics/metrics.go`、`configs/round8_survival.yaml`
