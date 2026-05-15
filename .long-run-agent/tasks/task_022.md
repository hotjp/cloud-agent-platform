# task_022

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_022.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/orchestrator/matcher.go`

产出类型：
- `AgentMatcher` struct — Agent 匹配器

### 2. 契约参考

```go
// 匹配算法
func (m *AgentMatcher) Match(subtask *Subtask) *AgentTemplate {
    // 综合得分 = 能力匹配度×0.5 + 历史成功率×0.3 + 成本效率×0.2
    // 排序后选择第一名
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `plugins/orchestrator.AgentTemplate` — T42a 定义

**本层产出（其他 Task 会依赖的）**：
- `plugins/orchestrator.AgentMatcher` — 匹配器

### 4. 约定

- Plugin 层实现
- 历史成功率从 AuditLog 统计
- 成本效率基于模型定价

### 5. 验收标准

- 测试命令：`go test ./plugins/orchestrator/... -v -run TestMatcher`
- 必须覆盖的 case：
  1. 匹配计算正确
  2. 排序正确
- Done 判定：测试通过 + `go build ./...`

## 描述

T43: Agent匹配算法 - 能力评分×0.5+历史成功率×0.3+成本效率×0.2



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T43:_Agent匹配算法_-_能力评.py




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