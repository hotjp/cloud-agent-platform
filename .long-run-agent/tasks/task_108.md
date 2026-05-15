# task_108

## Requirements
通过MCP协议连接server，验证9个Tools和4个Resources能被正确发现和调用。这是AI Agent（如我）接入平台的核心接口。

## Acceptance
- [ ] MCP客户端能连接并发现9个Tools
- [ ] 能调用submit_task工具提交任务
- [ ] 能调用get_task工具查询任务
- [ ] 能列出4个Resources
- [ ] 调用结果与REST API一致

## Design
用MCP客户端（如mcporter或内部MCP client）连接server的MCP端点，枚举工具并调用。

## Verification
- [ ] 对应的API/工具调用返回正确结果
- [ ] 无panic/error
