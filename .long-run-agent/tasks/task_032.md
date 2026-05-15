# task_032

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_032.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/storage/artifact_store.go`

产出类型：
- `ArtifactStore` struct — 产出物存储

### 2. 契约参考

```go
type ArtifactStore struct {
    client *minio.Client
    bucket string
}

func (s *ArtifactStore) Upload(ctx context.Context, artifact *Artifact) (string, error)
func (s *ArtifactStore) Download(ctx context.Context, id string) ([]byte, error)
func (s *ArtifactStore) GetSignedURL(ctx context.Context, id string) (string, error)
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `github.com/minio/minio-go/v7` — MinIO 客户端

**本层产出（其他 Task 会依赖的）**：
- `internal/storage.ArtifactStore` — 产出物存储

### 4. 约定

- L1-Storage 层
- 签名 URL 有效期 1 小时
- TTL 90 天
- 错误码：L1_001 ~ L1_199

### 5. 验收标准

- 测试命令：`go test ./internal/storage/... -v -run TestArtifactStore`
- 必须覆盖的 case：
  1. 上传成功
  2. 下载成功
  3. 签名 URL 正确
- Done 判定：`go build ./...`

## 描述

T63: MinIO冷存储 - 上传/下载/签名URL(有效期1小时)/90天TTL自动清理



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T63:_MinIO冷存储_-_上传/下.py




## 设计方案 (design)

快速任务


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