# PaiSmart-Go v2

一个用 Go 重建的企业级 RAG 知识库后端项目。当前仓库已经不再只是早期学习骨架，而是具备了认证、组织权限、文档处理、混合检索、WebSocket 对话和文档管理等完整后端能力。

## Overview

这个项目的目标是按真实迭代节奏，从零重建一个企业知识库系统，而不是一次性堆出完整框架。到目前为止，仓库已经完成了学习路径中阶段 1 到阶段 13 的后端部分，其中阶段 13 先完成了前端集成前所需的后端能力补齐。

当前系统支持：

- 用户注册、登录、JWT 鉴权、退出登录
- 组织标签管理、用户组织分配、主组织切换
- MinIO 文档上传、分片上传、断点续传、合并
- Kafka + Tika + Embedding + Elasticsearch 的异步文档处理流水线
- 基于组织权限的混合检索
- WebSocket 流式 RAG 对话
- Redis 会话历史保存与 HTTP 查询
- 文档列表、下载链接、在线预览、删除与管理员审计接口

## Current Status

已完成的主线阶段：

- 阶段 1-4：配置、日志、HTTP 服务、MySQL、JWT 认证
- 阶段 5：用户组织标签与权限基础
- 阶段 6-7：简单上传到分片上传演进
- 阶段 8-10：Kafka 异步处理、文本分块、Embedding、Elasticsearch 索引
- 阶段 11：混合检索
- 阶段 12：WebSocket 流式对话
- 阶段 12.5：Prompt 文件化、LLM 协议显式化、最小观测日志
- 阶段 13：文档管理、会话 HTTP 查询、管理员会话审计

当前边界：

- 后端能力已可以支撑前端集成设计
- 仓库内暂未完成前端联调
- 阶段 14+ 的评估、生产化、观测增强仍待继续推进

## Tech Stack

- Go 1.24
- Gin
- GORM + MySQL
- Redis
- MinIO
- Kafka
- Apache Tika
- Elasticsearch
- OpenAI-compatible Embedding / LLM API
- Zap

## Architecture

系统的主链路可以概括为两条：

**文档处理链路**

```text
Client Upload
  -> UploadHandler / UploadService
  -> MinIO
  -> Kafka file-processing topic
  -> Processor
  -> Tika text extraction
  -> chunk split
  -> embedding generation
  -> MySQL document_vectors
  -> Elasticsearch knowledge_base index
```

**RAG 对话链路**

```text
HTTP get websocket token
  -> WebSocket connect
  -> ChatService
  -> HybridSearch
  -> prompt assembly + history
  -> LLM SSE stream
  -> WebSocket chunk push
  -> Redis conversation history
```

## Project Layout

```text
cmd/server                server entrypoint
configs/                  config examples and prompt templates
internal/config           config loading
internal/handler          HTTP / WebSocket handlers
internal/middleware       auth / admin / logging middleware
internal/model            database and DTO models
internal/pipeline         async document processing pipeline
internal/repository       MySQL / Redis persistence layer
internal/service          business services
pkg/database              MySQL / Redis bootstrap
pkg/storage               MinIO bootstrap
pkg/kafka                 Kafka producer / consumer helpers
pkg/tika                  Tika client
pkg/embedding             embedding client
pkg/es                    Elasticsearch client
pkg/llm                   LLM streaming client
pkg/token                 JWT manager
scripts/acceptance        acceptance and probe scripts
docs/                     learning path and rebuild logs
```

## Quick Start

### 1. Prepare dependencies

本项目依赖以下外部服务：

- MySQL
- Redis
- MinIO
- Kafka
- Apache Tika
- Elasticsearch
- Embedding API
- LLM API

仓库当前没有内置 `docker-compose`，请按你自己的环境启动这些依赖，然后把连接信息写到配置文件中。

### 2. Configure

从示例配置开始：

```bash
cp configs/config.example.yaml configs/config.yaml
```

然后按你的本地环境修改：

- `database.mysql.dsn`
- `database.redis.addr`
- `minio.*`
- `kafka.*`
- `tika.base_url`
- `elasticsearch.*`
- `embedding.*`
- `llm.*`

