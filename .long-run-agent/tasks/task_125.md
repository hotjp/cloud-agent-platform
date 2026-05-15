# task_125

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_125.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

fix: Guardian 审批集成 — 高风险操作暂停等待用户确认


## 需求 (requirements)

当前 Guardian 有独立实现但没和执行流程串联。需要:1)Worker 执行前 Guardian 评估风险 2)高风险操作暂停并通过 WebSocket 推送审批请求 3)用户通过 MCP context_approve/reject 响应 4)审批通过后继续执行 5)超时自动拒绝



## 验收标准 (acceptance)


- 高风险改动自动暂停等待审批

- WebSocket 推送审批请求到前端

- MCP context_approve 恢复执行

- MCP context_reject 终止任务

- 超时自动拒绝

- go test 通过




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

在 Worker 执行流程中加入 Guardian.NeedsApproval 检查。高风险: 1)暂停 Worker 2)Guardian.RequestApproval 通过 WS 推送 3)启动超时定时器 4)等待审批/超时 5)继续或终止


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