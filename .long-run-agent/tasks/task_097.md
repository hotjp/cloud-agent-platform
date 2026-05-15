# task_097

## Requirements
GenerateCubeSandboxID和GenerateContainerID使用time.Now().UnixNano()生成ID，快速连续调用会产生相同ID。改用ULID保证唯一性。

## Acceptance
- [x] TestGenerateCubeSandboxID通过
- [x] TestGenerateContainerID通过

## Design
用oklog/ulid替换UnixNano()。

## Verification
- [x] go build ./... passes
- [x] Related tests pass

## deliverables
- plugins/workermanager/docker_backend.go (generateContainerID函数改为使用ULID)
