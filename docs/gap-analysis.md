# Cloud Agent Platform — 完整链路差距分析

> **日期**: 2026-05-15
> **版本**: v1.0
> **目的**: 识别 MCP Client → Complete 完整执行链路的断点

---

## 一、完整链路概述

```
MCP Client
    ↓
Gateway REST (internal/gateway/handler.go)
    ↓
Service.Submit (internal/service/task_service.go)
    ↓
Outbox Writer (internal/infra/outbox/writer.go)
    ↓
Outbox Poller → Redis Stream (internal/infra/outbox/poller.go)
    ↓
[断点] 无 Redis Stream 消费者路由到 Orchestrator
    ↓
[断点] Orchestrator.handleTaskSubmitted 是空壳
    ↓
[断点] Eino 图节点 LLM 未注入
    ↓
[断点] WorkerManager 从未被 Service 层调用
    ↓
[断点] Git Push / Complete 无实现
```

---

## 二、每步状态清单

| 步骤 | 组件 | 文件:行号 | 状态 | 说明 |
|------|------|----------|------|------|
| 1 | MCP Client → Gateway | `internal/mcp/executor.go:42-65` | ✅ 已实现 | 正确路由到 submitTask |
| 2 | Gateway → Service | `internal/gateway/handler.go:111-153` | ✅ 已实现 | 验证 auth 后调用 Submit |
| 3 | Service.Submit | `internal/service/task_service.go:89-224` | ✅ 已实现 | 事务写入 Task + OutboxEvent |
| 4 | Outbox Writer | `internal/infra/outbox/writer.go` | ✅ 已实现 | 事务内写入 outbox_events 表 |
| 5 | Outbox → Redis Stream | `internal/infra/outbox/poller.go:50-98` | ✅ 已实现 | RedisStreamForwarder.XADD 到 stream:domain_events |
| 6 | Redis Stream 消费 | `internal/gateway/ws/hub.go:267-276` | ⚠️ 部分实现 | 仅转发给 WebSocket 客户端，**不路由到 Orchestrator** |
| 7 | Orchestrator 事件处理 | `internal/orchestrator/orchestrator.go:815-822` | ❌ 空壳 | handleTaskSubmitted 只打日志不做任何事 |
| 8 | Orchestrator.StartTask | `internal/orchestrator/orchestrator.go:133-159` | ✅ 已实现 | 但从未被调用 |
| 9 | Eino Decompose 节点 | `plugins/orchestrator/nodes.go:430-543` | ⚠️ 存在但未调用 | MediumDecomposerNode 存在，LLM 字段为 nil |
| 10 | Worker Execute | `plugins/workermanager/workermanager.go:184-253` | ✅ 已实现 | 但从未被 Service 层调用 |
| 11 | Git Push | - | ❌ 未找到 | 整个链路中无 Git Push 实现 |

---

## 三、关键断点详解

### 断点 1: TaskService 无 Orchestrator 引用

**文件**: `internal/service/task_service.go:40-48`

```go
type TaskService struct {
    taskRepo     domain.TaskRepository
    subtaskRepo  domain.SubtaskRepository
    outboxWriter domain.OutboxWriter
    storage      TransactionManager
    logger       *zap.Logger
    metrics      *metrics.Recorder
    tracer       *tracing.SpanHelper
    // ❌ 缺少: orchestrator Orchestrator
    // ❌ 缺少: workerManager WorkerManager
}
```

**影响**: Submit 方法在事务内写入 TaskSubmittedV1 到 Outbox 后就结束了，没有触发任何编排行为。

---

### 断点 2: main.go 创建的是 Stub Orchestrator

**文件**: `cmd/server/main.go:320-326`

```go
// Initialize Orchestrator (Eino-based)
orch := orchestrator.New()          // 调用 plugins/orchestrator.New()
_ = orch                            // ❌ 创建后立即丢弃
```

**问题**:
- `plugins/orchestrator/orchestrator.go:6-13` 返回的是**空结构体**:
  ```go
  type Orchestrator struct {
      // TODO: Add Eino graph and dependencies
  }
  ```
- 真正的实现 `internal/orchestrator.NewOrchestrator()` (`orchestrator.go:96-131`) 有完整依赖注入，**从未被调用**

---

### 断点 3: 无 Redis Stream → Orchestrator 路由

**文件**: `internal/gateway/ws/hub.go:267-276`

WebSocket Hub 是唯一读取 `stream:domain_events` 的消费者，但它只做:
```go
case h.eventCh <- event:  // 推送给 WebSocket 客户端
    h.redisClient.XAck(...)
```

**缺少**: 一个专门路由 domain events 到 `Orchestrator.Dispatch()` 的消费者。

---

### 断点 4: handleTaskSubmitted 是空壳

**文件**: `internal/orchestrator/orchestrator.go:815-822`

```go
func (o *OrchestratorImpl) handleTaskSubmitted(ctx context.Context, event *domain.DomainEvent) error {
    o.logger.Info("received TaskSubmittedV1 event", ...)
    return nil  // ❌ 什么都没做
}
```

同样的问题存在于 `handleTaskAssigned` (825-830) 和 `handleTaskStarted` (834-839)。

---

### 断点 5: Orchestrator.Dispatch() 从未被调用

**文件**: `internal/orchestrator/orchestrator.go:793-795`

```go
func (o *OrchestratorImpl) Dispatch(ctx context.Context, event *domain.DomainEvent) error {
    return o.dispatcher.Dispatch(ctx, event)
}
```

**调用方搜索结果**:
- 测试文件: `orchestrator_test.go:545`
- **生产代码: 无**

---

### 断点 6: WorkerManager 从未被 Service 层调用

**文件**: `plugins/workermanager/workermanager.go:184-332`

