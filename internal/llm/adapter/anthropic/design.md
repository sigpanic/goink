# Claude Messages 适配器设计

状态：待实现。以已完成的 OpenAI Chat 适配器和现有公共接口为基础，使用已安装的 `github.com/anthropics/anthropic-sdk-go v1.81.0`。

## 目标与范围

实现 `anthropic_messages` 协议，将 Goink 消息、工具和调用参数转换为 Anthropic Messages 请求，将 SDK 响应转换为统一的 `Response` / `StreamEvent`，并保留后续工具循环需要的原生内容。

首阶段支持 Anthropic 官方 API 和兼容 Messages 的中转端点。Bedrock、Vertex 的鉴权与平台接入另行实现。SDK 负责请求、协议解码和流式累积；工具执行、审批、业务重试仍由 Goink 上层负责，不使用 SDK 自动工具循环。

本阶段只改 `internal/llm/`。Agent、Session、数据库迁移、前端与 Wails 绑定不在范围内。后续产品目标仍是同一会话可切换协议族，允许丢弃不兼容的推理状态；完成本包不等于已完成会话存储和切换接线。

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
  response.go            # 完整响应、usage、原生状态转换
  generate.go
  stream.go
  discover.go
  test_connection.go
  *_test.go
```

配置读写继续使用根包现有格式和 [合并流程](../../DESIGN.md)：内置项是实例，用户保存可编辑字段的覆盖，自定义项保存明确的类型和地址。协议内名称唯一；不同协议可以同名。本包接收已合并的配置，不再次实现配置文件读写或旧数据兼容。

## 先补齐原生响应状态契约

当前公共 `Response` / `StreamEvent` 只有显示用思考、文本、工具和用量，没有完整回放状态。仅拼接 `ThinkingContent` 无法恢复签名，因此先补最小公共契约，再实现 Claude。

计划增加 SDK 无关的响应状态：来源协议、provider、model、完成信息及不透明的原生 JSON 内容。`Response` 携带该状态；流式增加最终状态事件（暂称 `EventResponseState`，在现有事件末尾追加，保持已有值）。具体类型名在第一个 review 单元确定。

Claude 状态保存完整、有序的 assistant content，包括 `thinking`、`signature`、`redacted_thinking`、空块、文本和 `tool_use`。不能只保存一段签名，也不能把显示用思考当作原生块重建。若模型需要上下文前缀校验，应同时记录足以判断兼容性的依据。

历史输入通过明确的内部键（沿用总设计的 `_goink_llm_state`）传递状态。所有出站实现必须移除内部键，OpenAI 路径也须覆盖，不能通过 SDK extra fields 发出。适配器不修改调用方消息，不用进程内“最后一次响应”缓存替代消息存储。

兼容续聊回放原生 content；跨协议、跨模型或前缀变化时，按目标模型规则判断，无法确认兼容则重建文本与工具轨迹并丢弃不兼容推理。来源相同本身不足以证明签名仍有效，不伪造签名或 thinking 占位。状态落库、上下文版本及切换生命周期由后续 Agent / Session 阶段接入。

[Thinking 工具循环](https://platform.claude.com/docs/en/build-with-claude/thinking-tool-workflows)要求回传对应原生块；部分模型还存在[保留推理与前缀约束](https://platform.claude.com/docs/en/build-with-claude/preserved-thinking)。不把所有签名 400 都当成清空状态后重试的信号。

## 出站转换

`adapter.Request` 是 Goink 调用输入，不是 HTTP 请求。当前 `Messages` / `Tools` 仍采用 OpenAI 形状，由本包构造强类型 SDK 参数。

| Goink 输入 | Messages 输出与要求 |
| --- | --- |
| 开头的 system 消息 | 顶层 `system`，保持原始顺序；不能把中途注入的 system 任意挪到旧前缀，中途 system 的投影规则须单独验证 |
| user / assistant 文本 | 对应角色的 content blocks；按公共消息表示转换图片，不支持的内容明确报错 |
| `tools[].function` | `name`、`description`、`input_schema` |
| assistant `tool_calls` | `tool_use`，保留 ID，将 arguments JSON 转为对象并校验 |
| role=tool | 下一条 user 消息中的 `tool_result`，以 `tool_use_id` 配对并保留内容和错误语义 |
| 兼容的原生状态 | 回放完整 assistant content，避免由扁平字段重建丢失块顺序与签名 |
| 外部协议历史 | 从文本、调用及结果重建，不携带外部推理签名；不生成只有空内容的 assistant 占位 |

并行工具先收齐对应结果，在紧随调用的 user 消息中组织整组 `tool_result`，工具结果位于普通文本之前。同名工具按调用 ID / 原始位置配对，不能按工具名配对。不能合并掉工具调用与结果的配对边界。详见[官方工具结果规则](https://platform.claude.com/docs/en/agents-and-tools/tool-use/handle-tool-calls)。

参数转换集中在本包：将公共 `ToolChoice` 转为原生形式，明确拒绝不支持的组合。thinking 模式、effort、预算、温度及强制工具选择按模型能力处理；不能假定所有 Claude 都支持手动 budget、关闭思考或非默认 temperature。优先使用明确的模型能力和已验证预设；未知中转模型允许手工配置，但不猜测高级能力。参考[Thinking 文档](https://platform.claude.com/docs/en/build-with-claude/thinking)。

## Generate 与 Stream

`Generate` 使用 `Messages.New`，`Stream` 使用 `Messages.NewStreaming`。两者共享请求构建、完整响应转换和 usage 归一化，返回文本、显示用思考、工具调用、停止原因及原生状态。`GenerateText` 等根包辅助入口继续从统一响应提取所需部分。

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
- `chat_url` 在本协议表示 SDK API 根地址；可规范化明确可识别的完整 Messages 端点。保留中转路径前缀，避免重复拼接 `v1/messages`，不能直接截成 host。
- 对话、发现和连接测试共用规范化后的 SDK 配置。SDK 不能表达的任意自定义路由明确提示不支持，不另藏一套手写 HTTP 传输。

## 模型发现与连接测试

发现使用 SDK Models API 并处理分页，映射服务端实际返回的模型名称、上下文、输出限制与能力；缺失信息使用已验证预设或保留未知，不凭展示名推断。中转不支持发现时返回明确错误，仍允许手工添加模型，不回退其他协议。参考[Models API](https://platform.claude.com/docs/en/api/models/list)。

连接测试通过同一个实例 SDK 发送模型允许的最小合法请求，不复用 OpenAI 路径猜测或固定 `max_tokens=1`。区分路由 / 鉴权可能正确与实际生成成功，例如限流只给出相应告警，不伪装成成功生成。

## 实现与 review 路线

每个单元完成后验证并等待 review；这里的 commit 拆分是建议，不代表自动暂存或提交授权。

| 单元 | 文件范围 | 验收重点 |
| --- | --- | --- |
| 1. 原生状态契约 | `adapter/types.go`、必要的 OpenAI 出站过滤及相关测试 | 非 SDK 类型；内部状态不泄漏；已有事件值与普通请求行为保持一致 |
| 2. 实例与 SDK 构建 | 本包 `adapter.go`、`provider.go`、`providers.go` 及测试 | 真持有 providers；共享 HTTP、独立凭据、禁用 SDK 重试；快照隔离 |
| 3. 出站转换 | 本包 `request.go` 及测试 | 文本、图片、工具往返、选项校验；原生回放与外部历史投影；输入不变 |
| 4. 非流式生成 | 本包 `generate.go`、`response.go` 及测试 | 统一完整响应、usage、停止原因和原生状态 |
| 5. 流式生成 | 本包 `stream.go` 及回放测试 | 事件顺序、原始参数校验、取消、错误、完整收尾与签名保真 |
| 6. 发现、探测和注册 | 本包发现 / 探测文件、根包 `factory.go` 及相关测试 | 注册构建、类型校验和内置实例；分页、热重载、配置保存与运行时闭环 |

根包最后注册该协议，避免半成品适配器进入运行时。已有配置格式无需再设计；旧数据迁移和上层签名改造另行安排。

## 验证与交付边界

通过 `httptest.Server` 驱动真实 SDK，使用本地假凭据。复用 [Anthropic SSE 资料](../../testdata/sse/anthropic/)，但仅成功解析录制流不足以证明协议正确，必须检查后续出站请求。

重点覆盖：

- 首轮响应 → 状态序列化 / 恢复 → 工具结果 → 第二轮请求，检查原生块、签名、顺序和 ID。
- 签名分片、redacted / 空 thinking、多个同名并行工具、无效及截断参数；任一工具失败时整组不可执行。
- HTTP 错误、HTTP 200 流内错误、缺失结束事件、取消与消费阻塞；累计 usage 不重复统计。
- Generate 与 Stream 的结果语义一致；Models 分页、地址前缀、不同实例鉴权隔离；未知协议或 provider 不回退。
- 兼容状态保留，外部 / 不兼容状态丢弃推理后仍保留文本与工具配对，输入 map 不被修改。

从仓库根目录执行 `go build ./internal/llm/...`、`go test -race ./internal/llm/...`，并运行对应范围的 lint。若调用方尚未接线导致整仓失败，报告边界，不越界修改 Agent、App 或前端以凑编译通过。

现有样本中的测试签名不能证明真实服务端接受签名。真实 API 烟测需可用凭据，单独记录是否执行。此阶段交付是 llm 适配器和本地协议往返闭环；重启续聊、压缩和真实同会话切换需后续持久化与调用方阶段验收。

SDK 调用、传输注入和默认重试行为以[官方 Go SDK 文档](https://platform.claude.com/docs/en/cli-sdks-libraries/sdks/go)为准；消息字段以[Messages API](https://platform.claude.com/docs/en/api/messages/create)为准。本文细化当前实现阶段，既有全局设计中的旧进度描述不作为代码现状依据。
