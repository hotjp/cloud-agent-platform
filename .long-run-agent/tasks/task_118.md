# task_118

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_118.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

verify: 检查 REST Adapter 已实现的端点


## 需求 (requirements)

internal/gateway/rest_adapter.go 已存在且有部分实现。需要验证已实现的 handler(ListAgents/ListSessions/PlatformStatus/handleTasks/handleTaskOperations) 是否完整，响应格式是否正确



## 验收标准 (acceptance)


- 列出 RESTAdapter 已实现的全部 handler，每个 handler 的路由注册确认

- 响应 writeJSON/writeError 格式确认

- 编译通过




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

检查 rest_adapter.go 的所有导出函数，对照 gateway.go 中的路由注册，列出已实现和未实现的部分。准备一个验证报告。


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