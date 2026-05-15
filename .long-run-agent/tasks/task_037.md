# task_037

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_037.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/service/interfaces.go`

产出类型：
- `TaskRepository` interface — Task 仓储接口
- `SubtaskRepository` interface — Subtask 仓储接口
- `AuditLogRepository` interface — 审计日志仓储接口
- `TaskFilter` struct — 列表过滤条件

### 2. 契约参考

```go
type TaskRepository interface {
    Create(ctx context.Context, task *Task) error
    GetByID(ctx context.Context, id string) (*Task, error)
    Update(ctx context.Context, task *Task) error
    List(ctx context.Context, filter TaskFilter) ([]*Task, error)
    UpdateStatus(ctx context.Context, id string, status TaskStatus, version int) error
}

type SubtaskRepository interface {
    Create(ctx context.Context, subtask *Subtask) error
    GetByID(ctx context.Context, id string) (*Subtask, error)
    ListByTaskID(ctx context.Context, taskID string) ([]*Subtask, error)
    Update(ctx context.Context, subtask *Subtask) error
    UpdateStatus(ctx context.Context, id string, status TaskStatus, version int) error
}

type AuditLogRepository interface {
    Create(ctx context.Context, log *AuditLog) error
    ListByTaskID(ctx context.Context, taskID string) ([]*AuditLog, error)
}

type TaskFilter struct {
    Status   *TaskStatus
    ClientID *string
    Page     int
    PageSize int
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `internal/domainevents.DomainEvent` — T05a 定义
- `internal/domain.TaskStatus` — T05b 定义

**本层产出（其他 Task 会依赖的）**：
- `internal/service.TaskRepository` — Task 仓储接口
- `internal/service.SubtaskRepository` — Subtask 仓储接口
- `internal/service.AuditLogRepository` — 审计日志仓储接口

### 4. 约定

- 接口定义在 L4-Service 层，实现在 L1-Storage 层（插件倒置）
- 所有接口方法必须返回 error
- Update 方法必须包含 version 参数（乐观锁）
- List 方法必须支持分页
- 错误码范围：L4_Service [600, 799]

### 5. 验收标准

- 测试命令：`go build ./internal/service/...`
- 必须覆盖的 case：
  1. 接口方法签名完整
  2. 所有方法包含 context.Context
  3. 所有方法返回 error
- Done 判定：`go build` 无编译错误

## 描述

T05c: 仓库接口定义 - TaskRepository/SubtaskRepository/AuditLogRepository接口，所有接口方法签名

## 需求 (requirements)

T05c: 仓库接口定义 - TaskRepository/SubtaskRepository/AuditLogRepository接口，所有接口方法签名

## 验收标准 (acceptance)

- 所有接口定义完整
- go build 无编译错误

## 交付物 (deliverables)

- internal/service/interfaces.go — 所有仓储接口定义

## 设计方案 (design)

1. 在 `internal/service/` 下创建 `interfaces.go`
2. 定义三个 Repository 接口
3. 定义 TaskFilter 结构体

## 验证证据（完成前必填）

- [ ] **实现证明**: 所有仓储接口已定义
- [ ] **测试验证**: `go build ./internal/service/...` 无错误
- [ ] **影响范围**: T21 和 T22 依赖此接口

### 测试步骤
1. `go build ./internal/service/...`
2. 检查 interfaces.go 中所有接口方法签名

### 验证结果
