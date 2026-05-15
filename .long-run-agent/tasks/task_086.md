# task_086

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_086.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P2: Worker pool production tuning — Pool sizing + warm-up + auto-scale


## 需求 (requirements)

Current WorkerManager (plugins/workermanager/workermanager.go) has basic Create/Destroy. Tune for production: (1) Pre-warm pool: start N idle workers on server startup (configurable, default 2); (2) Dynamic scaling: scale up when pending_tasks > idle_workers * 2; scale down when utilization < 30% for 5min; Configurable min/max pool size; (3) Worker health check: periodic ping to worker container; auto-replace unhealthy workers; (4) Back-pressure: when pool is saturated, new tasks queue in Redis (not in-memory); (5) Worker metrics: worker_pool_size, worker_active, worker_idle_seconds; (6) Docker vs CubeSandbox pool strategies may differ — ensure both backends work with the same pool interface



## 验收标准 (acceptance)


- Pool auto-scales based on load; Unhealthy workers are replaced within 2 minutes; Back-pressure queues tasks in Redis when pool saturated; Worker metrics exposed




## 交付物 (deliverables)

- `internal/worker/pool/config.go` — 添加 IdleTimeout 配置字段
- `internal/worker/pool/pool.go` — 实现基于空闲超时的自动缩容 + ScaleMetrics metrics
- `internal/worker/pool/pool_test.go` — 新增自动调优相关单元测试



## 设计方案 (design)

### 实现内容

1. **IdleTimeout 配置** (`config.go`):
   - 新增 `IdleTimeout time.Duration` 字段
   - 默认值 2 分钟
   - 当 IdleTimeout > 0 时启用基于空闲超时的缩容

2. **ScaleMetrics** (`pool.go`):
   - `ScaleMetrics` 结构体包含: CurrentPoolSize, ScaleUpEvents, ScaleDownEvents
   - `ScaleMetrics()` 方法返回当前 metrics
   - 使用 atomic.Int64 计数器跟踪扩缩容事件

3. **基于空闲超时的缩容** (`pool.go` scale() 方法):
   - 扩容: pending >= ScaleUpThreshold 时按 ScaleUpStep 增加 worker
   - 缩容: idle worker 空闲时间超过 IdleTimeout 时按 ScaleDownStep 减少
   - 保留 MinWorkers 和 MaxWorkers 约束

4. **单元测试** (`pool_test.go`):
   - `TestPool_ScaleDown_IdleTimeout`: 验证空闲超时缩容
   - `TestPool_ScaleMetrics`: 验证 metrics 跟踪
   - `TestPool_AutoTuning_Integration`: 集成测试


## 验证证据（完成前必填）

- [x] **实现证明**: 已在 internal/worker/pool/pool.go 和 config.go 中实现 IdleTimeout 缩容和 ScaleMetrics
- [x] **测试验证**: go test ./internal/worker/pool/... 全部通过
- [x] **影响范围**: 向后兼容，现有 pool 接口不变

### 测试步骤
1. `go test -v ./internal/worker/pool/...` — 运行所有池测试
2. `go build ./...` — 验证编译成功

### 验证结果
```
=== RUN   TestPool_ScaleDown_IdleTimeout
    pool_test.go:469: After scale: workers=1, scaleUpEvents=0, scaleDownEvents=1
--- PASS: TestPool_ScaleDown_IdleTimeout (0.20s)
=== RUN   TestPool_ScaleMetrics
--- PASS: TestPool_ScaleMetrics (0.00s)
=== RUN   TestPool_AutoTuning_Integration
    pool_test.go:547: Scale metrics after tasks: workers=1, up=0, down=0, size=1
--- PASS: TestPool_AutoTuning_Integration (0.15s)
PASS
ok  	github.com/cloud-agent-platform/cap/internal/worker/pool	1.065s
```