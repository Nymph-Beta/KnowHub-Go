## 阶段六 MinIO 对象存储 & 简单文件上传

### 为什么需要对象存储

RAG 系统中用户上传的文档（PDF、Word 等）需要专业的存储方案：


| 方案                 | 问题                     |
| ------------------ | ---------------------- |
| 服务器本地磁盘            | 不可扩展，服务迁移后丢失           |
| 数据库 BLOB           | 数据库不擅长存大文件，性能差         |
| **对象存储（MinIO/S3）** | **专业文件存储，天然支持分布式和大文件** |


MinIO 是 S3 兼容的开源对象存储，本地开发时模拟 AWS S3。

核心概念：

- **Bucket（桶）**：类似根目录，所有文件都存在某个桶里
- **Object（对象）**：桶里的单个文件，由 key（路径）标识
- **ObjectName/Key**：对象的唯一标识，例如 `uploads/1/abc123/test.pdf`

MinIO vs MySQL 的分工：MySQL 存**结构化元数据**（文件名、大小、MD5、归属用户），MinIO 存**文件本体**（二进制内容）。

---

### 1. 启动 MinIO 并扩展配置

**MinIO 容器启动**

使用 Docker 独立启动，需要两个端口：

- API 端口（9300 → 9000）：代码通过此端口与 MinIO 交互
- Console 端口（9301 → 9001）：浏览器管理界面

```bash
docker run -d \
  --name minio-v2 \
  --restart always \
  -p 9300:9000 \
  -p 9301:9001 \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin \
  -v minio-v2-data:/data \
  minio/minio:RELEASE.2025-04-22T22-12-26Z \
  server /data --console-address ":9001"
```

端口选择说明：默认的 9000 和 9001 可能被其他服务占用（如 Elasticsearch 占 9200），所以映射到 9300/9301。

**配置扩展**

`configs/config.yaml` 新增：

```yaml
minio:
  endpoint: "127.0.0.1:9300"
  access_key: "minioadmin"
  secret_key: "minioadmin"
  use_ssl: false
  bucket: "uploads"
```

`internal/config/config.go` 新增 `MinIOConfig`：


| 字段                | 类型       | 含义                               |
| ----------------- | -------- | -------------------------------- |
| `Endpoint`        | `string` | MinIO 服务地址（API 端口，不是 Console 端口） |
| `AccessKeyID`     | `string` | 访问密钥（相当于用户名）                     |
| `SecretAccessKey` | `string` | 密钥（相当于密码）                        |
| `UseSSL`          | `bool`   | 是否启用 HTTPS（本地开发为 false）          |
| `BucketName`      | `string` | 默认桶名                             |


---

### 2. MinIO 客户端模块

新建：`pkg/storage/minio.go`

设计与 `database.InitMySQL`、`database.InitRedis` 一致：全局单例 + 启动时初始化 + 失败即 Fatal。

`InitMinio` 做两件事：

1. **创建客户端**：`minio.New(endpoint, &minio.Options{Creds, Secure})`
2. **确保桶存在**：`BucketExists` 检查 → 不存在则 `MakeBucket` 自动创建

```text
三个基础设施模块的统一模式：
InitMySQL  → 全局 database.DB        → 启动连接 + 配置连接池
InitRedis  → 全局 database.RDB       → 启动连接 + Ping 测试
InitMinio  → 全局 storage.MinIOClient → 启动连接 + 确保 Bucket 存在
```

##### 知识点：minio-go/v7 SDK

- `minio.New()` — 创建客户端，不会立即建立网络连接（惰性连接）
- `BucketExists()` — 第一次真正与 MinIO 通信的调用，兼做连通性测试
- `credentials.NewStaticV4(accessKey, secretKey, "")` — 使用静态凭证，第三个参数是 session token（留空即可）

---

### 3. 文件上传模型

新建：`internal/model/upload.go`

```go
type FileUpload struct {
    ID        uint      // 主键
    FileMD5   string    // 文件内容的 MD5 哈希，用于秒传和去重
    FileName  string    // 原始文件名
    TotalSize int64     // 文件大小（字节）
    Status    int       // 状态机：0=上传中, 1=上传完成, 2=上传失败
    UserID    uint      // 上传者 ID
    OrgTag    string    // 归属组织标签（用于权限过滤）
    IsPublic  bool      // 是否公开
    CreatedAt time.Time // 创建时间
    UpdatedAt time.Time // 更新时间
}
```

**为什么用 MD5 而不是 UUID？**

MD5 是基于文件内容计算的哈希，同样的内容一定得到同样的 MD5。这使得"秒传"成为可能：上传前先算 MD5，查数据库是否已有相同哈希 → 有则直接返回，无需重复上传。UUID 是随机生成的，无法实现这个优化。

**Status 状态机**

在阶段六（简单上传）中，上传是一次性完成的，所以 Status 直接设为 1（完成）。到阶段七（分片上传）才会用到 0 → 1 的状态流转。

---

### 4. Repository 层

新建：`internal/repository/upload_repository.go`

阶段六只需要 3 个持久化操作：


| 方法                       | 用途             | 阶段   |
| ------------------------ | -------------- | ---- |
| `Create`                 | 上传成功后记录文件元信息   | 阶段 6 |
| `FindByFileMD5AndUserID` | 秒传检查 + 下载查找    | 阶段 6 |
| `FindByID`               | 根据 ID 查找记录（预留） | 阶段 6 |


**为什么查询条件是 "MD5 + UserID" 而不仅仅是 "MD5"？**

不同用户上传同一个文件，在我们的设计中记录为**两条记录**。原因：

- 文件归属不同用户，权限隔离（用户 A 的文件不应该在用户 B 的列表中出现）
- OrgTag 可能不同（同一文件可以属于不同组织）
- 后续阶段的权限过滤依赖 UserID 维度

---

### 5. Service 层

新建：`internal/service/upload_service.go`

#### SimpleUpload 核心流程

```text
请求文件
  │
  ▼
┌──────────────────────────────────────┐
│ 0a. 校验文件扩展名（白名单）          │  ← 拦截 RAG 管线不支持的格式
│ 0b. OrgTag 为空 → 用 PrimaryOrg 回填 │  ← 确保权限过滤不遗漏
│ 1. io.TeeReader 边读边算 MD5          │  ← 一次遍历同时完成读取和哈希
│ 2. FindByFileMD5AndUserID             │  ← 秒传检查
│    ├─ 命中 → 返回已有记录（秒传）     │
│    └─ 未命中 → 继续                   │
│ 3. minioClient.PutObject              │  ← 上传到 MinIO
│ 4. uploadRepo.Create                  │  ← 写入 DB 记录
│ 5. 返回 UploadResult                  │
└──────────────────────────────────────┘
```

#### 关键技术点

**1. `io.TeeReader` 实现边读边算 MD5**

```go
hasher := md5.New()
teeReader := io.TeeReader(reader, hasher)
fileBytes, _ := io.ReadAll(teeReader)  // 读取内容的同时自动写入 hasher
fileMD5 := hex.EncodeToString(hasher.Sum(nil))
```

`TeeReader` 像一个 T 型管道：数据从 reader 流入，同时写入 hasher 和 fileBytes。避免了先读一遍算 MD5、再读一遍上传的两次遍历。

**2. MinIO PutObject 参数**

```go
minioClient.PutObject(ctx, bucketName, objectKey, reader, size, options)
```


| 参数           | 含义                                  |
| ------------ | ----------------------------------- |
| `bucketName` | 桶名（如 "uploads"）                     |
| `objectKey`  | 对象路径（如 `uploads/1/abc123/test.pdf`） |
| `reader`     | 文件内容的 io.Reader                     |
| `size`       | 文件大小（字节），-1 表示未知                    |
| `options`    | ContentType 等选项                     |


**3. 对象键设计**

```
uploads/<userID>/<md5>/<原始文件名>
```

按用户隔离 + MD5 去重 + 保留原始文件名，便于调试和运维时在 MinIO Console 中查看。

**4. 哨兵错误**


| 错误                       | 含义                     | HTTP 映射 |
| ------------------------ | ---------------------- | ------- |
| `ErrUnsupportedFileType` | 文件类型不在 RAG 管线支持范围内     | 400     |
| `ErrFileAlreadyExists`   | 秒传场景（目前直接返回成功，未作为错误抛出） | 409     |
| `ErrFileNotFound`        | 下载时文件记录不存在             | 404     |
| `ErrUploadFailed`        | 上传到 MinIO 失败           | 500     |


**5. 文件类型白名单**

只接受 RAG 管线能处理的格式，上传时在读取文件内容之前就校验，提前拦截无效上传：

```go
var allowedExtensions = map[string]bool{
    ".pdf": true, ".docx": true, ".doc": true,
    ".txt": true, ".md": true, ".csv": true,
    ".xlsx": true, ".xls": true, ".pptx": true,
}
```

**6. 依赖注入设计**