Prompt 模板默认使用：

- [chat_rag_system.tmpl](/home/yyy/Projects/paismart-go-v2/configs/prompts/chat_rag_system.tmpl)

### 3. Run

```bash
go mod download
go run cmd/server/main.go
```

健康检查：

```bash
curl http://localhost:8081/health
```

预期返回：

```json
{"message":"ok"}
```

### 4. Test

```bash
go test ./...
```

如果你想跑阶段性探针和验收脚本，可以看：

- [run_acceptance.sh](/home/yyy/Projects/paismart-go-v2/scripts/acceptance/run_acceptance.sh)
- [stage11_acceptance.sh](/home/yyy/Projects/paismart-go-v2/scripts/acceptance/stage11_acceptance.sh)
- [stage12_acceptance.sh](/home/yyy/Projects/paismart-go-v2/scripts/acceptance/stage12_acceptance.sh)

## API Overview

### Public / health

- `GET /health`
- `GET /ping`

### User

- `POST /api/v1/users/register`
- `POST /api/v1/users/login`
- `GET /api/v1/users/me`
- `POST /api/v1/users/logout`
- `PUT /api/v1/users/primary-org`
- `GET /api/v1/users/org-tags`

### Upload / document processing

- `POST /api/v1/upload/simple`
- `POST /api/v1/upload/check`
- `POST /api/v1/upload/chunk`
- `POST /api/v1/upload/merge`

### Search / chat

- `GET /api/v1/search/hybrid`
- `GET /api/v1/chat/websocket-token`
- `GET /chat/:token`
- `GET /api/v1/users/conversation`

### Document management

- `GET /api/v1/documents/accessible`
- `GET /api/v1/documents/uploads`
- `DELETE /api/v1/documents/:fileMd5`
- `GET /api/v1/documents/download`
- `GET /api/v1/documents/preview`

### Admin

- `GET /api/v1/admin/users`
- `GET /api/v1/admin/users/list`
- `PUT /api/v1/admin/users/:userId/org-tags`
- `GET /api/v1/admin/conversation`
- `POST /api/v1/admin/org-tags`
- `GET /api/v1/admin/org-tags`
- `GET /api/v1/admin/org-tags/tree`
- `PUT /api/v1/admin/org-tags/:id`
- `DELETE /api/v1/admin/org-tags/:id`

## WebSocket Chat

聊天接口采用两步流程：

1. 用已登录的 HTTP 请求获取短时 WebSocket token
2. 用该 token 连接 `/chat/:token`

当前消息协议：

- client: `{"type":"message","content":"..."}`
- client: `{"type":"stop","_internal_cmd_token":"..."}`
- server: `{"type":"started","status":"streaming","_internal_cmd_token":"..."}`
- server: `{"chunk":"..."}`
- server: `{"type":"completion","status":"finished|stopped"}`
- server: `{"error":"..."}`

## Notes

- 文档权限与检索权限使用同一套规则：本人上传、公开文档、有效组织标签可见。
- 文档删除会做完整清理：MySQL、`document_vectors`、Elasticsearch、MinIO、分片记录、Redis 上传标记。
- 会话历史当前存 Redis，不落 MySQL。
- 管理员会话查询使用 Redis `SCAN`，没有使用阻塞式 `KEYS`。
- 下载和预览接口优先建议使用 `fileMd5`，也兼容 `fileName` 查询。

## Documentation

- [LEARNING_PATH.md](/home/yyy/Projects/paismart-go-v2/docs/LEARNING_PATH.md)
- [rebuild_log.md](/home/yyy/Projects/paismart-go-v2/docs/rebuild_log.md)
- [rebuild_log_Part II.md](/home/yyy/Projects/paismart-go-v2/docs/rebuild_log_Part%20II.md)
- [rebuild_log_Part III.md](/home/yyy/Projects/paismart-go-v2/docs/rebuild_log_Part%20III.md)

如果你想看“为什么代码会长成现在这样”，重建日志比 README 更重要；如果你想直接理解“项目现在能做什么、怎么跑”，优先看这个 README。
