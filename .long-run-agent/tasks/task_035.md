# task_035

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_035.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/domain/events.go`

产出类型：
- `DomainEvent` struct — 领域事件结构体
- `OutboxWriter` interface — 事务内写入接口
- `EventSerializer` interface — 事件序列化接口

### 2. 契约参考

```go
// DomainEvent — 领域事件（entgo.io/ent schema 版本）
type DomainEvent struct {
    EventID       string    // ULID，全局唯一
    AggregateType string    // 聚合类型，如 "Task", "Subtask"
    AggregateID   string    // 聚合根ID
    EventType     string    // 格式：{Aggregate}{Action}V{Version}，如 TaskCreatedV1
    Payload       []byte    // JSON序列化的事件数据
    OccurredAt    time.Time // 事件发生时间（UTC）
    IdempotencyKey string   // 格式：{aggregate_id}:{event_type}:{version}
    Version       int       // 聚合根版本号（乐观锁）
}

// OutboxWriter — 事务内写入Outbox的接口
type OutboxWriter interface {
    Write(ctx context.Context, tx *ent.Tx, event *DomainEvent) error
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `github.com/oklog/ulid/v2` — ULID 生成

**本层产出（其他 Task 会依赖的）**：
- `internal/domain.DomainEvent` — 领域事件结构体
- `internal/domain.OutboxWriter` — Outbox写入接口

### 4. 约定

- L2-Domain 零外部依赖，禁止 import 第三方包（除 ULID）
- 事件类型命名：`{AggregateType}{Action}V{Version}`，如 `TaskCreatedV1`、`SubtaskCompletedV2`
- Payload 必须使用 `encoding/json` 序列化
- 每个事件必须包含 IdempotencyKey，格式：`{aggregate_id}:{event_type}:{version}`
- Version 字段用于乐观锁，每次状态转换后 +1

### 5. 验收标准

- 测试命令：`go test ./internal/domain/... -v -run TestDomainEvent`
- 必须覆盖的 case：
  1. DomainEvent 创建，EventID 必须为有效 ULID
  2. EventType 格式校验，非法格式返回错误
  3. Payload JSON 序列化/反序列化正确
  4. IdempotencyKey 格式校验
- Done 判定：上述测试全部通过 + `go vet ./internal/domain/...` 无错误

## 描述

T05a: 领域事件协议 - DomainEvent结构体定义，OutboxWriter接口，事件序列化

## 需求 (requirements)

T05a: 领域事件协议 - DomainEvent结构体定义，OutboxWriter接口，事件序列化

## 验收标准 (acceptance)

- DomainEvent 创建并序列化测试通过
- ULID 生成唯一性测试通过
- go vet 无错误

## 交付物 (deliverables)

- internal/domain/events.go — DomainEvent 结构体 + OutboxWriter 接口
- internal/domain/events_test.go — 单元测试

## 设计方案 (design)

1. 在 `internal/domain/` 下创建 `events.go`
2. 定义 DomainEvent struct，包含所有必需字段
3. 定义 OutboxWriter interface（由 L1-Storage 实现）
4. 实现 EventSerializer 进行 JSON 序列化
5. 在 `events_test.go` 中编写单元测试

## 验证证据（完成前必填）

- [ ] **实现证明**: DomainEvent struct 和 OutboxWriter 接口已定义
- [ ] **测试验证**: `go test ./internal/domain/...` 通过
- [ ] **影响范围**: T05b（状态机）和 T13（Outbox）依赖此接口

### 测试步骤
1. `go test ./internal/domain/... -v -run TestDomainEvent`
2. `go vet ./internal/domain/...`
3. 检查 events.go 中所有导出类型

### 验证结果
<!-- 粘贴验证截图、命令输出或测试结果 -->
