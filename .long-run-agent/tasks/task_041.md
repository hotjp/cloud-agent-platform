# task_041

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_041.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/storage/task_repo.go`、`internal/storage/subtask_repo.go`、`internal/storage/audit_log_repo.go`

产出类型：
- `TaskRepository` struct — 实现 `service.TaskRepository` 接口
- `SubtaskRepository` struct — 实现 `service.SubtaskRepository` 接口
- `AuditLogRepository` struct — 实现 `service.AuditLogRepository` 接口

### 2. 契约参考

```go
// TaskRepository 接口（来自 T05c）
type TaskRepository interface {
    Create(ctx context.Context, task *Task) error
    GetByID(ctx context.Context, id string) (*Task, error)
    Update(ctx context.Context, task *Task) error
    List(ctx context.Context, filter TaskFilter) ([]*Task, error)
    UpdateStatus(ctx context.Context, id string, status TaskStatus, version int) error
}

// 具体实现需要调用 ent.Client
type taskRepo struct {
    client *ent.Client
}

func (r *taskRepo) Create(ctx context.Context, task *Task) error {
    return r.client.Task.Create().
        SetID(task.ID).
        SetGoal(task.Goal).
        // ... 其他字段
        Exec(ctx)
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `internal/service/interfaces.go` — T05c 定义的接口（T05c 完成前不可用）
- `ent/generated` — ent 生成的代码（T21a 完成前不可用）

**本层产出（其他 Task 会依赖的）**：
- `internal/storage/taskRepo` — Task Repository 实现
- `internal/storage/subtaskRepo` — Subtask Repository 实现
- `internal/storage/auditLogRepo` — AuditLog Repository 实现

### 4. 约定

- L1-Storage 层实现 L4-Service 层定义的接口（插件倒置）
- 所有 Repository 方法必须通过 ent client 操作数据库
- UpdateStatus 使用 version 参数进行乐观锁（ent 内置支持）
- List 必须支持分页（Page + PageSize）
- 错误码：L1_001 ~ L1_199

### 5. 验收标准

- 测试命令：`go test ./internal/storage/... -v -run TestRepository`
- 必须覆盖的 case：
  1. Task Create 和 GetByID
  2. Task List 分页
  3. Subtask Create 和 ListByTaskID
  4. UpdateStatus 乐观锁（version 不匹配时返回错误）
- Done 判定：测试全部通过 + `go build ./...` 无错误

## 描述

T21b: Repository实现 - TaskRepo/SubtaskRepo/AuditLogRepo接口实现，ent client封装

## 需求 (requirements)

T21b: Repository实现 - TaskRepo/SubtaskRepo/AuditLogRepo接口实现，ent client封装

## 验收标准 (acceptance)

- 所有 Repository 方法实现
- 乐观锁测试通过

## 交付物 (deliverables)

- internal/storage/task_repo.go — TaskRepository 实现
- internal/storage/subtask_repo.go — SubtaskRepository 实现
- internal/storage/audit_log_repo.go — AuditLogRepository 实现
- internal/storage/repository_test.go — 单元测试

## 设计方案 (design)

1. 创建 `internal/storage/task_repo.go`
2. 实现 TaskRepository 接口
3. 创建 `internal/storage/subtask_repo.go`
4. 实现 SubtaskRepository 接口
5. 创建 `internal/storage/audit_log_repo.go`
6. 实现 AuditLogRepository 接口
7. 编写乐观锁测试

## 验证证据（完成前必填）

- [ ] **实现证明**: 三个 Repository 实现完整
- [ ] **测试验证**: `go test ./internal/storage/...` 通过
- [ ] **影响范围**: T22（TaskService）依赖此实现

### 测试步骤
1. `go test ./internal/storage/... -v -run TestRepository`
2. `go build ./...`

### 验证结果
