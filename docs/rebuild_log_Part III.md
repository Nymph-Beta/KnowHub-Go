# 阶段十二 WebSocket 流式对话

阶段十一已经把 Retrieval 跑通，阶段十二补的是完整 RAG 的最后一段链路：

```text
WebSocket 客户端消息
  -> ChatHandler 校验临时 ws token
  -> ChatService 调 HybridSearch 检索 chunk
  -> 构造 system prompt + 对话历史
  -> LLM SSE 流式生成
  -> WebSocket 按 chunk 回推
  -> Redis 保存最近一段会话历史
```

这一步完成后，系统第一次同时具备：

- 检索结果增强生成
- WebSocket 流式输出
- 可中断的生成过程
- 基于 Redis 的多轮上下文

---

## 1. 配置与令牌扩展

更新：

- `internal/config/config.go`
- `configs/config.yaml`
- `pkg/token/jwt.go`

阶段十二新增了 `llm` 配置段，分成三类参数：

- 基础连接：`api_key / base_url / model / timeout_seconds`
- 流式生成：`temperature / top_p / max_tokens`
- Prompt 与 ws token：`rules / ref_start / ref_end / no_result_text / websocket_token_expire_minutes`

同时把 JWT 扩展出 `TokenTypeWebSocket`，并新增 `GenerateWebSocketToken`。这样客户端不再直接拿长期 access token 去连 WebSocket，而是先通过受保护的 HTTP 接口换一个短时 token，再用它升级连接。

---

## 2. LLM 客户端：把 SSE 变成 chunk

新增：

- `pkg/llm/client.go`
- `pkg/llm/client_test.go`

这里沿用了阶段十 `pkg/embedding` 的风格，单独封装了一个 `llm.Client`：

```go
type Client interface {
    StreamChat(ctx context.Context, messages []Message, writer MessageWriter) error
}
```

`StreamChat` 做的事情比较单纯：

1. 组装 `/chat/completions` 请求
2. 打开 `stream=true` 的 SSE 响应
3. 逐行读取 `data: {...}`
4. 提取 `choices[0].delta.content`
5. 把每个内容块写给 `MessageWriter`

这样 `pkg/llm` 只关心 LLM 协议本身，不关心 WebSocket、Gin 或 Redis。

---

## 3. Redis 会话历史

新增：

- `internal/model/conversation.go`
- `internal/repository/conversation_repository.go`

会话历史没有落 MySQL，而是按阶段十二的目标先存 Redis。当前 key 设计是：

- `user:{uid}:current_conversation`
- `conversation:{conversationID}`

仓储负责三件事：

1. 获取或创建当前 conversation ID
2. 读取消息历史
3. 保存更新后的历史

实现里保留了两个约束：

- TTL 固定为 7 天
- 历史长度截断到最近 20 条消息

这让多轮上下文能工作，但不会无限膨胀。

---

## 4. ChatService：阶段十二真正的编排层

新增：

- `internal/service/chat_service.go`
- `internal/service/chat_service_test.go`

`ChatService.StreamResponse` 串起了完整流程：

1. 读取/创建当前会话
2. 调 `SearchService.HybridSearch`
3. 将 chunk 拼成 `<<REF>> ... <<END>>` 参考资料块
4. 组装 `system + history + user` 消息数组
5. 调 `llmClient.StreamChat`
6. 通过 `wsWriterInterceptor` 把每个 token 包装成 `{"chunk":"..."}` 发给前端
7. 发送 `{"type":"completion","status":"finished|stopped"}`
8. 用 `context.Background()` 回写 Redis 历史

`wsWriterInterceptor` 是这个阶段最关键的适配器。它一边把 token 转成 WebSocket JSON chunk，一边把内容累积到 `strings.Builder`，保证流式传输和“最终保存完整回答”使用的是同一份数据源。

如果检索结果为空，当前实现不会继续调用 LLM，而是直接返回 `no_result_text`。这是为了让“知识库没有命中”这个场景保持确定性，而不是让模型在无参考资料时自由发挥。

---

## 5. ChatHandler：WebSocket 协议与停止控制

新增：

- `internal/handler/chat_handler.go`
- `internal/handler/chat_handler_test.go`

新增的两个入口分别是：

- `GET /api/v1/chat/websocket-token`
- `GET /chat/:token`

`GetWebSocketToken` 运行在 JWT 认证路由下，只负责签发短时 ws token。真正的对话循环在 `HandleWebSocket` 中：

- 校验路径里的 ws token
- 升级连接
- 循环读取客户端消息
- 收到 `type=message` 时启动一次流式生成
- 收到 `type=stop` 时取消当前生成 context

