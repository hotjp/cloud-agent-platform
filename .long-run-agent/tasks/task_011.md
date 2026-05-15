# task_011

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_011.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/domain/task.go`、`internal/domain/subtask.go`

产出类型：
- `Task` struct — Task 领域实体
- `Subtask` struct — Subtask 领域实体
- `TaskStateMachine` — Task 状态机实例

### 2. 契约参考

```go
// Task（来自 Cloud-Agent-Platform.md 3.2）
type Task struct {
    ID                   string
    Goal                 string
    Status               TaskStatus
    Priority             int
    RepositoryURL        string
    BaseBranch           string
    ResultBranch         string
    Constraints          []string
    VerificationCriteria []string
    AgentHint            *AgentHint
    Progress             float64
    TokensUsed           int
    EstimatedCost        float64
    AgentsUsed           int
    ClientID             string
    Tags                 []string
    CreatedAt            time.Time
    StartedAt            *time.Time
    CompletedAt          *time.Time
}

// 9态状态机（来自 Cloud-Agent-Platform.md 3.2）
// pending → decomposing → dispatched → running → reviewing → completed
// confirming → running（用户批准）
// 任何状态 → failed/cancelled
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `internal/domain/state_machine.go` — T05b 定义
- `internal/domainevents.DomainEvent` — T05a 定义

**本层产出（其他 Task 会依赖的）**：
- `internal/domain.Task` — Task 领域实体
- `internal/domain.Subtask` — Subtask 领域实体

### 4. 约定

- L2-Domain 零外部依赖
- 状态机使用 T05b 框架
- 实体方法验证业务不变量
- 错误码：L2_200 ~ L2_399

### 5. 验收标准

- 测试命令：`go test ./internal/domain/... -v -run TestTask`
- 必须覆盖的 case：
  1. Task 创建和状态转换
  2. Subtask 创建和状态转换
  3. 非法状态转换返回错误
- Done 判定：测试通过 + `go vet` 无错误

## 描述

T20: Task领域模型 - Task+Subtask领域实体，9态状态机声明式定义



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T20:_Task领域模型_-_Task.py




## 设计方案 (design)

快速任务


## 验证证据（完成前必填）

<!-- 标记完成前，请提供以下证据： -->

- [ ] **实现证明**: 简要说明如何实现
- [ ] **测试验证**: 如何验证功能正常（测试步骤/截图/命令输出）
- [ ] **影响范围**: 是否影响其他功能

### 测试步骤
1. 
2. 
3. 

### 验证结果
<!-- 粘贴验证截图、命令输出或测试结果 -->