```go
type uploadService struct {
    uploadRepo  repository.UploadRepository  // 文件元数据 DB 操作
    userRepo    repository.UserRepository    // 用于 OrgTag 回填时查询用户信息
    minioClient *minio.Client                // 对象存储
    bucketName  string                       // 桶名
}
```

注入 `minioClient` 而非直接使用全局变量 `storage.MinIOClient`，与 `UserService` 注入 `userRepo` 的风格一致，便于单测时 mock。

注入 `userRepo` 是为了支持 OrgTag 默认回填：上传时如果 `orgTag` 为空，自动用用户的 `PrimaryOrg` 填充，确保后续权限过滤不会遗漏。

**7. DownloadFile 方法**

下载逻辑也封装在 Service 中（而非 Handler 直接访问 MinIO 全局变量），返回 `DownloadResult`：

```go
type DownloadResult struct {
    FileName    string        // 原始文件名（用于 Content-Disposition）
    ContentType string        // MIME 类型
    Size        int64         // 文件大小（用于 Content-Length）
    Reader      io.ReadCloser // MinIO 对象流（调用方需 Close）
}
```

这样 Handler 不需要导入 `pkg/storage` 或 `minio-go`，只做 HTTP 响应翻译，保持分层一致性。

**8. 使用标准库 `bytes.NewReader`**

上传到 MinIO 时，用标准库 `bytes.NewReader(fileBytes)` 包装已读取的字节切片，无需自定义 Reader 实现。`PutObject` 的参数类型是 `io.Reader`，不需要 `io.ReadCloser`，所以也不需要 `io.NopCloser` 包装。

---

### 6. Handler 层

新建：`internal/handler/upload_handler.go`

两个接口：

#### POST /api/v1/upload/simple — 简单上传

```text
multipart/form-data
├── file: 文件（必选）
└── orgTag: 组织标签（可选）

响应：
{
  "code": 200,
  "message": "Upload successful",
  "data": {
    "fileMd5": "abc123...",
    "fileName": "test.pdf",
    "totalSize": 1048576,
    "isQuick": false        // true 表示秒传命中
  }
}
```

关键 API：

- `c.Request.FormFile("file")` — 返回 `(multipart.File, *multipart.FileHeader, error)`
- `multipart.File` 实现了 `io.Reader`，可以直接传给 Service
- `header.Filename` / `header.Size` — 获取原始文件名和大小

#### GET /api/v1/documents/download?fileMd5=xxx — 文件下载

```text
流程：调用 Service.DownloadFile() → 获取 DownloadResult → 设置响应头 → 流式返回

响应头：
Content-Disposition: attachment; filename="test.pdf"  ← 触发浏览器下载
Content-Type: application/octet-stream
Content-Length: 1048576
```

Handler 只做 HTTP 翻译，所有 MinIO 交互封装在 Service 的 `DownloadFile` 方法中：

- Service 内部：`FindByFileMD5AndUserID()` → 拼 objectKey → `minioClient.GetObject()` → `object.Stat()` → 返回 `DownloadResult`
- Handler：从 `DownloadResult` 读取 `FileName`、`ContentType`、`Size`、`Reader` → `c.DataFromReader()` 流式响应

Handler 不再直接导入 `pkg/storage` 或 `minio-go`，保持与 Upload 一致的 DI 模式。

---

### 7. 错误映射扩展

更新：`internal/handler/helper.go`

在 `mapServiceError` 中新增 4 条上传相关错误映射：

```text
ErrUnsupportedFileType → 400 Bad Request
ErrFileAlreadyExists   → 409 Conflict
ErrFileNotFound        → 404 Not Found
ErrUploadFailed        → 500 Internal Server Error
```

保持统一的错误处理模式：Service 抛哨兵错误 → Handler 通过 `mapServiceError` 统一转换为 HTTP 状态码。

---

### 8. 路由注册与依赖注入

更新：`cmd/server/main.go`

初始化链：

```text
config → storage.InitMinio(cfg.MinIO)
       → uploadRepo := NewUploadRepository(database.DB)
       → uploadService := NewUploadService(uploadRepo, userRepo, storage.MinIOClient, cfg.MinIO.BucketName)
       → uploadHandler := NewUploadHandler(uploadService)
```

注意：`NewUploadService` 额外注入了 `userRepo`（用于 OrgTag 回填），`NewUploadHandler` 不再需要 `bucketName`（MinIO 交互已完全封装在 Service 中）。

路由注册：

```go
upload := r.Group("/api/v1")
upload.Use(middleware.AuthMiddleware(jwtManager, userService))  // 需要登录
{
    upload.POST("/upload/simple", uploadHandler.SimpleUpload)
    upload.GET("/documents/download", uploadHandler.Download)
}
```

更新：`pkg/database/mysql.go`

`RunMigrate` 中 AutoMigrate 添加 `&model.FileUpload{}`，启动时自动建表。

---

### 9. 整体架构

```text
Router
 └── /api/v1/upload/simple      → UploadHandler.SimpleUpload
     /api/v1/documents/download → UploadHandler.Download
                                    │
                              UploadService
                              ├── UploadRepository  → MySQL（文件元数据）
                              ├── UserRepository    → MySQL（OrgTag 回填时查用户）
                              └── MinIOClient       → MinIO（文件本体）
```

数据流向：

```text
上传：客户端 → Handler(解析form) → Service(校验扩展名+OrgTag回填+MD5+秒传+PutObject+写DB) → MinIO + MySQL
下载：客户端 → Handler(解析参数) → Service(查DB+GetObject) → Handler(设置响应头+流式返回) → 客户端
```

Handler 不直接接触 MinIO，所有存储交互封装在 Service 内，保持分层一致性。

---

### 本阶段更新文件


| 操作  | 文件                                                     |
| --- | ------------------------------------------------------ |
| 新增  | `pkg/storage/minio.go` — MinIO 客户端初始化                  |
| 新增  | `internal/model/upload.go` — FileUpload 模型             |
| 新增  | `internal/repository/upload_repository.go` — 文件记录 CRUD |
| 新增  | `internal/service/upload_service.go` — 上传/下载业务逻辑       |
| 新增  | `internal/handler/upload_handler.go` — HTTP 接口         |
| 更新  | `internal/config/config.go` — 添加 MinIOConfig           |
| 更新  | `configs/config.yaml` — 添加 minio 配置                    |
| 更新  | `internal/handler/helper.go` — 添加上传错误映射                |
| 更新  | `pkg/database/mysql.go` — AutoMigrate 添加 FileUpload    |
| 更新  | `cmd/server/main.go` — MinIO 初始化 + 依赖注入 + 路由注册         |


### 验收命令

```bash
# 上传文件
curl -X POST http://localhost:8081/api/v1/upload/simple \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@test.pdf"

# 上传文件（指定组织标签）
curl -X POST http://localhost:8081/api/v1/upload/simple \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@test.pdf" \
  -F "orgTag=my-org"

# 下载文件
curl -O http://localhost:8081/api/v1/documents/download?fileMd5=xxx \
  -H "Authorization: Bearer $TOKEN"
```

---

## 阶段七 分片上传 & Redis Bitmap

### 为什么需要分片上传

阶段六的简单上传将文件内容完整读入内存（`io.ReadAll`）后才上传到 MinIO。小文件（几 MB）没问题，但 RAG 系统用户可能上传几百 MB 甚至 GB 级的 PDF/Word。

分片上传解决三个问题：


| 问题   | 简单上传   | 分片上传           |
| ---- | ------ | -------------- |
| 大文件  | 内存溢出风险 | 每次只传 5MB       |
| 网络中断 | 从头重传   | 只重传失败的分片       |
| 重复上传 | 每次全量传输 | 秒传：相同 MD5 直接跳过 |


### 分片上传的三步协议

```text
客户端                        服务端                     MinIO        Redis
│                            │                          │            │
│── POST /upload/check ─────>│                          │            │
│   { md5: "abc" }          │── 查 DB 记录              │            │
│                            │── 查 Redis bitmap ───────>│            │──>
│<── { completed, chunks } ──│                          │            │
│                            │                          │            │
│── POST /upload/chunk ─────>│                          │            │
│   { fileMd5, index, file } │── PutObject(chunk) ────>│            │
│                            │── SetBit(index, 1) ─────>│            │──>
│<── { uploaded, progress } ─│                          │            │
│        ... 重复 N 次 ...    │                          │            │
│                            │                          │            │
│── POST /upload/merge ─────>│                          │            │
│   { fileMd5, fileName }    │── ComposeObject ────────>│            │
│                            │── 更新 DB status=1       │            │
│                            │── 清理 Redis + chunks ──>│            │──>
│<── { objectUrl } ──────────│                          │            │
```

---

### 1. Redis Bitmap 数据结构

Redis 的 String 类型底层是二进制安全的字节数组。Bitmap 就是在字节数组上以 bit 为单位做读写操作。

**核心命令：**


| 命令                    | 作用              | 时间复杂度 |
| --------------------- | --------------- | ----- |
| `SETBIT key offset 1` | 将第 offset 位设为 1 | O(1)  |
| `GETBIT key offset`   | 读取第 offset 位的值  | O(1)  |


