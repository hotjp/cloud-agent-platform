# task_091

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_091.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P2: Prometheus metrics integration — Wire internal metrics to /9090/metrics endpoint


## 需求 (requirements)

internal/observability/metrics/ has metrics.Recorder but metrics endpoint is not wired in main.go. Required: (1) Create http.Handler for /metrics that exposes Prometheus metrics (promhttp.Handler()); (2) Wire into main.go on metricsPort (9090); (3) Record business metrics: tasks_submitted_total, tasks_completed_total, tasks_failed_total, task_duration_seconds, agent_execution_time_seconds, llm_requests_total, llm_errors_total, llm_latency_seconds, token_usage_total, worker_pool_size, worker_idle_count; (4) Wire metrics.Recorder into TaskServiceInput and call RecordTaskSubmission/RecordTaskCompletion/etc. in task_service.go; (5) Verify metrics endpoint returns Prometheus format with cap_* metrics



## 验收标准 (acceptance)


- curl http://localhost:9090/metrics returns Prometheus metrics; cap_tasks_submitted_total increments on Submit; cap_tasks_completed_total increments on task completion; all documented metrics present




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

<!-- 在此填写架构设计、技术选型、实现思路 -->


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