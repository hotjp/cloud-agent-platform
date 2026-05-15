# task_082

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_082.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P2: Prometheus alerting rules + Grafana dashboard — Production metrics


## 需求 (requirements)

Create deploy/prometheus/alerts.yml and deploy/grafana/dashboards/cap-overview.json: (1) Prometheus alerting rules: TaskQueueDepth > 100 (warning) / > 500 (critical), TaskFailureRate > 5% (warning) / > 20% (critical), LLMErrorRate > 10% (critical), WorkerUtilization > 90% (warning), DBConnectionUtilization > 80% (warning), RedisConnectionUtilization > 80% (warning); (2) Grafana dashboard: Task throughput (submitted/completed/failed per minute), P50/P95/P99 task duration, Active tasks + pending queue depth, Worker pool utilization (idle/busy/total), LLM cost per model per day, Token usage per task type, Error rate by type



## 验收标准 (acceptance)


- Alert rules are valid Prometheus rules; Grafana dashboard loads without errors; all panels have queries against cap_* metrics




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