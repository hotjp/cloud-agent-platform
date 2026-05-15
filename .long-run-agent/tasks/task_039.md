# task_039

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_039.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/storage/outbox_poller.go`

产出类型：
- `OutboxPoller` struct — 后台轮询转发器
- `Start()` — 启动轮询（后台 goroutine）
- `Stop()` — 停止轮询

### 2. 契约参考

```go
type OutboxPoller struct {
    db       *sql.DB
    redis    *redis.Client
    interval time.Duration
    batchSize int
    stopCh   chan struct{}
}

// 轮询逻辑：
// 1. SELECT id, aggregate_type, aggregate_id, event_type, payload FROM outbox_events
//    WHERE status='pending' ORDER BY created_at ASC LIMIT 100
//    FOR UPDATE SKIP LOCKED
// 2. 发布到 Redis Stream: XADD cap:events * <event_fields>
// 3. UPDATE outbox_events SET status='processed' WHERE id=?
// 4. 失败：UPDATE outbox_events SET status='failed', retry_count=retry_count+1 WHERE id=?
```

Redis Stream 发布格式：
```
XADD cap:events * event_id <id> aggregate_type <type> aggregate_id <id> event_type <type> payload <json>
```

指数退避重试：
- retry_count < 3：立即重试
- retry_count >= 3：sleep = min(2^retry_count * 100ms, 30s)

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `internal/storage.OutboxWriter` — T13a 定义
- `github.com/redis/go-redis/v9` — Redis 客户端

**本层产出（其他 Task 会依赖的）**：
- `internal/storage.OutboxPoller` — 轮询转发器

### 4. 约定

- 后台 goroutine 必须在 `Stop()` 时优雅退出
- `FOR UPDATE SKIP LOCKED` 防止多实例重复消费
- 错误日志必须包含 event_id 和错误详情
- 进程退出时必须调用 `Stop()` 防止 goroutine 泄漏

### 5. 验收标准

- 测试命令：`go test ./internal/storage/... -v -run TestOutboxPoller`
- 必须覆盖的 case：
  1. 启动后定时轮询
  2. `Stop()` 信号导致优雅退出
  3. 多实例 `FOR UPDATE SKIP LOCKED` 不冲突
  4. 失败事件指数退避重试
- Done 判定：测试全部通过 + `go vet` 无错误

## 描述

T13b: Outbox轮询转发器 - 后台goroutine轮询pending事件，SELECT FOR UPDATE SKIP LOCKED，指数退避重试

## 需求 (requirements)

T13b: Outbox轮询转发器 - 后台goroutine轮询pending事件，SELECT FOR UPDATE SKIP LOCKED，指数退避重试

## 验收标准 (acceptance)

- 定时轮询正常
- 优雅退出无泄漏
- SKIP LOCKED 测试通过

## 交付物 (deliverables)

- internal/storage/outbox_poller.go — 轮询转发器
- internal/storage/outbox_poller_test.go — 单元测试

## 设计方案 (design)

1. 创建 `internal/storage/outbox_poller.go`
2. 实现 OutboxPoller 结构体
3. 实现轮询循环（FOR UPDATE SKIP LOCKED）
4. 实现 Redis Stream 发布
5. 实现指数退避重试
6. 实现优雅退出

## 验证证据（完成前必填）

- [ ] **实现证明**: 轮询器启动并正确处理事件
- [ ] **测试验证**: `go test ./internal/storage/...` 通过
- [ ] **影响范围**: 整个事件驱动架构依赖此组件

### 测试步骤
1. `go test ./internal/storage/... -v -run TestOutboxPoller`
2. `go vet ./internal/storage/...`

### 验证结果
