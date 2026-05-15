# task_050

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_050.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/orchestrator/nodes.go`

产出类型：
- `AnalyzerNode` struct — 任务分析节点
- `RouterNode` struct — 路由决策节点
- `ExecutorNode` struct — 执行器节点
- `GuardianNode` struct — 审查节点
- `ResultMerger` struct — 结果合并节点
- `MediumDecomposer` struct — 中等任务分解节点
- `ParallelExecutors` struct — 并行执行器节点

### 2. 契约参考

```go
// AnalyzerNode — 分析任务复杂度
type AnalyzerNode struct {
    llm LLMCaller
}

func (n *AnalyzerNode) Invoke(ctx context.Context, input *TaskInput) (*AnalysisOutput, error) {
    // 调用 LLM 分析任务复杂度
    // 返回：{ complexity: "simple"|"medium"|"complex", reason: string }
}

// RouterNode — 路由决策
type RouterNode struct{}

func (n *RouterNode) Invoke(ctx context.Context, input *AnalysisOutput) (string, error) {
    return input.Complexity, nil
}

// ExecutorNode — Agent 执行节点
type ExecutorNode struct {
    template string
    agent    *ReActAgent
}

func (n *ExecutorNode) Invoke(ctx context.Context, input *TaskInput) (*AgentOutput, error) {
    return n.agent.Run(ctx, &AgentInput{
        Goal:      input.Goal,
        Workspace: input.Workspace,
        Tools:     getToolsForTemplate(n.template),
        MaxSteps:  15,
    })
}

// GuardianNode — 审查节点（触发人工确认）
type GuardianNode struct{}

func (n *GuardianNode) Invoke(ctx context.Context, input *GuardianInput) (*GuardianOutput, error) {
    // 检查高风险操作
    // 返回 { needsConfirmation: bool, reason: string }
}

// ResultMerger — 结果合并
type ResultMerger struct{}

func (m *ResultMerger) Invoke(ctx context.Context, inputs ...*TaskOutput) (*TaskResult, error) {
    // 合并多个输出为最终结果
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `plugins/orchestrator.ReActAgent` — T31a 定义
- `plugins/llmrouter.LLMCaller` — T61 定义

**本层产出（其他 Task 会依赖的）**：
- `plugins/orchestrator.AnalyzerNode` — 分析节点
- `plugins/orchestrator.ExecutorNode` — 执行节点
- `plugins/orchestrator.ResultMerger` — 合并节点

### 4. 约定

- 所有节点实现 Eino Node 接口
- AnalyzerNode 调用 LLM 分析复杂度
- GuardianNode 检测高风险操作并触发 confirming 状态
- ResultMerger 合并简单/中等/复杂三条路径的结果

### 5. 验收标准

- 测试命令：`go test ./plugins/orchestrator/... -v -run TestNodes`
- 必须覆盖的 case：
  1. AnalyzerNode 正确分析复杂度
  2. RouterNode 正确路由
  3. ExecutorNode 调用 ReActAgent
  4. ResultMerger 正确合并多路径结果
- Done 判定：测试全部通过 + `go build ./plugins/orchestrator/...`

## 描述

T41b: Eino节点定义 - AnalyzerNode/RouterNode/ExecutorNode/GuardianNode/ResultMerger节点实现

## 需求 (requirements)

T41b: Eino节点定义 - AnalyzerNode/RouterNode/ExecutorNode/GuardianNode/ResultMerger节点实现

## 验收标准 (acceptance)

- 所有节点实现正确
- 节点间数据传递正确

## 交付物 (deliverables)

- plugins/orchestrator/nodes.go — 所有节点定义
- plugins/orchestrator/nodes_test.go — 测试

## 设计方案 (design)

1. 实现 AnalyzerNode
2. 实现 RouterNode
3. 实现 ExecutorNode
4. 实现 GuardianNode
5. 实现 ResultMerger

## 验证证据（完成前必填）

- [ ] **实现证明**: 所有节点定义完成
- [ ] **测试验证**: `go test ./plugins/orchestrator/...` 通过
- [ ] **影响范围**: T41a 依赖这些节点

### 测试步骤
1. `go test ./plugins/orchestrator/... -v -run TestNodes`
2. `go build ./plugins/orchestrator/...`

### 验证结果
