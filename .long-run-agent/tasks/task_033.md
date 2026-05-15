# task_033

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_033.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/telemetry/metrics.go`

产出类型：
- `NewMetrics()` — Metrics 实例

### 2. 契约参考

```go
// 指标定义（来自 Cloud-Agent-Platform.md 10.3）
var (
    TasksSubmitted = prometheus.NewCounter(...)
    TasksCompleted = prometheus.NewCounterVec(...)
    TasksFailed = prometheus.NewCounter(...)
    AgentExecutionTime = prometheus.NewHistogram(...)
    LLMTokensUsed = prometheus.NewCounterVec(...)
)
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `github.com/prometheus/client_golang` — Prometheus 客户端

**本层产出（其他 Task 会依赖的）**：
- `internal/telemetry.NewMetrics()` — Metrics 实例

### 4. 约定

- 前缀：`cap_`
- 端点：`:9090/metrics`

### 5. 验收标准

- 测试命令：`go test ./internal/telemetry/... -v`
- 必须覆盖的 case：
  1. 指标注册成功
  2. 端点可访问
- Done 判定：`go build ./...`

## 描述

T64: 业务Metrics - cap_前缀Prometheus指标全集(task/agent/llm各维度)



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T64:_业务Metrics_-_cap.py




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