WorkerManager 的 `Acquire` / `Execute` 方法都已实现，但在 main.go 创建后从未被使用。

**正确的流程应该是**:
1. Orchestrator.StartTask 调用 workerManager.Acquire 分配 worker
2. Orchestrator 调用 workerManager.Execute 执行任务
3. 但现在 Service 层根本不持有 workerManager 引用

---

### 断点 7: Eino 节点 LLM 未注入

**文件**: `plugins/orchestrator/nodes.go:82-101`

```go
type Dependencies struct {
    LLM          react.LLM      // ❌ nil
    ToolRegistry *react.ToolRegistry
    Logger       *zap.Logger
    // ...
}
```

`FullTaskGraph` (`plugins/orchestrator/full_graph.go:19-80`) 创建时传入的 `deps` 的 LLM 字段为 nil，导致:
- AnalyzerNode 无法进行复杂度判断
- MediumDecomposerNode 无法进行任务拆解
- 整个 Eino 图无法正常工作

---

### 断点 8: Git Push 无实现

在整个链路中未找到:
- Git push 节点或步骤
- Task 状态转为 Completed 的完整处理
- 最终结果写入或通知机制

---

## 四、修复优先级排序

### P0 — 核心链路断裂（阻塞主流程）

| 优先级 | 断点 | 修复方案 |
|--------|------|----------|
| P0-1 | main.go 创建 stub orchestrator | 改用 `internal/orchestrator.NewOrchestrator()` 并正确注入依赖 |
| P0-2 | TaskService 无 orchestrator 引用 | 将 orchestrator 注入 TaskService，Submit 成功后调用 `orch.StartTask()` |
| P0-3 | 无 Redis Stream → Orchestrator 路由 | 创建 EventConsumer 从 stream 消费并调用 `orch.Dispatch()` |
| P0-4 | handleTaskSubmitted 是空壳 | 实现: 解析 event → 调用 orchestrator.StartTask → 更新 task 状态 |

### P1 — 核心功能不可用

| 优先级 | 断点 | 修复方案 |
|--------|------|----------|
| P1-1 | Eino 节点 LLM 为 nil | 通过 LLMRouter 注入 LLM provider 到 Dependencies |
| P1-2 | WorkerManager 从未被调用 | 在 orchestrator 内调用 workerManager.Acquire/Execute |
| P1-3 | 状态机转换不完整 | 实现 handleTaskAssigned/Started 等处理函数 |

### P2 — 功能完善

| 优先级 | 断点 | 修复方案 |
|--------|------|----------|
| P2-1 | Git Push 无实现 | 在 TaskCompleted 后触发 Git Push 节点 |
| P2-2 | WebSocket Hub 重复消费 | 区分 consumer group，避免 WS 和 Orchestrator 争抢同一事件流 |

---

## 五、依赖关系图

```
当前状态（断裂点）:

Service.Submit
    ↓ [写入 Outbox]
Outbox Poller
    ↓ [XADD stream:domain_events]
Redis Stream
    ↓ [XReadGroup - ws consumer group]
WebSocket Hub
    ↓ [推送 WS 消息]
    ❌ 断点: Orchestrator 未收到事件

正确状态应该是:

Service.Submit
    ↓ [写入 Outbox]
Outbox Poller
    ↓ [XADD stream:domain_events]
Redis Stream
    ↓ [XReadGroup - orchestrator consumer group]
Orchestrator EventConsumer
    ↓ [Dispatch()]
Orchestrator.handleTaskSubmitted
    ↓ [StartTask()]
Eino Graph (Analyzer → Decomposer)
    ↓ [分解任务]
WorkerManager.Acquire/Execute
    ↓
Git Push
    ↓
Task Completed
```

---

## 六、关键文件索引

| 文件 | 用途 | 状态 |
|------|------|------|
| `cmd/server/main.go:320-326` | Orchestrator 初始化 | ❌ 创建 stub |
| `internal/service/task_service.go:40-48` | TaskService 结构 | ❌ 缺少 orchestrator 引用 |
| `internal/service/task_service.go:89-224` | Submit 方法 | ✅ 写入 Outbox 后结束 |
| `internal/orchestrator/orchestrator.go:96-131` | NewOrchestrator | ✅ 完整实现但从未调用 |
| `plugins/orchestrator/orchestrator.go:6-13` | Stub 实现 | ❌ 被 main.go 使用 |
| `internal/orchestrator/orchestrator.go:815-840` | 事件处理器 | ❌ 全是空壳 |
| `internal/orchestrator/orchestrator.go:793-795` | Dispatch 方法 | ❌ 从未被生产代码调用 |
| `internal/gateway/ws/hub.go:267-276` | Redis Stream 消费者 | ⚠️ 只转发 WS，不路由 Orchestrator |
| `plugins/orchestrator/nodes.go:82-101` | Dependencies | ❌ LLM 为 nil |
| `plugins/workermanager/workermanager.go:184-332` | Worker Acquire/Execute | ✅ 已实现但未调用 |
| `internal/infra/outbox/poller.go:50-98` | Outbox → Redis Stream | ✅ 已实现 |

---

## 七、结论

当前系统是一个**半完成的任务提交系统**:
- 用户可以提交任务 ✓
- 任务被持久化到数据库 ✓
- TaskSubmittedV1 事件写入 Outbox ✓
- 事件被转发到 Redis Stream ✓

**但是**:
- 没有任何消费者将事件路由到 Orchestrator ✗
- Orchestrator 即使收到事件也不做任何事 ✗
- WorkerManager 完全未被使用 ✗
- 任务永远不会开始执行 ✗

核心问题是: **Service 层和 Orchestrator 层完全割裂**，两者之间没有任何连接通道。
