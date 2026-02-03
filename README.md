# KnowHub-Go

从零构建企业级 RAG 知识库系统的学习记录

## 这是什么？

这是一个**正在从零开始构建**的 Go 语言 RAG（检索增强生成）知识库系统学习项目。

不是一个完成的项目，而是一份**开发复现笔记**，记录我如何一步步从零开始搭建一个企业级应用的过程。

## 学习目标

通过从零构建，深入学习：
- Go 语言后端开发
- 微服务架构设计
- RAG 系统实现原理
- 真实项目的迭代开发流程

## 为什么要这样做？

采用**渐进式迭代开发**方式，而非一次性搭建完整框架：
- ✅ 符合真实项目开发流程（MVP → 迭代 → 优化）
- ✅ 每个阶段专注一个技术点，便于理解
- ✅ 会频繁回头修改代码（模拟真实重构场景）
- ✅ 体验从简单到复杂的演进过程

## 当前进度

### 已完成 ✅
- **阶段 1**（2025-01-15）：项目初始化 - Viper 配置管理
- **阶段 2**（2025-01-16）：日志系统 - Zap 日志 + Gin HTTP 服务器 + 优雅停机
- **阶段 3**（2025-01-27）：数据库模型 - MySQL + GORM + User 模型

### 进行中 🚧
- **阶段 4**：JWT 认证系统

### 待开发 📋
- 阶段 5-14：详见 [学习路线](#学习路线)

## 技术栈（按阶段逐步引入）

### 已使用 ✅
- **Gin** - HTTP 框架
- **Viper** - 配置管理
- **Zap** - 日志系统
- **GORM** - ORM 框架
- **MySQL** - 数据库

### 计划引入 📋
- Redis、MinIO、Kafka、Elasticsearch、Apache Tika、DashScope...

## 当前项目结构

```
KnowHub-Go/
├── cmd/
│   └── server/
│       ├── main.go           # ✅ 主程序入口
│       └── gorm_demo.go      # ✅ GORM 测试代码
├── configs/
│   └── config.yaml           # ✅ 配置文件
├── internal/
│   ├── config/               # ✅ 配置加载
│   ├── middleware/           # ✅ 日志中间件
│   └── model/                # ✅ User 模型
├── pkg/
│   ├── log/                  # ✅ Zap 日志封装
│   └── database/             # ✅ MySQL 连接
├── docs/
│   ├── LEARNING_PATH.md      # 详细学习路线
│   └── rebuild_log.md        # 开发记录
└── README.md
```

<details>
<summary>📋 最终目标结构（点击展开）</summary>

```
将逐步增加：
├── internal/
│   ├── handler/              # HTTP/WebSocket 处理器
│   ├── repository/           # 数据访问层
│   ├── service/              # 业务逻辑层
│   └── pipeline/             # 文档处理流水线
├── pkg/
│   ├── hash/                 # 密码加密
│   ├── token/                # JWT
│   ├── storage/              # MinIO
│   ├── kafka/                # Kafka
│   ├── es/                   # Elasticsearch
│   └── ...
```
</details>

## 学习路线

采用 **14 阶段渐进式开发**，每个阶段专注一个技术点。

### 📐 前期（1-4）：打地基
- [x] 阶段 1：配置管理（Viper）
- [x] 阶段 2：日志 & HTTP（Zap + Gin）
- [x] 阶段 3：数据库 & 模型（MySQL + GORM）
- [ ] 阶段 4：JWT 认证

### 🔄 中期（5-10）：迭代增强
- [ ] 阶段 5：Redis & 组织标签
- [ ] 阶段 6：MinIO 简单上传
- [ ] 阶段 7：分片上传（重构阶段6）
- [ ] 阶段 8：Kafka 异步处理（增强阶段7）
- [ ] 阶段 9：文本分块
- [ ] 阶段 10：Elasticsearch & Embedding

### 🎯 后期（11-14）：整合
- [ ] 阶段 11：混合检索
- [ ] 阶段 12：WebSocket 对话
- [ ] 阶段 13-14：完善 & 优化

📖 **详细学习路线** → [docs/LEARNING_PATH.md](docs/LEARNING_PATH.md)

## 快速开始

### 克隆项目
```bash
git clone https://github.com/Nymph-Beta/KnowHub-Go.git
cd KnowHub-Go
```

### 启动 MySQL
```bash
docker run -d -p 3307:3306 \
  -e MYSQL_ROOT_PASSWORD=PaiSmart2025 \
  -e MYSQL_DATABASE=PaiSmart \
  --name mysql mysql:8
```

### 运行项目
```bash
go mod download
go run cmd/server/main.go
```

### 测试
```bash
curl http://localhost:8081/health
# 返回: {"message":"ok"}
```

## 开发记录

- 📖 [详细学习路线](docs/LEARNING_PATH.md) - 14个阶段的完整规划
- 📝 [开发复现日志](docs/rebuild_log.md) - 每个阶段的实现笔记和踩坑记录

## 参考资源

- [Go by Example](https://gobyexample.com/) - Go 语言学习
- [Gin 文档](https://gin-gonic.com/docs/) - HTTP 框架
- [GORM 文档](https://gorm.io/docs/) - ORM 框架

---

⭐ 这是一个学习复现项目，欢迎 Star 见证成长！

📧 问题反馈：通过 Issues 提交
