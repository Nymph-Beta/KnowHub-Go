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
