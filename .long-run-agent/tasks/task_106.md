# task_106

## Requirements
Decide接口接收用户prompt，返回推荐的分解策略和Agent角色。验证路由决策逻辑能工作：简单任务→单Agent，复杂任务→多Agent拆解。

## Acceptance
- [ ] 简单prompt返回single_agent策略
- [ ] 复杂prompt返回multi_agent策略
- [ ] 返回合理的Agent角色推荐

## Design
通过buf curl调用Decide接口，传入不同复杂度的prompt，验证返回的路由决策。

## Verification
- [ ] 对应的API/工具调用返回正确结果
- [ ] 无panic/error
