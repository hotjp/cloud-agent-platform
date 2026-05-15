# task_034

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_034.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/telemetry/tracing.go`

产出类型：
- `InitTracer()` — 链路追踪初始化函数

### 2. 契约参考

```go
// Spans（来自 Cloud-Agent-Platform.md 10.2）
// task.submit, task.decompose, subtask.execute, agent.react_loop, llm.request, tool.call
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `go.opentelemetry.io/otel` — OTel

**本层产出（其他 Task 会依赖的）**：
- `internal/telemetry.InitTracer()` — Tracer 实例

### 4. 约定

- 导出器：OTLP
- 采样：概率采样
- Span 名称必须小写

### 5. 验收标准

- 测试命令：`go test ./internal/telemetry/... -v`
- 必须覆盖的 case：
  1. Tracer 初始化成功
  2. Span 创建正确
- Done 判定：`go build ./...`

## 描述

T65: 链路追踪 - OTel Spans(submit/decompose/execute/llm/tool)，OTLP导出，概率采样



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T65:_链路追踪_-_OTel_Spa.py




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