这里专门加了 `wsJSONWriter` 做串行写保护。`gorilla/websocket.Conn` 不能并发写，如果不把 chunk 推送、错误回包、开始通知串到一个互斥写器上，很容易在 stop 或异常路径上把连接写坏。

当前消息协议已经收敛为：

- 客户端：`{"type":"message","content":"..."}`
- 客户端：`{"type":"stop","_internal_cmd_token":"..."}`
- 服务端：`{"type":"started","status":"streaming","_internal_cmd_token":"..."}`
- 服务端：`{"chunk":"..."}`
- 服务端：`{"type":"completion","status":"finished|stopped"}`
- 服务端：`{"error":"..."}`

---

## 6. 主程序接线

更新：

- `cmd/server/main.go`
- `go.mod`
- `go.sum`

`main.go` 现在会额外初始化：

- `ConversationRepository`
- `llm.Client`
- `ChatService`
- `ChatHandler`

并注册：

- `/api/v1/chat/websocket-token`
- `/chat/:token`

这里还加了一层可用性收口：只有在 `LLM` 和阶段十一的检索依赖都初始化成功时，chat 功能才会真正开放；否则 HTTP 服务照常启动，但 chat 接口返回 `503`。

---

## 7. 测试结果

新增测试覆盖了三块：

- `pkg/llm/client_test.go`：SSE 解析、非 200 错误
- `internal/service/chat_service_test.go`：正常流式、空检索结果、停止场景
- `internal/handler/chat_handler_test.go`：ws token 签发、服务不可用兜底

本地验证命令：

```bash
GOCACHE=/tmp/paismart-go-cache \
GOMODCACHE=/tmp/paismart-go-modcache \
go test ./...
```

结果：全仓 `go test ./...` 通过。

---

## 8. 本阶段更新文件

| 操作 | 文件 |
| --- | --- |
| 新增 | `pkg/llm/client.go` |
| 新增 | `pkg/llm/client_test.go` |
| 新增 | `internal/model/conversation.go` |
| 新增 | `internal/repository/conversation_repository.go` |
| 新增 | `internal/service/chat_service.go` |
| 新增 | `internal/service/chat_service_test.go` |
| 新增 | `internal/handler/chat_handler.go` |
| 新增 | `internal/handler/chat_handler_test.go` |
| 更新 | `internal/config/config.go` |
| 更新 | `configs/config.yaml` |
| 更新 | `pkg/token/jwt.go` |
| 更新 | `cmd/server/main.go` |
| 更新 | `go.mod` |
| 更新 | `go.sum` |

---

## 阶段十二.5 小收口：Prompt 文件化、LLM 接口显式化、最小观测

在阶段十二完成之后，又补了一次小收口，但这里有意控制了改造范围，没有把系统直接推向“多模型平台”。

### 1. 为什么现在只做小收口，不直接做完整抽象

如果完全按成熟 RAG 平台的思路继续扩展，下一步通常会想做：

- 多 Provider 统一抽象
- Prompt Center / Prompt 版本管理
- 多链路路由
- 更细的上下文预算与会话治理

这些方向本身没有问题，但它们都已经超出了阶段十二“先把 RAG 对话闭环跑通”的边界。当前学习路径中，更适合承接这些复杂度的是：

- **阶段 15**：先建立评估体系，避免 Prompt 或模型改造只能靠主观感觉判断
- **阶段 18**：再做上下文编排、记忆治理和 token-aware prompt 结构
- **阶段 20**：再做多链路路由、工具调用和更复杂的模型/能力分发

所以阶段十二.5 的原则不是“做得越多越好”，而是：

- 先把最容易造成后续返工的硬编码点拔掉
- 但不提前引入后面阶段才需要的大型抽象

### 2. Prompt 改为模板文件加载

更新：

- `configs/prompts/chat_rag_system.tmpl`
- `configs/config.yaml`
- `internal/config/config.go`
- `internal/service/chat_service.go`
- `internal/config/config_test.go`

现在 `llm.prompt.template_file` 指向一个外部模板文件，而不是把完整 system prompt 内联在 YAML 里。启动时配置加载会把模板文件内容读入内存，`ChatService` 在运行时再把：

- `RefStart`
- `RefEnd`
- `References`

填进模板，生成最终 system prompt。

这样做的价值不在于“换了个存放位置”，而在于把后续最频繁迭代的部分从代码和主配置中拆了出来。到了阶段十五做评估、阶段十八做上下文编排时，Prompt 迭代会远比当前频繁；如果此时仍然把整段提示词塞在 `config.yaml` 里，维护成本会迅速上升。

