# task_103

## Requirements
roles.go:815 role not found 和 tool.go:133 tool not found 用了 panic，运行时找不到应返回 error 不崩溃进程。

## Acceptance
- [x] 两个函数签名改为返回 error
- [x] 调用方处理 error
- [x] go test ./internal/agent/... 通过

## Design
将 panic 改为 fmt.Errorf 返回 error，调用方检查 error 做降级处理。

## Deliverables
- internal/agent/react/tool.go: MustGet 返回 (Tool, error)
- internal/agent/roles/roles.go: MustGet 返回 (RoleDefinition, error)
- internal/agent/react/react_test.go: 添加 MustGet 和 MustGetError 测试
- internal/agent/roles/roles_test.go: 更新 MustGet 测试用例
