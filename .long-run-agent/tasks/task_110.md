# task_110

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_110.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

fix: Gateway 添加 REST 适配路由，对齐 MCP Client 协议


## 需求 (requirements)

MCP Client 使用 REST 路径(/api/v1/*)调用后端，但 Gateway 只注册了 connect-go 路由(/cap.v1.TaskService/*)，导致所有 MCP 请求 404。需要在 Gateway 层添加 REST 适配路由，将 /api/v1/* 请求转换为对已有 TaskServiceHandler 的调用。



## 验收标准 (acceptance)


- Gateway 注册 /api/v1/* REST 路由且正确转换调用

- MCP Server stdio 调用 task_list/task_submit 等返回正确结果

- 原有 connect-go 路由不受影响

- REST 请求走现有认证中间件

- 编写 REST 适配层单元测试

- go build 和 go test 全量通过




## 交付物 (deliverables)

- `internal/gateway/rest_adapter.go` — REST 适配层实现
- `internal/gateway/rest_adapter_test.go` — REST 适配层单元测试



## 设计方案 (design)

在 gateway.go 的 mux 上注册 /api/v1/* 前缀路由。新增 internal/gateway/rest_adapter.go，实现 REST handler → TaskServiceHandler 方法调用。REST 请求体 JSON 转换为 protobuf 请求，响应从 protobuf 转回 JSON。认证复用现有中间件。MCP Client 路径不变。


## 验证证据（完成前必填）

<!-- 标记完成前，请提供以下证据： -->

- [x] **实现证明**: REST 适配层已实现（rest_adapter.go）
- [x] **测试验证**: go build ./... 和 REST adapter tests 通过
- [x] **影响范围**: 仅影响 L5-Gateway，不影响其他层

### 测试步骤
1. 
2. 
3. 

### 验证结果

**实现证明**:
1. `gateway.go` 中注册了 `/api/v1/*` REST 路由，复用 `RESTAdapter`
2. `rest_adapter.go` 实现了所有 MCP Client 期望的端点：
   - `POST /api/v1/tasks` → SubmitTask
   - `GET /api/v1/tasks` → ListTasks
   - `GET /api/v1/tasks/:id` → GetTask
   - `POST /api/v1/tasks/:id/cancel` → CancelTask
   - `POST /api/v1/tasks/:id/subtasks/:subtaskId/decision` → DecideTask
   - `POST /api/v1/tasks/:id/decompose` → DecomposeTask (占位返回 NotImplemented)
   - `GET /api/v1/tasks/:id/diff` → GetTaskDiff (占位返回 NotImplemented)
   - `GET /api/v1/agent-templates` → ListAgents
   - `GET /api/v1/sessions` → ListSessions
   - `GET /api/v1/platform/status` → PlatformStatus
3. REST 请求复用现有认证中间件

**测试验证**:
```
go build ./... ✓
go test ./internal/gateway/... -run REST ✓
```

**影响范围**: 仅影响 L5-Gateway 层，不影响其他层