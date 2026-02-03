# KnowHub-Go

企业级 RAG 知识库系统 - 从零构建学习项目

## 项目简介

这是一个基于 Go 语言的企业级 RAG（Retrieval-Augmented Generation）知识库系统，通过从零开始构建来深入学习：
- Go 后端开发最佳实践
- 微服务架构设计
- AI 应用开发与集成
- RAG 检索增强生成技术

本项目采用**渐进式迭代开发**的方式，模拟真实项目从 MVP 到完整功能的演进过程。

## 核心特性

- 🔐 **完整的用户认证系统** - JWT Token + 多租户组织管理
- 📁 **大文件分片上传** - 支持断点续传、秒传、并发上传
- 🤖 **智能文档处理** - Apache Tika 文本提取 + 自动分块
- 🔍 **混合检索引擎** - KNN 向量检索 + BM25 关键词检索
- 💬 **流式对话系统** - WebSocket 实时响应 + 上下文管理
- 🏢 **多租户权限控制** - 组织标签 + 层级权限隔离

## 技术栈

### 后端框架
- **Gin** - 高性能 HTTP Web 框架
- **GORM** - 优雅的 ORM 数据库操作
- **Viper** - 灵活的配置管理
- **Zap** - 高性能结构化日志

### 数据存储
- **MySQL 8.0** - 关系型数据库
- **Redis 7.0** - 缓存和分片上传状态管理（Bitmap）
- **MinIO** - 分布式对象存储
- **Elasticsearch 8.x** - 向量检索和全文搜索

### 消息队列
- **Kafka** - 异步文档处理任务队列

### AI 能力
- **Apache Tika** - 多格式文档文本提取
- **阿里云 DashScope** - Embedding 向量化（text-embedding-v3）
- **DeepSeek / Ollama** - LLM 大语言模型对话

## 项目结构

```
KnowHub-Go/
├── cmd/                        # 应用程序入口
│   ├── server/main.go          # 主程序入口（依赖注入、启动流程）
│   └── goenvcheck/             # 环境检查工具
├── configs/                    # 配置文件
│   └── config.yaml             # 主配置文件
├── internal/                   # 内部应用代码（不对外导出）
│   ├── config/                 # 配置加载模块 (Viper)
│   ├── handler/                # HTTP/WebSocket 处理器 (Gin 控制器)
│   │   ├── user_handler.go     # 用户认证（登录/注册/Token刷新）
│   │   ├── upload_handler.go   # 文件上传（分片上传/秒传/合并）
│   │   ├── document_handler.go # 文档管理（列表/删除/详情）
│   │   ├── chat_handler.go     # WebSocket 聊天（流式对话）
│   │   └── admin_handler.go    # 管理员接口（用户/组织管理）
│   ├── middleware/             # 中间件
│   │   ├── auth.go             # JWT 认证中间件
│   │   ├── admin_auth.go       # 管理员权限中间件
│   │   └── logging.go          # 请求日志中间件
│   ├── model/                  # 数据模型
│   │   ├── user.go             # 用户模型
│   │   ├── upload.go           # 文档上传模型
│   │   ├── document_vector.go  # 文档向量块模型
│   │   ├── conversation.go     # 对话历史模型
│   │   └── org_tag.go          # 组织标签模型
│   ├── pipeline/               # 文档处理流水线
│   │   └── processor.go        # 文档处理器（Tika → 分块 → 向量化 → ES）
│   ├── repository/             # 数据访问层 (DAO)
│   │   ├── user_repository.go
│   │   ├── upload_repository.go
│   │   ├── document_vector_repository.go
│   │   └── conversation_repository.go
│   └── service/                # 业务逻辑层
│       ├── user_service.go     # 用户服务（认证/注册）
│       ├── upload_service.go   # 上传服务（分片管理/合并）
│       ├── document_service.go # 文档服务（列表/删除/统计）
│       ├── search_service.go   # 检索服务（混合检索/权限过滤）
│       ├── chat_service.go     # 聊天服务（对话管理/流式输出）
│       └── admin_service.go    # 管理员服务（用户/组织管理）
├── pkg/                        # 可复用的公共包
│   ├── database/               # 数据库初始化 (MySQL, Redis)
│   ├── embedding/              # 向量化客户端 (DashScope)
│   ├── es/                     # Elasticsearch 客户端
│   ├── hash/                   # 密码哈希工具 (bcrypt)
│   ├── kafka/                  # Kafka 生产者/消费者
│   ├── llm/                    # LLM 客户端 (DeepSeek/Ollama)
│   ├── log/                    # 日志封装 (Zap)
│   ├── storage/                # MinIO 对象存储客户端
│   ├── tika/                   # Tika 文档解析客户端
│   └── token/                  # JWT Token 管理
├── frontend/                   # Vue 3 前端项目（待开发）
├── deployments/                # 部署配置
│   └── docker-compose.yaml     # Docker Compose 编排文件
├── docs/                       # 文档
│   ├── LEARNING_PATH.md        # 详细学习路线
│   ├── rebuild_log.md          # 开发复现记录
│   └── ddl.sql                 # 数据库表结构
├── logs/                       # 运行日志
├── go.mod                      # Go 模块依赖
└── README.md                   # 本文档
```

