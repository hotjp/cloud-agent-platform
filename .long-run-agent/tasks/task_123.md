# task_123

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_123.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

fix: Eino 编排图接入 LLM — 任务分析和拆解需要真实 LLM 调用


## 需求 (requirements)

当前 Orchestrator 的 ComplexityRouter/AnalyzerNode 是规则判断不是 LLM 驱动。MediumDecomposerNode 需要 LLM 来分析任务并生成 subtask 计划。需要:1)Eino Graph 节点接入 LLMRouter 2)ComplexityRouter 使用 LLM 分析 goal 判断复杂度 3)DecomposerNode 使用 LLM 生成 subtask 列表 4)配置 LLM API Key 后整个链路可用



## 验收标准 (acceptance)


- ComplexityRouter 使用 LLM 分析任务复杂度(不是规则匹配)

- DecomposerNode 使用 LLM 生成 subtask(不是硬编码)

- 配置 API Key 后端到端可用

- LLM 调用失败有 fallback

- go test 通过




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

通过 Eino 的 chat model 接口接入 LLMRouter。在 plugins/orchestrator/Dependencies 中注入 LLM provider。使用 eino.WithChatModel 配置。复杂度判断 prompt 和拆解 prompt 在代码中定义。


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