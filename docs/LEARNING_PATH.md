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
- [ ] 创建 `internal/service/search_service.go`
- [ ] 实现查询标准化:
  - 移除停用词（"是谁"、"怎么" 等）
  - 提取核心短语
- [ ] 实现混合检索逻辑:
  - KNN 向量搜索（k=topK*30）
  - Query 文本匹配（BM25）
  - Rescore 二次打分
  - 权限过滤（public / org_tag / user_id）
- [ ] 创建 `internal/handler/search_handler.go`
- [ ] 实现 API: `GET /api/v1/search/hybrid?query=xxx&topK=10`

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

## 阶段 15: RAG 质量评估体系搭建 📊
**目标**: 先建立度量基准，才能驱动后续所有优化——没有评估就没有迭代

### 背景
当前项目没有任何 RAG 质量评估手段，所有改进都无法量化。主流生产级 RAG 系统依赖 RAGAS 四大指标（Context Recall、Context Precision、Faithfulness、Answer Relevancy）来衡量检索与生成质量。先搭好评估再做优化，才能确保每一步改动真的有效。

### 实现任务

- [ ] 构建评估数据集:
  - 手动整理 30-50 组"问题 + 标准答案 + 期望命中的源文件"
  - 存入 `testdata/eval_dataset.json`，格式:
    ```json
    [
      {
        "query": "什么是 RAG？",
        "ground_truth": "RAG 是检索增强生成...",
        "expected_sources": ["rag_intro.pdf"]
      }
    ]
    ```

- [ ] 实现检索层指标计算 `internal/eval/retrieval_metrics.go`:
  - Precision@K: 返回的 K 个结果中有多少是相关的
  - Recall@K: 相关文档中有多少被召回
  - MRR（Mean Reciprocal Rank）: 第一个相关结果的排名倒数
  - 对接现有 `SearchService.HybridSearch()`，跑评估数据集并输出指标

- [ ] 实现生成层指标计算 `internal/eval/generation_metrics.go`:
  - Faithfulness: 调用 LLM 判断回答中的每个声明是否能在检索结果中找到依据
  - Answer Relevancy: 调用 LLM 判断回答是否切题回答了问题
  - 输出结构化评估报告（JSON/CSV）

- [ ] 创建评估入口 `cmd/eval/main.go`:
  - 读取评估数据集，依次调用 SearchService 和 ChatService
  - 输出各指标的平均值和逐条明细
  - 支持 `go run cmd/eval/main.go --dataset testdata/eval_dataset.json`

### 关键技术点
- LLM-as-Judge 模式: 用 LLM 自身评判回答质量
- 评估与业务代码解耦: eval 包只读取 service 层结果
- 基线记录: 第一次跑出的指标就是"改造前基线"，后续每次优化都跑一次对比

