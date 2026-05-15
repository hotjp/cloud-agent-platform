# task_127

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_127.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

test: 端到端集成测试 — 从 MCP submit 到 Agent 执行完成


## 需求 (requirements)

前面的修复完成后，写一个端到端测试验证完整链路:1)MCP task_submit 创建任务 2)自动触发 Decompose 3)Worker 执行(可 mock) 4)状态正确流转 5)可通过 task_status 查询进度 6)可通过 task_list 看到任务。使用 mock LLM 和 mock Worker 确保测试可靠



## 验收标准 (acceptance)


- 端到端测试覆盖 submit→decompose→execute→complete 全流程

- 使用 mock LLM/Worker 确保测试稳定

- task_status 能查到正确状态

- go test 通过




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

创建 test/e2e/full_pipeline_test.go。使用 httptest 启动真实 Gateway+Service，mock LLM 返回固定 decompose 结果，mock Worker 返回成功。验证每一步的状态流转。


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