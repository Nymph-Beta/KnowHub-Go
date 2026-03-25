# PaiSmart-Go 从零构建学习路线

## 学习目标
通过从零开始构建一个企业级 RAG 知识库系统，深入学习 Go 后端开发、微服务架构、AI 应用开发。

## 前置知识要求
- Go 基础语法（结构体、接口、goroutine、channel）
- HTTP 协议基础
- SQL 数据库基础
- Docker 基础使用

---

## 🔄 学习模式：渐进式迭代开发

**重要**: 这不是一个"先搭框架再填充"的过程，而是**"持续迭代、逐步增强"**的真实项目开发体验。

### 核心特点

#### ✅ **会频繁回头修改代码**
- 后续阶段会**扩展、重构、增强**之前阶段的代码
- 这不是失误，而是刻意设计的学习路径
- 模拟真实项目：MVP → 迭代 → 优化

#### 📐 **三个发展阶段**

**1. 前期阶段（1-4）：打地基**
```
阶段1: 配置加载
  ↓ (累加)
阶段2: + 日志 + HTTP服务器
  ↓ (累加)
阶段3: + 数据库 + User模型
  ↓ (累加)
阶段4: + JWT认证
```
**特点**: 主要是**累加**，不回头修改

---

**2. 中期阶段（5-10）：迭代增强**
```
阶段5: 扩展User模型 ← 回头修改阶段3/4
       添加 org_tags, role 字段

阶段6: 简单文件上传 (单文件直接上传)

阶段7: 分片上传 ← 重构阶段6
       替换简单上传为分片逻辑
       新增 chunk_info 表

阶段8: Kafka异步处理 ← 增强阶段7
       在 MergeChunks 后发送消息
       新增 processor 处理管道

阶段9: 文本分块 ← 扩展阶段8
       在 processor 中添加分块逻辑
       新增 document_vector 表

阶段10: 向量化 ← 扩展阶段9
        在 processor 中添加embedding
        索引到 Elasticsearch
```
**特点**: **频繁回头修改**，模拟真实迭代

---

**3. 后期阶段（11-12）：整合所有**
```
阶段11: 混合检索
        使用阶段10的ES数据
        使用阶段5的User权限

阶段12: WebSocket对话
        使用阶段11的搜索服务
        使用阶段10的ES索引
        使用阶段5的组织权限
        整合所有前期工作
```
**特点**: **组合使用**之前的所有模块

---

### 🎯 为什么这样设计？

#### 1. **符合真实开发流程**
```
真实项目开发顺序:
MVP(最小可用) → 功能迭代 → 性能优化 → 完善细节

本学习路径:
简单上传(阶段6) → 分片上传(阶段7) → 异步处理(阶段8)
```

#### 2. **学习更有层次感**
- **阶段6**: 专注学习 MinIO 基本操作
- **阶段7**: 在熟悉 MinIO 的基础上学习分片原理
- **阶段8**: 在熟悉上传的基础上学习异步解耦

如果第一次就实现完整的"分片+Kafka"，会：
- 代码太多，难以理解
- 无法聚焦单一技术点
- 出错难以调试

#### 3. **培养重构能力**
- 真实工作中经常需要重构代码
- 学会在保留核心逻辑的基础上扩展功能
- 理解"开闭原则"（对扩展开放，对修改关闭）

---

### 📝 各阶段修改关系表

| 后续阶段 | 会修改哪些前期代码 | 修改类型 |
|---------|------------------|---------|
| 阶段5 | 阶段3的User模型<br>阶段4的Auth中间件 | 扩展字段<br>添加权限逻辑 |
| 阶段7 | 阶段6的上传Handler/Service | 完全重构<br>替换为分片逻辑 |
| 阶段8 | 阶段7的MergeChunks | 增强功能<br>添加Kafka消息 |
| 阶段9 | 阶段8的Processor | 扩展功能<br>添加分块逻辑 |
| 阶段10 | 阶段9的Processor | 扩展功能<br>添加向量化 |
| 阶段11 | 使用阶段5的权限<br>使用阶段10的ES数据 | 组合使用 |
| 阶段12 | 使用阶段11的检索<br>使用阶段5的权限 | 整合所有 |

---

### ✅ 学习建议

#### 1. **预期会回头改代码**
- 阶段5-10会频繁修改之前的代码
- **这是正常的，不要惊讶！**
- 享受迭代优化的过程

#### 2. **使用Git管理版本**
```bash
# 每个阶段完成后打标签
git commit -am "完成阶段3: 数据库&模型"
git tag stage-3

git commit -am "完成阶段5: 扩展User模型支持组织"
git tag stage-5

# 可以随时回看
git checkout stage-3  # 查看未扩展的版本
git checkout stage-5  # 查看扩展后的版本
```

#### 3. **对比原项目代码**
当你重构代码时，对比原项目看看：
- 他们是一次性写完的吗？
- 还是也经历了类似的演化？
- 有哪些可以优化的地方？

#### 4. **写重构笔记**
每次重构时记录：
- 为什么要重构？
- 保留了什么，修改了什么？
- 学到了什么新技术？

---

### 🎓 总结

这个学习路径是：

✅ **渐进式迭代**（不是分块组合）
✅ **会频繁回头修改**（特别是阶段5-10）
✅ **越往后越整合**（阶段11-12整合所有）
✅ **符合真实开发**（MVP → 迭代 → 优化）

**就像搭乐高**:
- 阶段1-4: 搭底座（稳定不变）
- 阶段5-10: 搭主体（会拆掉部分重新搭）
- 阶段11-12: 搭顶部装饰（把所有部分连起来）

**你将体验到真实项目的迭代开发过程！** 🚀

---

## 阶段 1: 项目初始化 & 配置管理 ⚙️
**目标**: 搭建项目骨架，实现配置文件加载

### 学习内容
- Go 项目结构最佳实践
- `go mod` 依赖管理
- Viper 配置库使用

### 实现任务
- [ ] 创建项目目录结构
  ```
  paismart-go-v2/
  ├── cmd/server/main.go
  ├── configs/config.yaml
  ├── internal/config/
  ├── go.mod
  └── .gitignore
  ```
