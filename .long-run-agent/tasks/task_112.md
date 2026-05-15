# task_112

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_112.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

fix: Gateway REST 适配 - 任务 CRUD 端点(Submit/Get/List/Cancel)


## 需求 (requirements)

在 Gateway 添加 4 个核心 REST 端点: POST /api/v1/tasks(Submit), GET /api/v1/tasks/:id(Get), GET /api/v1/tasks(List带page/pageSize/status/tag参数), POST /api/v1/tasks/:id/cancel(Cancel)。端点将 REST JSON 请求转换为 service 层调用，响应转换为 MCP Client 期望的 {ok:true,data:{...}} 格式



## 验收标准 (acceptance)


- 4个端点注册到mux并响应正确

- 响应格式匹配APIResponse(ok/data/error)

- 认证走现有中间件

- connect-go路由不受影响

- go test ./internal/gateway/... 通过




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

新增 internal/gateway/rest_adapter.go。每个端点: 解析URL参数/请求体->构造service.Request->调用service方法->包装为APIResponse JSON返回


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