# task_058

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_058.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/mcpserver/tools.go`

产出类型：
- `TaskSubmitTool` — 任务提交
- `TaskStatusTool` — 任务状态
- `TaskListTool` — 任务列表
- `TaskCancelTool` — 任务取消
- `TaskDecideTool` — 任务决策
- `TaskDiffTool` — 任务diff
- `TaskWaitTool` — 任务等待
- `AgentTemplatesTool` — Agent模板
- `PlatformStatusTool` — 平台状态

### 2. 契约参考

```go
// 9个 MCP Tools（来自 Cloud-Agent-Platform.md 5.3）
type TaskSubmitTool struct {
    service *TaskServiceHandler
}

type TaskStatusTool struct {
    service *TaskServiceHandler
}

// 映射到 Protobuf RPC 方法
// task_submit → SubmitTask
// task_status → GetTask
// task_list → ListTasks
// task_cancel → CancelTask
// task_decide → DecideTask
// task_diff → GetDiff
// task_wait → GetTask（轮询直到完成）
// agent_templates → ListAgentTemplates
// platform_status → GetPlatformStatus
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `plugins/mcpserver.MCPServer` — T49a 定义
- `internal/service.TaskServiceHandler` — T22 定义

**本层产出（其他 Task 会依赖的）**：
- `plugins/mcpserver.TaskSubmitTool`
- `plugins/mcpserver.TaskStatusTool`
- 等 9 个 Tool

### 4. 约定

- 每个 Tool 对应一个 Protobuf RPC 方法
- InputSchema 必须与 Protobuf 定义一致
- task_wait 工具需要轮询实现
- 所有错误需要格式化为 MCP 错误响应

### 5. 验收标准

- 测试命令：`go test ./plugins/mcpserver/... -v -run TestTools`
- 必须覆盖的 case：
  1. 9 个 Tool 全部实现
  2. InputSchema 与 Protobuf 一致
  3. task_wait 轮询逻辑正确
- Done 判定：测试全部通过 + `go build ./plugins/mcpserver/...`

## 描述

T49b: MCP Tools实现 - 9个Tool(task_submit/status/list/cancel/decide/diff/wait/templates/status)完整实现

## 需求 (requirements)

T49b: MCP Tools实现 - 9个Tool(task_submit/status/list/cancel/decide/diff/wait/templates/status)完整实现

## 验收标准 (acceptance)

- 9个Tool全部实现
- InputSchema正确

## 交付物 (deliverables)

- plugins/mcpserver/tools.go — 9个Tool实现

## 设计方案 (design)

1. 实现 TaskSubmitTool
2. 实现 TaskStatusTool
3. 实现 TaskListTool
4. 实现 TaskCancelTool
5. 实现 TaskDecideTool
6. 实现 TaskDiffTool
7. 实现 TaskWaitTool（轮询）
8. 实现 AgentTemplatesTool
9. 实现 PlatformStatusTool

## 验证证据（完成前必填）

- [ ] **实现证明**: 9个Tool全部实现
- [ ] **测试验证**: `go test ./plugins/mcpserver/...` 通过
- [ ] **影响范围**: MCP 协议调用

### 测试步骤
1. `go test ./plugins/mcpserver/... -v -run TestTools`
2. `go build ./plugins/mcpserver/...`

### 验证结果
