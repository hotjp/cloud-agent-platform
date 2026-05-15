# task_014

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_014.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/gateway/handler.go`、`internal/gateway/middleware.go`

产出类型：
- `TaskServiceHandler` — connect-go handler
- 中间件：Recovery/RequestID/Metrics/Logging/CORS/Auth/RateLimit

### 2. 契约参考

```go
// 中间件顺序（architecture.md）
// Recover → RequestID → Metrics → Logging → CORS → Auth → Routing

// JWT 仅解密不验证（由 L3-Authz 验证）
// sentinel-go 限流：全局 100req/s + client_id 10req/s
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `internal/service.TaskService` — T22 定义

**本层产出（其他 Task 会依赖的）**：
- `internal/gateway.TaskServiceHandler` — Handler 实现

### 4. 约定

- L5-Gateway 层，协议适配
- JWT 仅解密，验证在 L3
- 错误码：L5_800 ~ L5_999

### 5. 验收标准

- 测试命令：`go test ./internal/gateway/... -v`
- 必须覆盖的 case：
  1. Handler 注册成功
  2. 中间件顺序正确
- Done 判定：`go build ./...`

## 描述

T23: Gateway层 - connect-go handler，JWT解密，sentinel-go限流，中间件链路



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T23:_Gateway层_-_conn.py




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