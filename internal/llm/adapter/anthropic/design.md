# Claude Messages 适配器设计

状态：实例与 SDK 构建、出站转换已提交；2026-10-22 非流式生成单元待 review，流式和连接测试尚未实现。使用仓库已安装的 `github.com/anthropics/anthropic-sdk-go v1.81.0`，不在本阶段升级依赖。

## 目标与范围

实现 `anthropic_messages` 协议，将 Goink 消息、工具和调用参数转换为 Anthropic Messages 请求，将 SDK 响应转换为统一的 `Response` / `StreamEvent`，并保留后续工具循环需要的原生内容。

首阶段支持 Anthropic 官方 API 和兼容 Messages 的中转端点。Bedrock、Vertex 的鉴权与平台接入另行实现。SDK 负责请求、协议解码和流式累积；工具执行、审批、业务重试仍由 Goink 上层负责，不使用 SDK 自动工具循环。

首阶段输入限定为文本和客户端函数工具。图片输入、服务端内置工具及其续接流程另做，不为本包提前扩展公共消息类型。

本阶段只改 `internal/llm/`。Agent、Session、数据库迁移、前端与 Wails 绑定不在范围内。后续产品目标仍是同一会话可切换协议族，允许丢弃不兼容的推理状态；完成本包不等于已完成会话存储和切换接线。

已完成的前置工作见[结构化消息实施路线](../../../../docs/feat/v1.8.0/structured-messages-implementation.md)：`Request.Messages` 为 `[]adapter.Message`，`Request.Tools` 为 `[]adapter.ToolDefinition`；`Response.State`、`EventResponseState` 及 session 双向映射均已存在。OpenAI 已自行构造出站消息和工具，但 App / Agent 尚未迁移，原生响应状态的生产、实际落库及切换回放尚未接通。本包直接复用这些契约，不重做基建。

## 所有权与包边界

调用路径：`llm.Client → adapters[协议] → anthropic.Adapter → providers[实例名] → SDK client`。

- `Adapter` 真正持有本协议的 `map[string]Provider`，实现公共 [ProtocolAdapter](../adapter.go)。未知实例直接报错，不跨协议查找或回退。
- 包内 `Provider` 组合公共 [adapter.Provider](../provider.go)，增加私有 SDK client 和必要的协议能力信息。实例名是本协议内的索引，`ProviderType` 只选择行为，不作为实例索引。
- 初期行为类型为 `anthropic`，官方实例和兼容中转实例可使用同一类型。只有确认存在行为差异后才增加类型，不按中转名称或模型展示名称猜测协议。
- 根包构建并注入共享 HTTP client；子包不再自行构造另一套 transport。每个实例使用自己的凭据和 SDK 配置，通过 `option.WithHTTPClient` 复用传输层。
- SDK 使用 `option.WithMaxRetries(0)`，避免和上层重试叠加。SDK 类型不进入公共接口；公共类型直接复用，不新增重复定义和无意义转换。
- `Providers()` 返回公共配置的独立快照，复用已有 `Clone`。不引入公共 Base，不复制 OpenAI hooks 来满足目录对称。

计划文件如下，具体辅助函数按实现需要归并：

```text
anthropic/
  design.md
  adapter.go             # 接口实现、实例查找、构建
  provider.go            # 组合配置与 SDK client
  providers.go           # 内置实例、行为类型及已知模型信息
  request.go             # 消息、工具、选项的出站转换
  request_messages.go    # 历史角色投影和工具配对
  request_state.go       # 私有 payload 与原生状态回放校验
  response.go            # 完整响应、usage、原生状态转换
  errors.go              # SDK HTTP 错误及 Retry-After 转换
  generate.go
  stream.go
  discover.go
  test_connection.go
  *_test.go
```

配置读写继续使用根包现有格式和 [合并流程](../../DESIGN.md)：内置项是实例，用户保存可编辑字段的覆盖，自定义项保存明确的类型和地址。协议内名称唯一；不同协议可以同名。本包接收已合并的配置，不再次实现配置文件读写或旧数据兼容。

## 复用公共消息与原生状态

公共 [Message / ResponseState](../message.go) 与 [Request / Response / StreamEvent](../types.go) 不包含 SDK 类型。Claude 出站读取业务字段和可选 `Message.State`；入站分别生成公共文本、工具调用及原生状态，session 不从原生块提取工具或解释签名。

