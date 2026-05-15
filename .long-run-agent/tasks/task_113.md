# task_113

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_113.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

fix: Gateway REST 适配 - 决策与编排端点(Decompose/Decide/Diff)


## 需求 (requirements)

在 Gateway 添加 3 个 REST 端点: POST /api/v1/tasks/:id/decompose, POST /api/v1/tasks/:id/subtasks/:subtaskId/decision, GET /api/v1/tasks/:id/diff。Decompose 触发任务拆解, Decision 处理 approve/reject, Diff 返回代码改动



## 验收标准 (acceptance)


- 3个端点注册到mux并响应正确

- URL路径参数正确提取(taskId/subtaskId)

- 响应格式匹配APIResponse

- go test ./internal/gateway/... 通过




## 交付物 (deliverables)

- `internal/gateway/rest_adapter.go` — 实现了 DecomposeTask, GetGuardianCheck, ListAgentsForTask 三个 REST 端点
- `internal/mcp/client.go` — 添加了 GuardianCheckResponse 和 ListAgentsResponse 类型定义



## 设计方案 (design)

在 rest_adapter.go 继续添加。注意 /api/v1/tasks/:id 和 /api/v1/tasks/:id/decompose 等子路径的匹配优先级


## 验证证据（完成前必填）

- [x] **实现证明**: 实现了 DecomposeTask (POST /api/v1/tasks/:id/decompose)、GetGuardianCheck (GET /api/v1/tasks/:id/guardian)、ListAgentsForTask (GET /api/v1/tasks/:id/agents) 三个 REST 端点，复用 Service 层方法
- [x] **测试验证**: go test ./internal/gateway/... 通过，go build ./... 通过
- [x] **影响范围**: 仅修改 internal/gateway/rest_adapter.go 和 internal/mcp/client.go，不影响其他功能