### 参考
- [RAGAS 框架](https://docs.ragas.io/)
- Precision@K / Recall@K / MRR 经典 IR 指标定义

---

## 阶段 16: 智能分块改造 🧩
**目标**: 将固定窗口滑动切分升级为语义感知 + 结构感知的分块策略

### 背景
当前 `processor.go` 中 `splitText()` 按 1000 字符 + 100 重叠做纯机械切分，不区分文档类型，不感知段落/标题/表格边界。主流做法是语义分块（Semantic Chunking）和结构感知分块，可将检索准确率提升 15-30%。

### 改造思路

**第一步：结构感知分块（低成本高回报）**

- [ ] 重构 `internal/pipeline/processor.go` 中的 `splitText()`，拆分为策略模式:
  ```go
  // internal/pipeline/chunker.go
  type Chunker interface {
      Chunk(text string, meta DocumentMeta) []TextChunk
  }

  type TextChunk struct {
      Content  string
      Index    int
      Metadata map[string]string // title, section, page 等
  }

  type DocumentMeta struct {
      FileName string
      FileType string // pdf, docx, md, txt
  }
  ```

- [ ] 实现 `MarkdownChunker`:
  - 按 `#`/`##`/`###` 标题层级切分
  - 每个 chunk 自动携带其所属的标题路径作为 metadata
  - 超长章节再做二次切分（递归分割）

- [ ] 实现 `GeneralChunker`（改进版固定窗口）:
  - 优先在段落边界（`\n\n`）切分
  - 其次在句号/换行处切分
  - 最后才按字符数回退切分
  - 保留重叠区域

- [ ] 根据文件扩展名在 `Process()` 中自动选择 Chunker:
  ```
  .md  → MarkdownChunker
  .txt → GeneralChunker（段落优先）
  .pdf/.docx/.ppt → GeneralChunker（句子优先）
  ```

**第二步：语义分块（中成本高回报）**

- [ ] 实现 `SemanticChunker`:
  - 先按句子拆分文本
  - 对相邻句子对计算 embedding 余弦相似度
  - 在相似度突降处（低于阈值）切分为新 chunk
  - 需调用现有的 `embedding.Client.CreateEmbedding()`

- [ ] 性能优化:
  - 批量 embedding 请求，减少 API 调用次数
  - 在 `pkg/embedding/client.go` 中添加 `CreateBatchEmbedding()` 方法

**第三步：元数据增强**

- [ ] 扩展 ES 索引 mapping，新增 metadata 字段:
  ```json
  {
    "section_title": {"type": "keyword"},
    "page_number": {"type": "integer"},
    "chunk_type": {"type": "keyword"}
  }
  ```

- [ ] 修改 `model.EsDocument`，携带 metadata 入库
- [ ] 检索时支持按 metadata 过滤（如只搜某个章节）

### 验证方式
- 跑阶段 15 的评估脚本，对比改造前后 Recall@10 和 Precision@10
- 目标: Recall@10 提升 10% 以上

---

## 阶段 17: 查询增强与 Reranking 🔍
**目标**: 在检索前做查询智能改写，在检索后加 Reranking 精排，双端提升检索质量

### 背景
当前 `search_service.go` 只做了停用词清洗（`normalizeQuery`），缺少语义层面的查询增强；检索后用 ES 自带 BM25 rescore，没有 Cross-Encoder 级别的精排。主流系统用 HyDE/Multi-Query 做查询扩展可提升召回率，用 Cross-Encoder 做重排可提升 10-40% 准确率。

### 改造思路

**第一步：查询改写层**

- [ ] 新建 `internal/service/query_enhancer.go`:
  ```go
  type QueryEnhancer interface {
      Enhance(ctx context.Context, originalQuery string) ([]string, error)
  }
  ```

- [ ] 实现 HyDE（Hypothetical Document Embedding）:
  - 调用 LLM 为用户问题生成一个"假设性答案片段"（100-200 字）
  - 用假设答案的 embedding 替代原始问题的 embedding 去做向量检索
  - Prompt 示例: `"请为以下问题写一个简短的知识库文档片段作为回答（不需要真实性，只需格式和风格像知识库内容）：{query}"`

- [ ] 实现 Multi-Query 改写:
  - 调用 LLM 将一个问题改写为 3 个不同角度的表述
  - 分别检索后合并去重
  - Prompt 示例: `"请将以下问题从3个不同角度重新表述，每行一个：{query}"`

- [ ] 在 `SearchService.HybridSearch()` 的入口处插入 QueryEnhancer:
  - 配置开关: `config.yaml` 中 `search.query_enhancement: "hyde" | "multi_query" | "none"`
  - 默认关闭，可按需打开

**第二步：Cross-Encoder Reranking**

- [ ] 新建 `pkg/rerank/client.go`:
  ```go
  type Reranker interface {
      Rerank(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error)
  }

  type RerankResult struct {
      Index int
      Score float64
  }
  ```

- [ ] 实现方案（任选其一）:
  - **方案 A — 调用外部 Rerank API**（推荐起步）:
    - 对接 Cohere Rerank API 或智谱/阿里的重排序接口
    - 简单 HTTP 调用，无需本地部署模型
  - **方案 B — LLM-as-Reranker**:
    - 用现有 DeepSeek API，Prompt 让它给每个候选结果打 1-10 分
    - 成本较高但无需额外依赖
  - **方案 C — 本地 Cross-Encoder**（进阶）:
    - 部署 `bge-reranker-v2-m3` 到一个 Python microservice
    - Go 通过 HTTP 调用

- [ ] 在 `SearchService` 中集成 Reranker:
  - 第一阶段: ES 返回 topK * 3 = 30 条候选
  - 第二阶段: Reranker 精排，取 top 10 返回
  - 配置开关: `search.reranking.enabled: true/false`

**第三步：检索结果融合优化**

- [ ] 将当前 ES rescore 替换为 RRF（Reciprocal Rank Fusion）:
  - 分别拿到 knn 排名和 BM25 排名
  - `RRF_score = 1/(k + rank_knn) + 1/(k + rank_bm25)`，k 常取 60
  - 比硬编码权重更鲁棒，不依赖分数归一化

### 验证方式
- 跑阶段 15 评估脚本，对比各方案的 Precision@5、Recall@10、MRR
- 目标: Precision@5 提升 15% 以上

---

## 阶段 18: 上下文管理升级 🧠
**目标**: 从"硬截断 20 条消息"升级为 token 感知的动态上下文管理

### 背景
当前 `conversation_repository.go` 直接截断保留最近 20 条消息，`chat_service.go` 将 system + 全部历史 + 检索结果无差别拼接。没有 token 计数，可能超出模型上下文窗口，也可能浪费预算在无关历史上。主流做法是 token 预算分配 + 历史压缩 + 检索结果排序优化。

### 改造思路

**第一步：Token 计数与预算分配**

- [ ] 新建 `pkg/tokenizer/counter.go`:
  - 实现中英文混合文本的 token 估算（简易版: 中文 1 字 ≈ 1.5 token，英文按空格分词）
  - 或对接 tiktoken 的 Go 移植版（如 `github.com/pkoukk/tiktoken-go`）
  ```go
  type TokenCounter interface {
      Count(text string) int
  }
  ```

- [ ] 在 `chat_service.go` 的 `composeMessages()` 中实现预算分配:
  ```
  总预算 = model_max_tokens（如 deepseek-chat = 64K）
  - 预留生成空间 = max_tokens 配置（如 2000）
  - system prompt 预算 = 实际占用
  - 检索结果预算 = 总预算 * 40%
  - 历史预算 = 剩余空间
  ```
  - 按预算从后往前填充历史（最近的优先）
  - 检索结果按相关性排序后，从高到低填充直到预算耗尽

**第二步：历史消息压缩**

- [ ] 新建 `internal/service/context_compressor.go`:
  ```go
  type ContextCompressor interface {
      Compress(ctx context.Context, messages []model.ChatMessage, tokenBudget int) ([]model.ChatMessage, error)
  }
  ```

- [ ] 实现分层压缩策略:
  - 最近 2-4 轮对话: 保留原文
  - 更早的历史: 调用 LLM 生成摘要替代原文
  - 摘要 Prompt: `"请将以下多轮对话压缩为一段简短的上下文摘要，保留关键信息：{messages}"`
  - 缓存摘要到 Redis，避免重复压缩

**第三步：检索结果排序优化（anti "Lost in the Middle"）**

- [ ] 修改 `buildContextText()`:
  - 将检索结果按相关性排序后，交替放置（最高 → 最低 → 次高 → 次低 ...）
  - 确保最相关的内容出现在上下文的开头和末尾
  - 这是应对 LLM "Lost in the Middle" 问题的简单有效策略

**第四步：多会话支持**

- [ ] 改造 `ConversationRepository`:
  - 当前每个用户只有一个 `current_conversation`，新建对话会覆盖旧的
  - 改为支持多会话: `user:{uid}:conversations` 存储会话列表
  - 新增 API: 创建会话、切换会话、列出历史会话
  - 前端聊天页面增加会话列表侧栏

### 验证方式
- 构造超长对话场景（30+ 轮），对比改造前后的回答质量
- 用 token 计数验证消息不超出模型上下文窗口
- 跑阶段 15 评估脚本，对比 Faithfulness 指标

---

## 阶段 19: 多模态与高级文档处理 📸
**目标**: 支持图片/表格内容提取、聊天发图识图、扩展文档类型

### 背景
当前只支持文本类文档（pdf/doc/xls/ppt/txt/md），Tika 提取的是纯文本，丢失了表格结构和图片内容。聊天也只支持发送纯文本。主流 RAG 系统已支持图片 OCR 入库、对话中发图让模型识图、表格结构化提取等多模态能力。

### 改造思路

**第一步：增强文档处理——表格与图片提取**

- [ ] 扩展 `pkg/tika/client.go`:
  - 除提取纯文本外，额外调用 Tika 的 `/rmeta` 端点获取结构化元数据
  - 解析 Tika 返回的 XHTML 格式，识别 `<table>` 和 `<img>` 标签
  - 表格转为 Markdown 表格格式后入 chunk
  - 图片引用记录到 metadata

- [ ] 对 PDF 中的图片做 OCR:
  - 方案 A: 使用 Tika 内置 Tesseract OCR（需要在 Docker 镜像中安装 Tesseract）
  - 方案 B: 调用视觉模型 API（如 GPT-4o / Qwen-VL）描述图片内容后入 chunk
  - 在 `pipeline/processor.go` 的 `Process()` 中增加图片处理分支

- [ ] 扩展支持的文件类型:
  - 在 `upload_service.go` 的 `typeMapping` 中增加: `.png`, `.jpg`, `.jpeg`, `.csv`, `.html`
  - 图片文件: 直接调用视觉模型生成描述 → 描述文本作为 chunk 索引
  - CSV 文件: 按行/按固定行数分块，保留表头

**第二步：聊天发图识图（多模态对话）**

- [ ] 升级 LLM Message 结构以支持多模态:
  ```go
  // pkg/llm/client.go
  type MessageContent struct {
      Type     string `json:"type"`      // "text" 或 "image_url"
      Text     string `json:"text,omitempty"`
      ImageURL *struct {
          URL string `json:"url"`
      } `json:"image_url,omitempty"`
  }

  type MultimodalMessage struct {
      Role    string           `json:"role"`
      Content []MessageContent `json:"content"`
  }
  ```

- [ ] 后端改造:
  - WebSocket 消息协议扩展: 支持发送 `{"type":"image","data":"base64..."}` 或图片 URL
  - 收到图片后上传到 MinIO，生成临时访问 URL
  - 构建多模态 Message 发给支持视觉的 LLM（需切换到支持视觉的模型如 deepseek-vl 或 qwen-vl）

- [ ] 前端改造:
  - 聊天输入框增加图片上传按钮/粘贴板图片支持
  - 消息列表支持渲染图片消息
  - 发送时将图片转为 base64 或先上传获取 URL

- [ ] 配置:
  ```yaml
  # configs/config.yaml
  llm:
    vision:
      enabled: true
      model: "qwen-vl-plus"  # 或其他视觉模型
      base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1"
  ```

### 验证方式
- 上传含表格/图片的 PDF，验证表格内容和图片描述能被检索到
- 在聊天中发送一张截图，验证模型能正确描述图片内容
- 跑阶段 15 评估脚本（增加含表格/图片的评估用例）

---

## 阶段 20: Agent 化与高级能力 🤖
**目标**: 从 RAG 问答升级为具备工具调用、任务分解、联网检索能力的 Agent 系统

### 背景
当前系统是经典的"检索→生成"单链路 RAG，用户问什么就搜什么答什么。主流 Agent 系统具备意图识别、多步推理、工具调用（计算器/代码执行/联网搜索）、任务分解等能力，能处理复杂的多步骤问题。

### 改造思路

**第一步：Function Calling / Tool Use 框架**

- [ ] 新建 `internal/agent/` 包:
  ```go
  // internal/agent/tool.go
  type Tool struct {
      Name        string
      Description string
      Parameters  json.RawMessage // JSON Schema
      Execute     func(ctx context.Context, args map[string]interface{}) (string, error)
  }

  // internal/agent/executor.go
  type AgentExecutor struct {
      llmClient    llm.Client
      tools        []Tool
      maxIterations int
  }
  ```

- [ ] 实现 ReAct 循环（Reasoning + Acting）:
  - LLM 输出 "Thought → Action → Observation" 循环
  - 解析 LLM 返回中的工具调用请求
  - 执行工具 → 将结果反馈给 LLM → 继续推理
  - 设置最大迭代次数防止无限循环

- [ ] 内置工具集:
  - `knowledge_search`: 调用现有 `SearchService.HybridSearch()`（当前 RAG 检索能力变成一个工具）
  - `calculator`: 简单数学计算
  - `current_time`: 返回当前时间
  - `web_search`: 联网搜索（对接 SerpAPI/Tavily/Bing Search API）

**第二步：联网深度搜索（DeepSearch）**

- [ ] 新建 `pkg/websearch/client.go`:
  ```go
  type WebSearchClient interface {
      Search(ctx context.Context, query string, maxResults int) ([]WebSearchResult, error)
  }

  type WebSearchResult struct {
      Title   string
      URL     string
      Snippet string
  }
  ```

- [ ] 实现搜索 + 内容抓取 + 摘要链路:
  - 调用搜索 API 获取 top 5 结果
  - 对每个 URL 抓取正文内容（可用 `go-readability` 提取正文）
  - 将网页内容作为额外上下文喂给 LLM

- [ ] 在 Agent 中注册为 `web_search` 工具:
  - 当知识库检索无结果时，Agent 自动判断是否需要联网搜索
  - 用户也可以显式请求"帮我搜索一下..."

**第三步：意图识别与路由**

- [ ] 新建 `internal/service/intent_router.go`:
  - 用 LLM 做意图分类: 知识问答 / 闲聊 / 联网搜索 / 工具使用
  - 不同意图走不同处理链路:
    ```
    知识问答 → RAG 链路（现有流程）
    闲聊     → 直接 LLM 对话（不检索）
    联网搜索 → web_search 工具
    复杂问题 → Agent ReAct 循环
    ```

- [ ] 在 `ChatHandler` 中集成路由:
  - 替换当前直接调用 `chatService.StreamResponse()` 的逻辑
  - 改为先经过 IntentRouter 判断，再分发到对应处理器

### 验证方式
- 测试闲聊场景不触发检索（节省资源）
- 测试知识库无法回答的问题，Agent 自动联网搜索并整合答案
- 测试多步问题（如"帮我查一下 XX 的最新进展，然后和知识库中的 YY 对比"）

---

### 阶段 15-20 改造路线总览

```
阶段15: 评估体系      ← 一切改进的基础，先能量化才能优化
  ↓
阶段16: 智能分块      ← 数据质量是 RAG 的根基
  ↓
阶段17: 查询增强+重排  ← 检索质量是 RAG 的天花板
  ↓
阶段18: 上下文管理    ← 用好检索结果，防止超窗口/信息丢失
  ↓
阶段19: 多模态        ← 扩展能力边界（图片/表格）
  ↓
阶段20: Agent 化      ← 从 RAG 进化为智能体
```

**核心原则**: 每个阶段改完后，都跑一次阶段 15 的评估脚本，用数据证明改进有效。

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
| 15. RAG 评估体系 |  |  |  |
| 16. 智能分块改造 |  |  |  |
| 17. 查询增强+Reranking |  |  |  |
| 18. 上下文管理升级 |  |  |  |
| 19. 多模态文档处理 |  |  |  |
| 20. Agent 化 |  |  |  |

---

## 开始第一步 🎬

准备好了吗？让我们从**阶段 1: 项目初始化**开始！

告诉我你准备好了，我会帮你一步步完成第一个阶段的代码编写。