**Key 设计：** `upload:{userID}:{fileMD5}` — 每个用户每个文件一个 bitmap

**为什么用 Bitmap 而不是 Set？**

1000 个分片用 Set 需要约 8KB，用 Bitmap 只需要 125 字节。且 SETBIT/GETBIT 是 O(1) 原子操作，天然并发安全。

**Bitmap 解析：** Redis 在每个字节中使用大端序（big-endian）排列：bit 0 对应 byte[0] 的最高位。解析代码：

```go
for i := 0; i < totalChunks; i++ {
    byteIdx := i / 8
    bitIdx := 7 - (i % 8)
    if byteIdx < len(data) && (data[byteIdx]>>uint(bitIdx))&1 == 1 {
        uploaded = append(uploaded, i)
    }
}
```

---

### 2. ChunkInfo 模型

新增 `ChunkInfo`，与 `FileUpload` 通过 `FileMD5` 关联（1:N）：

```go
type ChunkInfo struct {
    ID          uint      // 主键
    FileMD5     string    // 关联到哪个文件
    ChunkIndex  int       // 第几个分片
    StoragePath string    // MinIO 中的存储路径（如 chunks/abc123/0）
    CreatedAt   time.Time
}
```

同时给 `FileUpload` 新增 `MergedAt *time.Time` 字段，记录合并完成时间。使用指针类型是因为简单上传和上传中的文件没有合并时间，需要表示 NULL。

---

### 3. Repository 层扩展

这是阶段七最大的变化：Repository 从纯 GORM 扩展为 GORM + Redis。

**构造函数变化：**

```go
// 阶段六
func NewUploadRepository(db *gorm.DB) UploadRepository

// 阶段七
func NewUploadRepository(db *gorm.DB, rdb *redis.Client) UploadRepository
```

**新增接口方法：**


| 方法                           | 存储    | 用途                    |
| ---------------------------- | ----- | --------------------- |
| `UpdateFileUploadStatus`     | GORM  | 合并后将 status 从 0 更新为 1 |
| `CreateChunkInfo`            | GORM  | 记录单个分片元信息             |
| `FindChunksByFileMD5`        | GORM  | 查询某文件的所有分片记录          |
| `IsChunkUploaded`            | Redis | GETBIT 检查某分片是否已上传     |
| `MarkChunkUploaded`          | Redis | SETBIT 标记分片已上传        |
| `GetUploadedChunksFromRedis` | Redis | 读取 bitmap 解析所有已上传分片索引 |
| `DeleteUploadMark`           | Redis | 合并完成后删除 bitmap key    |


**Redis key 生成：**

```go
func uploadBitmapKey(userID uint, fileMD5 string) string {
    return fmt.Sprintf("upload:%d:%s", userID, fileMD5)
}
```

---

### 4. 分片大小与总分片数

```go
const DefaultChunkSize int64 = 5 * 1024 * 1024 // 5MB

func calcTotalChunks(totalSize int64) int {
    return int((totalSize + DefaultChunkSize - 1) / DefaultChunkSize)
}
```

使用整数除法（向上取整）代替 `math.Ceil` 避免浮点精度问题。

5MB 是一个平衡点：太小（1MB）导致分片数量过多，HTTP 请求开销大；太大（100MB）则失去分片上传的优势，且 MinIO ComposeObject 对源对象有最小 5MB 限制。

---

### 5. Service 层 — 三个新方法

保留 `SimpleUpload`（阶段六成果），新增分片上传能力。

#### 5.1 CheckFile — 秒传 & 断点续传

```text
fileMD5 + userID
  │
  ├─ DB 无记录 → { completed: false, uploadedChunks: [] }（全新上传）
  ├─ status=1  → { completed: true }（秒传命中）
  └─ status=0  → 从 Redis bitmap 读取已上传分片列表 → { completed: false, uploadedChunks: [0,1,3,...] }
```

#### 5.2 UploadChunk — 单分片上传

```text
1. 校验文件扩展名 + OrgTag 回填
2. 计算 totalChunks = ceil(totalSize / 5MB)
3. FindOrCreate FileUpload（首个分片创建，后续复用）
4. GETBIT 幂等检查 → 已上传则跳过
5. PutObject 到 chunks/{fileMD5}/{chunkIndex}
6. 创建 ChunkInfo DB 记录
7. SETBIT 标记分片
8. 返回 { uploadedChunks, progress }
```

**幂等性保证：** 重复上传同一分片不报错，通过 GETBIT 检测跳过重复的 PutObject，直接返回当前进度。

**对象路径设计：** 分片使用 `chunks/` 前缀而非 `uploads/`，因为这些是临时对象，合并后清理。

#### 5.3 MergeChunks — 合并分片

```text
1. 查 DB 获取 FileUpload（拿到 totalSize）
2. 从 Redis bitmap 验证 len(uploadedChunks) == totalChunks
3. MinIO 合并：
   ├─ 单分片 → CopyObject（chunks/md5/0 → uploads/uid/md5/name）
   └─ 多分片 → ComposeObject（多个源对象合并为一个）
4. 更新 DB：status=1, mergedAt=now
5. 异步 goroutine 清理：
   ├─ 删除 Redis bitmap key
   └─ 逐个删除 MinIO 中的 chunks/{md5}/* 临时对象
```

**为什么区分单分片和多分片？** MinIO 的 `ComposeObject` 要求每个非最后源对象 >= 5MB。如果文件只有一个分片且小于 5MB，`ComposeObject` 会报错，所以用 `CopyObject` 处理。

**异步清理：** 合并成功后立即返回响应，清理操作放在 goroutine 中执行，避免客户端等待。使用 `context.Background()` 创建独立的 context，不会因请求 context 取消而中断清理。

---

### 6. Handler 层 — 三个端点


| 端点                   | 请求格式                | 关键参数                                                               |
| -------------------- | ------------------- | ------------------------------------------------------------------ |
| `POST /upload/check` | JSON                | `md5`                                                              |
| `POST /upload/chunk` | multipart/form-data | `fileMd5, fileName, totalSize, chunkIndex, orgTag, isPublic, file` |
| `POST /upload/merge` | JSON                | `fileMd5, fileName`                                                |


**UploadChunk 注意点：** form 字段中的 `totalSize` 和 `chunkIndex` 是字符串，需要 `strconv.ParseInt` / `strconv.Atoi` 转换为数值类型。`isPublic` 接受 `"true"` 或 `"1"` 作为 true 值。

---

### 7. 错误映射扩展

在 `mapServiceError` 中新增：


| 哨兵错误                  | HTTP 状态码                  | 场景                                |
| --------------------- | ------------------------- | --------------------------------- |
| `ErrChunksIncomplete` | 400 Bad Request           | 合并时发现还有分片未上传                      |
| `ErrMergeFailed`      | 500 Internal Server Error | MinIO ComposeObject/CopyObject 失败 |


---

### 8. 路由注册与依赖注入

初始化链更新：

```text
uploadRepo := NewUploadRepository(database.DB, database.RDB)  ← 注入 Redis 客户端
uploadService := NewUploadService(uploadRepo, userRepo, storage.MinIOClient, cfg.MinIO.BucketName)
uploadHandler := NewUploadHandler(uploadService)
```

路由注册（在已有的 upload 组中追加）：

```go
upload.POST("/upload/check", uploadHandler.CheckFile)
upload.POST("/upload/chunk", uploadHandler.UploadChunk)
upload.POST("/upload/merge", uploadHandler.MergeChunks)
```

数据库迁移 `RunMigrate()` 中启用 `&model.ChunkInfo{}`。

---

### 9. 整体架构

```text
Router
 └── /api/v1/upload/simple      → UploadHandler.SimpleUpload    (阶段六)
     /api/v1/upload/check       → UploadHandler.CheckFile       (阶段七)
     /api/v1/upload/chunk       → UploadHandler.UploadChunk     (阶段七)
     /api/v1/upload/merge       → UploadHandler.MergeChunks     (阶段七)
     /api/v1/documents/download → UploadHandler.Download        (阶段六)
                                    │
                              UploadService
                              ├── UploadRepository  → MySQL（FileUpload + ChunkInfo）
                              │                     → Redis（Bitmap 分片状态）
                              ├── UserRepository    → MySQL（OrgTag 回填）
                              └── MinIOClient       → MinIO（分片存储 + 合并）
```

数据流向：

```text
Check:  客户端 → Handler(解析JSON) → Service(查DB+查Redis) → 返回状态
Chunk:  客户端 → Handler(解析form) → Service(幂等检查+PutObject+ChunkInfo+SETBIT) → MinIO + MySQL + Redis
Merge:  客户端 → Handler(解析JSON) → Service(验证完整性+ComposeObject+更新状态) → MinIO + MySQL
                                        └→ goroutine(清理Redis+清理MinIO临时分片)
```

---

### 本阶段更新文件


