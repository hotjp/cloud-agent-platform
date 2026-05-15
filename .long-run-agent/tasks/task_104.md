# task_104

## Requirements
启动server后，用connect-go客户端（或buf curl）调用Submit提交一个简单任务，然后用Get查询单个任务，用List列出所有任务。验证三个基本CRUD操作能走通。不需要真正执行任务，只验证API链路。

## Acceptance
- [ ] curl/buf curl 能调通 /cap.v1.TaskService/Submit
- [ ] curl/buf curl 能调通 /cap.v1.TaskService/Get
- [ ] curl/buf curl 能调通 /cap.v1.TaskService/List
- [ ] 返回正确的protobuf JSON响应

## Design
写一个scripts/e2e-verify.sh脚本：启动server → 用buf curl或curl发送connect-go格式的JSON请求 → 验证响应。或者写一个Go test在test/e2e/里直接调connect客户端。

## Verification
- [ ] 对应的API/工具调用返回正确结果
- [ ] 无panic/error
