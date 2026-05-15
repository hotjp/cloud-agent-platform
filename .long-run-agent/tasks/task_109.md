# task_109

## Requirements
把以上所有验证步骤整合成一个脚本：docker-compose up → go run server → 跑所有验证 → 输出报告。作为CI/CD的基础。

## Acceptance
- [ ] scripts/verify-all.sh 能一键运行
- [ ] 输出每个验证步骤的PASS/FAIL
- [ ] 最终输出汇总报告

## Design
bash脚本，依次调上面各验证步骤，收集结果。

## Verification
- [ ] 对应的API/工具调用返回正确结果
- [ ] 无panic/error