| 操作  | 文件                                                                                 |
| --- | ---------------------------------------------------------------------------------- |
| 更新  | `internal/model/upload.go` — 新增 ChunkInfo 模型，FileUpload 新增 MergedAt 字段             |
| 更新  | `internal/repository/upload_repository.go` — 注入 Redis，新增 bitmap 操作和 ChunkInfo CRUD |
| 更新  | `internal/service/upload_service.go` — 新增 CheckFile、UploadChunk、MergeChunks        |
| 更新  | `internal/handler/upload_handler.go` — 新增三个分片上传端点                                  |
| 更新  | `internal/handler/helper.go` — 新增 ErrChunksIncomplete、ErrMergeFailed 映射            |
| 更新  | `cmd/server/main.go` — Repository 注入 Redis，注册三个新路由                                 |
| 更新  | `pkg/database/mysql.go` — AutoMigrate 启用 ChunkInfo                                 |


### 验收命令

```bash
# 1. 检查文件（秒传/断点续传）
curl -X POST http://localhost:8081/api/v1/upload/check \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"md5":"abc123"}'

# 2. 上传分片（循环上传每个分片）
for i in {0..9}; do
  curl -X POST http://localhost:8081/api/v1/upload/chunk \
    -H "Authorization: Bearer $TOKEN" \
    -F "fileMd5=abc123" \
    -F "fileName=large.pdf" \
    -F "totalSize=10485760" \
    -F "chunkIndex=$i" \
    -F "file=@chunk_$i.bin"
done

# 3. 合并分片
curl -X POST http://localhost:8081/api/v1/upload/merge \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"fileMd5":"abc123","fileName":"large.pdf"}'
```

---

## 阶段八 Kafka & Apache Tika 文档处理

### 为什么从同步改为异步

阶段七的 `MergeChunks` 已经能把文件合并进 MinIO，但如果在 HTTP 请求里同步做 "下载文件 → 文本提取"，用户会被长耗时阻塞，失败也难重试。

阶段八把流程拆为：

```text
上传完成(SimpleUpload / MergeChunks)
  → 发送 Kafka FileProcessingTask
  → Consumer 拉取任务
  → Processor 从 MinIO 下载
  → Tika 提取文本
```

这样上传接口只负责"入库 + 入存储 + 发任务"，重处理交给后台 worker。

---

### 1. 配置扩展

`internal/config/config.go` 与 `configs/config.yaml` 新增：

- `kafka.brokers/topic/group_id/max_retry/retry_key_ttl_seconds`
- `tika.base_url/timeout_seconds`

默认值：

- `topic: file-processing`
- `group_id: file-processing-group`
- `max_retry: 3`
- `retry_key_ttl_seconds: 86400`（24h）

---

### 2. 任务结构（消息契约）

新增：`pkg/tasks/tasks.go`

```go
type FileProcessingTask struct {
    FileMD5   string `json:"file_md5"`
    FileName  string `json:"file_name"`
    UserID    uint   `json:"user_id"`
    OrgTag    string `json:"org_tag"`
    IsPublic  bool   `json:"is_public"`
    ObjectKey string `json:"object_key"`
}
```

这里明确**不放 `total_size`**。Processor 下载对象和提取文本不依赖这个字段，减少了 Producer/Consumer 的契约面积。

`ObjectKey` 由生产端直接传入，消费端不再拼接路径规则，降低后续路径改动时的耦合风险。

消息字段选择思考：

- 当前字段：`file_md5/file_name/user_id/org_tag/is_public/object_key`
- JSON 命名使用 snake_case，和配置与跨服务消息体风格一致，便于后续多语言消费者接入
- 不带 `total_size`，因为阶段八链路（MinIO 下载 + Tika 提取）不依赖该字段，减少了契约面

---

### 3. Kafka 模块（segmentio/kafka-go）

新增：`pkg/kafka/client.go`

能力拆分：

- Producer：`InitProducer`、`ProduceFileTask`、`CloseProducer`
- Consumer：`StartConsumer`
- 解耦接口：`TaskProcessor`（consumer 只调 `Process`，不关心内部实现）

这里使用 `TaskProcessor` 接口而不是直接依赖具体 `pipeline.Processor`，目的是把"消费控制流"和"业务处理逻辑"解耦：

- Kafka 层专注 Fetch/Commit/重试
- 处理逻辑可以替换（后续阶段叠加分块、向量化）并可单测注入 fake

#### 重试策略（按业务实体 fileMD5 计数）

重试 key 设计为：`kafka:retry:{fileMD5}`

处理失败时：

```text
INCR kafka:retry:{fileMD5}
  ├─ count < max_retry  → 不提交 offset，等待 Kafka 重投递
  └─ count >= max_retry → 提交 offset，跳过毒丸消息
```

处理成功时删除该 key。

这意味着：同一个损坏文件（同 MD5）即便重复发多条消息，也共享失败预算，避免无限浪费；用户修复后重传会产生新 MD5，自然获得新的处理机会。

offset 提交策略思考：

- 如果消息处理失败但 offset 已提交，消息会被视为已消费，失败任务可能永久丢失
- 当前实现只在"成功处理"或"达到最大重试后主动跳过"时提交 offset
- 未达上限时不提交 offset，交给 Kafka 重投递，不做本地 sleep 重试，避免阻塞消费循环

---

### 4. Tika 客户端

新增：`pkg/tika/client.go`

核心行为：

- `PUT {base_url}/tika`
- `Accept: text/plain`
- `Content-Type` 根据文件扩展名推断（`mime.TypeByExtension`）
- 非 200 统一当作错误返回

错误与结果处理思考：

- Tika 非 200：按处理失败走 Kafka 重试/跳过策略，不能当作成功
- 提取结果为空文本：阶段八先记录长度与日志；后续可在阶段九/十引入 OCR 或更细粒度策略

---

### 5. Pipeline Processor

新增：`internal/pipeline/processor.go`

`Process` 流程：

1. 用 `object_key` 从 MinIO 获取对象流
2. 调 `tikaClient.ExtractText`
3. 记录提取成功日志（文本长度）

阶段八只做到文本提取，不做分块和向量化（留给阶段九/十）。

处理细节思考：

- 下载路径直接用消息内 `object_key`，而不是在 consumer 里按规则拼 `uploads/{uid}/{md5}/{name}`
- 0 字节文件当前会进入 Tika 并可能得到空文本；更合理的是在上传阶段增加最小大小校验（可作为后续增强）

---

### 6. UploadService 双入口触发任务

更新：`internal/service/upload_service.go`

新增可注入 `TaskProducer`，并封装 `produceFileTask`：

- `SimpleUpload`：MinIO 上传 + DB 写入成功后发送 Kafka 任务
- `MergeChunks`：合并成功并更新状态后发送 Kafka 任务

发送失败只记日志，不影响上传/合并主流程成功返回。

这里保留了两条触发入口，是为了避免"仅 merge 触发导致 simple 上传文件不进 RAG 管线"的问题。

Kafka 发送失败不回滚上传请求的原因：文件本体与元数据已写成功，接口应返回成功并通过日志告警进入后续补偿。

---

### 7. main.go 启动顺序

更新：`cmd/server/main.go`

新增初始化链路：

```text
InitMinio
→ InitProducer (失败即退出)
→ NewUploadService(..., kafkaProducer)
→ NewTikaClient
→ NewProcessor
→ go StartConsumer(...)
```

运行策略：

- Kafka Producer 初始化失败：阻断启动（fail-fast）
- Tika/Consumer 初始化失败：仅记录错误，服务继续提供上传能力
- 停机时：先 cancel consumer context，再优雅关闭 HTTP 服务

这组策略背后的取舍是：没有 Producer 就无法触发异步任务，属于硬依赖；而 Tika/Consumer 故障不应拖垮核心上传链路，属于可降级能力。

---

### 8. 服务启动方式（与阶段五 1.1 一致）

这里不直接落地 `docker-compose.yml`，先采用"按需逐服务启动"的方式，等项目整体完成后再统一整理 compose。

和阶段五的取舍一致：

- 当前阶段目标是先把代码链路跑通，避免过早维护一份跨阶段的大型 compose 文件
- 只维护 `config.yaml` 一份连接配置（`127.0.0.1:9092` / `127.0.0.1:9998`）
- 部署阶段再统一沉淀到 `deployments/docker-compose.yml`

示例启动命令（手动起 Kafka/Tika）：

