# task_099

## Requirements
internal/worker/sandbox的TestCubeSandbox_Exec和TestCubeSandbox_Destroy失败。检查测试逻辑并修复。

## Acceptance
- [ ] go test ./internal/worker/sandbox/... 全部通过

## Design
检查测试是否依赖外部CubeSandbox服务，如果是则加环境检测跳过，如果是逻辑错误则修复。

## Verification
- [ ] go build ./... passes
- [ ] Related tests pass
