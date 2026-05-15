# task_025

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_025.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/service/context_service.go`

产出类型：
- `ContextRouter` — 上下文路由器

### 2. 契约参考

```go
type ContextRouter struct {}

func (r *ContextRouter) Route(ctx *TaskContext, mode string) (*TaskContext, error) {
    // full: <5K tokens，完整传递
    // summary: 5K-20K，LLM 生成摘要
    // delta: >20K，只传变更部分
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `internal/domain.TaskContext` — T44 定义
- `plugins/llmrouter.LLMCaller` — T61 定义

**本层产出（其他 Task 会依赖的）**：
- `internal/service.ContextRouter` — 上下文路由

### 4. 约定

- L4-Service 层
- 传递模式可配置

### 5. 验收标准

- 测试命令：`go test ./internal/service/... -v -run TestContextRouter`
- 必须覆盖的 case：
  1. full 模式正常
  2. summary 模式正常
- Done 判定：测试通过 + `go build ./...`

## 描述

T46: 上下文传递服务 - full/summary/delta三种传递模式



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T46:_上下文传递服务_-_full/.py




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