```bash
# 1) 创建网络（一次即可）
docker network create paismart-v2-net

# 2) ZooKeeper
docker run -d \
  --name paismart-zookeeper \
  --restart always \
  --network paismart-v2-net \
  -p 2181:2181 \
  -e ZOOKEEPER_CLIENT_PORT=2181 \
  -e ZOOKEEPER_TICK_TIME=2000 \
  confluentinc/cp-zookeeper:7.6.1

# 3) Kafka
docker run -d \
  --name paismart-kafka \
  --restart always \
  --network paismart-v2-net \
  -p 9092:9092 \
  -e KAFKA_BROKER_ID=1 \
  -e KAFKA_ZOOKEEPER_CONNECT=paismart-zookeeper:2181 \
  -e KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=PLAINTEXT:PLAINTEXT,PLAINTEXT_HOST:PLAINTEXT \
  -e KAFKA_LISTENERS=PLAINTEXT://0.0.0.0:29092,PLAINTEXT_HOST://0.0.0.0:9092 \
  -e KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://paismart-kafka:29092,PLAINTEXT_HOST://127.0.0.1:9092 \
  -e KAFKA_INTER_BROKER_LISTENER_NAME=PLAINTEXT \
  -e KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1 \
  confluentinc/cp-kafka:7.6.1

# 4) Apache Tika
docker run -d \
  --name paismart-tika \
  --restart always \
  -p 9998:9998 \
  apache/tika:latest-full
```

---

### 9. 测试补充

- `pkg/kafka/client_test.go`
  - Producer 序列化发送
  - Consumer 失败未达阈值不提交
  - 失败达阈值提交
  - 成功后清理重试 key 并提交
- `pkg/tika/client_test.go`
  - 空配置校验
  - 200 成功路径
  - 非 200 错误路径
  - MIME 推断
- `internal/service/upload_service_test.go`
  - 任务投递 helper 成功/失败分支

---

### 本阶段更新文件


| 操作  | 文件                                                                      |
| --- | ----------------------------------------------------------------------- |
| 更新  | `internal/config/config.go` — 新增 Kafka/Tika 配置结构                        |
| 更新  | `configs/config.yaml` — 增加 Kafka/Tika 默认配置                              |
| 新增  | `pkg/tasks/tasks.go` — FileProcessingTask 消息结构                          |
| 新增  | `pkg/kafka/client.go` — Producer/Consumer + Redis 重试策略                  |
| 新增  | `pkg/tika/client.go` — Tika 文本提取客户端                                     |
| 新增  | `internal/pipeline/processor.go` — MinIO 下载 + Tika 提取                   |
| 更新  | `internal/service/upload_service.go` — SimpleUpload/MergeChunks 双入口发送任务 |
| 更新  | `cmd/server/main.go` — 初始化 Kafka/Tika/Consumer                          |
| 新增  | `pkg/kafka/client_test.go`、`pkg/tika/client_test.go`                    |
| 更新  | `internal/service/upload_service_test.go`                               |


### 验收命令

```bash
# 1) 启动中间件（按需逐服务）
#    参考上文第 8 节的 docker run 命令启动 zookeeper/kafka/tika

# 2) 走简单上传（会触发 Kafka 任务）
curl -X POST http://localhost:8081/api/v1/upload/simple \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@test.pdf"

# 3) 或走分片 + merge（merge 后触发 Kafka 任务）
curl -X POST http://localhost:8081/api/v1/upload/merge \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"fileMd5":"xxx","fileName":"test.pdf"}'

# 4) 观察日志关键词
# [Kafka] 文件处理任务发送成功
# [Consumer] 收到 Kafka 消息
# [Processor] 开始处理文件
# [Processor] Tika 提取文本成功
```

## 阶段九 文本分块 & 文档向量表

阶段八的 `Processor` 只做到"从 MinIO 下载文件并交给 Tika 提取文本"。这能证明异步链路打通了，但还不够支持后续 RAG 检索，因为检索的最小单位不应该是整篇文档，而应该是一段段较短的 chunk。

阶段九的目标就是把阶段八管线继续往后推进：

```text
Kafka 消息
  -> Processor 下载对象
  -> Tika 提取原始文本
  -> 按 rune 做固定长度 + overlap 分块
  -> 删除旧 chunk 记录（幂等）
  -> 批量写入 document_vectors
```

---

### 1. DocumentVector 模型

新增：`internal/model/document_vector.go`

```go
type DocumentVector struct {
    ID           uint
    FileMD5      string
    ChunkID      int
    TextContent  string
    ModelVersion string
    UserID       uint
    OrgTag       string
    IsPublic     bool
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

字段取舍：

- `file_md5 + chunk_id` 标识"同一文件的第几个文本块"
- `text_content` 存 chunk 原文，使用 `text` 类型而不是 `varchar`
- `model_version` 先保留为空，阶段九不做 embedding，阶段十才会真正用到
- `user_id / org_tag / is_public` 直接冗余到 chunk 级别，后续检索时可以直接按权限过滤，而不用每次回表查 `file_uploads`

表名保持为 `document_vectors`，与学习文档和原项目命名一致。

---

### 2. DocumentVectorRepository

新增：`internal/repository/document_vector_repository.go`

提供三个核心能力：

- `BatchCreate(vectors []model.DocumentVector)`：使用 `CreateInBatches(..., 100)` 分批写入
- `FindByFileMD5(fileMD5 string)`：按 `chunk_id ASC` 查询一个文件的全部分块
- `DeleteByFileMD5(fileMD5 string)`：删除旧记录，保证重复消费同一文件消息时不会累积重复 chunk

Repository 的实现重点不在于提供增删查本身，而在于明确这一层对后续检索链路承担的职责。虽然阶段九尚未引入真正的向量字段，但模型中仍保留了 `model_version`。这一字段的意义在于为后续 embedding 模型演进预留锚点：一旦阶段十发生模型切换，旧向量与新向量将不再处于同一语义空间，系统必须能够区分不同模型版本下生成的结果，以支持重建、灰度切换和回滚。

同样，`chunk_id` 不能由数据库主键 `id` 替代。自增主键仅表示记录的存储顺序，而 `chunk_id` 表示文档内部的语义片段顺序。后续检索命中 chunk 之后，如果需要扩展上下文、拼接相邻片段或恢复原始顺序，依赖的必须是稳定的 `chunk_id`，而不是每次重建时都会变化的自增值。因此，这里实际采用的是 `file_md5 + chunk_id` 作为文档内部顺序标识，而将主键保留为数据库实现细节。

写入策略采用 `CreateInBatches(..., 100)`，而不是一次性构造超长 `INSERT`。原因并非单纯出于性能考虑，而是为了控制单次 SQL 的体积。文档被切块后产生几十条甚至上百条 chunk 是常态，一次性写入容易触发 MySQL `max_allowed_packet` 或 SQL 过长问题。分批写入可以将行为限定在更可预测的范围内，使实现对不同文档体量更稳健。

`DeleteByFileMD5` 则直接服务于幂等性目标。Kafka 链路天然满足至少一次投递语义，重复消费同一文件消息应被视为常态输入，而非异常分支。如果缺少删除旧记录这一步，同一个 `file_md5` 每重放一次消息都会累积一组新的 chunk，最终使 `document_vectors` 偏离真实状态。因此，阶段九采用“先删除旧记录，再批量写入新结果”的策略，使重复处理能够收敛到同一最终状态。

这里没有额外引入事务包裹 `Delete + BatchCreate`。这一选择意味着系统接受一个短暂窗口：删除成功而写入失败时，数据库会暂时缺失该文件的 chunk 结果。之所以在阶段九接受这一点，是因为当前数据由 Kafka 驱动生成，且 chunk 本身属于可重建的派生结果；写入失败后，消息仍会重试，系统可以回到最终一致状态。与其在这一阶段提前引入更复杂的事务边界，不如先将实现维持在清晰、可恢复且易于验证的层级。

---

### 3. Processor 扩展：从抽文本到落 chunk

更新：`internal/pipeline/processor.go`

`Processor` 新增依赖：

```go
type Processor struct {
    tikaClient    *tika.Client
    minioClient   *minio.Client
    bucketName    string
    docVectorRepo repository.DocumentVectorRepository
}
```

`Process` 现在的完整流程变为：

1. 校验 `tikaClient / minioClient / docVectorRepo / objectKey`
2. 从 MinIO 获取对象并 `Stat`
3. 调用 Tika 提取文本
4. 空文本直接 warning 并返回，不写库
5. 调用 `splitText(text, 1000, 100)` 做固定长度重叠分块
6. 先 `DeleteByFileMD5`
7. 把 chunk 映射为 `[]DocumentVector`
8. `BatchCreate`
9. 记录分块数和写库成功日志

新增的幂等逻辑：

```text
重复消息到来
  -> 先删旧的 document_vectors
  -> 再写新的 document_vectors
  -> 最终数据库只保留一份当前结果
```

---

### 4. splitText 设计

本阶段最核心的算法就是 `splitText`。

实现选择：

- 按 `[]rune` 而不是 `[]byte` 分块，避免中文被截断成乱码
- 参数固定为 `chunkSize=1000`、`overlap=100`
- 当 `chunkSize <= overlap` 时直接返回错误，避免步长为 0 或负数导致死循环
- 空字符串返回空切片
- 纯空白 chunk 会被跳过

核心逻辑：

```go
runes := []rune(text)
step := chunkSize - overlap

