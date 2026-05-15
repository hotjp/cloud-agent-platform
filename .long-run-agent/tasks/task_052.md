# task_052

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_052.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/orchestrator/agent_observer.go`、`plugins/orchestrator/agent_strategist.go`、`plugins/orchestrator/agent_executor.go`

产出类型：
- `ObserverAgent` struct — 观察者 Agent
- `StrategistAgent` struct — 策略师 Agent
- `ExecutorAgent` struct — 执行者 Agent

### 2. 契约参考

```go
// Observer — 分析观察者（来自 Cloud-Agent-Platform.md 3.3）
// 职责：分析代码结构、识别依赖、评估影响
// 默认模型：Claude Sonnet
// 能力：分析5、研究4
type ObserverAgent struct {
    base *BaseAgent
}

func (a *ObserverAgent) Run(ctx context.Context, input *AgentInput) (*AgentOutput, error) {
    prompt := a.renderPrompt("observer", input)
    return a.llm.Call(ctx, prompt, a.tools)
}

// Strategist — 策略规划师
// 职责：制定修改策略、设计实现方案
// 默认模型：Claude Sonnet
// 能力：分析5、编码4
type StrategistAgent struct {
    base *BaseAgent
}

// Executor — 代码执行者
// 职责：执行具体代码编写和修改
// 默认模型：Claude Sonnet
// 能力：编码5
type ExecutorAgent struct {
    base *BaseAgent
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `plugins/orchestrator.ReActAgent` — T31a 定义
- `plugins/orchestrator.AgentTemplate` — T42a 定义

**本层产出（其他 Task 会依赖的）**：
- `plugins/orchestrator.ObserverAgent` — Observer 实现
- `plugins/orchestrator.StrategistAgent` — Strategist 实现
- `plugins/orchestrator.ExecutorAgent` — Executor 实现

### 4. 约定

- 每个 Agent 组合 ReActAgent + 特定 prompt 模板
- 输入上下文：Observer 接收完整 goal，Strategist 接收 Observer 的分析结果，Executor 接收策略
- 输出格式：{ summary: string, artifacts: [], suggestions: [] }
- 错误处理：返回 failed 状态

### 5. 验收标准

- 测试命令：`go test ./plugins/orchestrator/... -v -run TestObserverAgent`
- 必须覆盖的 case：
  1. Observer 分析代码结构
  2. Strategist 制定策略
  3. Executor 执行代码修改
- Done 判定：测试全部通过 + `go build ./plugins/orchestrator/...`

## 描述

T42b: 前3个Agent实现 - observer/strategist/executor角色完整实现

## 需求 (requirements)

T42b: 前3个Agent实现 - observer/strategist/executor角色完整实现

## 验收标准 (acceptance)

- 3个Agent实现正确
- Prompt模板正确

## 交付物 (deliverables)

- plugins/orchestrator/agent_observer.go — Observer Agent
- plugins/orchestrator/agent_strategist.go — Strategist Agent
- plugins/orchestrator/agent_executor.go — Executor Agent

## 设计方案 (design)

1. 实现 ObserverAgent
2. 实现 StrategistAgent
3. 实现 ExecutorAgent
4. 编写 prompt 模板

## 验证证据（完成前必填）

- [ ] **实现证明**: 3个Agent实现完成
- [ ] **测试验证**: `go test ./plugins/orchestrator/...` 通过
- [ ] **影响范围**: 复杂任务编排依赖

### 测试步骤
1. `go test ./plugins/orchestrator/... -v -run TestObserverAgent`
2. `go build ./plugins/orchestrator/...`

### 验证结果