这里仍然保留了 `rules / ref_start / ref_end` 作为 fallback，而没有立刻删掉，原因是要保持兼容性，避免一次小收口把现有链路变成“只能依赖模板文件启动”。

### 3. `provider` / `api_style` 显式化

更新：

- `internal/config/config.go`
- `configs/config.yaml`
- `pkg/llm/client.go`
- `pkg/llm/client_test.go`

阶段十二的实现虽然默认配置写的是 DeepSeek，但客户端实际上走的是 OpenAI-compatible `chat/completions` 协议。为了避免“代码其实是通用协议，文档却看起来像 DeepSeek 专用”的语义混淆，这里显式增加了：

- `llm.provider`
- `llm.api_style`

当前只支持：

```text
api_style = openai_compatible
```

并且在 `llm.NewClient` 初始化时直接校验。如果未来要支持其他协议族，比如 Anthropic Messages、Gemini 原生 schema 或本地私有协议，可以在不打破当前配置语义的前提下继续扩展。

这里故意没有在阶段十二.5 就做 `Provider` 接口层、工厂注册表或多模型路由。原因很明确：那会把“单链路流式对话”问题提前升级成“模型平台”问题，复杂度和学习焦点都会偏掉。

### 4. 给 Chat 链路补最小观测日志

更新：

- `internal/service/chat_service.go`

为了给阶段十五的评估与观测体系预埋基础，当前 chat 流程新增了三组结构化日志：

1. `chat stream started`
2. `chat retrieval completed`
3. `chat stream finished`

当前记录的字段控制在“有用但不过量”的范围内，包括：

- `user_id`
- `conversation_id`
- `provider`
- `api_style`
- `model`
- `question_preview`
- `question_len`
- `hits`
- `status`
- `answer_len`
- `latency_ms`

这一步的目标不是现在就做完整线上观测，而是让后面阶段十五接评估报表、做回归对比时，服务侧已经有最基础的运行痕迹可对照。

### 5. 为什么只做到这里

当前实现仍然是“阶段十二级别”的设计，而不是“成熟 RAG 平台”的设计，主要体现在：

- 只支持一种 API 协议族，而不是多协议 Provider
- Prompt 只是文件模板化，还没有版本管理、灰度和评估闭环
- 观测日志只是最小埋点，还没有阶段十五所要求的完整评估报告

这是刻意取舍，不是遗漏。因为当前阶段最重要的是先保持：

- 对话链路稳定可运行
- 配置语义清晰
- 最容易返工的硬编码点先抽掉

而不是在没有评估基线的前提下，过早把系统推进到高度抽象化。

### 6. 接下来更合理的演进顺序

在当前阶段十二.5 之后，更推荐的后续顺序是：

1. **阶段 15**：先建立 retrieval / generation 评估和基线报告
2. **阶段 16-17**：继续提升检索质量，而不是先折腾模型平台
3. **阶段 18**：再把 prompt、history、retrieval context 纳入 token-aware 上下文编排
4. **阶段 20**：再考虑多链路路由、多模型能力分发、Agent 化

也就是说，阶段十二.5 解决的是“别把阶段十八和二十最容易返工的点继续写死”；但真正的深层改造，仍然应该按学习路径在后续阶段推进。

---

# 阶段十三（后端先行版）完善功能：文档管理、会话查询、管理员审计

阶段十三原本包含“完善功能 + 前端集成”，但这一轮有意只先补后端前置能力，不启动前端。

原因很直接：如果文档管理、HTTP 会话历史和管理员审计接口还没稳定，前端联调只会把问题混在一起，定位成本很高。更合理的顺序是：

1. 先把阶段十三要求的后端接口补齐
2. 再确认接口语义和权限模型稳定
3. 最后再进入前端集成设计与联调

这次改造完成后，系统已经具备“可以进入前端集成设计”的后端基础。

## 1. 文档管理从上传域里独立出来

新增：

- `internal/service/document_service.go`
- `internal/handler/document_handler.go`
- `internal/service/storage_keys.go`

阶段七到阶段十二期间，文件相关能力主要围绕“上传”和“处理流水线”展开；但到了阶段十三，真正缺的是“已入库文档的管理能力”。

所以这一步没有继续把逻辑塞进 `UploadService`，而是单独抽出了 `DocumentService`。这样职责边界会更清晰：

- `UploadService` 负责上传、分片、合并
- `DocumentService` 负责列表、删除、下载、预览

当前补齐的接口是：