for start := 0; start < len(runes); start += step {
    end := min(start+chunkSize, len(runes))
    chunks = append(chunks, string(runes[start:end]))
    if end == len(runes) {
        break
    }
}
```

这是一种固定长度的重叠分块策略。其优势在于实现简单、行为稳定且结果可预测；代价在于分块边界可能落在句子或段落中间。阶段九选择接受这一权衡，是因为当前目标是先建立可重复、可验证、可持久化的 chunk 生产链路，而不是在一开始就引入更复杂的语义边界切分。

实现中最关键的约束是按 `rune` 而不是 `byte` 分块。该约束并非实现细节，而是中文文档正确处理的前提。UTF-8 编码下，一个汉字通常占 3 个字节；如果按 byte 切片，只要切点落在字符中间，就会直接生成乱码 chunk，并进一步污染后续 embedding 与检索结果。将输入转换为 `[]rune` 后，分块算法的最小单位才真正变为“字符”。

边界条件也在这一层被显式约束。`chunkSize <= overlap` 时，步长 `step = chunkSize - overlap` 将变为 0 或负数，循环无法推进，因此这里直接返回错误，而不是依赖调用方自行规避。空文本返回空切片；文本长度小于 `chunkSize` 时，仅生成一个 chunk；纯空白 chunk 则在写库前被跳过，以避免无检索价值的噪声进入存储层。

参数最终固定为 `chunkSize=1000, overlap=100`，而没有在当前阶段做配置化。该选择并不意味着这组参数是唯一最优值，而是表示它在当前阶段具备较好的工程折中：`1000` 个字符足以承载相对完整的语义片段，不至于过碎；`100` 的 overlap 又能覆盖 chunk 边界附近的上下文，降低跨边界语义断裂的概率。在阶段九，这种稳定、可解释的默认值比过早暴露大量配置更符合实现目标。

---

### 5. main.go 与迁移接线

更新：

- `pkg/database/mysql.go`
- `cmd/server/main.go`

接线内容：

1. `RunMigrate()` 新增 `&model.DocumentVector{}`
2. `main.go` 新建 `docVectorRepo := repository.NewDocumentVectorRepository(database.DB)`
3. `NewProcessor(...)` 注入 `docVectorRepo`

这一步完成之后，阶段八的“上传成功 -> Kafka -> Tika 提取文本”异步链路，第一次产出了数据库侧可直接用于检索准备的中间结果。阶段九在这里有意停留在 chunk 落库，而没有继续向 embedding 推进，目的是保持系统边界清晰。“抽文本并切块”主要关注文本处理、幂等写入和一致性策略；“生成向量并建立索引”则会进一步引入模型选择、向量维度、重建策略和索引同步等另一层复杂度。将 `document_vectors` 先沉淀为稳定中间层，有助于阶段十在此基础上继续扩展，而不必让 `Processor` 同时承担两层职责。

同一思路也体现在元数据设计上。`user_id / org_tag / is_public` 虽然理论上可以通过 `file_uploads` 回表取得，但仍然被冗余到了 chunk 记录中。原因在于，后续检索的天然结果粒度就是 chunk；如果每次命中 chunk 后都需要再回主表做权限过滤，查询链路会更长，也会增加未来与 Elasticsearch 等外部索引保持一致的复杂度。将权限元数据前移到 chunk 级别，可以为后续“检索 + 权限过滤”提供更直接的数据基础。

---

### 6. 测试补充

新增测试：

- `internal/pipeline/processor_test.go`
  - 长文本重叠分块
  - 中文 rune 安全分块
  - 短文本单 chunk
  - 空文本
  - 非法配置（`chunkSize <= overlap`）
  - `buildDocumentVectors` 元数据映射
- `internal/repository/document_vector_repository_test.go`
  - `BatchCreate`
  - `FindByFileMD5`
  - `DeleteByFileMD5`

本地回归结果：

```bash
env GOCACHE=$(pwd)/.tmp/gocache /home/yyy/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.11.linux-amd64/bin/go test ./...
```

结果：

- `internal/pipeline` 通过
- `internal/repository` 通过
- `internal/service` 通过
- 全仓 `go test ./...` 通过

---

### 7. 本阶段更新文件


| 操作  | 文件                                                              |
| --- | --------------------------------------------------------------- |
| 新增  | `internal/model/document_vector.go` — 文档分块模型                    |
| 新增  | `internal/repository/document_vector_repository.go` — 文档分块仓储    |
| 新增  | `internal/repository/document_vector_repository_test.go` — 仓储测试 |
| 更新  | `internal/pipeline/processor.go` — 增加分块、幂等删除和批量写入               |
| 新增  | `internal/pipeline/processor_test.go` — 分块算法测试                  |
| 更新  | `pkg/database/mysql.go` — 迁移 `document_vectors`                 |
| 更新  | `cmd/server/main.go` — 注入 `docVectorRepo` 到 `Processor`         |


---

### 8. 验收方式

```bash
# 1) 上传一个可被 Tika 提取正文的文档
curl -X POST http://localhost:8081/api/v1/upload/simple \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@sample.pdf"

# 2) 观察日志
# [Processor] Tika 提取文本成功: md5=..., textLength=...
# [Processor] 文本分块完成: md5=..., chunks=...
# [Processor] 批量写入 document_vectors 成功: md5=...

# 3) 数据库验证
mysql> SELECT file_md5, chunk_id, LEFT(text_content, 50)
       FROM document_vectors
       WHERE file_md5 = 'xxx'
       ORDER BY chunk_id;
```

到这里，阶段九已经把阶段八的"抽文本"推进成了"抽文本并沉淀 chunk"，阶段十就可以在这个结果集上继续做 embedding 和向量索引，而不必再回头处理原始长文本。

## 阶段十 Elasticsearch & Embedding 集成

阶段十的目标，是把阶段九已经稳定落库的 chunk 继续推进成“可检索的向量文档”：

```text
Kafka 消息
  -> Processor 下载对象
  -> Tika 提取文本
  -> 分块并写入 MySQL(document_vectors)
  -> 从 MySQL 读回刚写入的 chunk
  -> 调用 Embedding API 生成向量
  -> 构建 EsDocument
  -> 批量写入 Elasticsearch
