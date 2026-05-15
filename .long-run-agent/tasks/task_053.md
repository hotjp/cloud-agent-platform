# task_053

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_053.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/orchestrator/agent_guardian.go`、`plugins/orchestrator/agent_tester.go`、`plugins/orchestrator/agent_researcher.go`

产出类型：
- `GuardianAgent` struct — 安全审查 Agent
- `TesterAgent` struct — 测试工程师 Agent
- `ResearcherAgent` struct — 深度研究员 Agent

### 2. 契约参考

```go
// Guardian — 安全审查员（来自 Cloud-Agent-Platform.md 3.3）
// 职责：审查安全性、检查约束条件
// 默认模型：GLM-5.1（性价比高）
// 能力：审查5
// 触发条件：
//   - 修改安全相关代码（认证/授权/加密）
//   - 删除超过50行代码
//   - 修改配置文件
//   - 添加外部依赖
type GuardianAgent struct {
    base *BaseAgent
}

// Tester — 测试工程师
// 职责：编写和执行测试
// 默认模型：GLM-5.1
// 能力：测试5
type TesterAgent struct {
    base *BaseAgent
}

// Researcher — 深度研究员
// 职责：技术研究、调研最佳实践
// 默认模型：Claude Sonnet
// 能力：研究5
type ResearcherAgent struct {
    base *BaseAgent
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `plugins/orchestrator.ReActAgent` — T31a 定义
- `plugins/orchestrator.AgentTemplate` — T42a 定义

**本层产出（其他 Task 会依赖的）**：
- `plugins/orchestrator.GuardianAgent` — Guardian 实现
- `plugins/orchestrator.TesterAgent` — Tester 实现
- `plugins/orchestrator.ResearcherAgent` — Researcher 实现

### 4. 约定

- Guardian 必须检测高风险操作并触发 confirming 状态
- Tester 必须能够执行测试命令并验证结果
- Researcher 可以调用 ask_llm 进行技术调研
- 所有 Agent 错误时返回 failed 状态

### 5. 验收标准

- 测试命令：`go test ./plugins/orchestrator/... -v -run TestGuardianAgent`
- 必须覆盖的 case：
  1. Guardian 检测高风险操作
  2. Tester 执行测试
  3. Researcher 进行技术调研
- Done 判定：测试全部通过 + `go build ./plugins/orchestrator/...`

## 描述

T42c: 后3个Agent实现 - guardian/tester/researcher角色完整实现

## 需求 (requirements)

T42c: 后3个Agent实现 - guardian/tester/researcher角色完整实现

## 验收标准 (acceptance)

- 3个Agent实现正确
- 高风险操作检测正确

## 交付物 (deliverables)

- plugins/orchestrator/agent_guardian.go — Guardian Agent
- plugins/orchestrator/agent_tester.go — Tester Agent
- plugins/orchestrator/agent_researcher.go — Researcher Agent

## 设计方案 (design)

1. 实现 GuardianAgent（高风险检测）
2. 实现 TesterAgent（测试执行）
3. 实现 ResearcherAgent（技术调研）
4. 编写 prompt 模板

## 验证证据（完成前必填）

- [ ] **实现证明**: 3个Agent实现完成
- [ ] **测试验证**: `go test ./plugins/orchestrator/...` 通过
- [ ] **影响范围**: 复杂任务编排依赖

### 测试步骤
1. `go test ./plugins/orchestrator/... -v -run TestGuardianAgent`
2. `go build ./plugins/orchestrator/...`

### 验证结果