- `GET /api/v1/documents/accessible`
- `GET /api/v1/documents/uploads`
- `DELETE /api/v1/documents/:fileMd5`
- `GET /api/v1/documents/download`
- `GET /api/v1/documents/preview`

这组接口都挂在认证路由下，直接复用现有登录态。

## 2. 文档权限模型与阶段十一检索保持一致

更新：

- `internal/repository/upload_repository.go`
- `internal/repository/org_tag_repository.go`

阶段十三最容易做错的地方，是文件列表权限和检索权限写成两套不同规则。当前实现刻意复用了阶段十一的权限口径：

```text
status = 1 AND (
  user_id = 当前用户
  OR is_public = true
  OR org_tag IN 用户有效组织标签
)
```

这里不是直接拿 `user.OrgTags` 原始字符串做判断，而是继续走 `GetUserEffectiveOrgTags`，保证“文档可见范围”和“检索可见范围”一致，不会出现：

- 某文档能被检索命中，但列表里看不到
- 或者列表里能看到，但检索时又被权限过滤掉

另外，上传列表和可访问列表返回时，会批量补上 `orgTagName`，避免前端收到一堆只能自己再查名字的标签 ID。

## 3. 删除文档做成完整清理，而不是只删一条 MySQL 记录

更新：

- `pkg/es/client.go`
- `internal/repository/document_vector_repository.go`（复用已有删除能力）
- `internal/repository/upload_repository.go`
- `internal/service/document_service.go`

这一步没有沿用“只删对象文件和 `file_uploads` 记录”的最小实现，而是直接做成完整清理链路。删除一个文档时，现在会依次处理：

1. 删除 Elasticsearch 中该 `file_md5` 对应的检索文档
2. 删除 MySQL `document_vectors`
3. 删除 MinIO 中的最终合并文件
4. 删除可能残留的分片对象
5. 删除 Redis 上传标记
6. 删除 `chunk_infos`
7. 最后删除 `file_uploads`

这里故意把主记录放在最后删。原因是只要主记录还在，系统就仍然保留了：

- 对象键推导信息
- 用户归属信息
- chunk 关联信息

一旦先删主记录，中途任何一步失败，后续清理就会变得更被动。

这一步的价值不只是“删除功能可用”，而是避免留下检索脏数据。否则用户虽然看起来删掉了文件，但 ES 命中和 RAG 对话上下文里仍可能继续出现已删除内容。

## 4. 下载与预览接口兼容前端，但内部优先用 `fileMd5`

当前下载和预览接口都支持两种查询方式：

- `fileMd5`
- `fileName`

内部优先按 `fileMd5` 处理，因为它才是稳定主键；但为了后面接现有前端，仍然兼容 `fileName` 查询。

这里额外做了一个安全约束：如果只传 `fileName` 且当前用户可访问范围内命中了多个同名文件，接口不会猜测目标，而是直接返回参数错误，要求调用方改用 `fileMd5`。

这样可以兼顾两个目标：

- 现在先不改前端依赖方式
- 后面真正联调时也不会因为同名文件把下载/预览指错对象

预览能力本身是通过：

- MinIO 取对象流
- Tika 提取纯文本

来完成的。如果 Tika 客户端未初始化，预览接口会明确返回 `503`，而不是伪装成“文件不存在”。

## 5. 对话历史不再只存在于 WebSocket 内部

新增：

- `internal/service/conversation_service.go`
- `internal/handler/conversation_handler.go`

阶段十二时，会话历史虽然已经落 Redis，但它只服务于 WebSocket 对话流程本身。阶段十三补的是“把这份历史变成可被 HTTP 读取的业务能力”。

当前新增了两个接口：

- `GET /api/v1/users/conversation`
- `GET /api/v1/admin/conversation`

这里没有让 handler 直接拼装 `GetConversationID + GetConversationHistory`，而是抽了 `ConversationService`，把会话读取逻辑集中起来。这样可以避免：

- chat 链路自己管一套会话访问
- HTTP 查询再各写一套

用户接口返回当前会话历史；管理员接口返回跨用户聚合后的会话消息列表，并支持：

- `userid`
- `start_date`
- `end_date`

三种过滤维度。

## 6. 管理员会话扫描显式改为 `SCAN`，不使用 `KEYS`

更新：

- `internal/repository/conversation_repository.go`

管理员查看所有对话，本质上需要先从 Redis 找到：

```text
user:*:current_conversation
```

对应的映射关系。

这里刻意没有用最省事的 `KEYS`，而是改成了 `SCAN`。原因很明确：

- `KEYS` 会阻塞式扫描整个 key 空间
- key 数量一大就会影响 Redis 实例响应