- [ ] 编写 `config.yaml` 配置文件（服务器端口、数据库等）
- [ ] 实现 `internal/config/config.go` 配置加载
- [ ] 在 `main.go` 中加载配置并打印验证

### 验收标准
```bash
go run cmd/server/main.go
# 输出: 成功加载配置，打印关键配置项
```

### 关键技术点
- `viper.SetConfigFile()`, `viper.ReadInConfig()`
- 结构体 tag: `mapstructure:"key"`
- 配置验证和默认值

---

## 阶段 2: 日志系统 & HTTP 服务器 📝
**目标**: 建立日志基础设施，启动基础 HTTP 服务器

### 学习内容
- Zap 结构化日志库
- Gin 框架基础
- HTTP 路由和中间件

### 实现任务
- [ ] 实现 `pkg/log/log.go` (Zap logger 封装)
- [ ] 在 `main.go` 中初始化日志系统
- [ ] 创建 Gin HTTP 服务器
- [ ] 实现健康检查接口 `GET /health`
- [ ] 添加请求日志中间件

### 验收标准
```bash
go run cmd/server/main.go
curl http://localhost:8081/health
# 输出: {"status":"ok"}
# 日志: 打印请求信息
```

### 关键技术点
- `zap.NewProduction()`, `zap.Logger.Sugar()`
- `gin.Default()`, `gin.SetMode()`
- `router.Use(middleware)`
- `graceful shutdown` (signal handling)

---

## 阶段 3: 数据库连接 & 用户模型 💾
**目标**: 连接 MySQL，创建用户表和基础 CRUD

### 学习内容
- GORM ORM 框架
- 数据库连接池配置
- 表结构设计和迁移

### 实现任务
- [ ] 启动 MySQL Docker 容器
  ```bash
  docker run -d -p 3306:3306 -e MYSQL_ROOT_PASSWORD=123456 \
    -e MYSQL_DATABASE=paismart mysql:8.0
  ```
- [ ] 实现 `pkg/database/mysql.go` (连接初始化)

- [ ] 创建 `internal/model/user.go` (完整 User 模型，包含 Role, OrgTags, PrimaryOrg 字段)
  - 注意：Role 使用 ENUM('USER', 'ADMIN')，默认 'USER'
  - OrgTags 用逗号分隔存储多个组织标签
  - 参考 `docs/ddl.sql` 了解完整字段设计

- [ ] 实现 AutoMigrate 自动建表
- [ ] 创建测试数据

### 验收标准
```bash
go run cmd/server/main.go
# 日志: 数据库连接成功，表创建成功
# MySQL: 查看 users 表结构
```

### 关键技术点
- `gorm.Open()`, `gorm.Config{}`
- 模型定义: `gorm:"column:xxx;type:varchar(100)"`
- `db.AutoMigrate(&User{})`
- 连接池: `SetMaxOpenConns`, `SetMaxIdleConns`

### 参考资料

查看 docs/ddl.sql 了解完整的表结构设计，包括索引、外键约束等

---

## 阶段 4: 用户认证系统（JWT）🔐
**目标**: 实现用户注册、登录、JWT token 生成和验证

### 学习内容
- JWT 原理和实践
- Bcrypt 密码加密
- HTTP 中间件认证

### 实现任务
- [ ] 实现 `pkg/hash/bcrypt.go` (密码加密)
- [ ] 实现 `pkg/token/jwt.go` (JWT 生成/验证)
- [ ] 创建 `internal/repository/user_repository.go` (数据访问层)
- [ ] 创建 `internal/service/user_service.go` (业务逻辑层)
- [ ] 创建 `internal/handler/user_handler.go` (HTTP 处理层)
- [ ] 实现接口:
  - `POST /api/v1/users/register` (注册)
  - `POST /api/v1/users/login` (登录，返回 token)
  - `GET /api/v1/users/me` (获取当前用户信息，需认证)
- [ ] 实现 `internal/middleware/auth.go` (JWT 认证中间件)

- [ ] 实现 `pkg/hash/bcrypt.go` (密码加密/验证)

### 验收标准
```bash
# 注册用户
curl -X POST http://localhost:8081/api/v1/users/register \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"123456"}'

# 登录
curl -X POST http://localhost:8081/api/v1/users/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"123456"}'
# 返回: {"accessToken":"eyJhbGc..."}

# 获取用户信息
curl http://localhost:8081/api/v1/users/me \
  -H "Authorization: Bearer eyJhbGc..."
# 返回: {"id":1,"username":"test"}
```

### 关键技术点
- `bcrypt.GenerateFromPassword()`, `bcrypt.CompareHashAndPassword()`
- `jwt.NewWithClaims()`, `token.SignedString()`
- `jwt.Parse()`, custom claims
- Gin 中间件: `c.Get("user")`, `c.Set("user", user)`
- Repository 接口模式

---

## 阶段 5: Redis 集成 & 组织标签 🏢
**目标**: 集成 Redis，实现多租户组织管理

### 学习内容
- Redis Go 客户端使用
- 多租户数据隔离设计
- 层级数据结构（树形）

### 实现任务
- [ ] 启动 Redis Docker 容器
  ```bash
  docker run -d -p 6379:6379 redis:7-alpine
  ```
- [ ] 实现 `pkg/database/redis.go`
- [ ] 创建 `internal/model/org_tag.go` (组织标签模型)
- [ ] 扩展 User 模型：添加 `org_tags`, `primary_org`, `role` 字段
- [ ] 实现组织标签 CRUD API:
  - `POST /api/v1/admin/org-tags` (创建，需要 admin 权限)
  - `GET /api/v1/admin/org-tags/tree` (树形结构)
  - `PUT /api/v1/users/:userId/org-tags` (分配用户到组织)
- [ ] 实现 `internal/middleware/admin_auth.go` (管理员权限中间件)
- [ ] 在 Redis 中缓存用户组织信息

