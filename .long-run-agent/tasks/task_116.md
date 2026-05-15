# task_116

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_116.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

test: MCP Server 端到端验证 - stdio 调用全工具


## 需求 (requirements)

REST适配完成后通过stdio启动MCP Server逐个调用全部9个tools(task_submit/task_status/task_list/task_cancel/task_decompose/context_approve/context_reject/agent_list/session_list)验证每个工具的请求解析和响应格式正确



## 验收标准 (acceptance)


- 9个tools全部返回正确结果(非404/panic/error)

- MCP Server无panic

- 响应JSON格式符合MCP规范

- 更新或补充test/e2e/mcp_test.go的覆盖




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

写集成测试: 启动真实Gateway+Service+MockStorage通过MCP Server stdio发送tools/call请求验证响应。或补充现有mcp_test.go使其走真实REST路由而非mock


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