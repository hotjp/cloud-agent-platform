# task_101

## Requirements
确认cmd/server/main.go启动时执行ent schema auto-migration。如果没有需要添加。

## Acceptance
- [ ] server启动后PG中自动创建所有表

## Design
检查main.go中client.Schema.Create()调用。

## Verification
- [ ] go build ./... passes
- [ ] Related tests pass
