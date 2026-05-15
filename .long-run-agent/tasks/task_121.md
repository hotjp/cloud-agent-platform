# task_121

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_121.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

fix: 实现 handleTaskSubmitted — 自动触发任务拆解和执行


## 需求 (requirements)

当前 orchestrator.handleTaskSubmitted 只打日志就返回了。需要实现:1)收到 TaskSubmittedV1 后调用 Eino 编排图分析任务复杂度 2)根据复杂度自动选择 simple/medium/complex 路径 3)simple 路径直接执行 4)medium/complex 路径先 Decompose 再执行



## 验收标准 (acceptance)


- handleTaskSubmitted 调用 Eino 编排图

- Eino 根据 goal 判断复杂度

- simple 任务直接触发 Worker 执行

- medium/complex 任务触发 Decompose

- go test 通过




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

在 orchestrator.go 的 handleTaskSubmitted 中实现完整流程。调用已有的 ComplexityRouter.Invoke 判断复杂度，然后走 SimpleExecutorNode 或 MediumDecomposerNode。


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