```

这一阶段真正引入了第二个存储系统。`document_vectors` 继续作为事实源，负责沉淀 chunk 原文和权限元数据；Elasticsearch 只承担检索引擎角色，负责存放向量、文本和过滤字段。这样即使 ES 索引损坏、mapping 需要重建，系统也不需要重新走一遍 MinIO 下载和 Tika 提取，只需要从 MySQL 重放即可。

---

### 1. 配置扩展与 ES 文档模型

更新：

- `internal/config/config.go`
- `configs/config.yaml`
- `internal/model/es_document.go`

新增配置：

- `embedding.api_key / base_url / model / dimensions / timeout_seconds`
- `elasticsearch.addresses / index_name / vector_dims / analyzer / search_analyzer / refresh_on_write`

对应新增 `EsDocument`：

```go
type EsDocument struct {
    VectorID     string
    FileMD5      string
    ChunkID      int
    TextContent  string
    Vector       []float32
    ModelVersion string
    UserID       uint
    OrgTag       string
    IsPublic     bool
}
```

`VectorID` 固定为 `{file_md5}_{chunk_id}`。这个设计延续了阶段九对 `chunk_id` 的重视：MySQL 中用 `file_md5 + chunk_id` 表示文档内部稳定顺序，ES 中则进一步把它提升为文档主键。这样重复消费同一 Kafka 消息时，同一个 chunk 在 ES 中会被覆盖而不是累积，幂等语义才真正贯穿到外部索引层。

这里还做了一个刻意的工程取舍：`analyzer` 默认配置为 `standard`，而不是直接写死 `ik_max_word`。原因并不是否定 IK 对中文检索的价值，而是当前仓库还没有配套的 ES Dockerfile 或插件安装流程；如果把 mapping 硬编码成 IK，阶段十在很多本地环境中会卡在“索引建不起来”这一非核心问题上。因此实现保留了 analyzer 的可配置能力，安装好 IK 后只需把配置改为 `ik_max_word / ik_smart` 即可，不必再改代码。

---

### 2. Embedding 客户端

新增：`pkg/embedding/client.go`

`Embedding` 侧采用接口抽象：

```go
type Client interface {
    CreateEmbedding(ctx context.Context, text string) ([]float32, error)
}
```

具体实现使用 DashScope 的兼容 OpenAI `/embeddings` 接口，负责：

- 发送 `Authorization: Bearer {api_key}`
- 传递 `model` 与 `dimensions`
- 处理非 200 响应
- 校验空向量响应
- 校验返回维度与配置一致

这一层显式把“维度一致性”收紧到了客户端内，而不是等到 ES 索引时报错。原因在于，一旦维度不一致，错误本质上属于“模型配置与存储契约不匹配”，越早失败，问题定位越直接；如果等到 bulk 写 ES 才看到 `dense_vector dims mismatch`，排查成本会更高。

---

### 3. Elasticsearch 客户端

新增：`pkg/es/client.go`

ES 客户端使用官方 `github.com/elastic/go-elasticsearch/v8`，提供三个核心能力：

- `EnsureIndex(ctx)`：检查索引是否存在，不存在则创建
- `BulkIndexDocuments(ctx, docs)`：批量写入 `EsDocument`
- `IndexName()`：暴露索引名给上层日志使用

索引 mapping 关键字段：

- `text_content`：`text`
- `vector`：`dense_vector`
- `model_version / file_md5 / org_tag / vector_id`：`keyword`
- `user_id`：`long`
- `is_public`：`boolean`

批量写入使用 `_bulk`，每个文档通过 `_id = vector_id` 写入。相比逐条 `IndexDocument`，bulk 更符合阶段十的目标：文档一旦被切成十几到几十个 chunk，再逐条发请求会放大网络往返和 refresh 成本。这里虽然还没有进一步做 embedding 批量化，但至少先把 ES 写入合并到了单次 bulk 请求中。

`refresh_on_write` 也被保留成了配置项。开发阶段默认开着，可以让 `curl _search` 立即看到刚写入的文档；如果后续进入更高吞吐的环境，再关闭即可，避免每次索引都触发 refresh。

---

### 4. Processor 扩展：从“写 chunk”到“写向量索引”

更新：`internal/pipeline/processor.go`

`Processor` 的依赖现在变为：

```go
type Processor struct {
    tikaClient    *tika.Client
    minioClient   *minio.Client
    bucketName    string
    docVectorRepo repository.DocumentVectorRepository
    embedding     embedding.Client
    esClient      es.Client
    embeddingCfg  config.EmbeddingConfig
}
```

`Process` 的后半段新增流程：

1. 阶段九逻辑照旧：抽文本、分块、删旧记录、批量写入 `document_vectors`
2. 用 `FindByFileMD5` 读回刚写入的 chunk
3. 遍历 chunk 调 `embedding.CreateEmbedding`
4. 把结果映射成 `EsDocument`
5. 调 `esClient.BulkIndexDocuments`
6. 输出 embedding 成功、ES 索引成功、整文件处理完成日志

这里依然坚持“先落 MySQL，再做向量化”。看起来比“直接对内存里的 chunk 生成向量并写 ES”多了一次查询，但这一步实际上在把系统边界进一步稳定下来：只要 `document_vectors` 写成功，后半段即使失败，系统也保留了可重试的中间态。

同样基于这个考虑，`buildDocumentVectors` 现在会直接把 `embeddingCfg.Model` 写入 `model_version`。阶段九里该字段留空，是因为还没有真正发生向量化；到了阶段十，如果仍然让 `model_version` 为空，就等于只把“文本结果”落了库，却没有把“这批文本将对应哪个语义空间”这个关键信息保存下来。把模型名在 chunk 入库时一并写入，可以让 MySQL 与 ES 在模型版本上保持同一份语义标签。

幂等性在这一阶段继续成立：

- MySQL 侧：`DeleteByFileMD5 + BatchCreate`
- ES 侧：同一 chunk 使用同一 `vector_id` 覆盖写
- Kafka 侧：任意中途失败直接 `return error`，让 consumer 走现有重试策略

因此，即使 20 个 chunk 中第 13 个向量化失败，前 12 个已经写入 ES 的文档也不会在重试后产生重复，它们只会被新的同 ID 文档覆盖。

---

### 5. main.go 接线与启动策略

更新：`cmd/server/main.go`

启动阶段新增：

1. 初始化 Embedding 客户端
2. 初始化 Elasticsearch 客户端
3. `EnsureIndex()` 确保索引存在
4. 把 `embeddingClient / esClient / embeddingCfg` 注入 `Processor`

这里沿用了阶段八对 Tika 的容错策略：这些依赖都只服务于后台文档处理链路，不应阻断 HTTP 主服务启动。因此实现采用“初始化失败则跳过 Consumer，但 API 服务照常启动”的策略，而不是把 ES 或 DashScope 失败升级为整个进程 fatal。

这一步还顺手移除了启动时 `fmt.Println(config.Conf)` 的行为。阶段十新增了 `embedding.api_key` 后，这种无差别打印配置的做法会直接把密钥打到标准输出，风险已经从“日志噪音”上升为“敏感信息泄露”，因此这里直接收掉。

---

### 6. 测试补充

新增/扩展测试：

- `pkg/embedding/client_test.go`
  - 请求头与请求体校验
  - 维度不匹配错误
- `pkg/es/client_test.go`
  - 缺失索引时自动创建
  - bulk 请求体构造
  - mapping 默认 analyzer 逻辑
- `internal/pipeline/processor_test.go`
  - `buildDocumentVectors` 写入 `model_version`
  - `buildEsDocument`
  - `vectorizeDocuments` 成功与失败路径

这里测试没有使用 `httptest.NewServer()`，而是改为自定义 `RoundTripper` 驱动 HTTP 客户端。原因不是业务需求，而是当前沙箱环境禁止测试进程监听本地端口；如果继续依赖 `httptest`，测试会因为环境限制而失败。通过 `RoundTripper` 直接模拟响应，可以覆盖同样的协议逻辑，同时保持测试可在当前仓库环境下稳定执行。

本地回归结果：

```bash
env GOTOOLCHAIN=local GOCACHE=$(pwd)/.tmp/gocache \
  /home/yyy/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.11.linux-amd64/bin/go test ./...
```

结果：全仓 `go test ./...` 通过。

---

### 7. 本阶段更新文件

| 操作 | 文件 |
| --- | --- |
| 新增 | `internal/model/es_document.go` — ES 检索文档模型 |
| 新增 | `pkg/embedding/client.go` — DashScope Embedding 客户端 |
| 新增 | `pkg/embedding/client_test.go` — Embedding 客户端测试 |
| 新增 | `pkg/es/client.go` — Elasticsearch 客户端 |
| 新增 | `pkg/es/client_test.go` — ES 客户端测试 |
| 更新 | `internal/config/config.go` — 新增 ES / Embedding 配置 |
| 更新 | `configs/config.yaml` — 增加阶段十配置项 |
| 更新 | `internal/pipeline/processor.go` — 增加向量化与 ES 索引流程 |
| 更新 | `internal/pipeline/processor_test.go` — 增加阶段十辅助逻辑测试 |
| 更新 | `cmd/server/main.go` — 初始化 ES/Embedding 并注入 Processor |
| 更新 | `go.mod` / `go.sum` — 增加 ES 客户端依赖 |

---

### 8. 验收方式

```bash
# 1) 配置 DashScope API Key，并确保 Elasticsearch 已启动

# 2) 上传一个可被 Tika 提取正文的文档
curl -X POST http://localhost:8081/api/v1/upload/simple \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@sample.pdf"

# 3) 观察日志
# [Processor] Tika 提取文本成功: md5=..., textLength=...
# [Processor] 文本分块完成: md5=..., chunks=...
# [Processor] 批量写入 document_vectors 成功: md5=...
# [Processor] Embedding 生成成功: md5=..., chunks=..., dims=2048, model=text-embedding-v4
# [Processor] Elasticsearch 索引成功: md5=..., docs=..., index=knowledge_base
# [Processor] 文件处理成功完成: md5=...

# 4) Elasticsearch 验证
curl 'http://localhost:9200/knowledge_base/_search?size=1'
```

如果后续环境已经安装 IK 插件，可直接把：

- `elasticsearch.analyzer` 改为 `ik_max_word`
- `elasticsearch.search_analyzer` 改为 `ik_smart`

这样阶段十就完成了“从 chunk 到向量索引”的闭环，阶段十一就可以在这个基础上继续实现 KNN + BM25 的混合检索，而不必再回头处理文档解析和索引写入问题。

## 阶段十一 混合检索实现

阶段十一的目标，是把阶段十已经写入 Elasticsearch 的 chunk 真正变成“可读路径”能力：

```text
用户查询
  -> SearchHandler 解析 query/topK
  -> SearchService 标准化查询
  -> Embedding 生成查询向量
  -> Elasticsearch 执行 KNN + BM25 + Rescore
  -> UploadRepository 回填 file_name
  -> 返回 SearchResponseDTO