### 验收标准
```bash
# 创建组织标签（管理员）
curl -X POST http://localhost:8081/api/v1/admin/org-tags \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"name":"技术部","parent_tag":null}'

# 查看组织树
curl http://localhost:8081/api/v1/admin/org-tags/tree \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

### 关键技术点
- `redis.NewClient()`, `rdb.Set()`, `rdb.Get()`
- 自引用外键: `parent_tag` → `org_tags.tag_id`
- 递归查询树形结构
- 中间件组合: `AuthMiddleware + AdminAuthMiddleware`

---

## 阶段 6: MinIO 对象存储 & 简单文件上传 📦
**目标**: 集成 MinIO，实现基础文件上传（不分片）

### 学习内容
- 对象存储概念
- MinIO Go SDK
- 文件上传和下载

### 实现任务
- [ ] 启动 MinIO Docker 容器
  ```bash
  docker run -d -p 9000:9000 -p 9001:9001 \
    -e MINIO_ROOT_USER=minioadmin \
    -e MINIO_ROOT_PASSWORD=minioadmin \
    minio/minio server /data --console-address ":9001"
  ```
- [ ] 实现 `pkg/storage/minio.go`
- [ ] 创建 `internal/model/upload.go` (文件上传记录模型)
- [ ] 实现简单文件上传 API:
  - `POST /api/v1/upload/simple` (单文件上传)
  - `GET /api/v1/documents/download?fileMd5=xxx` (下载)
- [ ] 在数据库记录文件元信息

### 验收标准
```bash
# 上传文件
curl -X POST http://localhost:8081/api/v1/upload/simple \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@test.pdf"
# 返回: {"fileMd5":"xxx","objectUrl":"uploads/xxx.pdf"}

# 下载文件
curl -O http://localhost:8081/api/v1/documents/download?fileMd5=xxx
```

### 关键技术点
- `minio.New()`, `mc.MakeBucket()`, `mc.PutObject()`
- `c.FormFile("file")`, `file.Open()`
- 计算 MD5: `crypto/md5`
- Content-Type 和 Content-Disposition headers

---

## 阶段 7: 分片上传 & Redis Bitmap 优化 🧩
**目标**: 实现大文件分片上传，使用 Redis Bitmap 跟踪上传状态

### 学习内容
- 分片上传原理（断点续传）
- Redis Bitmap 数据结构
- MinIO ComposeObject API

### 实现任务
- [ ] 创建 `internal/model/chunk_info.go` (分片信息模型)
- [ ] 实现分片上传 3 步骤 API:
  - `POST /api/v1/upload/check` (检查文件是否已上传，秒传)
  - `POST /api/v1/upload/chunk` (上传单个分片)
  - `POST /api/v1/upload/merge` (合并分片)
- [ ] 使用 Redis Bitmap 记录已上传分片:
  - Key: `upload:{userID}:{fileMD5}`
  - SetBit(chunkIndex, 1) 标记已上传
- [ ] 实现 MinIO 分片合并逻辑

### 验收标准
```bash
# 1. 检查文件
curl -X POST http://localhost:8081/api/v1/upload/check \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"md5":"abc123"}'

# 2. 上传分片（循环）
for i in {0..9}; do
  curl -X POST http://localhost:8081/api/v1/upload/chunk \
    -H "Authorization: Bearer $TOKEN" \
    -F "fileMd5=abc123" \
    -F "fileName=large.pdf" \
    -F "totalSize=10485760" \
    -F "chunkIndex=$i" \
    -F "file=@chunk_$i.bin"
done

# 3. 合并
curl -X POST http://localhost:8081/api/v1/upload/merge \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"fileMd5":"abc123","fileName":"large.pdf"}'
```

### 关键技术点
- `rdb.SetBit()`, `rdb.BitCount()`
- `mc.ComposeObject()` 或多次 `CopyObject()`
- 幂等性保证：重复上传同一分片应返回成功
- 并发安全：使用 Redis 原子操作

---

## 阶段 8: Kafka & Apache Tika 文档处理 📄
**目标**: 集成 Kafka 消息队列，实现异步文档文本提取

### 学习内容
- Kafka 生产者/消费者模式
- Apache Tika 文本提取
- Goroutine 后台任务处理

### 实现任务
- [ ] 启动 Kafka 和 Tika Docker 容器
  ```bash
  # 使用 deployments/docker-compose.yml 中的 kafka 和 tika 配置
  docker-compose up -d kafka zookeeper tika
  ```
- [ ] 实现 `pkg/kafka/client.go` (生产者和消费者)
- [ ] 实现 `pkg/tika/client.go` (文本提取客户端)
- [ ] 定义 `pkg/tasks/tasks.go` (FileProcessingTask 消息结构)
- [ ] 修改 `UploadService.MergeChunks()`:
  - 合并成功后发送 Kafka 消息
- [ ] 创建 `internal/pipeline/processor.go`:
  - 从 MinIO 下载文件
  - 调用 Tika 提取文本
  - 打印提取结果（后续阶段会做分块和向量化）
- [ ] 在 `main.go` 启动 Kafka Consumer goroutine

### 验收标准
```bash
# 上传文件并合并
curl -X POST http://localhost:8081/api/v1/upload/merge \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"fileMd5":"xxx","fileName":"doc.pdf"}'

# 查看日志
# 输出: Kafka 消息发送成功
# 输出: 消费到消息，开始处理
# 输出: Tika 提取文本成功，内容长度：xxxx
```

### 关键技术点
- `sarama.NewSyncProducer()`, `producer.SendMessage()`
- `sarama.NewConsumerGroup()`, `ConsumeClaim()`
- `http.Post()` 调用 Tika REST API
- Goroutine: `go kafka.StartConsumer()`
- Context 和 graceful shutdown

---

## 阶段 9: 文本分块 & 文档向量表 ✂️
**目标**: 实现文本分块算法，持久化到 document_vectors 表

### 学习内容
- 文本分块策略（固定长度 + 重叠）
- Unicode 字符处理（中文）
- 批量数据库写入

### 实现任务
- [ ] 创建 `internal/model/document_vector.go` (文档向量模型)
- [ ] 在 `pipeline/processor.go` 中实现文本分块逻辑:
  - 按 rune（字符）而非 byte 分块
  - 参数: chunkSize=1000, overlap=100
  - 生成 chunk_id (从 0 开始递增)
- [ ] 将分块结果批量插入 `document_vectors` 表
- [ ] 删除旧记录保证幂等性（重复处理同一文件）

### 验收标准
```bash
# 上传并处理文档
# 查看日志:
# 输出: 文本提取成功，长度：10000 字符
# 输出: 分块完成，共 15 个分块
# 输出: 数据库写入成功

