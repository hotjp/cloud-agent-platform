# task_114

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_114.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

fix: Gateway REST 适配 - 查询端点(AgentTemplates/Sessions/PlatformStatus)


## 需求 (requirements)

在 Gateway 添加 3 个只读 REST 端点: GET /api/v1/agent-templates, GET /api/v1/sessions, GET /api/v1/platform/status。AgentTemplates 返回可用 Agent 模板列表, Sessions 返回当前会话, PlatformStatus 返回平台状态



## 验收标准 (acceptance)


- 3个端点注册到mux并响应正确

- 响应格式匹配APIResponse

- go test ./internal/gateway/... 通过




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

在 rest_adapter.go 继续添加。这3个端点是只读的不需要请求体解析。AgentTemplates 可选支持 capability 过滤参数


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