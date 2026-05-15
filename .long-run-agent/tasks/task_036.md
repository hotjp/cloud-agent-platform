# task_036

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_036.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/domain/state_machine.go`、`internal/domain/task_status.go`

产出类型：
- `TaskStatus` enum — 任务状态枚举（9态）
- `StateMachine` struct — 状态机声明式框架
- `Transition` struct — 转换规则定义

### 2. 契约参考

```go
// TaskStatus — 任务状态枚举
type TaskStatus string
const (
    StatusPending     TaskStatus = "pending"
    StatusDecomposing TaskStatus = "decomposing"
    StatusDispatched  TaskStatus = "dispatched"
    StatusRunning     TaskStatus = "running"
    StatusReviewing   TaskStatus = "reviewing"
    StatusConfirming  TaskStatus = "confirming"
    StatusCompleted   TaskStatus = "completed"
    StatusFailed      TaskStatus = "failed"
    StatusCancelled   TaskStatus = "cancelled"
)

// StateMachine — 声明式状态机
type StateMachine struct {
    Initial     TaskStatus
    States      []TaskStatus
    Transitions []Transition
    VersionField string
}
```

状态转换规则：
```
pending ──[开始拆解]──▶ decomposing
decomposing ──[拆解完成]──▶ dispatched
dispatched ──[Agent开始执行]──▶ running
running ──[Agent完成]──▶ reviewing
reviewing ──[审查通过]──▶ completed
confirming ──[用户批准]──▶ running
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `internal/domainevents.DomainEvent` — T05a 定义

**本层产出（其他 Task 会依赖的）**：
- `internal/domain.StateMachine` — 状态机框架
- `internal/domain.TaskStatus` — 状态枚举
- `internal/domain.NewTaskStateMachine()` — 工厂函数

### 4. 约定

- L2-Domain 零外部依赖，纯 Go + 标准库
- 每次成功转换必须 `IncrementVersion()`（乐观锁）
- 非法转换返回 `L2_201` 错误码
- 终态（completed/failed/cancelled）不可再转换

### 5. 验收标准

- 测试命令：`go test ./internal/domain/... -v -run TestStateMachine`
- 必须覆盖的 case：
  1. pending → decomposing 合法转换
  2. pending → failed 非法转换（应返回错误）
  3. completed 再次转换（应返回错误）
  4. Version 每次转换后 +1
- Done 判定：测试全部通过 + `go vet` 无错误

## 描述

T05b: 状态机框架 - 声明式状态机定义(states/transitions/guards/actions)，每次转换自动increment_version

## 需求 (requirements)

T05b: 状态机框架 - 声明式状态机定义(states/transitions/guards/actions)，每次转换自动increment_version

## 验收标准 (acceptance)

- 9态状态机声明正确
- 所有合法/非法转换测试通过
- Version 乐观锁测试通过

## 交付物 (deliverables)

- internal/domain/state_machine.go — 状态机框架
- internal/domain/task_status.go — TaskStatus 枚举
- internal/domain/state_machine_test.go — 单元测试

## 设计方案 (design)

1. 创建 `state_machine.go` 和 `task_status.go`
2. 定义 TaskStatus 枚举（9个状态）
3. 实现 StateMachine 框架
4. 实现 CanTransition/Transition 方法
5. 编写转换规则测试

## 验证证据（完成前必填）

- [ ] **实现证明**: StateMachine 框架实现，9态状态机声明
- [ ] **测试验证**: `go test ./internal/domain/...` 通过
- [ ] **影响范围**: T20 和所有 Service 层任务依赖此框架

### 测试步骤
1. `go test ./internal/domain/... -v -run TestStateMachine`
2. `go vet ./internal/domain/...`

### 验证结果
