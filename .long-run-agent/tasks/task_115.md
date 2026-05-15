# task_115

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_115.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

fix: MCP Client 请求/响应字段映射验证与修复


## 需求 (requirements)

验证 MCP Client (internal/mcp/client.go) 的请求结构体字段名是否与 service 层期望的字段名完全一致。特别是: TaskSubmitRequest 的 goal/repository/constraints/verificationCriteria/priority/tags, ListTasks 的 status/tags/limit, DecideTask 的 taskId/subtaskId/feedback。修复所有字段名不匹配



## 验收标准 (acceptance)


- MCP Client 每个请求结构体字段与 service 层逐字段对齐

- 列出字段对照表

- 修复所有不一致

- go test ./internal/mcp/... 通过




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

逐一比对 client.go 的 Request struct 与 service/task_service.go 的 Request struct，列出字段对照表，修复不匹配的 json tag 或字段名


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