# task_111

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_111.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

fix: MCP Client APIResponse 解析空指针防护


## 需求 (requirements)

internal/mcp/client.go 的 do() 方法返回 APIResponse 后各调用方直接访问 apiResp.Error.Code/Message，但当后端返回非标准格式时 apiResp.Error 为 nil 导致 panic。需要在 apiResp.OK==false 且 apiResp.Error==nil 时返回通用错误



## 验收标准 (acceptance)


- client.go 所有方法在 apiResp.Error 为 nil 时不会 panic

- 添加 safeError 辅助函数

- go test ./internal/mcp/... 通过




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

在 client.go 添加 safeError(apiResp) 辅助函数统一处理 nil error 情况，所有 !apiResp.OK 分支调用此函数


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