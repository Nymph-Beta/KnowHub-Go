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
