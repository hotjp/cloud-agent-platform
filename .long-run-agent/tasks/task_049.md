# task_049

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_049.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/orchestrator/full_graph.go`

产出类型：
- `FullTaskGraph` struct — 完整 Eino Graph
- `ComplexityRouter` struct — 复杂度路由器
- 包含简单/中等/复杂三条路径

### 2. 契约参考

```go
// 完整 Graph（来自 Cloud-Agent-Platform.md 6.2）
//                    ┌─────────────┐
//                    │  analyzer   │
//                    └──────┬──────┘
//                           │
//                    ┌──────▼──────┐
//                    │   router    │
//                    └──────┬──────┘
//           ┌───────────────┼───────────────┐
//           │               │               │
//    ┌──────▼──────┐ ┌─────▼─────┐ ┌──────▼──────┐
//    │   simple    │ │  medium   │ │   complex    │
//    │  executor  │ │decomposer │ │   observer   │
//    └──────┬──────┘ └─────┬─────┘ └──────┬──────┘
//           │               │               │
//           │         ┌─────▼─────┐ ┌──────▼──────┐
//           │         │  medium   │ │ strategist  │
//           │         │executors  │ └──────┬──────┘
//           │         └─────┬─────┘         │
//           │               │         ┌──────▼──────┐
//           └──────┬────────┘         │  executor   │
//                  │                  └──────┬──────┘
//           ┌──────▼──────┐                  │
//           │   merger    │◄────────┌────────┘
//           └─────────────┘         │
//                             ┌─────▼─────┐
//                             │  guardian │
//                             └─────┬─────┘
//                                   │
//                             ┌─────▼─────┐
//                             │   tester  │
//                             └─────┬─────┘
//                                   │
//                             ┌─────▼─────┐
//                             │   merger   │
//                             └───────────┘

// 复杂度判断规则
func (r *ComplexityRouter) Route(ctx context.Context, input *TaskInput) string {
    // 简单：单文件修改、<100行、无依赖
    if isSimpleTask(input.Goal) {
        return "simple"
    }
    // 中等：多文件、模块间依赖
    if isMediumTask(input.Goal) {
        return "medium"
    }
    // 复杂：架构级变更、多角色协作
    return "complex"
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `github.com/cloudwego/eino/compose` — Eino Graph
- `plugins/orchestrator/nodes.go` — T41b 节点定义

**本层产出（其他 Task 会依赖的）**：
- `plugins/orchestrator.FullTaskGraph` — 完整编排图

### 4. 约定

- 使用 `compose.WithEdgeCondition` 定义条件边
- 简单路径：analyzer → simple_executor → merger
- 中等路径：analyzer → medium_decomposer → medium_executors → merger
- 复杂路径：analyzer → observer → strategist → executor → guardian → tester → merger
- 所有边使用 `compose.WithContextPassing` 传递上下文

### 5. 验收标准

- 测试命令：`go test ./plugins/orchestrator/... -v -run TestFullGraph`
- 必须覆盖的 case：
  1. Graph 编译成功
  2. 简单任务走 simple_executor
  3. 中等任务走 medium 路径
  4. 复杂任务走 observer → strategist → executor → guardian → tester
- Done 判定：测试全部通过 + `go build ./plugins/orchestrator/...`

## 描述

T41a: Eino Graph框架 - 3路路由(简单/中等/复杂)，Graph定义，条件边连接

## 需求 (requirements)

T41a: Eino Graph框架 - 3路路由(简单/中等/复杂)，Graph定义，条件边连接

## 验收标准 (acceptance)

- 3路路由正确
- 条件边连接正确

## 交付物 (deliverables)

- plugins/orchestrator/full_graph.go — 完整 Graph 定义
- plugins/orchestrator/full_graph_test.go — 测试

## 设计方案 (design)

1. 创建 `plugins/orchestrator/full_graph.go`
2. 实现 FullTaskGraph 结构体
3. 定义 3 条路径的节点和边
4. 使用 WithEdgeCondition 条件路由

## 验证证据（完成前必填）

- [ ] **实现证明**: 完整 Graph 定义完成
- [ ] **测试验证**: `go test ./plugins/orchestrator/...` 通过
- [ ] **影响范围**: T42 依赖此 Graph

### 测试步骤
1. `go test ./plugins/orchestrator/... -v -run TestFullGraph`
2. `go build ./plugins/orchestrator/...`

### 验证结果
