# task_040

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_040.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`ent/schema/task.go`、`ent/schema/subtask.go`、`ent/schema/audit_log.go`

产出类型：
- `Task` ent.Schema — Task 表定义
- `Subtask` ent.Schema — Subtask 表定义
- `AuditLog` ent.Schema — AuditLog 表定义

### 2. 契约参考

```go
// Task Schema（来自 Cloud-Agent-Platform.md 4.1）
type Task struct {
    ID                   string   // ULID，max 64
    Goal                 string   // NotEmpty
    Status               string   // enum: pending/decomposing/dispatched/running/reviewing/confirming/completed/failed/cancelled
    Priority             int      // range: 0-9, default 5
    RepositoryURL        string   // NotEmpty
    BaseBranch           string   // NotEmpty
    ResultBranch         string
    Constraints          []string // JSON
    VerificationCriteria []string // JSON
    AgentHint            map[string]interface{} // JSON
    TokensUsed           int      // default 0
    EstimatedCost        float64  // default 0
    AgentsUsed           int      // default 0
    Progress             float64  // default 0
    ClientID             string   // NotEmpty
    Tags                 []string // JSON
    CreatedAt            time.Time
    StartedAt            *time.Time
    CompletedAt          *time.Time
}

// Subtask Schema
type Subtask struct {
    ID            string   // ULID
    TaskID        string   // FK -> Task
    Type          string   // enum: analysis/coding/review/testing/research
    Description   string   // Text
    AgentTemplate string   // NotEmpty
    AgentInstance *string
    Status        string   // 同 Task Status 枚举
    Summary       *string
    Artifacts     []map[string]interface{} // JSON
    TokensUsed    int
    Dependencies  []string // JSON
    StartedAt     *time.Time
    CompletedAt   *time.Time
}

// AuditLog Schema
type AuditLog struct {
    ID             string
    TaskID         string   // FK -> Task
    SubtaskID      *string
    AgentTemplate  *string
    Action         string   // NotEmpty
    Level          string   // enum: info/warning/error/critical
    Message        string   // Text
    Details        map[string]interface{} // JSON
    Timestamp      time.Time
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `entgo.io/ent` — ent ORM
- `entgo.io/ent/schema/field` — 字段定义
- `entgo.io/ent/schema/edge` — 边定义

**本层产出（其他 Task 会依赖的）**：
- `ent/schema/task.go` — Task Schema
- `ent/schema/subtask.go` — Subtask Schema
- `ent/schema/audit_log.go` — AuditLog Schema

### 4. 约定

- ent Schema 文件放在 `ent/schema/` 目录
- 使用 `field.String("id").MaxLen(64)` 定义 ULID 字段
- 枚举字段使用 `field.Enum().Values(...)`
- JSON 字段使用 `field.JSON`
- 必须定义索引：`status`, `client_id`, `created_at`
- Edge 定义：Task → Subtask (One-Many)，Task → AuditLog (One-Many)

### 5. 验收标准

- 测试命令：`go generate ./ent` 或 `ent generate ./ent`
- 必须覆盖的 case：
  1. `ent generate` 成功生成代码
  2. 生成的 `task.go`, `subtask.go`, `audit_log.go` 包含所有字段
  3. Edge 关系正确（Task.HasMany("subtasks"), Task.HasMany("audit_logs")）
- Done 判定：`ent generate` 成功 + `go build ./ent/...` 无错误

## 描述

T21a: ent Schema定义 - Task/Subtask/AuditLog三个Schema，字段/Edge/Index定义

## 需求 (requirements)

T21a: ent Schema定义 - Task/Subtask/AuditLog三个Schema，字段/Edge/Index定义

## 验收标准 (acceptance)

- ent generate 成功
- 所有字段和关系正确

## 交付物 (deliverables)

- ent/schema/task.go — Task Schema
- ent/schema/subtask.go — Subtask Schema
- ent/schema/audit_log.go — AuditLog Schema

## 设计方案 (design)

1. 创建 `ent/schema/` 目录
2. 按 Cloud-Agent-Platform.md 4.1 节定义三个 Schema
3. 运行 `ent generate ./ent`
4. 验证生成代码

## 验证证据（完成前必填）

- [ ] **实现证明**: 三个 Schema 定义完成
- [ ] **测试验证**: `ent generate` 成功
- [ ] **影响范围**: T21b（Repository 实现）依赖此 Schema

### 测试步骤
1. `ent generate ./ent`
2. `go build ./ent/...`
3. 检查生成的文件

### 验证结果