对当前学习项目来说，`KEYS` 也许“能跑”；但阶段十三已经属于会被前端和管理员直接依赖的能力，继续把这种明显不适合扩展的实现写进仓库，只会把后续返工留到更晚。

所以这里虽然仍是简化版实现，但至少在 key 扫描方式上，已经朝更合理的方向收口。

## 7. 路由兼容性小收口

更新：

- `cmd/server/main.go`

除了新增文档管理和会话查询接口外，这一步还额外补了一个兼容别名：

- `GET /api/v1/admin/users/list`

当前它直接复用已有的管理员用户列表逻辑。这样做的目的不是增加一套新实现，而是给后面接前端时保留兼容面，避免因为“后端已经有 `/admin/users`，前端却写的是 `/admin/users/list`”而出现无意义的联调阻塞。

## 8. 测试补齐情况

新增或扩展测试覆盖了：

- `internal/service/document_service_test.go`
- `internal/service/conversation_service_test.go`
- `internal/handler/document_handler_test.go`
- `internal/handler/conversation_handler_test.go`
- `internal/repository/conversation_repository_test.go`
- `internal/repository/upload_repository_test.go`
- `internal/repository/org_tag_repository_test.go`
- `pkg/es/client_test.go`

重点验证的场景包括：

- 文档可访问列表权限过滤
- `orgTagName` 批量回填
- 删除文档的完整清理顺序
- 同名文件下载/预览歧义处理
- 用户会话历史查询
- 管理员按用户和日期过滤会话
- Redis `SCAN` 会话映射读取
- Elasticsearch 按 `file_md5` 删除

本地验证命令：

```bash
GOCACHE=/tmp/paismart-go-cache \
/home/yyy/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.11.linux-amd64/bin/go test ./...
```

结果：全仓 `go test ./...` 通过。

## 9. 到这里为什么可以进入前端集成设计

当前阶段十三后端先行版完成后，前端最依赖的基础能力已经具备：

- 用户能看到自己可访问的文档
- 用户能看到自己上传的文档
- 用户能删除自己的文档
- 用户能下载和预览文档
- 用户能通过 HTTP 查看会话历史
- 管理员能查看所有对话记录

也就是说，现在缺的已经不再是“后端有没有接口”，而是：

- 前端页面怎么组织这些能力
- 前端调用参数统一走 `fileMd5` 还是兼容 `fileName`
- 页面状态和交互如何对齐现有响应格式

所以从阶段节奏上说，这里就是一个很合适的分界点：

**后端前置必要功能已经实现完，可以开始前端集成设计。**

---

# 阶段十三.5 前端集成首版：零构建工作台与联调闭环

这一步开始真正进入前端实现，但在动手之前，先回头补了三个会直接卡住联调的后端缺口。原因不是“后端必须完美才配写前端”，而是这三个点已经属于前端运行时依赖，而不是可选增强：

1. `POST /api/v1/auth/refreshToken`
2. `GET /api/v1/upload/supported-types`
3. `GET /api/v1/upload/status` 与 `POST /api/v1/upload/fast-upload`

另外还顺手收了一个权限差异：`DeleteDocument` 不再只允许文件所有者删除；当前实现里，`ADMIN` 也可以删除他人的文件记录。这里额外加了 `userId` 查询参数来消除歧义，因为在当前数据模型下，同一个 `fileMd5` 允许被不同用户各自持有一条记录；如果管理员删除时不显式指定目标用户，遇到重复 `fileMd5` 会出现“删谁都不够严谨”的问题。

## 1. 为什么这几个后端点必须先补

### Refresh Token 不是“优化项”，而是前端会话链路本体

前端只要开始做统一的请求封装，就一定会碰到 access token 过期。此时如果没有：

- `POST /api/v1/auth/refreshToken`

那么请求层一旦收到 `401`，只能直接清掉登录态并把用户踢回登录页。那样虽然“能跑”，但不是真正的登录体验，也无法验证后端 refresh token 语义是否正确。

所以这一轮先把：

- `UserService.RefreshToken`
- `UserHandler.RefreshToken`
- `main.go` 路由注册

一起补齐，并在前端请求封装里接成自动刷新链路：受保护请求遇到 `401` 时，优先用 refresh token 换新 access token，成功后自动重试原请求；只有刷新失败才真正退出登录。

### `supported-types` 决定上传控件能否和后端白名单保持一致

后端上传早就有扩展名白名单，但如果前端不通过接口读取它，就只能把允许类型再手写一份：

- 前端写一份 `accept`
- 后端再写一份 `allowedExtensions`

