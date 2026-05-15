# task_043

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_043.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/orchestrator/simple_executor.go`

产出类型：
- `SimpleExecutor` struct — 单 Agent 直通执行器
- 实现 Eino Node 接口

### 2. 契约参考

```go
// SimpleExecutor — 简单任务单 Agent 执行
type SimpleExecutor struct {
    agent    *ReActAgent      // T31a
    workerMgr WorkerManager   // T32a
}

func (e *SimpleExecutor) NodeID() string {
    return "simple_executor"
}

func (e *SimpleExecutor) Invoke(ctx context.Context, input *TaskInput) (*TaskOutput, error) {
    // 1. 获取 Worker（Sandbox）
    worker, err := e.workerMgr.Acquire(ctx, spec)
    if err != nil {
        return nil, err
    }
    defer e.workerMgr.Release(ctx, worker)

    // 2. 启动 ReAct Agent
    result, err := e.agent.Run(ctx, &AgentInput{
        Goal:       input.Goal,
        Workspace:  worker.Workspace(),
        Tools:      []Tool{readFileTool, writeFileTool},
        MaxSteps:   15,
    })

    // 3. 返回结果
    return &TaskOutput{
        TaskID: input.TaskID,
        Status: "completed",
        Result: result,
    }, nil
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `plugins/orchestrator.ReActAgent` — T31a 定义
- `plugins/workermanager.WorkerManager` — T32a 定义

**本层产出（其他 Task 会依赖的）**：
- `plugins/orchestrator.SimpleExecutor` — 单 Agent 执行器

### 4. 约定

- Phase 0 最小闭环：pending → running → completed
- 只使用 read_file 和 write_file 两个工具
- Worker 获取后必须释放（defer）
- 错误时返回 failed 状态

### 5. 验收标准

- 测试命令：`go test ./plugins/orchestrator/... -v -run TestSimpleExecutor`
- 必须覆盖的 case：
  1. 简单任务执行成功
  2. Worker 正确获取和释放
  3. Agent 执行超时时返回错误
- Done 判定：测试全部通过 + `go build ./plugins/orchestrator/...`

## 描述

T30b: 单Agent路径 - pending→running→completed最小执行路径，单角色直通

## 需求 (requirements)

T30b: 单Agent路径 - pending→running→completed最小执行路径，单角色直通

## 验收标准 (acceptance)

- 单 Agent 路径测试通过
- Worker 获取释放正常

## 交付物 (deliverables)

- plugins/orchestrator/simple_executor.go — SimpleExecutor
- plugins/orchestrator/simple_executor_test.go — 测试

## 设计方案 (design)

1. 创建 `plugins/orchestrator/simple_executor.go`
2. 实现 SimpleExecutor 结构体
3. 实现 Eino Node 接口
4. 集成 ReActAgent 和 WorkerManager

## 验证证据（完成前必填）

- [ ] **实现证明**: SimpleExecutor 实现完整
- [ ] **测试验证**: `go test ./plugins/orchestrator/...` 通过
- [ ] **影响范围**: 最小闭环的核心路径

### 测试步骤
1. `go test ./plugins/orchestrator/... -v -run TestSimpleExecutor`
2. `go build ./plugins/orchestrator/...`

### 验证结果
