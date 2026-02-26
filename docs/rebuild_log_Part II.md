## 阶段六 MinIO 对象存储 & 简单文件上传

### 为什么需要对象存储

RAG 系统中用户上传的文档（PDF、Word 等）需要专业的存储方案：

| 方案 | 问题 |
| :--- | :--- |
| 服务器本地磁盘 | 不可扩展，服务迁移后丢失 |
| 数据库 BLOB | 数据库不擅长存大文件，性能差 |
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

| 字段 | 类型 | 含义 |
| :--- | :--- | :--- |
| `Endpoint` | `string` | MinIO 服务地址（API 端口，不是 Console 端口） |
| `AccessKeyID` | `string` | 访问密钥（相当于用户名） |
| `SecretAccessKey` | `string` | 密钥（相当于密码） |
| `UseSSL` | `bool` | 是否启用 HTTPS（本地开发为 false） |
| `BucketName` | `string` | 默认桶名 |

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

| 方法 | 用途 | 阶段 |
| :--- | :--- | :--- |
| `Create` | 上传成功后记录文件元信息 | 阶段 6 |
| `FindByFileMD5AndUserID` | 秒传检查 + 下载查找 | 阶段 6 |
| `FindByID` | 根据 ID 查找记录（预留） | 阶段 6 |

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

| 参数 | 含义 |
| :--- | :--- |
| `bucketName` | 桶名（如 "uploads"） |
| `objectKey` | 对象路径（如 `uploads/1/abc123/test.pdf`） |
| `reader` | 文件内容的 io.Reader |
| `size` | 文件大小（字节），-1 表示未知 |
| `options` | ContentType 等选项 |

**3. 对象键设计**

```
uploads/<userID>/<md5>/<原始文件名>
```

按用户隔离 + MD5 去重 + 保留原始文件名，便于调试和运维时在 MinIO Console 中查看。

**4. 哨兵错误**

| 错误 | 含义 | HTTP 映射 |
| :--- | :--- | :--- |
| `ErrUnsupportedFileType` | 文件类型不在 RAG 管线支持范围内 | 400 |
| `ErrFileAlreadyExists` | 秒传场景（目前直接返回成功，未作为错误抛出） | 409 |
| `ErrFileNotFound` | 下载时文件记录不存在 | 404 |
| `ErrUploadFailed` | 上传到 MinIO 失败 | 500 |

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

| 操作 | 文件 |
| :--- | :--- |
| 新增 | `pkg/storage/minio.go` — MinIO 客户端初始化 |
| 新增 | `internal/model/upload.go` — FileUpload 模型 |
| 新增 | `internal/repository/upload_repository.go` — 文件记录 CRUD |
| 新增 | `internal/service/upload_service.go` — 上传/下载业务逻辑 |
| 新增 | `internal/handler/upload_handler.go` — HTTP 接口 |
| 更新 | `internal/config/config.go` — 添加 MinIOConfig |
| 更新 | `configs/config.yaml` — 添加 minio 配置 |
| 更新 | `internal/handler/helper.go` — 添加上传错误映射 |
| 更新 | `pkg/database/mysql.go` — AutoMigrate 添加 FileUpload |
| 更新 | `cmd/server/main.go` — MinIO 初始化 + 依赖注入 + 路由注册 |

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
