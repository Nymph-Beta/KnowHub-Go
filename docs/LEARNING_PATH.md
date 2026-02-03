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

---

## 开始第一步 🎬

准备好了吗？让我们从**阶段 1: 项目初始化**开始！

告诉我你准备好了，我会帮你一步步完成第一个阶段的代码编写。
