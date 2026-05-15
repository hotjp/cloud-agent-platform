# task_031

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_031.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/compression/engine.go`

产出类型：
- `CompressionEngine` struct — 压缩引擎
- `RuleCompressor` — L1 规则压缩
- `LLMCompressor` — L3 LLM 压缩

### 2. 契约参考

```go
type CompressionEngine struct {
    l1 *RuleCompressor
    l3 *LLMCompressor
}

// 必须保留：goal, constraints, user_decisions, error_log
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `internal/domain.TaskContext` — T44 定义
- `plugins/llmrouter.LLMCaller` — T61 定义

**本层产出（其他 Task 会依赖的）**：
- `plugins/compression.CompressionEngine` — 压缩引擎

### 4. 约定

- Plugin 层实现
- L1 零成本
- L3 高成本，按需触发

### 5. 验收标准

- 测试命令：`go test ./plugins/compression/... -v`
- 必须覆盖的 case：
  1. L1 压缩正常
  2. L3 压缩正常
- Done 判定：测试通过 + `go build ./...`

## 描述

T62: 上下文压缩引擎 - L1规则压缩+L3 LLM智能压缩(必须保留goal/constraints/user_decisions/error_log)



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T62:_上下文压缩引擎_-_L1规则压.py




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