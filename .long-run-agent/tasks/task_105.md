# task_105

## Requirements
提交任务后，验证任务能从pending状态流转到running再到completed。这涉及：submit创建任务 → orchestrator拆解 → worker执行 → 状态更新。先用mock worker（不需要真正执行代码），验证状态机流转正确。

## Acceptance
- [x] 提交任务后状态为pending
- [x] orchestrator拆解后子任务创建
- [x] worker认领后状态变为running
- [x] 执行完成后状态变为completed
- [x] 每一步都能通过Get API查到正确状态

## Design
写一个集成测试：启动server + mock worker → 提交任务 → 轮询Get API直到completed → 验证中间状态变化。可能需要先确认worker pool和orchestrator的DI是否正确连接。

## Verification
- [x] 对应的API/工具调用返回正确结果
- [x] 无panic/error

## deliverables
- test/e2e/smoke_test.go: 添加了 TestSmokeE2E_StateTransitions 和 TestSmokeE2E_InvalidStateTransitions 两个E2E测试
