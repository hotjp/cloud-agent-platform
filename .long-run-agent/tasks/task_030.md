# task_030

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_030.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/llmrouter/router.go`

产出类型：
- `LLMRouter` struct — LLM 路由器
- `LLMCaller` interface — LLM 调用接口

### 2. 契约参考

```go
type LLMCaller interface {
    Call(ctx context.Context, prompt string, model string) (string, error)
}

// 自适应升降级
// 连续3次成功率<80% → 降级
// 连续5次成功率>95% → 升级
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- 无上游依赖

**本层产出（其他 Task 会依赖的）**：
- `plugins/llmrouter.LLMRouter` — 路由器
- `plugins/llmrouter.LLMCaller` — 调用接口

### 4. 约定

- Plugin 层实现
- 支持 Claude 和 GLM 模型
- 升降级策略可配置

### 5. 验收标准

- 测试命令：`go test ./plugins/llmrouter/... -v`
- 必须覆盖的 case：
  1. 路由正确
  2. 升降级正确
- Done 判定：测试通过 + `go build ./...`

## 描述

T61: LLM路由插件 - Claude/GLM多模型路由，自适应升降级



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T61:_LLM路由插件_-_Claud.py




## 设计方案 (design)

快速任务


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