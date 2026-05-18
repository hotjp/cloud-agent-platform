# task_132

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_132.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

T-E2E-02: 两个Agent并行改不同仓库


## 需求 (requirements)

taskA改octocat/Hello-World,taskB改另一个公开仓库



## 验收标准 (acceptance)


- 创建两个不同Git容器

- 独立执行互不影响

- 两个task都COMPLETED




## 交付物 (deliverables)

- `test/e2e/parallel_agents_test.go` - E2E tests for parallel agents modifying different repositories



## 设计方案 (design)

<!-- 在此填写架构设计、技术选型、实现思路 -->


## 验证证据（完成前必填）

- [x] **实现证明**: Created `test/e2e/parallel_agents_test.go` with 5 test cases:
  - `TestParallelAgents_TwoAgentsDifferentRepos`: Two agents modify different repos simultaneously
  - `TestParallelAgents_RepoIsolation`: Verifies complete repository isolation between agents
  - `TestParallelAgents_ConcurrentSubmission`: 5 concurrent task submissions
  - `TestParallelAgents_ParallelExecutionWithMockLLM`: Full MCP protocol flow with parallel execution
  - `TestParallelAgents_SameRepoConflict`: Two agents modifying same repo

- [x] **测试验证**: `go test -p 2 -count=1 -timeout 60s -run 'TestParallelAgents' ./test/e2e/` - PASSED (0.659s)

- [x] **影响范围**: No impact on other tests; uses httptest + mock provider only

### 测试步骤
1. `go test -p 2 -count=1 -timeout 60s -run 'TestParallelAgents' ./test/e2e/`
2. Tests create independent mock handlers for each scenario
3. Tests verify repo isolation, concurrent execution, and status tracking

### 验证结果
```
ok  	github.com/cloud-agent-platform/cap/test/e2e	0.659s
```