## 学习路线

项目分为 14 个阶段，采用渐进式迭代开发模式：

### 📐 前期阶段（1-4）：打地基
1. ✅ **项目初始化** - Viper 配置管理
2. ✅ **日志 & HTTP** - Zap 日志系统 + Gin HTTP 服务器 + 优雅停机
3. ✅ **数据库 & 模型** - MySQL 连接 + GORM + User 模型
4. **JWT 认证** - 用户注册/登录 + JWT 中间件

### 🔄 中期阶段（5-10）：迭代增强
5. **Redis & 组织** - Redis 集成 + 多租户组织标签
6. **MinIO & 上传** - 对象存储 + 简单文件上传
7. **分片上传** - 大文件分片 + Redis Bitmap 状态跟踪
8. **Kafka & Tika** - 异步队列 + 文档文本提取
9. **文本分块** - 智能分块算法 + 持久化
10. **ES & Embedding** - Elasticsearch 向量索引 + DashScope 向量化

### 🎯 后期阶段（11-14）：整合所有
11. **混合检索** - KNN 向量检索 + BM25 关键词检索 + 权限过滤
12. **WebSocket 对话** - 流式 LLM 响应 + 对话历史管理
13. **完善功能** - 文档管理 + 管理员功能 + 前端集成
14. **优化部署** - 性能优化 + 监控 + 生产环境部署

📖 **详细学习路线请查看** → [docs/LEARNING_PATH.md](docs/LEARNING_PATH.md)

## 为什么采用渐进式迭代？

### ✅ 符合真实开发流程
```
真实项目: MVP → 功能迭代 → 性能优化 → 完善细节
本项目:   简单上传(阶段6) → 分片上传(阶段7) → 异步处理(阶段8)
```

### ✅ 学习更有层次感
- **阶段 6**: 专注学习 MinIO 基本操作
- **阶段 7**: 在熟悉基础上学习分片原理
- **阶段 8**: 在熟悉上传基础上学习异步解耦

### ✅ 培养重构能力
- 模拟真实工作中的代码迭代
- 理解"开闭原则"（对扩展开放，对修改关闭）
- 学会在保留核心逻辑基础上扩展功能

## 快速开始

### 前置要求
- Go 1.21+
- Docker & Docker Compose
- MySQL 8.0
- Redis 7.0

### 1. 克隆项目

```bash
git clone https://github.com/your-username/KnowHub-Go.git
cd KnowHub-Go
```

### 2. 安装依赖

```bash
go mod download
```

### 3. 启动基础服务

```bash
# 启动 MySQL
docker run -d -p 3307:3306 \
  -e MYSQL_ROOT_PASSWORD=PaiSmart2025 \
  -e MYSQL_DATABASE=PaiSmart \
  --name mysql mysql:8

# 启动 Redis
docker run -d -p 6379:6379 --name redis redis:7-alpine
```