`Response.State` 与最终 `EventResponseState.State` 承载同一语义的 `ResponseState`：来源包含协议、provider 实例、providerType 和 model，`Payload` 是不透明原生 JSON。现有事件值保持不变，不再增加另一套状态事件。

现有状态字段需区分用途：`Complete` 仅表示单次协议响应完整，不证明签名或前缀兼容；`StopReason` 保留原始协议原因，不作为跨协议统一枚举。`TurnComplete` 当前没有公共消费者，是否移除需单独 review；本包不据此设计业务 turn 状态机或服务端续接。本文同步现状，不视为已修改或定稿这些公共字段。

Claude 状态保存完整、有序的 assistant content，包括 `thinking`、`signature`、`redacted_thinking`、空块、文本和 `tool_use`。不能只保存一段签名，也不能把显示用思考当作原生块重建。若模型需要上下文前缀校验，应同时记录足以判断兼容性的依据。

历史原生状态直接通过 `Message.State` 传递，不使用 map 保留键。SDK 请求由本包显式构造，公共状态结构不能整体发送或通过 extra fields 泄漏。适配器不修改调用方消息，不用进程内“最后一次响应”缓存替代消息存储。

兼容续聊回放原生 content；跨协议、跨模型或前缀变化时，按目标模型规则判断，无法确认兼容则重建文本与工具轨迹并丢弃不兼容推理。来源相同本身不足以证明签名仍有效，不伪造签名或 thinking 占位。session 已提供状态映射，实际调用映射和落库、上下文版本及切换生命周期仍由后续上层接线完成；llm 不依赖 session。