# 查询数据库
mysql> SELECT file_md5, chunk_id, LEFT(text_content, 50) FROM document_vectors;
```

### 关键技术点
- `[]rune(text)` 转换为字符数组
- `text[start:end]` 切片
- `db.Create(&chunks)` 批量插入
- 事务: `db.Transaction(func(tx *gorm.DB) error {})`

---

## 阶段 10: Elasticsearch & Embedding 集成 🔍
**目标**: 集成 Elasticsearch，生成向量并索引

### 学习内容
- Elasticsearch Go 客户端
- 向量索引创建（dense_vector）
- 阿里云 DashScope Embedding API

### 实现任务
- [ ] 启动 Elasticsearch Docker 容器
  ```bash
  docker run -d -p 9200:9200 -p 9300:9300 \
    -e "discovery.type=single-node" \
    -e "xpack.security.enabled=false" \
    docker.elastic.co/elasticsearch/elasticsearch:8.10.0
  ```
- [ ] 实现 `pkg/es/client.go`:
  - 初始化 ES 客户端
  - 创建索引（包含 vector 字段映射）
- [ ] 实现 `pkg/embedding/client.go` (调用 DashScope API)
- [ ] 创建 `internal/model/es_document.go` (ES 文档结构)
- [ ] 在 `pipeline/processor.go` 中添加向量化和索引步骤:
  - 对每个分块调用 Embedding API
  - 构建 EsDocument
  - 批量索引到 Elasticsearch

### 验收标准
```bash
# 上传文档后查看日志
# 输出: Embedding 生成成功，维度：2048
# 输出: Elasticsearch 索引成功，文档数：15

# 查询 ES
curl http://localhost:9200/knowledge_base/_search?size=1
```

### 关键技术点
- `elasticsearch.NewClient()`, `es.Indices.Create()`
- Index mapping: `{"type":"dense_vector","dims":2048,"similarity":"cosine"}`
- HTTP 调用 DashScope API: `Authorization: Bearer API_KEY`
- `es.Bulk()` 批量索引
- 错误重试: 失败记录写入 Redis，定时重试

---

## 阶段 11: 混合检索实现 🎯
**目标**: 实现 KNN 向量检索 + BM25 关键词检索

### 学习内容
- Elasticsearch KNN 查询
- BM25 算法
- Rescore 二次排序

### 实现任务
- [x] 创建 `internal/service/search_service.go`
- [x] 实现查询标准化:
  - 移除停用词（"是谁"、"怎么" 等）
  - 提取核心短语
- [x] 实现混合检索逻辑:
  - KNN 向量搜索（k=topK*30）
  - Query 文本匹配（BM25）
  - Rescore 二次打分
  - 权限过滤（public / org_tag / user_id）
- [x] 创建 `internal/handler/search_handler.go`
- [x] 实现 API: `GET /api/v1/search/hybrid?query=xxx&topK=10`

### 验收标准
```bash
# 搜索
curl "http://localhost:8081/api/v1/search/hybrid?query=如何使用Go语言&topK=5" \
  -H "Authorization: Bearer $TOKEN"

# 返回:
{
  "results": [
    {
      "fileMd5": "abc123",
      "fileName": "go_tutorial.pdf",
      "chunkId": 3,
      "textContent": "Go语言使用...",
      "score": 0.85
    }
  ]
}
```

### 关键技术点
- ES Query DSL: `knn` + `query` + `rescore`
- 中文分词: 确保 IK analyzer 已安装
- 用户权限过滤: `should` clause with `minimum_should_match=1`
- 层级组织标签展开: 递归获取子标签

---

## 阶段 12: WebSocket 流式对话 💬
**目标**: 实现 WebSocket 连接，流式返回 LLM 响应

### 学习内容
- WebSocket 协议和升级
- SSE (Server-Sent Events) 处理
- LLM 流式 API 调用

### 实现任务
- [ ] 实现 `pkg/llm/client.go`:
  - 调用 DeepSeek API (或 Ollama)
  - 处理流式响应 (SSE)
  - 支持停止信号
- [ ] 创建 `internal/model/conversation.go` (对话历史模型)
- [ ] 实现 `internal/repository/conversation_repository.go` (Redis 存储)
- [ ] 创建 `internal/service/chat_service.go`:
  - 检索上下文（调用 SearchService）
  - 构建系统消息（包含 context）
  - 加载对话历史
  - 调用 LLM 流式生成
  - 保存对话记录
- [ ] 创建 `internal/handler/chat_handler.go`:
  - `GET /api/v1/chat/websocket-token` (获取临时 token)
  - `GET /chat/:token` (WebSocket 升级)
  - 处理消息循环
  - 处理停止信号
- [ ] 实现 WebSocket 消息格式:
  - 客户端: `{"type":"message","content":"query"}`
  - 服务端: `{"chunk":"text"}` (流式)
  - 服务端: `{"type":"completion","status":"finished"}`

### 验收标准
```bash
# 1. 获取 WebSocket token
TOKEN=$(curl -s http://localhost:8081/api/v1/chat/websocket-token \
  -H "Authorization: Bearer $USER_TOKEN" | jq -r .cmdToken)

# 2. 连接 WebSocket (使用 wscat 或前端)
wscat -c "ws://localhost:8081/chat/$TOKEN"

# 3. 发送消息
> {"type":"message","content":"Go语言有什么特点？"}

