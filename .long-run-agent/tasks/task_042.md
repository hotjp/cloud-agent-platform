# task_042

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_042.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/orchestrator/graph.go`、`plugins/orchestrator/nodes.go`

产出类型：
- `TaskGraph` struct — Eino Graph 封装
- `ComplexityRouter` struct — 复杂度路由器
- `GraphConfig` struct — 图配置

### 2. 契约参考

```go
// TaskInput — 图输入
type TaskInput struct {
    TaskID         string
    Goal           string
    RepositoryURL  string
    BaseBranch     string
    Constraints    []string
}

// TaskOutput — 图输出
type TaskOutput struct {
    TaskID     string
    Status     string
    Result     *TaskResult
    Error      error
}

// ComplexityRouter — 复杂度路由
type ComplexityRouter struct{}

func (r *ComplexityRouter) Route(ctx context.Context, input *TaskInput) string {
    // 简单：单文件修改、<100行 → "simple"
    // 中等：多文件、模块间依赖 → "medium"
    // 复杂：架构级变更、多角色协作 → "complex"
}
```

Eino Graph 定义（来自 Cloud-Agent-Platform.md 6.2）：
```
analyzer → router
router → simple_executor (if complexity == "simple")
router → medium_decomposer → medium_executors (if complexity == "medium")
router → observer → strategist → executor → guardian → tester (if complexity == "complex")
{simple_executor, medium_executors, tester} → merger
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `github.com/cloudwego/eino/compose` — Eino Graph
- `internal/service/interfaces.go` — T05c 定义的仓储接口

**本层产出（其他 Task 会依赖的）**：
- `plugins/orchestrator.TaskGraph` — 编排图实例

### 4. 约定

- 插件层实现，接口定义在 L4-Service
- 使用 `eino/compose.NewGraph` 构建图
- 节点注册：`g.AddNode("name", nodeInstance)`
- 边连接：`g.AddEdge("from", "to")`
- 图编译：`g.Compile(context.Background())`

### 5. 验收标准

- 测试命令：`go test ./plugins/orchestrator/... -v -run TestGraph`
- 必须覆盖的 case：
  1. Graph 编译成功
  2. 简单任务路由到 simple_executor
  3. 复杂任务路由到 observer → strategist → executor
- Done 判定：测试全部通过 + `go build ./plugins/orchestrator/...`

## 描述

T30a: 编排框架 - 复杂度路由入口，Eino Graph最小图定义，节点注册机制

## 需求 (requirements)

T30a: 编排框架 - 复杂度路由入口，Eino Graph最小图定义，节点注册机制

## 验收标准 (acceptance)

- Graph 编译成功
- 复杂度路由正确

## 交付物 (deliverables)

- plugins/orchestrator/graph.go — Graph 定义
- plugins/orchestrator/nodes.go — 节点实现
- plugins/orchestrator/graph_test.go — 测试

## 设计方案 (design)

1. 创建 `plugins/orchestrator/graph.go`
2. 实现 TaskGraph 和 ComplexityRouter
3. 定义图节点和边
4. 编译图并返回

## 验证证据（完成前必填）

- [ ] **实现证明**: Graph 编译成功
- [ ] **测试验证**: `go test ./plugins/orchestrator/...` 通过
- [ ] **影响范围**: T30b 和 T41 依赖此框架

### 测试步骤
1. `go test ./plugins/orchestrator/... -v`
2. `go build ./plugins/orchestrator/...`

### 验证结果