这种做法短期看起来快，长期一定漂移。当前实现把支持的扩展名直接通过：

- `GET /api/v1/upload/supported-types`

暴露给前端，上传控件的 `accept` 由接口结果驱动。这样后面只要后端白名单增减，前端不用再跟着改常量。

### `status` 与 `fast-upload` 先补兼容面，避免后续再返工上传组件

这版前端首发并没有把完整的浏览器端分片上传全部做进去，而是先用简单上传把联调主链路打通。但原始前端的上传域接口约定里，本来就包含：

- 秒传检查
- 上传进度查询

如果现在完全跳过，后面扩展上传组件时还得再改一次接口封装和页面状态模型。更合理的做法是先把兼容接口补齐：

- `POST /api/v1/upload/fast-upload`
- `GET /api/v1/upload/status`

这样当前页面虽然只把它们做成“工具位”能力，但前端状态结构已经和后续增强方向对齐了。

## 2. 为什么首版前端不用 React/Vite，而是静态 SPA

这里刻意没有继续开一个新的 `frontend/` 工程、也没有引入 Vite/React，不是因为它们不好，而是因为在当前阶段里，那样做的收益不够高。

当前更重要的是：

1. 先让后端接口真的被页面调用起来
2. 先验证认证刷新、文档操作、搜索、聊天的联调闭环
3. 先把“页面组织方式”试出来，而不是先搭建另一套工程基础设施

因此这版前端采用的是：

- `web/index.html`
- `web/styles.css`
- `web/app.js`

再由 Gin 直接托管：

- `GET /` -> 重定向到 `/app/`
- `r.Static("/app", "./web")`

这样做的好处很直接：

- 不新增 Node 依赖和构建步骤
- 和当前 Go 仓库天然同源，避免 CORS 和多端口切换
- 页面一写完就能直接随着服务跑起来
- 如果后面确定要迁到 React/Vite，当前这版也能先作为交互原型和接口样板

换句话说，这一版的目标不是“框架选型定终局”，而是**用最小工程复杂度完成第一次真实前后端闭环**。

## 3. 前端页面怎么组织

最终没有拆成传统后台那种左侧深层菜单，而是先做成一个单页工作台，核心区域按真实联调顺序展开：

1. Session：登录、注册、当前用户信息
2. Upload：上传、支持类型、秒传检查、上传状态查询
3. Documents：可访问文档与我的上传
4. Search：混合检索结果面板
5. Chat：WebSocket 对话与 HTTP 会话历史
6. Admin：管理员专属视图（用户 / 组织树 / 会话审计）

这种组织方式比“先按后端模块一比一映射菜单”更适合当前阶段，因为它更贴近真实使用顺序：

- 先登录
- 再上传
- 再看文档
- 再检索
- 最后进入对话与管理

也更容易在一个页面里观察“上传 -> 搜索 -> 对话”是否真的串起来。

## 4. 前端请求层怎么收口

这一轮前端没有把 `fetch` 散落在每个按钮回调里，而是先做了一个统一的请求封装：

- 自动附带 `Authorization: Bearer <accessToken>`
- 统一解析后端 `{code,message,data}` 结构
- 收到 `401` 时自动尝试 refresh
- refresh 成功后自动重放原请求
- refresh 失败后统一清空本地会话

这一步非常关键。因为一旦不先把认证逻辑收进同一个入口，后面每个文档操作、搜索请求、管理员请求都会各自长出一套 401 处理分支，代码很快会变乱。

## 5. 聊天为什么仍然保留“HTTP 获取 ws token + WebSocket 连接”的两段式

前端聊天区域没有偷懒成“直接拿 access token 连 WebSocket”，而是严格跟后端设计保持一致：

1. 先用受保护 HTTP 接口拿 `cmdToken`
2. 再连接 `/chat/:token`

这样做的价值在阶段十二就已经确定了：WebSocket 连接使用的是短时 token，不直接暴露长期 access token。前端这轮做的事情，只是把这套设计真正跑进浏览器，并补上：

- started / chunk / completion 协议处理
- stop 指令发送
- 聊天完成后刷新 HTTP 会话历史

也就是说，当前页面不只是“能发消息”，而是第一次把：

- HTTP 登录态
- WebSocket 临时令牌
- 流式响应
- Redis 历史读取

这些原本分散在后端的能力统一展示在了一个前端界面里。

## 6. 文档删除为什么要把管理员分支做进前端参数模型

当前文档卡片上的删除按钮，会根据身份决定行为：

- 普通用户：删除自己的文件
- 管理员：如果删除的是他人文件，会附带 `userId`

