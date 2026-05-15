# task_126

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_126.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

fix: REST Adapter placeholder 端点补全(ListAgents/Sessions/PlatformStatus)


## 需求 (requirements)

当前 ListAgents/ListSessions/PlatformStatus 返回空数据。ListAgents 应返回预定义的 Agent 模板列表。PlatformStatus 应返回平台运行状态。Sessions 应返回活跃会话信息。



## 验收标准 (acceptance)


- ListAgents 返回预定义的 Agent 模板(observer/strategist/executor/guardian/tester/researcher)

- PlatformStatus 返回服务状态(uptime/task_count/worker_count)

- Sessions 返回活跃 session 列表




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

ListAgents 从配置或常量返回预定义模板。PlatformStatus 从 TaskService/List 统计任务数+从 WorkerPool 统计 Worker 数。Sessions 暂返回空列表(需要 session 管理模块)


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