# 4. 接收流式响应
< {"chunk":"Go"}
< {"chunk":"语言"}
< {"chunk":"具有"}
< {"chunk":"以下"}
< {"chunk":"特点"}
...
< {"type":"completion","status":"finished"}
```

### 关键技术点
- `websocket.Upgrader`, `conn.ReadMessage()`, `conn.WriteJSON()`
- LLM 流式响应: `bufio.Scanner`, `strings.HasPrefix("data: ")`
- `wsWriterInterceptor`: 拦截 SSE 输出写入 WebSocket
- `shouldStop()` 函数: 检查停止信号
- Context 传递和取消
- 对话历史: Redis List 或 JSON 数组

---

## 阶段 13: 完善功能 & 前端集成 🎨
**目标**: 实现剩余 API，集成 Vue 前端

### 实现任务
- [ ] 实现文档管理 API:
  - `GET /api/v1/documents/accessible` (可访问文档列表)
  - `DELETE /api/v1/documents/:fileMd5` (删除文档)
  - `GET /api/v1/documents/preview` (在线预览)
- [ ] 实现管理员功能:
  - `GET /api/v1/admin/users/list` (用户列表)
  - `GET /api/v1/admin/conversation` (查看所有对话)
- [ ] 启动 Vue 前端:
  ```bash
  cd frontend
  pnpm install
  pnpm run dev
  ```
- [ ] 配置前端 API 地址（`.env` 文件）
- [ ] 测试完整流程:
  1. 注册/登录
  2. 上传文档
  3. 等待处理完成
  4. 搜索测试
  5. 对话测试

---

## 阶段 14: 优化与生产准备 🚀
**目标**: 性能优化、错误处理、监控、部署

### 实现任务
- [ ] 添加更多单元测试
- [ ] 实现接口限流（rate limiting）
- [ ] 添加 Prometheus metrics
- [ ] 优化 Elasticsearch 查询性能
- [ ] 配置生产环境 docker-compose
- [ ] 编写部署文档

- [ ] 编写 `internal/service/user_service_test.go` 基础测试

---

## 学习建议 💡

### 1. **每个阶段独立完成**
- 不要跨阶段，确保当前阶段可运行
- 每个阶段都写测试验证

### 2. **对照原项目代码**
- 先自己实现，遇到困难时参考原代码
- 理解为什么这样设计

### 3. **记录学习笔记**
- 每个阶段的关键技术点
- 遇到的问题和解决方案

### 4. **循序渐进**
- 从简单到复杂
- 从单体到分布式
- 从同步到异步

### 5. **动手实践**
- 不要只看代码，一定要自己写
- 修改参数，观察效果
- 故意制造错误，学习调试

---

## 参考资料 📚

- **Go 语言**:
  - [Go by Example](https://gobyexample.com/)
  - [Effective Go](https://go.dev/doc/effective_go)

- **Gin 框架**: https://gin-gonic.com/docs/
- **GORM**: https://gorm.io/docs/
- **Elasticsearch Go**: https://github.com/elastic/go-elasticsearch
- **Kafka Go**: https://github.com/IBM/sarama
- **MinIO Go**: https://min.io/docs/minio/linux/developers/go/minio-go.html

- **原项目文档**:
  - `/home/yyy/Projects/paismart-go/CLAUDE.md`
  - `/home/yyy/Projects/paismart-go/README.md`

---

## 阶段 15: 2026 版 RAG 评估与观测体系 📊
**目标**: 先把“检索是否有效、生成是否可信、线上是否退化”量化出来，再推动后续所有优化

### 背景
当前项目没有系统性的 RAG 评估和观测能力。只有功能能跑通，并不能说明检索设计正确，更不能说明改造真的提升了效果。到 2026，成熟的 RAG 系统通常会同时具备三类评估：

1. **离线检索评估**：衡量召回、排序和上下文质量
2. **离线生成评估**：衡量回答是否忠实、是否切题
3. **线上观测与回归保护**：衡量新版本是否在真实流量下退化

单靠“主观感觉回答变好了”已经不够。后续阶段 16-20 的每一步，都应该能在阶段 15 的框架下被量化验证。

### 改造思路（按优先级）

**P0：先搭建 2026 版评估数据集与检索指标（必须先做）**

- [ ] 构建分层评估数据集 `testdata/eval_dataset.jsonl`:
  - 每条样本至少包含：`query`、`gold_answer`、`relevant_chunks`、`relevant_docs`、`query_type`
  - `query_type` 建议覆盖：
    - exact keyword
    - paraphrase / 口语 query
    - low-overlap semantic query
    - multi-hop / decomposition
    - long-context query
    - multimodal-ready（为阶段 19 预留）

- [ ] 推荐数据结构:
  ```json
  {
    "id": "q_001",
    "query": "什么是 RAG？",
    "gold_answer": "RAG 是检索增强生成...",
    "relevant_docs": ["rag_intro.pdf"],
    "relevant_chunks": ["rag_intro.pdf#3"],
    "query_type": "exact"
  }
  ```

- [ ] 实现检索评估入口 `internal/eval/retrieval_metrics.go`:
  - Precision@5
  - Recall@10 / Recall@20
  - MRR@10
  - nDCG@10
  - HitRate@5
  - Context Precision

- [ ] 输出维度不只看总平均，还要按 `query_type` 分组输出

**P1：补生成质量评估与引用质量评估**

- [ ] 实现 `internal/eval/generation_metrics.go`:
  - Faithfulness
  - Answer Relevancy
  - Citation Accuracy / Groundedness
  - Context Utilization（回答是否真的利用了检索上下文）

- [ ] 对接 LLM-as-Judge，但要求：
  - prompt 固定
  - 结果结构化
  - 支持缓存 judge 结果，避免重复花费

- [ ] 每条评估结果输出：
  - 检索结果
  - 最终回答
  - 引用 chunk
  - judge 打分
  - 失败类型标签

**P2：线上观测与回归保护（2026 必备）**

- [ ] 新建 `cmd/eval/main.go`:
  - 支持运行整套离线评估
  - 支持只跑 retrieval / 只跑 generation

- [ ] 增加评估结果快照:
  - `eval_reports/baseline_YYYYMMDD.json`
  - `eval_reports/experiment_xxx.json`

- [ ] 在服务侧增加最小观测字段:
  - query
  - normalized query
  - expansion queries
  - top retrieved chunks
  - model / reranker / retriever versions

- [ ] 为后续阶段预留线上回归比较能力:
  - 新版本与 baseline 做 diff
  - 设定“关键指标退化阈值”

### 验证方式（2026 版）

- 先跑一次阶段十一 baseline，生成第一版基线报告
- 后续阶段每次改动至少对比：
  - 总平均指标
  - 各类 query 分组指标
  - 最差 10 条案例的变化

- 最低验收要求：
  - 能稳定输出 retrieval + generation 双报告
  - 能看到 query_type 分组结果
  - 能从报告中定位失败样本与失败类型

---

## 阶段 16: 2026 版分块与检索单元重构 🧩
**目标**: 将固定窗口滑动切分升级为结构感知 + Contextual Retrieval + Parent-Child/Small-to-Big 的检索单元体系

### 背景
当前 `processor.go` 中 `splitText()` 按 1000 字符 + 100 重叠做纯机械切分，不区分文档类型，不感知标题、段落、表格和页面边界。这个方案能跑通阶段 9-11，但到 2026 已经不够作为高质量 RAG 的检索底座：

- 纯窗口切分会打断章节语义，导致 chunk 既不完整也不稳定
- chunk 只保存局部文本，缺少“它在整篇文档中的位置与上下文”
- 检索命中后只能返回局部片段，无法自然扩展到父段落/父章节
- 对长文档和讲义类 PDF，用户问的是“主题”，而不是一个孤立的 1000 字片段

2026 更稳妥的做法不是只追求“更聪明地切块”，而是把检索单元设计成三层：

1. **结构感知 child chunks**：用于精确命中
2. **parent sections / small-to-big context**：用于命中后扩展上下文
3. **contextualized chunk text**：用于提升召回，避免孤立 chunk 语义不完整

### 改造思路（按优先级）

**P0：结构感知分块与统一 Chunk Schema（必须先做）**

- [ ] 重构 `internal/pipeline/processor.go` 中的 `splitText()`，拆分为策略模式
- [ ] 统一 `TextChunk` 结构，新增：
  - `ContextualContent`
  - `ParentID`
  - `TitlePath`
  - `Metadata`

- [ ] 实现 `MarkdownChunker`:
  - 按标题层级切分
  - 继承标题路径 metadata
  - 超长章节再做二次切分

- [ ] 实现 `GeneralChunker`:
  - 段落优先
  - 句子优先
  - 字符数回退
  - overlap 从主策略降级为兜底策略

- [ ] 扩展 ES / MySQL 的 chunk metadata 字段：
  - `title_path`
  - `parent_id`
  - `page_number`
  - `chunk_type`
  - `contextual_text`

**P1：Contextual Retrieval（优先级高于纯语义分块）**

- [ ] 为每个 child chunk 生成 contextualized text：
  - 文档标题
  - 父章节标题
  - 页码
  - 邻近段落摘要

- [ ] 第一版先做规则式 contextualization
- [ ] 第二版再接 LLM 生成上下文化摘要
- [ ] 向量化时优先使用 contextualized text，展示时仍返回原始 chunk

**P2：Parent-Child / Small-to-Big Retrieval**

- [ ] 新建 parent chunk 结构
- [ ] child 负责召回，parent 负责上下文展开
- [ ] 命中后按 `parent_id` 聚合，并可返回 siblings / parent section
- [ ] 在 `SearchService` 中新增扩展模式：`none | parent | siblings`

**P3：语义边界切分（做，但优先级低于 contextual retrieval）**

- [ ] 实现 `SemanticChunker`
- [ ] 支持批量 embedding 计算
- [ ] 仅在结构边界不足的文档类型上启用

### 验证方式（2026 版）

- 用阶段 15 的评估集对比：
  - Recall@10
  - Precision@5
  - MRR@10
  - nDCG@10
  - Context Precision

- 建议按消融顺序验证：
  1. 固定窗口 baseline
  2. + 结构感知 chunking
  3. + contextual retrieval
  4. + parent-child expansion
  5. + semantic chunking

---

## 阶段 17: 2026 版查询增强、多路召回与精排 🔍
**目标**: 将当前单路混合检索升级为“查询增强 → 多路召回 → 融合 → Rerank”的现代检索栈

### 背景
当前 `search_service.go` 只做了停用词清洗（`normalizeQuery`）；检索层仍然以 KNN + BM25 + ES rescore 为主。这个 baseline 在阶段十一是合理的，但到 2026 主流高质量 RAG 通常至少具备四层能力：

1. **查询增强**：HyDE、Multi-Query、必要时做 query decomposition
2. **多路召回**：dense + lexical + sparse，不再只依赖 BM25
3. **结果融合**：RRF 优于硬编码线性加权
4. **Reranking**：Cross-Encoder / Rerank API 精排 top candidates

更进一步的系统会加入：

- **Learned Sparse Retrieval**：如 SPLADE / BGE-M3 sparse
- **Multi-Vector / Late Interaction Retrieval**：如 ColBERT 风格

### 改造思路（按优先级）

**P0：把当前检索拆成可扩展的多阶段架构（必须先做）**

- [ ] 重构 `SearchService.HybridSearch()`，拆成：
  - query enhancement
  - candidate retrieval
  - result fusion
  - rerank
  - final formatting

- [ ] 新建统一 Retriever / Candidate / ResultFuser 接口
- [ ] 第一版至少拆出：
  - `DenseRetriever`
  - `LexicalRetriever`
  - `SparseRetriever`（先预留）

**P1：查询增强层**

- [ ] 实现 `QueryEnhancer`
- [ ] 实现 **HyDE**
- [ ] 实现 **Multi-Query**
- [ ] 可选实现 **Query Decomposition**
- [ ] 支持配置：`none | hyde | multi_query | hyde_multi`

**P2：结果融合层**

- [ ] 将当前 ES `rescore` 降级为可选
- [ ] 默认采用 **RRF**
- [ ] 融合 dense / lexical / sparse 结果后，保留 top 30~50 候选进入下一阶段

**P3：Cross-Encoder / Rerank API 精排（高 ROI 项）**

- [ ] 新建 `pkg/rerank/client.go`
- [ ] 优先支持外部 Rerank API
- [ ] 第二阶段再支持本地 Cross-Encoder
- [ ] 仅把 LLM-as-Reranker 作为实验或兜底

**P4：Learned Sparse / Multi-Vector Retrieval（进阶项）**

- [ ] 规划 learned sparse 路线:
  - 目标：替代“只有 BM25 的 sparse 侧”
  - 可选模型：SPLADE、BGE-M3 sparse

- [ ] 规划 multi-vector / late interaction 路线:
  - 目标：提升细粒度匹配能力
  - 典型思路：ColBERT 风格

- [ ] 技术取舍建议：
  - 若继续以 Elasticsearch 为主：优先做 `dense + lexical + learned sparse + RRF + rerank`
  - 若未来要追求更高检索质量：再评估 multi-vector 引擎

### 验证方式（2026 版）

- 用阶段 15 的评估集对比：
  - Precision@5
  - Recall@10 / Recall@20
  - MRR@10
  - nDCG@10
  - HitRate@5

- 消融顺序建议：
  1. 当前阶段十一 baseline
  2. + RRF
  3. + Cross-Encoder rerank
  4. + HyDE / Multi-Query
  5. + learned sparse
  6. + multi-vector（若实现）

---

## 阶段 18: 2026 版上下文编排与记忆管理 🧠
**目标**: 从“简单拼接历史与检索结果”升级为 token-aware、source-aware、memory-aware 的上下文编排系统

### 背景
当前 `conversation_repository.go` 只保留最近若干消息，`chat_service.go` 将 system + 历史 + 检索结果直接拼接。这个方式在 Demo 阶段够用，但到 2026 更稳妥的上下文系统至少要解决四个问题：

- token 预算是否可控
- 哪些历史该保留，哪些该压缩
- 检索结果如何摆放，才能减少 “Lost in the Middle”
- 用户长期偏好和会话级记忆是否应该分层

### 改造思路（按优先级）

**P0：Token 预算与上下文槽位编排（必须先做）**

- [ ] 新建 `pkg/tokenizer/counter.go`
- [ ] 在 `chat_service.go` 中实现固定槽位预算：
  - system prompt
  - recent turns
  - retrieval context
  - tool outputs
  - reserved generation budget

- [ ] 不再“先拼再截断”，改成“先预算再填充”

**P1：历史压缩与分层记忆**

- [ ] 新建 `internal/service/context_compressor.go`
- [ ] 分层保存：
  - 最近对话：原文
  - 老历史：摘要
  - 长期偏好：独立 memory slot

- [ ] 对摘要结果做缓存，避免重复压缩
- [ ] 为后续 Agent 阶段预留 tool-state memory

**P2：检索结果编排优化**

- [ ] 修改 `buildContextText()`：
  - 支持 anti “Lost in the Middle” 排列
  - 支持 parent-child 检索结果合并展示
  - 支持 source-aware 格式，例如按文档和章节分组

- [ ] 区分三类上下文：
  - user chat history
  - retrieved knowledge
  - tool observations

**P3：多会话与记忆治理**

- [ ] 改造 `ConversationRepository`，支持多会话
- [ ] 为每个会话保存：
  - title
  - summary
  - last active time
  - pinned memory

- [ ] 增加记忆治理策略：
  - 何时生成摘要
  - 何时淘汰
  - 何时写入长期偏好

### 验证方式（2026 版）

- 构造 30+ 轮长对话评测
- 验证消息不会超出模型上下文窗口
- 对比改造前后：
  - Faithfulness
  - Answer Relevancy
  - Context Utilization

---

## 阶段 19: 2026 版多模态知识库与文档理解 📸
**目标**: 把知识库从“纯文本索引”升级为对表格、图片、截图和结构化内容都可检索、可对话的多模态知识库

### 背景
当前只支持文本类文档，Tika 提取后主要得到纯文本，表格结构、图片信息、页面布局基本都丢失。到 2026，多模态 RAG 的重点不只是“让模型看图”，而是把图片、表格和版面结构纳入可检索知识单元。

### 改造思路（按优先级）

**P0：增强文档处理，让表格和图片进入知识库**

- [ ] 扩展 `pkg/tika/client.go`：
  - 调 `/rmeta`
  - 提取 XHTML / metadata
  - 识别 `<table>`、`<img>`、页面信息

- [ ] 表格处理：
  - 表格转 Markdown / structured JSON
  - 保留表头、行列语义
  - 作为独立 chunk 或 metadata 附加索引

- [ ] 图片处理：
  - OCR 文本
  - 图片描述（caption）
  - 图片区域与页码 metadata

**P1：原生支持图像文件和结构化文件**

- [ ] 支持 `.png/.jpg/.jpeg/.csv/.html`
- [ ] 图片文件：
  - OCR + caption
  - 作为 document chunks 入索引
- [ ] CSV 文件：
  - 保留表头
  - 按主题块切分，而不是简单按固定行数

**P2：聊天发图识图（多模态对话）**

- [ ] 升级 LLM message schema 为 multimodal message
- [ ] WebSocket 协议支持图像输入
- [ ] 上传图片到 MinIO 并生成临时 URL
- [ ] 接入支持视觉的模型

**P3：多模态检索与结果融合**

- [ ] 把文本、OCR、caption、表格内容统一纳入 retrieval pipeline
- [ ] 支持 metadata 过滤：页码、区域、对象类型
- [ ] 为后续阶段 20 的 Agent 增加 `image_understanding` / `document_layout_search` 工具接口

### 验证方式（2026 版）

- 上传含表格/图片的 PDF，验证表格内容和图片描述能被检索到
- 上传单独图片文件，验证其 caption / OCR 可检索
- 在聊天中发送截图，验证模型能理解并结合知识库回答
- 用阶段 15 评估集增加 multimodal subset

---

## 阶段 20: 2026 版 Agent、工具调用与深度搜索 🤖
**目标**: 从“单轮 RAG 问答”升级为具备工具调用、深度搜索、规划执行和可观测性的 Agent 系统

### 背景
当前系统是经典的检索→生成链路。到 2026，更成熟的知识型 Agent 不只是“遇到问题就搜知识库”，而是先判断问题类型，再决定是：

- 直接回答
- 调知识库检索
- 做多步检索
- 调外部工具
- 联网深度搜索

### 改造思路（按优先级）

**P0：先把现有能力工具化**

- [ ] 新建 `internal/agent/` 包
- [ ] 实现通用 Tool 定义
- [ ] 将现有能力封装为工具：
  - `knowledge_search`
  - `calculator`
  - `current_time`
  - `conversation_memory`

- [ ] 增加 execution trace，保留每次工具调用轨迹

**P1：引入可控的 Agent Loop**

- [ ] 实现有限步的 ReAct / tool-calling 循环
- [ ] 增加：
  - `max_iterations`
  - timeout
  - loop detection
  - tool allowlist

- [ ] 输出结构化 trace:
  - thought summary
  - selected tool
  - tool input
  - observation
  - final answer

**P2：联网深度搜索（DeepSearch）**

- [ ] 新建 `pkg/websearch/client.go`
- [ ] 实现：
  - 搜索 API 调用
  - 页面抓取
  - 正文抽取
  - 网页摘要

- [ ] 把 `web_search` 作为工具接入 Agent
- [ ] 允许“知识库无结果”或“用户显式要求联网”时触发 deep search

**P3：意图识别与路由**

- [ ] 新建 `internal/service/intent_router.go`
- [ ] 路由到不同链路：
  - 闲聊 → 直接对话
  - 知识问答 → RAG
  - 多步复杂问题 → Agent loop
  - 需要最新信息 → web search / deep search

- [ ] 在 `ChatHandler` 中替换单一路径调用

**P4：Agent 评估与安全治理**

- [ ] 用阶段 15 的评估框架扩展 agent subset
- [ ] 评估指标增加：
  - task completion rate
  - tool selection accuracy
  - average steps
  - web citation quality

- [ ] 增加安全治理：
  - 工具权限控制
  - 外部搜索域名白名单
  - prompt injection 防护
  - 成本和步数上限

### 验证方式（2026 版）

- 测试闲聊场景不触发检索
- 测试知识库无法回答的问题，Agent 自动联网搜索并整合答案
- 测试多步问题，例如：
  - “帮我查一下 XX 的最新进展，然后和知识库中的 YY 对比”
- 验证 execution trace 可被记录和回放

---

### 阶段 15-20 改造路线总览

```
阶段15: 评估与观测        ← 先定义成功，再优化
  ↓