### 4. 配置文件

编辑 `configs/config.yaml`：

```yaml
server:
  port: "8081"
  mode: "debug"

log:
  level: "debug"
  format: "json"

database:
  mysql:
    dsn: "root:PaiSmart2025@tcp(localhost:3307)/PaiSmart?charset=utf8mb4&parseTime=True&loc=Local"
```

### 5. 运行项目

```bash
go run cmd/server/main.go
```

### 6. 测试接口

```bash
# 健康检查
curl http://localhost:8081/health

# 注册用户（阶段 4 完成后可用）
curl -X POST http://localhost:8081/api/v1/users/register \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"123456"}'
```

## 开发进度

| 阶段 | 状态 | 完成日期 | 备注 |
|------|------|----------|------|
| 1. 项目初始化 | ✅ | 2025-01-15 | Viper 配置管理 |
| 2. 日志 & HTTP | ✅ | 2025-01-16 | Zap + Gin + 优雅停机 |
| 3. 数据库 & 模型 | ✅ | 2025-01-27 | MySQL + GORM + User 模型 |
| 4. 用户认证 | 🚧 |  | 进行中 |
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

📝 **详细开发记录** → [docs/rebuild_log.md](docs/rebuild_log.md)

## 核心功能预览

### 文件上传流程
```
1. 前端计算文件 MD5
2. 调用 /upload/check 检查是否秒传
3. 如果需要上传，将文件分片（每片 5MB）
4. 并发上传分片到 /upload/chunk（Redis Bitmap 记录进度）
5. 调用 /upload/merge 合并分片
6. 后台 Kafka 异步处理（Tika 提取 → 分块 → 向量化 → ES 索引）
```

### 混合检索流程
```
1. 用户输入查询词
2. 调用 DashScope 生成查询向量
3. Elasticsearch 执行：
   - KNN 向量检索（k=topK*30）
   - BM25 关键词检索
   - Rescore 二次排序
   - 权限过滤（public / org_tag / user_id）
4. 返回 Top-K 文档块
```

### WebSocket 对话流程
```
1. 获取临时 WebSocket Token
2. 建立 WebSocket 连接
3. 发送用户消息
4. 后端调用混合检索获取上下文
5. 构建 Prompt 调用 LLM
6. 流式返回 LLM 响应
7. 保存对话历史到 Redis
```

## 学习资源

### Go 语言
- [Go by Example](https://gobyexample.com/) - 通过实例学习 Go
- [Effective Go](https://go.dev/doc/effective_go) - Go 最佳实践

### 框架文档
- [Gin 文档](https://gin-gonic.com/docs/) - HTTP 框架
- [GORM 文档](https://gorm.io/docs/) - ORM 框架
- [Viper 文档](https://github.com/spf13/viper) - 配置管理

### 中间件文档
- [Elasticsearch Go](https://github.com/elastic/go-elasticsearch) - ES 客户端
- [Sarama](https://github.com/IBM/sarama) - Kafka Go 客户端
- [MinIO Go SDK](https://min.io/docs/minio/linux/developers/go/minio-go.html) - 对象存储

## 常见问题

### Q1: 为什么不一次性实现完整功能？
A: 渐进式开发更符合真实项目流程，每个阶段专注一个技术点，便于理解和调试。

### Q2: 可以跳过某些阶段吗？
A: 不建议。后续阶段会频繁修改前期代码，跳过会导致理解断层。

### Q3: 如何对比我的代码和原项目？
A: 每个阶段完成后，对比原项目对应部分，理解设计差异。

### Q4: 遇到问题怎么办？
A: 1) 查看 `docs/rebuild_log.md` 开发记录；2) 对照 `docs/LEARNING_PATH.md` 学习路线；3) 参考原项目代码。

## License

MIT License

## 致谢

本项目为学习复现项目，感谢原项目提供的参考和灵感。

---

⭐ 如果这个项目对你有帮助，欢迎 Star！

📧 问题反馈：通过 Issues 提交
