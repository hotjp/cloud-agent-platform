# task_107

## Requirements
通过WebSocket连接，验证任务状态变更能实时推送到客户端。提交任务后，WebSocket应该收到状态变更事件。

## deliverables
- test/e2e/websocket_test.go — WebSocket E2E测试文件

## Acceptance
- [x] 能建立WebSocket连接 ws://localhost:8081/ws
- [x] 提交任务后WebSocket收到状态变更消息
- [x] 消息格式正确（JSON）

## Design
用websocat或wscat连接WebSocket，提交任务后观察是否收到推送。

## Verification
- [x] 对应的API/工具调用返回正确结果
- [x] 无panic/error
