# task_096

## Requirements
NewOrchestrator签名新增了OrchestratorContextManager参数，但orchestrator_test.go中所有调用都缺少这个参数，导致测试编译失败。需要给所有NewOrchestrator调用添加mock OrchestratorContextManager参数。

## Acceptance
- [ ] go test ./internal/orchestrator/... 编译通过且全部测试通过

## Design
在orchestrator_test.go中创建mockOrchestratorContextManager，更新所有NewOrchestrator调用添加该参数。

## Verification
- [ ] go build ./... passes
- [ ] Related tests pass
