# task_122

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_122.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

fix: TaskService/Orchestrator 调用 Worker 执行子任务


## 需求 (requirements)

当前 TaskService 没有 WorkerManager 依赖，Decompose 后没有触发 Worker 执行。需要:1)Orchestrator 或 TaskService 能获取空闲 Worker 2)将 subtask 的指令(goal+context+constraints)发送给 Worker 3)Worker 在沙箱中执行并返回结果 4)结果回写到 subtask 状态



## 验收标准 (acceptance)


- Decompose 后自动分配 Worker 执行每个 subtask

- Worker 在 Docker/CubeSandbox 中执行

- subtask 状态从 pending→running→completed 自动流转

- 执行失败时 subtask 标记为 failed

- go test 通过




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

1)TaskService 或 Orchestrator 增加 WorkerManager 依赖 2)创建 subtask executor 循环 3)对每个 subtask: Acquire Worker→SubmitTask→收集结果→Update subtask 4)Worker 执行内容: clone repo + LLM 生成代码 + git commit


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