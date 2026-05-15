# task_023

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_023.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/domain/context.go`

产出类型：
- `TaskContext` — 任务上下文
- `FileState` — 文件状态
- `ConversationTurn` — 对话轮次

### 2. 契约参考

```go
type TaskContext struct {
    TaskID    string
    Goal      string
    Constraints []string
    FileStates []FileState
    Turns     []ConversationTurn
    TokenUsed int
    Budget    int // 默认 50K
}

type FileState struct {
    Path      string
    Content   string
    Checksum  string
    Summary   string
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- 无上游依赖

**本层产出（其他 Task 会依赖的）**：
- `internal/domain.TaskContext` — 上下文模型

### 4. 约定

- L2-Domain 零外部依赖
- Token 预算默认 50K

### 5. 验收标准

- 测试命令：`go test ./internal/domain/... -v -run TestContext`
- 必须覆盖的 case：
  1. TaskContext 创建正确
  2. Token 计数正确
- Done 判定：测试通过 + `go vet`

## 描述

T44: 上下文领域模型 - TaskContext/FileState/ConversationTurn领域实体



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T44:_上下文领域模型_-_TaskC.py




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