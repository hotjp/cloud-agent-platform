# task_027

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_027.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/service/approval.go`

产出类型：
- `ApprovalService` struct — 审批服务

### 2. 契约参考

```go
// 触发条件
// - Guardian 检测到高风险操作
// - 用户设置 requireApproval: true
// - estimatedCost > 1元

type ApprovalService struct {
    wsHub *WebSocketHub
    taskRepo TaskRepository
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `internal/service/interfaces.go` — T05c 定义
- `internal/gateway.WebSocketHub` — T40 定义

**本层产出（其他 Task 会依赖的）**：
- `internal/service.ApprovalService` — 审批服务

### 4. 约定

- L4-Service 层
- 超时默认 5 分钟
- WebSocket 推送 `task.confirmation_required`

### 5. 验收标准

- 测试命令：`go test ./internal/service/... -v -run TestApproval`
- 必须覆盖的 case：
  1. 审批超时处理
  2. WebSocket 推送正确
- Done 判定：测试通过 + `go build ./...`

## 描述

T48: 人工审批 - Guardian触发confirming状态，超时机制(默认5分钟)，WebSocket推送approval_required



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T48:_人工审批_-_Guardian.py




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