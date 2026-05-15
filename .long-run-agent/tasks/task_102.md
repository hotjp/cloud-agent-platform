# task_102

## Requirements
docker-compose up启动PG+Redis+MinIO，然后go run ./cmd/server，验证/health返回200。

## Acceptance
- [ ] 服务正常启动
- [ ] curl localhost:8080/health返回200

## Design
先启动依赖，等healthy后启动server，验证healthcheck。

## Verification
- [ ] go build ./... passes
- [ ] Related tests pass