阶段16: 分块与检索单元重构  ← 决定知识库的基本质量
  ↓
阶段17: 查询增强与多路召回  ← 决定 Retrieval 上限
  ↓
阶段18: 上下文编排与记忆    ← 决定回答是否稳定利用检索结果
  ↓
阶段19: 多模态知识库        ← 扩展知识输入与检索边界
  ↓
阶段20: Agent 与深度搜索    ← 从 RAG 升级为可规划的智能系统
```

**核心原则**:
- 每个阶段改完后，都跑一次阶段 15 的评估脚本
- 不只看总分，要看不同 query 类型、不同文档类型和最差案例
- 优先做高 ROI 的 P0 / P1 项，再做进阶的 P2 / P3 / P4

---

## 学习建议 💡

### 1. **每个阶段独立完成**
- 不要跨阶段，确保当前阶段可运行
- 每个阶段都写测试验证

### 2. **对照原项目代码**
- 先自己实现，遇到困难时参考原代码
- 理解为什么这样设计

### 3. **记录学习笔记**
- 每个阶段的关键技术点
- 遇到的问题和解决方案

### 4. **循序渐进**
- 从简单到复杂
- 从单体到分布式
- 从同步到异步

### 5. **动手实践**
- 不要只看代码，一定要自己写
- 修改参数，观察效果
- 故意制造错误，学习调试

---

## 参考资料 📚

- **Go 语言**:
  - [Go by Example](https://gobyexample.com/)
  - [Effective Go](https://go.dev/doc/effective_go)

- **Gin 框架**: https://gin-gonic.com/docs/
- **GORM**: https://gorm.io/docs/
- **Elasticsearch Go**: https://github.com/elastic/go-elasticsearch
- **Kafka Go**: https://github.com/IBM/sarama
- **MinIO Go**: https://min.io/docs/minio/linux/developers/go/minio-go.html

- **原项目文档**:
  - `/home/yyy/Projects/paismart-go/CLAUDE.md`
  - `/home/yyy/Projects/paismart-go/README.md`

- **RAG项目成熟实现**
  参考资料：
  - RAGAS metrics: https://docs.ragas.io/en/stable/concepts/metrics/
  - Elasticsearch RRF: https://www.elastic.co/docs/reference/elasticsearch/rest-apis/reciprocal-rank-fusion
  -Lost in the Middle (TACL 2024): https://aclanthology.org/2024.tacl-1.9/
  - OpenAI tool/function calling: https://developers.openai.com/api/docs/guides/function-calling
  - OWASP LLM Top 10 v1.1: https://owasp.org/www-project-top-10-for-large-language-model-applications/
  - Anthropic Contextual Retrieval: https://www.anthropic.com/engineering/contextual-retrieval
  - Microsoft GraphRAG: https://microsoft.github.io/graphrag/

---

## 进度追踪 ✅

| 阶段 | 状态 | 完成日期 | 备注 |
|------|------|----------|------|
| 1. 项目初始化 |  |  |  |
| 2. 日志 & HTTP |  |  |  |
| 3. 数据库 & 模型 |  |  |  |
| 4. 用户认证 |  |  |  |
| 5. Redis & 组织 |  |  |  |
| 6. MinIO & 上传 |  |  |  |
| 7. 分片上传 |  |  |  |
| 8. Kafka & Tika |  |  |  |
| 9. 文本分块 |  |  |  |
| 10. ES & Embedding |  |  |  |
| 11. 混合检索 |  |  |  |
| 12. WebSocket 对话 |  |  |  |
| 13. 完善功能 |  |  |  |
| 14. 优化部署 |  |  |  |
| 15. 评估与观测体系 |  |  |  |
| 16. 分块与检索单元重构 |  |  |  |
| 17. 查询增强与多路召回 |  |  |  |
| 18. 上下文编排与记忆管理 |  |  |  |
| 19. 多模态知识库与文档理解 |  |  |  |
| 20. Agent 与深度搜索 |  |  |  |

---

## 开始第一步 🎬

准备好了吗？让我们从**阶段 1: 项目初始化**开始！

告诉我你准备好了，我会帮你一步步完成第一个阶段的代码编写。


