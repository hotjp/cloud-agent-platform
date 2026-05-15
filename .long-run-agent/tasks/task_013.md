# task_013

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_013.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/service/task_service.go`

产出类型：
- `TaskService` struct — Task 服务实现
- 5 个方法实现

### 2. 契约参考

```go
// TaskService 接口
type TaskService interface {
    SubmitTask(ctx context.Context, req *SubmitTaskRequest) (*SubmitTaskResponse, error)
    GetTask(ctx context.Context, req *GetTaskRequest) (*Task, error)
    ListTasks(ctx context.Context, req *ListTasksRequest) (*ListTasksResponse, error)
    CancelTask(ctx context.Context, req *CancelTaskRequest) (*CancelTaskResponse, error)
    DecideTask(ctx context.Context, req *DecideTaskRequest) (*DecideTaskResponse, error)
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `internal/service/interfaces.go` — T05c 定义
- `internal/domainevents.OutboxWriter` — T05a 定义

**本层产出（其他 Task 会依赖的）**：
- `internal/service.TaskService` — Task 服务实现

### 4. 约定

- L4-Service 层，输入校验 + 事务边界
- SubmitTask 创建 Task + Subtask + AuditLog
- 所有方法返回 error
- 错误码：L4_600 ~ L4_799

### 5. 验收标准

- 测试命令：`go test ./internal/service/... -v -run TestTaskService`
- 必须覆盖的 case：
  1. SubmitTask 创建任务
  2. GetTask 获取任务
  3. CancelTask 取消任务
- Done 判定：测试通过 + `go build ./...`

## 描述

T22: Task服务层 - TaskService的5个方法(Submit/Get/List/Cancel/Decide)，输入校验，事务边界



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T22:_Task服务层_-_TaskS.py




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