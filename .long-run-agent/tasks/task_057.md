# task_057

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_057.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/mcpserver/server.go`

产出类型：
- `MCPServer` struct — MCP Server
- `Tool` interface — Tool 接口
- `Resource` interface — Resource 接口

### 2. 契约参考

```go
// MCP Server 接口（来自 Cloud-Agent-Platform.md 5.3）
type MCPServer struct {
    tools    []Tool
    resources []Resource
    transport string  // "stdio" | "sse"
}

// Tool 接口
type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]interface{}
    Handle(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)
}

// Resource 接口
type Resource interface {
    URI() string
    MimeType() string
    Read(ctx context.Context) ([]byte, error)
}

// 传输方式
// stdio：本地 AI CLI 使用
// SSE：生产环境使用
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `internal/service` — TaskService 实现（T22）

**本层产出（其他 Task 会依赖的）**：
- `plugins/mcpserver.MCPServer` — MCP Server 实例

### 4. 约定

- MCP Server 挂载到 `/mcp` 路径
- SSE 传输使用 `text/event-stream` Content-Type
- Tool 名称格式：`task_submit`, `task_status`, `task_list`, `task_cancel`, `task_decide`, `task_diff`, `task_wait`, `agent_templates`, `platform_status`
- Resource URI 格式：`cap://tasks/{taskId}/log`

### 5. 验收标准

- 测试命令：`go test ./plugins/mcpserver/... -v -run TestMCPServer`
- 必须覆盖的 case：
  1. Server 启动成功
  2. Tool 列表正确返回
  3. Resource 读取成功
  4. SSE 传输正常
- Done 判定：测试全部通过 + `go build ./plugins/mcpserver/...`

## 描述

T49a: MCP框架 - MCP Server骨架，SSE传输，Tool/Resource接口定义

## 需求 (requirements)

T49a: MCP框架 - MCP Server骨架，SSE传输，Tool/Resource接口定义

## 验收标准 (acceptance)

- MCP Server 启动成功
- 接口定义正确

## 交付物 (deliverables)

- plugins/mcpserver/server.go — MCP Server 骨架
- plugins/mcpserver/server_test.go — 测试

## 设计方案 (design)

1. 创建 `plugins/mcpserver/` 目录
2. 定义 Tool 和 Resource 接口
3. 实现 MCPServer 结构体
4. 实现 SSE 传输

## 验证证据（完成前必填）

- [ ] **实现证明**: MCP Server 骨架完成
- [ ] **测试验证**: `go test ./plugins/mcpserver/...` 通过
- [ ] **影响范围**: T49b 和 T49c 依赖

### 测试步骤
1. `go test ./plugins/mcpserver/... -v`
2. `go build ./plugins/mcpserver/...`

### 验证结果