[Thinking 工具循环](https://platform.claude.com/docs/en/build-with-claude/thinking-tool-workflows)要求回传对应原生块；部分模型还存在[保留推理与前缀约束](https://platform.claude.com/docs/en/build-with-claude/preserved-thinking)。不把所有签名 400 都当成清空状态后重试的信号。

## 出站转换

`adapter.Request` 是 Goink 调用输入，不是 HTTP 请求。本包直接将 `[]adapter.Message` 和 `[]adapter.ToolDefinition` 转为强类型 SDK 参数，不经过 OpenAI map。

DB metadata 提取和真实用户的稳定时间前缀由 session 映射负责，本包使用传入的 Content。整组历史的角色组织、工具配对处理和 system 投影由本包负责，不调用 OpenAI 的 `buildMessages` / `buildTools`，也不套用强制公共去重流水线。

| Goink 输入 | Messages 输出与要求 |
| --- | --- |
| 开头的 system 消息 | 顶层 `system`，保持原始顺序；不能把中途注入的 system 任意挪到旧前缀，中途 system 的投影规则须单独验证 |
| user / assistant 的 Content | 对应角色的文本 content blocks；ThinkingContent 仅为显示用思考，不伪造成原生 thinking |
| ToolDefinition.Name / Description / InputSchema | `name`、`description`、`input_schema`；保留 schema，按本协议要求校验并构造 SDK 类型 |
| assistant 的 ToolCalls | `tool_use`，保留 ID / Name，将 Arguments 原始 JSON 转为 SDK 所需对象并校验，避免大整数精度丢失 |
| tool 的 Content / ToolCallID / ToolName | 下一条 user 消息中的 `tool_result`，以 ID 配对并保留结果正文；错误标志的映射依据须显式定义 |
| 兼容的 Message.State | 回放完整 assistant content，避免由扁平字段重建丢失块顺序与签名 |
| 外部协议历史 | 从文本、调用及结果重建，不携带外部推理签名；不生成只有空内容的 assistant 占位 |

并行工具先收齐对应结果，在紧随调用的 user 消息中组织整组 `tool_result`，工具结果位于普通文本之前。同名工具按调用 ID / 原始位置配对，不能按工具名配对。不能合并掉工具调用与结果的配对边界。详见[官方工具结果规则](https://platform.claude.com/docs/en/agents-and-tools/tool-use/handle-tool-calls)。

参数转换集中在本包。当前 `CallOptions.ToolChoice` 仍是 OpenAI 形状的 `any`，尚未完成协议无关建模；实现出站转换时明确支持的现有语义及到原生形式的映射，未知形式和不支持的组合明确报错，必要的公共契约调整单独 review。thinking 模式、effort、预算、温度及强制工具选择按模型能力处理；不能假定所有 Claude 都支持手动 budget、关闭思考或非默认 temperature。优先使用明确的模型能力和已验证预设；未知中转模型允许手工配置，但不猜测高级能力。参考[Thinking 文档](https://platform.claude.com/docs/en/build-with-claude/thinking)。

本出站单元的具体约定：

- `Provider.buildRequest` 返回 SDK 参数，不发送请求。开头 system 依次投影为顶层块，中途 system 在原位置变成 user 的 `<system-reminder>` 文本；后续 user 文本合入前一个 user 消息。首条有效消息必须是 user，不支持末尾 assistant prefill。
- 只在 assistant 紧邻的连续 tool 记录中按 ID 找结果，同组重复项保留首项；过滤孤立结果和缺少结果的调用，不跨消息边界找结果、不伪造工具结果。冲突声明、跨组重用 ID 和名称不匹配报错。结果按调用顺序放在 user 内容开头；无公共错误标志时不从正文猜测 `is_error`。
- ToolChoice 支持 `auto`、`none`、`required` 及现有具名 function 对象，分别映射为原生 auto、none、any、tool；拒绝未知形式和不存在的工具。工具 schema 和参数要求 object，原始 JSON 独立复制，保留额外 schema 关键字及大整数。
- 已验证高级选项仅覆盖四个内置精确模型 ID：默认使用 adaptive + summarized 和配置的首个 effort。Opus 5.5、Sonnet 5.5、Fable 5.1 不接受关闭思考或强制工具；Haiku 5.5 仅在 high 及以下允许关闭思考。Sonnet 的 between_tools 不等于完全关闭，故不把 ThinkingEnabled=false 偷换成该模式。现代预设的默认温度 1 省略，非默认值报错；未知模型允许基础请求和显式温度，不猜测 thinking/effort 能力。SDK API 版本保持既有默认，不在本单元切换 API 版本。
- 私有 payload 使用 `{"content":[...],"request_hash":"..."}`。摘要绑定生成该响应的完整 SDK 请求、端点和凭据，密钥只参与摘要、不写入 payload。后续 Generate / Stream 入站转换负责生产此结构；本单元测试通过消息 JSON 往返构造该状态。
- 原生回放要求来源一致、Complete、摘要及公共正文/调用一致。来源、前缀、工具、选项、端点或凭据变化则重建公共内容并丢弃原生推理；同源且前缀匹配但原生块损坏或不在本阶段支持范围时明确报错。保留块顺序、空 thinking、signature、redacted data 及已支持块内的额外字段；本地摘要不代替真实服务端的签名验证。

## Generate 与 Stream

`Generate` 使用 `Messages.New`，`Stream` 使用 `Messages.NewStreaming`。两者共享请求构建、完整响应转换和 usage 归一化，返回文本、显示用思考、工具调用、停止原因及原生状态。`GenerateText` 等根包辅助入口继续从统一响应提取所需部分。

已实现的非流式单元：

- `Generate` 按实例查找配置，复用 `buildRequest` 和 `messagesURLOption`。显式设置 10 分钟请求超时，避免 SDK 因大输出上限在发送前要求流式；不降低 max_tokens，调用方更短的 context deadline 仍优先。长请求的上游超时仍可能失败，本单元不自动切换传输模式。
- 完整响应按块顺序拼接显示文本和思考，函数调用只返回声明。整组 ID、名称、对象参数与 stop_reason 校验全部通过后才交付；任一工具无效时不返回部分工具或可回放状态。拒绝服务器工具、provider-executed caller、未知块和 pause_turn；不提前实现服务端续接。文本因 max_tokens 停止仍返回真实停止原因和完整协议状态。
- 原生 payload 直接保存 SDK 的 `message.JSON.Content.Raw()` 和出站请求摘要，保留空 thinking、签名、redacted data、扩展字段及大整数。来源的 Model 使用请求所选标识，允许服务端返回实际快照名；本单元测试覆盖响应 → 公共消息序列化/恢复 → 工具结果 → 下一次真实 SDK 请求。
- SDK 会宽松转换标量；用于原生回放的字符串字段和 usage 整数核对原始 JSON，防止公共内容与回放内容不一致或统计被截断。usage 的 prompt_tokens 为 input + cache_creation + cache_read，cache miss 为 input + cache_creation；总量只再加 output，thinking 作为 completion 明细，不重复累加，所有数字输出为 float64。缓存 TTL 细分不再加一次。参考[缓存计数口径](https://platform.claude.com/docs/en/build-with-claude/prompt-caching#tracking-cache-performance)。
- SDK HTTP 错误转成 APIError，保留状态码、原生错误类型、消息、可重试标记和 Retry-After；context 与传输错误保留错误链。HTTP 400 签名错误不自动清状态或重试；合法 HTTP 响应不等于合法 Messages 响应。
- State.Complete 表示合法的完整协议响应；TurnComplete 仅对 end_turn、stop_sequence、refusal 为 true，本包不据此驱动业务循环。流式事件、根包注册及真实服务端签名验收仍在后续范围。

流式使用已安装 SDK 的 `Message.Accumulate` 累积原生消息，并映射事件：

| Anthropic 事件 | Goink 行为 |
| --- | --- |
| text delta | `EventContent` |
| thinking delta | `EventThinking` |
| tool_use block start | `EventToolCallStart`，记录 ID、名称和原始位置 |
| input_json delta | `EventToolCallArguments`，沿用当前累计参数字符串语义 |
| signature delta | 累积到原生状态，不作为可显示思考输出 |
| message_delta usage | 更新累计值，不重复相加 |
| message_stop | 结合 SDK 错误及所有工具参数校验完成本轮收尾 |
| HTTP 错误或流内 error | 转成统一 `APIError` / `EventError` |

先验证整组工具，再发布可执行的 `EventToolCallEnd`。成功收尾顺序为最终 usage、完整响应状态、按原始顺序的全部工具结束事件。任何一个工具参数无效，整组都不发可执行结束事件，保留 `tool_args_invalid` 错误类型。

**独立保存原始工具参数增量。** 已安装 SDK 的 [`messageutil.go`](https://github.com/anthropics/anthropic-sdk-go/blob/v1.81.0/messageutil.go) 在刷新 content raw JSON 时可能将无效的 tool input 改为 `{}`；不能仅校验 SDK 最终对象，否则截断参数可能被误认为合法空对象。须测试 SDK 累积与原始 JSON 的往返保真。

缺少 `message_stop` 的 EOF、取消、流内错误或截断的工具参数不能产生完整回放状态和可执行工具组。文本因 `max_tokens` 停止时保留真实停止原因，不一概等同网络失败。所有 channel 发送均响应 context，退出时关闭 SDK 流，并检查最终 `stream.Err()`。未知事件按协议扩展规则处理；SDK 无法保真的新内容块不能静默用于回放。

HTTP 200 仍可能包含 SSE error；签名增量、累计 usage 和消息结束顺序按[官方 Streaming 文档](https://platform.claude.com/docs/en/build-with-claude/streaming)验证。

## 用量、错误与地址

- Anthropic usage 转成现有 Goink 输入、输出、总量及缓存明细口径；准确处理未缓存输入、缓存读取和缓存创建，避免重复计数。保持当前消费方需要的数字表示（现有 map 统计路径使用 `float64`），不能直接透传 SDK `int64` 后假定统计可用。
- SDK 错误保留 HTTP 状态、可重试标记、`Retry-After` 及足够的错误类型。参数、历史和签名错误不按通用网络错误重试；本包不执行业务退避。
- `chat_url` 表示完整 Messages 端点，官方默认值为 `https://api.anthropic.com/v1/messages`。正式生成不补后缀、不猜版本、不做网络探测；保留任意代理路径、转义路径和查询参数，禁止用户信息及 fragment。构建时只校验 URL，不要求路径以 `/v1/messages` 结尾，也允许根地址作为后续探测候选。
- `messagesURLOption(rawURL)` 返回单次请求的 SDK middleware，克隆请求后覆盖完整 URL 和 Host。仅传给 `Messages.New` / `NewStreaming`，不挂到整个 SDK client 或共享 HTTP transport。正式请求传保存的 ChatURL，连接测试传当前候选；SDK 继续负责请求编码、鉴权、响应解码及流累计。
- SDK client 的默认 BaseURL 仅取实例 origin，避免采用官方默认站点；这不是模型发现的路由策略。Models、count_tokens 等其他 API 的端点分别处理，不从任意 Messages 路径推断，也不能复用 Messages 的 URL 覆盖选项。

## 模型发现与连接测试

发现使用 SDK Models API 并处理分页，映射服务端实际返回的模型名称、上下文、输出限制与能力；缺失信息使用已验证预设或保留未知，不凭展示名推断。中转不支持发现时返回明确错误，仍允许手工添加模型，不回退其他协议。参考[Models API](https://platform.claude.com/docs/en/api/models/list)。

连接测试保留 OpenAI 的使用语义：先请求用户原地址，失败后按 Messages 路由生成去重候选，最后尝试 host 根路径兜底；每个候选使用同一实例 SDK 和 `messagesURLOption`，设置单候选超时、响应取消、关闭自动重试。验证 Anthropic SSE 而非 OpenAI `choices`；成功返回完整 URL，由上层保存，全部失败汇总各候选错误，不在 SDK client 或配置快照内修改地址。最小请求按模型能力选择，不照搬固定 `max_tokens=1`；限流只给相应告警，不伪装成成功生成。本单元只准备 URL 覆盖能力，候选生成和探测循环后续实现。

## 实现与 review 路线

从实例与 SDK 构建开始，公共结构化消息、工具及状态契约不再列为待重做单元。状态字段和 ToolChoice 的必要调整在其首次使用前单独确认，不把整个上层接线纳入本包。

每个单元完成后验证并等待 review；这里的 commit 拆分是建议，不代表自动暂存或提交授权。允许阶段性编译失败，不通过空壳方法或越界改调用方来凑齐接口。

| 单元 | 文件范围 | 验收重点 |
| --- | --- | --- |
| 1. 实例与 SDK 构建 | 本包 `adapter.go`、`provider.go`、`providers.go` 及测试 | 真持有 providers；共享 HTTP、独立凭据、禁用 SDK 重试；完整端点覆盖、单次候选隔离和快照隔离；不实现 TestConnection、不注册根包 |
| 2. 出站转换 | 本包 `request*.go` 及测试 | 结构化文本、工具定义和历史调用 / 结果转换；system 和配对规则；选项校验；原生回放与外部历史投影；输入不变 |
| 3. 非流式生成 | 本包 `generate.go`、`response.go` 及测试 | 统一完整响应、usage、停止原因和原生状态；模型输出不执行工具 |
| 4. 流式生成 | 本包 `stream.go` 及回放测试 | 事件顺序、原始参数校验、取消、错误、完整收尾与签名保真 |
| 5. 发现、探测和注册 | 本包发现 / 探测文件、根包 `factory.go` 及相关测试 | 注册构建、类型校验和内置实例；分页、热重载、配置保存与运行时闭环 |

根包最后注册该协议，避免半成品适配器进入运行时。已有配置格式无需再设计；旧数据迁移和上层签名改造另行安排。

## 验证与交付边界

通过 `httptest.Server` 驱动真实 SDK，使用本地假凭据。复用 [Anthropic SSE 资料](../../testdata/sse/anthropic/)，但仅成功解析录制流不足以证明协议正确，必须检查后续出站请求。

重点覆盖：

- 首轮响应 → 状态序列化 / 恢复 → 工具结果 → 第二轮请求，检查原生块、签名、顺序和 ID。
- 签名分片、redacted / 空 thinking、多个同名并行工具、无效及截断参数；任一工具失败时整组不可执行。
- HTTP 错误、HTTP 200 流内错误、缺失结束事件、取消与消费阻塞；累计 usage 不重复统计。
- Generate 与 Stream 的结果语义一致；Models 分页、地址前缀、不同实例鉴权隔离；未知协议或 provider 不回退。
- 兼容状态保留，外部 / 不兼容状态丢弃推理后仍保留文本与工具配对，输入 Message、ToolDefinition 和其中的 RawMessage 不被修改。

从仓库根目录执行 `go build ./internal/llm/...`、`go test -race ./internal/llm/...`，并运行对应范围的 lint。若调用方尚未接线导致整仓失败，报告边界，不越界修改 Agent、App 或前端以凑编译通过。

现有样本中的测试签名不能证明真实服务端接受签名。真实 API 烟测需可用凭据，单独记录是否执行。此阶段交付是 llm 适配器和本地协议往返闭环；重启续聊、压缩和真实同会话切换需后续持久化与调用方阶段验收。

SDK 调用、传输注入和默认重试行为以[官方 Go SDK 文档](https://platform.claude.com/docs/en/cli-sdks-libraries/sdks/go)为准；消息字段以[Messages API](https://platform.claude.com/docs/en/api/messages/create)为准。本文细化当前实现阶段，既有全局设计中的旧进度描述不作为代码现状依据。
