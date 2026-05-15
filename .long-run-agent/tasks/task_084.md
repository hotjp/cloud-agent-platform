# task_084

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_084.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P2: OTel end-to-end tracing verification — All spans wired + Jaeger UI


## 需求 (requirements)

Trace spans exist in code (tracing.StartTaskSubmit etc.) but tracing may not be initialized and exported. Required: (1) Initialize OTLP exporter in main.go using cfg.Telemetry (endpoint, sample rate); (2) Verify all key spans: task.submit, task.decompose, subtask.execute, agent.react_loop, llm.request, tool.call; (3) Wire span context propagation through goroutines (context.WithValue → trace context); (4) Add span attributes: task_id, subtask_id, agent_template, model, tool_name; (5) Verify with Jaeger UI (docker-compose with observability profile); (6) Create Taskfile target: make trace-verify that starts jaeger and runs a simple smoke test then opens Jaeger



## 验收标准 (acceptance)


- Jaeger UI shows complete trace from SubmitTask RPC through to individual tool calls; All spans have correct attributes; Trace context propagates through goroutines




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