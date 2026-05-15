# task_044

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_044.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/orchestrator/agent.go`

产出类型：
- `ReActAgent` struct — ReAct 循环执行器
- `AgentInput` struct — Agent 输入
- `AgentOutput` struct — Agent 输出
- `Step` struct — 一步执行记录

### 2. 契约参考

```go
// ReAct 循环（来自 Cloud-Agent-Platform.md 3.4）
// 1. 思考（Thought）：LLM 分析当前状态，决定下一步
// 2. 行动（Action）：调用工具或输出结果
// 3. 观察（Observation）：获取工具返回值
// 重复 2-4，直到：
//   - 目标完成 → 返回结果
//   - 达到最大步数（默认15步）→ 返回部分结果
//   - 无法恢复的错误 → 返回错误

type AgentInput struct {
    Goal      string
    Workspace string
    Tools     []Tool
    MaxSteps  int // 默认15
}

type AgentOutput struct {
    Steps     []Step
    FinalOutput string
    TokensUsed int
    Error     error
}

type Step struct {
    Thought     string
    Action      string
    ActionInput map[string]interface{}
    Observation string
}

// Tool 接口（来自 T05/T47）
type Tool interface {
    Info() ToolInfo
    Run(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `plugins/llmrouter.LLMCaller` — LLM 调用接口（T61）
- `plugins/tools.Tool` — 工具接口（T47）

**本层产出（其他 Task 会依赖的）**：
- `plugins/orchestrator.ReActAgent` — ReAct Agent 实现

### 4. 约定

- 使用 Eino 的 LLMCaller 调用 LLM
- Prompt 模板：`plugins/orchestrator/prompts/react.go`
- 工具调用结果作为 Observation 反馈给 LLM
- 最大步数限制防止无限循环
- Token 消耗需要累计上报

### 5. 验收标准

- 测试命令：`go test ./plugins/orchestrator/... -v -run TestReActAgent`
- 必须覆盖的 case：
  1. ReAct 循环正常执行（至少 3 步）
  2. 达到最大步数时正确停止
  3. 工具调用返回 Observation
  4. LLM 调用失败时正确处理
- Done 判定：测试全部通过 + `go build ./plugins/orchestrator/...`

## 描述

T31a: ReAct循环框架 - Thought→Action→Observation循环，最多15步，LLM调用封装

## 需求 (requirements)

T31a: ReAct循环框架 - Thought→Action→Observation循环，最多15步，LLM调用封装

## 验收标准 (acceptance)

- ReAct 循环正确执行
- 最大步数限制生效
- 错误处理正确

## 交付物 (deliverables)

- plugins/orchestrator/agent.go — ReActAgent
- plugins/orchestrator/prompts/react.go — Prompt 模板
- plugins/orchestrator/agent_test.go — 测试

## 设计方案 (design)

1. 创建 `plugins/orchestrator/agent.go`
2. 实现 ReActAgent 结构体
3. 实现 Run 方法（循环逻辑）
4. 实现 step 方法（一步执行）

## 验证证据（完成前必填）

- [ ] **实现证明**: ReAct 循环完整实现
- [ ] **测试验证**: `go test ./plugins/orchestrator/...` 通过
- [ ] **影响范围**: 所有 Agent 执行依赖此框架

### 测试步骤
1. `go test ./plugins/orchestrator/... -v -run TestReActAgent`
2. `go build ./plugins/orchestrator/...`

### 验证结果
