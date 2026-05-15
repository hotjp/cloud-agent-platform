# task_059

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_059.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/mcpserver/resources.go`

产出类型：
- `TaskLogResource` — 任务日志
- `TaskTimelineResource` — 决策时间线
- `TaskArtifactResource` — 产出物文件
- `PlatformStatusResource` — 平台状态

### 2. 契约参考

```go
// 4个 MCP Resources（来自 Cloud-Agent-Platform.md 5.3）
type TaskLogResource struct {
    taskRepo TaskRepository
}

type TaskTimelineResource struct {
    auditRepo AuditLogRepository
}

type TaskArtifactResource struct {
    minio *minio.Client
}

type PlatformStatusResource struct {
    workerMgr WorkerManager
}

// Resource URI 格式
// cap://tasks/{taskId}/log → 任务执行日志
// cap://tasks/{taskId}/timeline → 决策时间线
// cap://tasks/{taskId}/artifact/{id} → 产出物文件
// cap://platform/status → 平台实时状态

// MIME 类型
// log: text/plain
// timeline: application/json
// artifact: application/octet-stream
// status: application/json
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `plugins/mcpserver.MCPServer` — T49a 定义
- `internal/service.TaskRepository` — T05c 定义
- `internal/storage.MinIOClient` — T63 定义

**本层产出（其他 Task 会依赖的）**：
- `plugins/mcpserver.TaskLogResource`
- `plugins/mcpserver.TaskTimelineResource`
- `plugins/mcpserver.TaskArtifactResource`
- `plugins/mcpserver.PlatformStatusResource`

### 4. 约定

- Resource URI 使用 `cap://` 协议
- artifact 资源从 MinIO 读取（签名 URL）
- log 资源实时流式读取
- 错误返回 404 Not Found

### 5. 验收标准

- 测试命令：`go test ./plugins/mcpserver/... -v -run TestResources`
- 必须覆盖的 case：
  1. 4 个 Resource 全部实现
  2. URI 解析正确
  3. MIME 类型正确
- Done 判定：测试全部通过 + `go build ./plugins/mcpserver/...`

## 描述

T49c: MCP Resources实现 - 4个Resource(cap://tasks/{id}/log/timeline/artifact/{id}, cap://platform/status)

## 需求 (requirements)

T49c: MCP Resources实现 - 4个Resource(cap://tasks/{id}/log/timeline/artifact/{id}, cap://platform/status)

## 验收标准 (acceptance)

- 4个Resource全部实现
- URI解析正确

## 交付物 (deliverables)

- plugins/mcpserver/resources.go — 4个Resource实现

## 设计方案 (design)

1. 实现 TaskLogResource
2. 实现 TaskTimelineResource
3. 实现 TaskArtifactResource
4. 实现 PlatformStatusResource

## 验证证据（完成前必填）

- [ ] **实现证明**: 4个Resource全部实现
- [ ] **测试验证**: `go test ./plugins/mcpserver/...` 通过
- [ ] **影响范围**: MCP 协议调用

### 测试步骤
1. `go test ./plugins/mcpserver/... -v -run TestResources`
2. `go build ./plugins/mcpserver/...`

### 验证结果