```

到这里，系统第一次具备了 RAG 中 Retrieval 的完整 HTTP 入口。前十阶段完成的是“文档如何进入系统”，阶段十一开始回答“用户如何把文档再找出来”。

---

### 1. 搜索结果模型与文件名回填

更新：

- `internal/model/es_document.go`
- `internal/repository/upload_repository.go`

阶段十的 `EsDocument` 只覆盖了 ES 中需要落库的字段，但真正返回给前端的搜索结果还缺两个关键信息：

- `score`：告诉调用方这段 chunk 为什么排在前面
- `fileName`：让结果能被用户直接识别，而不只是一个 `file_md5`

因此这里补了 `SearchResponseDTO`，并给 `UploadRepository` 新增 `FindBatchByMD5s`。设计上没有把 `file_name` 冗余进 ES，而是继续把 MySQL 视为上传元数据事实源。这样做的原因不是节省一个字段，而是保持职责边界清晰：ES 负责“找出相关 chunk”，MySQL 负责“补齐文件展示信息”。后续即使文件名修改或补录，也不需要重建整批向量索引。

---

### 2. Elasticsearch 客户端扩展：从只写入到可搜索

更新：

- `pkg/es/client.go`
- `pkg/es/client_test.go`

阶段十的 `es.Client` 只有 `EnsureIndex / BulkIndexDocuments / IndexName`，只能支撑写入链路。阶段十一把它扩展成同时承担检索执行器：

```go
type Client interface {
    EnsureIndex(ctx context.Context) error
    BulkIndexDocuments(ctx context.Context, docs []model.EsDocument) error
    SearchDocuments(ctx context.Context, req SearchRequest) ([]SearchHit, error)
    IndexName() string
}
```

这里没有让 `SearchService` 直接持有官方 `*elasticsearch.Client`，而是继续把“HTTP 调 ES、拼接 DSL、解析 hits”收敛在 `pkg/es` 内。这样业务层看到的是 `SearchRequest / SearchHit` 这种仓库内自有模型，而不是直接操作原始 JSON 响应，边界更稳定。

`SearchDocuments` 内部构建的请求结构是：

- `knn`：对查询向量做语义召回
- `query.bool.must`：对标准化后的文本做 BM25 匹配
- `query.bool.filter`：做 `public / user_id / org_tag` 权限过滤
- `rescore`：对 BM25 命中再做一次窗口内重排

一个值得注意的实现细节是：权限过滤同时出现在 `knn.filter` 和顶层 `query.filter`。原因在于，KNN 召回和 BM25 查询在 ES 中是两条并行评分路径；如果只在一侧加过滤，另一侧仍可能把无权限文档带进候选集。

---

### 3. SearchService：查询标准化与混合检索编排

新增：`internal/service/search_service.go`

`SearchService` 承担的是阶段十一真正的业务逻辑，而不是简单地“转发 ES 请求”：

1. 校验 `query / topK / user`
2. 调用 `GetUserEffectiveOrgTags`
3. 对原始查询执行 `normalizeQuery`
4. 用原始查询生成 embedding
5. 调用 `esClient.SearchDocuments`
6. 批量查 `file_md5 -> file_name`
7. 组装 `SearchResponseDTO`

这里保留了一个很重要的非对称设计：

- 向量检索使用原始查询
- BM25 使用标准化后的查询

原因和阶段十一教学指导一致。Embedding 模型本身对“请问”“是什么”这类口语噪音容忍度较高，但 BM25 是词项匹配，噪音词会直接稀释分数。因此 `normalizeQuery` 做的是轻量清洗，而不是强行改写用户意图。

查询标准化具体包含：

- 转小写
- 移除常见中文口语停用词
- 只保留中文、英文、数字
- 把连续空白折叠为单空格

如果标准化后文本为空，当前实现不会直接让检索失败，而是退回到“仅保留合法字符”的宽松版本，尽量保住 KNN 召回能力。

---

### 4. 权限过滤语义：沿用阶段五的“向下展开”

阶段十一真正把前面埋下的权限元数据用起来了。当前仓库在阶段五实现的是“用户持有某个组织标签时，同时拥有其子标签的可见范围”。因此这里搜索权限也保持同一语义：

- `is_public == true`
- `user_id == 当前用户`
- `org_tag IN GetUserEffectiveOrgTags(user.ID)`

三者满足任一即可命中。

教学指导里特别提醒了“向上展开还是向下展开”的问题。这里没有临时推翻已有实现，而是选择沿用仓库当前已经建立的组织可见性语义。原因很简单：如果上传、列表、权限判断是一套规则，而搜索突然换成另一套继承方向，系统会在最难排查的地方出现“明明能看见文档，却搜不到”的行为分裂。阶段十一优先保证权限语义一致，后续若要调整继承方向，应整体修改阶段五及相关依赖，而不是只改搜索。

---

### 5. Handler 与主程序接线

新增：

- `internal/handler/search_handler.go`
- `internal/handler/search_handler_test.go`

更新：

- `cmd/server/main.go`

新接口为：

```text
GET /api/v1/search/hybrid?query=xxx&topK=10
Authorization: Bearer <token>
```

路由注册在 `/api/v1` 认证分组下，因此搜索与上传、下载一样，默认要求登录。`SearchHandler` 只做 HTTP 层翻译：

- 解析 `query` 和 `topK`
- 从 context 读取当前用户
- 调用 `SearchService.HybridSearch`
- 返回统一 JSON 响应

`main.go` 里也做了一个结构性调整：Embedding 客户端和 ES 客户端不再只为后台 `Processor` 初始化，而是被提升为主程序共享依赖，同时服务于“后台索引写入”和“前台在线搜索”。如果 ES 或 Embedding 初始化失败，HTTP 服务仍然启动，但搜索接口会进入不可用状态，后台 Consumer 也会跳过。这延续了阶段十“外部依赖故障不直接拖垮主进程”的策略。

---

### 6. 测试补充

新增/扩展测试：

- `pkg/es/client_test.go`
  - 混合检索请求体构造
  - 搜索结果解析
  - 空文本查询时退化为 filter-only 查询
- `internal/service/search_service_test.go`
  - 查询标准化
  - 混合检索成功路径
  - embedding 失败路径
  - `topK` 归一化与 `file_md5` 去重
- `internal/handler/search_handler_test.go`
  - 成功响应
  - 非法 `topK`
  - service 错误映射
  - 服务不可用兜底
- `internal/repository/upload_repository_test.go`
  - `FindBatchByMD5s`
- `scripts/acceptance/stage11_acceptance.sh`
  - 复用阶段十验收，继续验证真实搜索链路
  - owner 搜索可命中刚写入 ES 的文档
  - stranger 在私有文档场景下不可命中
  - 非法 `topK` 返回 400

本地回归结果：

```bash
env GOCACHE=$(pwd)/.tmp/gocache \
  /home/yyy/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.11.linux-amd64/bin/go test ./...
```

结果：全仓 `go test ./...` 通过。

---

### 7. 本阶段更新文件

| 操作 | 文件 |
| --- | --- |
| 新增 | `internal/service/search_service.go` — 混合检索业务编排 |
| 新增 | `internal/service/search_service_test.go` — SearchService 测试 |
| 新增 | `internal/handler/search_handler.go` — 搜索 HTTP 端点 |
| 新增 | `internal/handler/search_handler_test.go` — 搜索 Handler 测试 |
| 更新 | `pkg/es/client.go` — 增加混合检索执行能力 |
| 更新 | `pkg/es/client_test.go` — 增加搜索请求/响应测试 |
| 更新 | `internal/model/es_document.go` — 增加 `SearchResponseDTO` |
| 更新 | `internal/repository/upload_repository.go` — 增加 `FindBatchByMD5s` |
| 更新 | `internal/repository/upload_repository_test.go` — 增加批量查文件元数据测试 |
| 更新 | `internal/service/upload_service_test.go` — 补齐仓储 fake 接口 |
| 更新 | `cmd/server/main.go` — 注册搜索路由并共享 ES / Embedding 依赖 |
| 更新 | `docs/LEARNING_PATH.md` — 勾选阶段十一任务 |
| 新增 | `scripts/acceptance/stage11_acceptance.sh` — 阶段十一端到端验收脚本 |
| 更新 | `scripts/acceptance/stage10_acceptance.sh` — 支持导出阶段十结果供阶段十一复用 |
| 更新 | `scripts/acceptance/run_acceptance.sh` — 增加阶段十一入口 |

---

### 8. 验收方式

```bash
# 1) 确保阶段十已经成功把文档写入 Elasticsearch

# 2) 执行混合检索
curl "http://localhost:8081/api/v1/search/hybrid?query=Go语言并发&topK=5" \
  -H "Authorization: Bearer $TOKEN"

# 3) 预期响应
{
  "code": 200,
  "message": "Search successful",
  "data": [
    {
      "fileMd5": "abc123",
      "fileName": "go_tutorial.pdf",
      "chunkId": 3,
      "textContent": "Go语言的并发模型基于 goroutine ...",
      "score": 7.82,
      "userId": 1,
      "orgTag": "dept:tech",
      "isPublic": false
    }
  ]
}
```

到这里，阶段十一已经把系统从“能写入知识库”推进到了“能按权限做混合检索”。阶段十二就可以在这个检索接口之上继续接 WebSocket 对话与上下文拼装，而不必再回头实现 Retrieval 基础能力。

补充：本地已经实际跑通 `scripts/acceptance/stage11_acceptance.sh`。当前脚本不再只截取首个 chunk 前缀，而是优先对已知验收文档使用固定 query 白名单；对 `Hashimoto.pdf`，分别使用一组精确词和一组主题词做断言。一次真实验收的结果是：

- owner 用户上传并索引 `Hashimoto.pdf` 后，精确词 `Tatsunori Hashimoto` 与主题词 `pretrained language models` 均成功命中该文档
- 文档为 `isPublic=false` 时，自动注册的 stranger 用户无法搜索到同一文档
- 非法 `topK=abc` 返回 400
