# task_117

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_117.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

fix: docker-compose.prod.yml 容器配置错误(PostgreSQL/Redis/MinIO)


## 需求 (requirements)

docker-compose.prod.yml 的 PostgreSQL 因挂载含错误参数的 postgresql.conf 启动失败(stats_fetch_regular_interval不存在)。Redis command 格式有误(requirepass与后续参数混在一起)。MinIO 的 --memory-limit 参数不被支持



## 验收标准 (acceptance)


- docker compose -f docker-compose.prod.yml up -d 全部 healthy

- pg 配置移除不支持参数

- redis command 格式正确

- minio 移除不支持 flag




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

PostgreSQL: 移除 postgresql.conf 中的 stats_fetch_regular_interval。Redis: 修正 command 格式用正确语法。MinIO: 移除 --memory-limit


## 验证证据（完成前必填）

<!-- 标记完成前，请提供以下证据： -->

- [ ] **实现证明**: 简要说明如何实现
- [ ] **测试验证**: 如何验证功能正常（测试步骤/截图/命令输出）
- [ ] **影响范围**: 是否影响其他功能

### 测试步骤
1. 
2. 
3. 

### 验证结果
<!-- 粘贴验证截图、命令输出或测试结果 -->