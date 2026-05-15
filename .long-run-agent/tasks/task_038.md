# task_038

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_038.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/storage/outbox.go`

产出类型：
- `OutboxWriter` struct — 事务内 Outbox 写入器
- `NewOutboxWriter()` — 工厂函数

### 2. 契约参考

```go
// OutboxWriter — 事务内写入Outbox
type OutboxWriter struct {
    db *sql.DB
}

func (w *OutboxWriter) Write(ctx context.Context, tx *sql.Tx, event *DomainEvent) error {
    // 同库同事务写入 outbox_events 表
    query := `INSERT INTO outbox_events
        (id, aggregate_type, aggregate_id, event_type, payload, occurred_at, idempotency_key, version, status, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())`

    _, err := tx.ExecContext(ctx, query,
        event.EventID, event.AggregateType, event.AggregateID,
        event.EventType, event.Payload, event.OccurredAt,
        event.IdempotencyKey, event.Version, "pending")
    return err
}
```

outbox_events 表结构（来自 T11 迁移）：
```sql
CREATE TABLE outbox_events (
    id VARCHAR(64) PRIMARY KEY,
    aggregate_type VARCHAR(64) NOT NULL,
    aggregate_id VARCHAR(64) NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    payload JSONB NOT NULL,
    occurred_at TIMESTAMP NOT NULL,
    idempotency_key VARCHAR(256) NOT NULL UNIQUE,
    version INT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    retry_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `internal/domainevents.DomainEvent` — T05a 定义
- `internal/service/interfaces.go` — T05c 定义（T05c 完成前不可用）

**本层产出（其他 Task 会依赖的）**：
- `internal/storage.OutboxWriter` — Outbox 写入器实现

### 4. 约定

- L1-Storage 层使用 `pgx/v5` 和 `ent`
- OutboxWriter 必须与业务操作在同一事务内提交
- 写入时 status='pending'，retry_count=0
- 错误码：L1_010（Outbox写入失败）

### 5. 验收标准

- 测试命令：`go test ./internal/storage/... -v -run TestOutboxWriter`
- 必须覆盖的 case：
  1. 事务内写入 outbox_events 成功
  2. 事务回滚时 outbox 记录不残留
  3. 唯一约束冲突（idempotency_key 重复）返回错误
- Done 判定：测试全部通过 + `go vet` 无错误

## 描述

T13a: Outbox写入器 - 事务内OutboxWriter接口，同库同事务写入outbox_events表

## 需求 (requirements)

T13a: Outbox写入器 - 事务内OutboxWriter接口，同库同事务写入outbox_events表

## 验收标准 (acceptance)

- 事务内写入成功
- 事务回滚时无残留
- 唯一约束测试通过

## 交付物 (deliverables)

- internal/storage/outbox.go — OutboxWriter 实现
- internal/storage/outbox_test.go — 单元测试

## 设计方案 (design)

1. 创建 `internal/storage/outbox.go`
2. 实现 OutboxWriter 结构体和 Write 方法
3. 在 ent transaction 内调用 Write
4. 编写事务测试

## 验证证据（完成前必填）

- [ ] **实现证明**: OutboxWriter 在事务内写入 outbox_events
- [ ] **测试验证**: `go test ./internal/storage/...` 通过
- [ ] **影响范围**: T13b 依赖此写入器

### 测试步骤
1. `go test ./internal/storage/... -v -run TestOutboxWriter`
2. `go vet ./internal/storage/...`

### 验证结果