这一步看起来只是多传了一个参数，但它背后实际上是把“管理员删除能力”从一句文档描述，真正变成了可被前端稳定调用的协议。否则管理员虽然理论上“有权限”，但前端并不知道该怎么精准指定目标记录。

## 7. 视觉层面的取舍

这一版刻意没有做成默认白底表格后台，而是用了更偏工作台的视觉语言：

- 暖色纸面背景 + 青绿色信息色
- serif 标题 + sans 正文
- 大块卡片 + 单页纵向工作流

原因不是为了“炫”，而是因为当前页面承载的是上传、检索、对话这些连续操作；如果完全做成传统 CRUD 后台样式，很容易把搜索和聊天也做成“又一个表单页面”，读起来不够像同一个产品。

不过这里也有明确边界：当前视觉重点只放在**信息组织和操作流**，没有继续深挖到完整设计系统、图标体系或组件抽象。因为当前阶段真正要验证的还是联调和流程，而不是视觉规范本身。

## 8. 本轮落地结果

新增：

- `web/index.html`
- `web/styles.css`
- `web/app.js`

更新：

- `cmd/server/main.go`
- `internal/service/user_service.go`
- `internal/handler/user_handler.go`
- `internal/service/upload_service.go`
- `internal/handler/upload_handler.go`
- `internal/service/document_service.go`
- `internal/handler/document_handler.go`

以及对应测试：

- `internal/handler/user_handler_test.go`
- `internal/handler/upload_handler_test.go`
- `internal/handler/document_handler_test.go`
- `internal/service/user_service_test.go`
- `internal/service/upload_service_test.go`
- `internal/service/document_service_test.go`

当前这版前端已经覆盖了首轮联调最关键的能力：

- 登录 / 注册 / 自动刷新 token
- 支持类型拉取与简单上传
- 秒传检查与上传状态查询
- 文档列表、预览、下载、删除
- 混合检索
- WebSocket 流式聊天
- HTTP 会话历史读取
- 管理员用户/组织/会话只读视图

## 9. 为什么这版前端是合适的“第一版”

它并不是终局架构，也没有试图一次做完所有浏览器端能力，比如：

- 浏览器端 MD5 计算
- 完整分片上传 UI
- 更细的状态管理拆分
- React 组件化与工程化构建

但它已经完成了当前最重要的目标：

**把后端已经实现的关键能力，第一次用真实页面完整串起来。**

接下来如果要继续演进，更合理的顺序是：

1. 先根据真实联调体验修正接口细节和状态模型
2. 再决定是否把前端迁到独立工程（如 React/Vite）
3. 再补浏览器端 MD5、分片上传与更细的管理员写操作

也就是说，这一版前端不是“临时糊一个页面”，而是一个有意收敛复杂度的第一版集成面。

# 阶段十三.6 真机联调补记：`/app/` 入口、浏览器端分片上传与管理员写操作

前面阶段十三.5 已经把前端首版挂到了 Go 服务里，但那一版更多是在讲“为什么这样设计”。这一步补的是**真正跑起来以后暴露出的行为结论**，尤其是：

- `/app/` 入口在真实服务上的可用性
- Elasticsearch 晚于服务启动时的退化表现
- 浏览器端分片上传协议是否真的能闭环
- 管理员写操作是否不仅“接口存在”，而且真能删文档、改标签、查会话

## 1. `/app/` 入口联调结果

服务启动后，根路由会 `307` 跳转到 `/app/`，静态资源可直接由当前 Go 服务返回：

- `/app/`：前端入口 HTML 正常
- `/app/app.js`：主前端逻辑正常
- `/app/md5.js`：浏览器端 MD5 计算模块正常

也就是说，当前仓库已经不是“后端接口 + 独立文档说明”的状态，而是**可以直接从服务端打开一个真实工作台入口**。

## 2. 真实依赖行为：Elasticsearch 晚启动时，搜索/聊天/删文档不会自动恢复

这次联调里有一个非常具体、而且很值得记录的现象：

1. 先启动 Go 服务、但 Elasticsearch 还没起来时：
   - 上传、登录、静态页面都能正常工作
   - 搜索返回 `500`
   - `GET /api/v1/chat/websocket-token` 返回 `503`
   - 删除文档也会返回 `503`
2. 后来再把 Elasticsearch 容器拉起来，如果**不重启服务**，这些链路不会自动恢复
3. 重启服务后，搜索、聊天 token、文档删除恢复正常

这说明当前搜索/聊天/文档删除相关依赖的可用性判定，实质上仍然是**启动时注入一次**；如果 ES 在服务启动之后才可用，就需要重启服务重新建立依赖。

