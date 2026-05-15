# task_019

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_019.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/gateway/ws_hub.go`

产出类型：
- `WebSocketHub` struct — WebSocket Hub

### 2. 契约参考

```go
// WSEvent 类型
type WSEvent struct {
    Type    string
    Payload map[string]interface{}
}

// 事件类型
// task.status_changed, task.subtask_completed, task.completed, task.failed
// agent.log, agent.tool_called, agent.llm_request
// platform.worker_pool

type WebSocketHub struct {
    rooms map[string]map[*Client]bool
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- 无上游依赖

**本层产出（其他 Task 会依赖的）**：
- `internal/gateway.WebSocketHub` — Hub 实现

### 4. 约定

- 房间按 taskId 分
- 连接认证：首条消息 JWT
- 错误码：L5_800 ~ L5_999

### 5. 验收标准

- 测试命令：`go test ./internal/gateway/... -v -run TestWebSocket`
- 必须覆盖的 case：
  1. 房间创建和销毁
  2. 广播消息
- Done 判定：测试通过 + `go build ./...`

## 描述

T40: WebSocket Hub - 房间管理，所有事件类型



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T40:_WebSocket_Hub_-.py




## 设计方案 (design)

快速任务


## 验证证据（完成前必填）

<!-- 标记完成前，请提供以下证据： -->

- [ ] **实现证明**: 简要说明如何实现
- [ ] **测试验证**: 如何验证功能正常（测试步骤/截图/命令输出）
- [ ] **影响范围**: 是否影响其他功能

### 测试步骤
1. 
2. 
3. 

### 验证结果
<!-- 粘贴验证截图、命令输出或测试结果 -->