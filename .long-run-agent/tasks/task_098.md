# task_098

## Requirements
TestDockerBackend_Integration在macOS上失败因为storage-opt需要xfs。非Linux环境跳过。

## Acceptance
- [ ] macOS上go test不报FAIL

## Design
加runtime.GOOS检测，非Linux时t.Skip()。

## Verification
- [ ] go build ./... passes
- [ ] Related tests pass