这个结论很重要，因为它不是“某次机器偶发异常”，而是当前启动模型的真实表现。后面如果要继续做健壮性改造，方向会很明确：

- 要么接受“依赖晚启动需要重启服务”的当前约束，并写进运行说明
- 要么把 ES 相关 client/service 变成可重试或延迟探测模式

## 3. 浏览器端分片上传的真实闭环

这次继续补了浏览器端分片上传 UI，并按前端约定真正跑了一轮分片协议：

- 浏览器端先计算整文件 MD5
- 调 `POST /api/v1/upload/check`
- 按固定 `5MB` 分片逐个调 `POST /api/v1/upload/chunk`
- 最后调 `POST /api/v1/upload/merge`

联调时用一份约 `16.52MB` 的测试文件实际验证，结果如下：

- 文件 MD5：`ad305a07590b966e317620e4f71e782e`
- 总分片数：`4`
- 合并成功
- `/api/v1/documents/uploads` 能看到新文件记录

这里还额外打出了一个很细但很关键的协议语义：

`GET /api/v1/upload/status?fileMd5=...` 在**合并完成之后**返回的是：

- `completed=true`
- `progress=100`
- `uploadedChunks=[]`

`uploadedChunks` 为空并不代表上传失败，而是因为合并后的清理逻辑已经把 Redis 中的分片上传标记删掉了。也就是说，前端对这个接口的判断应该是：

- 断点续传阶段：看 `uploadedChunks`
- 合并完成之后：优先看 `completed/progress`

不能把“合并后 `uploadedChunks` 为空”误判成错误状态。

## 4. 管理员写操作不是只读占位，而是已经完成一轮真实验证

这次没有停留在“管理员页面能展示用户和组织树”，而是直接做了一轮带回滚的管理员联调：

1. 创建临时管理员账号，并提升为 `ADMIN`
2. 创建临时组织标签
3. 把标签分配给目标用户（用户 `27`）
4. 查询管理员会话审计
5. 用管理员身份删除目标用户刚上传的文件
6. 恢复原始组织标签，删除临时标签，并把临时管理员降回普通用户

实际结果：

- 管理员用户列表、组织树接口都能正常访问
- 组织标签创建/分配/删除完整走通
- 管理员会话审计按 `userid` 过滤成功，这轮实际返回了 `2` 条记录
- 管理员删除他人文档成功
- 删除后目标用户的上传列表中已不再包含该文件

这说明当前管理员视图已经不只是“只读面板”，而是具备了：

- 标签写操作
- 会话审计过滤
- 跨用户文档删除

这几个真正会影响数据状态的能力。

## 5. 真机联调打出来的一个前端兼容问题：`fileMd5` vs `fileMD5`

这轮联调还暴露出一个非常具体的前端 bug：

- 后端文档列表接口返回字段是 `fileMd5`
- 前端文档列表渲染最初写成了 `doc.fileMD5`

结果就是：

- 列表中的 MD5 文本可能显示为空
- 预览 / 下载 / 删除按钮带出的 `data-md5` 可能为空
- 某些情况下只能退回按文件名操作，风险明显更高

这个问题已经在前端里做了兼容修正：新增一层 `getDocumentMd5()` 归一化处理，同时兼容 `fileMd5` 和 `fileMD5`。这类问题很典型，说明当前阶段继续做真实联调是有价值的，因为很多问题并不在接口“有没有”，而在**字段语义是否真的一字不差地对齐**。

## 6. 当前阶段的结论

到这里，阶段十三.5 当时列出来的“下一步”里，有两项已经实际推进并完成验证：

- 浏览器端 MD5 + 分片上传 UI 已补上
- 管理员写操作已不再停留在只读展示，而是完成了真实联调

不过这次也顺便验证出一个现实边界：当前环境里没有现成的图形浏览器/无头浏览器可直接驱动，所以这一轮“浏览器端联调”主要是通过：

- 服务端实际返回 `/app/` 静态资源
- 前端代码与页面结构核对
- 本地真实服务 + MySQL + Elasticsearch 的端到端脚本联调

来完成的。

这并不影响当前阶段结论的有效性，因为接口、状态流转和权限路径都已经在真实运行环境里跑通；但如果下一步要做更细的交互验证，比如：

- 进度条动画是否与实际上传进度完全同步
- 表单回填、按钮禁用、弹窗确认等交互体验
- 管理员界面在不同数据量下的可用性

那就适合在后续引入真正的浏览器自动化测试，或者在本机直接做一次 GUI 手工回归。
