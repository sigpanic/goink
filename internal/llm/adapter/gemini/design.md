# Google Interactions 适配器实施路线

> 2026-10-23。前三单元已提交，第四单元已实现，待 review；发现 / 探测和注册尚未实现。本文替代旧总设计中的 Gemini GenerateContent 实施路线。本阶段继续自底向上，仅改 `internal/llm/`，每个单元完成后等待 review，不自动暂存或提交。

## 目标与边界

使用已安装的 `google.golang.org/genai v1.75.0` 中的官方 Interactions SDK，接入文本、显示用思考和客户端函数工具，产出既有 `Response` / `StreamEvent` 和可回放的原生状态。

- 包为 `adapter/gemini`，协议标识新增 `gemini_interactions`；不复用或改写 `gemini_generate_content` 的含义，也不在请求失败后切换协议。
- `Adapter` 持有 `map[string]Provider`，包内 `Provider` 嵌入公共配置并持有独立 SDK client；根包继续按协议选择 adapter，包内按 providerName 查实例，provider_type 初期为 `google`。
- 每次发送完整历史，显式 `store=false`，不发送 previous_interaction_id，不使用后台任务。会话、工具执行和业务重试仍由 Goink 管理。
- 复用公共 Message、ToolDefinition、ResponseState 及配置格式；不新增第二套统一消息，不改 Session、Agent、App、前端、绑定或 migrate。允许上层暂时无法编译。
- Vertex、Live、GenerateContent、服务端工具和托管 Agent 不在本轮范围。中转必须实际支持 Interactions，不凭模型名判断。

依据：[官方总览](https://ai.google.dev/gemini-api/docs/interactions-overview)、[迁移说明](https://ai.google.dev/gemini-api/docs/migrate-to-interactions)、[v1 API](https://ai.google.dev/api/interactions-api-v1)、[v1beta API](https://ai.google.dev/api/interactions-api)。服务 GA、URL 版本和 SDK schema/revision 分别核对，不能只改 URL 就认为已适配另一个版本。

## 请求、事件和原生状态

```text
上层准备 []adapter.Message + []ToolDefinition
  → gemini 构造 system_instruction + input steps + tools + generation_config
  → SDK Interactions.Create（同一端点，stream 区分传输模式）
  → 本包转换完整响应 / 累积 step 事件
  → Goink Response / StreamEvent + ResponseState
  → 后续上层通过已有 session 映射保存
```

| 公共信息 | Interactions 表达 |
| --- | --- |
| 开头 system | system_instruction；中途注入保持顺序，单独定义投影，不随意挪到历史开头 |
| user 正文 | user_input step，文本 content |
| assistant 正文 | model_output step，文本 content；不只读取 SDK OutputText，以免漏掉交错正文 |
| assistant.ToolCalls | function_call step，保留 id、name、对象 arguments |
| tool 结果 | function_result step，call_id 关联调用；普通结果作为 text content |
| 可兼容 State.Payload | 回放有序的原生 steps，包括 thought 的 summary / signature；显示用 ThinkingContent 不伪造成签名状态 |

本包处理工具配对及历史投影，不调用 OpenAI 或 Claude 的转换器。同名并行工具按调用 ID 配对；孤立、缺失、重复及冲突分别处理，不跨用户消息寻找结果，不伪造工具完成。

原生 payload 保存本次模型产出的有序 steps、来源及必要兼容依据，不保存凭据或重复的整个会话；工具业务字段继续单独归一化。兼容回放必须核对来源、公共内容及目标请求上下文，不能仅凭相同 provider 判定。跨协议以正文与工具轨迹重建，丢弃不兼容推理；签名不可伪造，也不照搬 GenerateContent 的 parts/thoughtSignature 或跳过校验标记。

SDK 函数参数使用 map，已知 step 类型的再次编码也可能丢扩展字段：第二单元必须先验证大整数、原始 JSON 和签名往返，再决定包内最小的原文保留方法，不能默认 SDK 对象等于原文。

流式按 step 索引累计 text、thought summary / signature 和 function arguments，转换实时文本、思考和累计参数事件。SDK 解码 SSE，本包负责协议累积和完整性校验，不假定它提供 Claude 的 Message.Accumulate。终态 completed 与合法 requires_action 分别表示回复结束和等待本地工具；后者仍可是一份完整协议响应。failed、cancelled、incomplete 和提前 EOF 必须分别处理，不能因为某个 step.stop 就交付可执行工具。

最终 usage、ResponseState 和整组 ToolCallEnd 的交付规则沿用公共契约：完整终态、流无错误、所有工具参数校验通过后才交付成功收尾。累计 usage 覆盖而非相加，数字保持 float64；input、output、thought、cached 和 total 按 Interactions 口径逐项核对，不沿用 GenerateContent 或 Claude 的计算公式。

## SDK 与完整端点

- 优先直接构建公开子包 `google.golang.org/genai/interactions` 的 client。它支持注入 HTTP client、显式凭据和 WithRetryConfig；避免顶层 genai.NewClient 的环境回退，以及其构建 Interactions client 时未转发 RetryOptions 的问题。不修改 SDK、vendor 或 go.mod / go.sum。
- 显式关闭 SDK 重试；共享根包注入的 HTTP client，不创建第二个 transport，不修改共享 client 的 Transport 或 Timeout，不读取环境凭据补全用户配置。
- ChatURL 保存完整 Interactions 端点，正式调用不补路径、不探测、不删查询参数。官方有 `/v1/interactions` 和 `/v1beta/interactions`；第一单元对照 SDK 固定 revision `2026-05-20` 验证请求与响应 schema，再确定内置默认地址，不能将所有路径版本视为可互换。
- SDK Create 固定拼接 `/{api_version}/interactions`；虽然公开 operations 包有 WithURLOverride，当前 Create 不接受此选项。第一单元验证请求专用的轻量 Do 包装：克隆 SDK 请求、覆盖完整 URL / Host，委托注入的共享 HTTP client。包装只服务 Interactions.Create；发现走独立路由，探测候选不修改正式实例快照。
- 模型发现是独立 Models API，不能将 Interactions 的托管 Agent 列表当模型列表；到最后单元验证可用的目录路由及分页，不从任意代理路径猜地址。没有可靠发现能力时明确报错，允许手工配置。

## Commit / review 单元

| 单元 | 建议 subject | 范围与验收 |
| --- | --- | --- |
| 1. 实例与 SDK 基建 | `feat(llm): bootstrap the Gemini Interactions adapter` | 新协议常量；gemini/adapter.go、provider.go、providers.go 及测试。真持有 providers、独立凭据、共享 HTTP、无环境回退与 SDK 重试；正式和候选端点的路径 / 转义 / 查询保真。核对 SDK revision 和官方 schema，准备内置实例；不实现生成和探测，不注册根包，不写空壳接口方法。 |
| 2. 出站与回放 | `feat(llm): build stateless Gemini interaction requests` | request*.go 及测试。完整 steps 历史、system、工具 schema / 配对、选项映射、store=false 和原生回放。只按验证的模型能力处理 thinking / effort / 温度；先验证 SDK JSON 保真，必要处理内聚本包，不扩展公共类型来绕过问题。 |
| 3. 非流式生成 | `feat(llm): normalize Gemini interaction responses` | generate.go、response.go、errors.go 及测试。正文、思考、整组工具、终态、usage、原生 payload；错误保留 HTTP 状态和 Retry-After。完成响应 → 公共消息 JSON 往返 → 工具结果 → 下一次真实 SDK 请求的本地闭环。 |
| 4. 流式生成 | `feat(llm): stream Gemini interaction steps` | stream*.go、协议样本和测试。真实 SDK SSE、step 累积、参数分片及独立签名、终态及错误、取消释放连接；任一工具损坏不交付部分可执行组。旧 Google testdata 是 GenerateContent，不能当作 Interactions 正确性证据。 |
| 5. 发现、探测与注册 | `feat(llm): register Gemini interaction discovery and probes` | 子包 discover.go / test_connection.go、根包 factory.go 及相关测试。Models 目录映射 / 分页，原地址优先探测并返回完整端点；配置保存、无密钥过滤、热重载、同名跨协议隔离；接口完整后才注册。 |

## 第一单元的完成条件

1. 只动公共协议标识和 gemini 子包；不升级依赖、不注册半成品、不碰调用方或数据库。
2. 构造不发网络请求；Providers 返回独立快照。未知名称和类型直接报错，不回退其他协议或官方实例。
3. 用 httptest 驱动 SDK Create，验证完整 `/v1/interactions`、`/v8/interactions`、自定义代理路径及查询参数透传，覆盖 JSON / SSE；路径透传通过不等于真实上游支持该版本。
4. 多实例凭据隔离，环境中配置其他 Google 凭据 / 地址不影响请求；429 / 5xx 及连接错误不自动重试，context 取消可释放请求。
5. SDK schema / revision、原文保真或端点覆盖存在问题时在本单元暴露，不带着未验证假设继续堆积转换代码。官方默认端点与版本选择记录在实现结果中；真实服务验收无凭据时明确未执行。

从仓库根目录运行 `go build ./internal/llm/...`、`go test -race ./internal/llm/...` 和对应范围 lint。每个单元先交付工作区供 review；用户明确授权时才暂存 / 提交，已授权的 no-verify 不代表可自动提交。

## 第一单元实现结果

- 新增 ProtocolGeminiInteractions，保留旧 GenerateContent 标识；Adapter 真持有 providers，使用公共 Clone 返回快照。内置 Google 实例尚未注册，预设覆盖 Argon、3.x Flash / Flash-Lite / Pro Preview 及 2.5 三款，共 10 个文本模型；按各模型支持范围分别设置推理等级，Preview 保留明确标识。
- 模型规格取自[各模型官方页面](https://ai.google.dev/gemini-api/docs/models)，推理等级取自[Interactions thinking](https://ai.google.dev/gemini-api/docs/thinking#controlling-thinking)及[3.1 Flash-Lite 等模型的等级表](https://ai.google.dev/gemini-api/docs/gemini-3#level-of-thinking)。Argon 的 ContextWindow 为输入输出合计，MaxOutputTokens 取前台调用的 65,536；当前不启用后台 long decode。2.5 系列按[官方说明](https://ai.google.dev/gemini-api/docs/deprecations)仍提供给存量用户，不混入已下线型号或自动重定向别名。
- 3.8 Flash 的默认等级同步为官方 medium；2.5 Pro / Flash 未公布固定默认等级，Goink 显式选择 medium。2.5 Flash-Lite 的 SupportsThinking 表示能力，官方默认仍为关闭；后续请求转换须区分默认行为与显式开启，不能仅凭该字段自动开启推理。
- 默认完整端点为 `https://generativelanguage.googleapis.com/v1/interactions`。SDK 使用明确的 v1 路由和其既有 Api-Revision: 2026-05-20；对照官方 v1 结构完成本地 Create 请求、JSON 响应和 SSE event_type / step 事件解码，未执行真实服务验收。任意 v8 / 代理地址测试只证明透传，不证明相应服务支持该 schema。
- Provider 组合公共配置，持有 SDK Create 服务及注入的共享 HTTP client；clientForURL 为后续候选构建独立 SDK 服务，正式 SDK client / ChatURL 不变。HTTP 委托仅接受 SDK Create 的 POST 路由，再克隆请求替换完整 URL / Host；不改 SDK、共享 transport、go.mod 或 go.sum。
- 直接使用 Interactions 子包，显式配置凭据和 retry.Strategy=none。测试覆盖空密钥不回退环境、多个已构建实例交替调用、候选隔离、路径 / 查询 / 转义、JSON / SSE、429 / 5xx / 连接错误不重试，以及请求和流读取时取消。
- 已复现 SDK 的两项限制：已知 function_call 解码后会丢未知扩展字段，大整数进入 float64 后再次编码会失真；流即使已有 interaction.completed，EOF 前缺少 `[DONE]` 仍报 io.ErrUnexpectedEOF。SDK 不修改：第二单元先解决包内原始 JSON 保留，第四单元明确真实协议终态与传输结束的判定，不能直接把 SDK 对象当可回放原文或一概忽略 EOF。

## 第二单元实现结果

- `Provider.buildRequest` 接收公共 Request，返回 SDK CreateModelInteraction 参数，不发请求。固定 `store=false`、`background=false`，不设置 previous_interaction_id。开头 system 用空行连接；中途 system 转为原位置的 user_input / system-reminder，不挪到开头，不套用 Claude 的消息交替规则。
- 工具仅与紧邻的 tool 记录配对，按调用顺序发回结果。同组重复保留首项，冲突声明 / 结果、全局复用调用 ID 和名称不匹配报错；孤立结果与无结果调用过滤，空结果仍发送。显示用 ThinkingContent 不参与原生回放。
- 工具 schema 使用复制的 RawMessage；function_call 与已验证的原生 steps 通过 SDK 公开的 NewStepUnknown 原文通道发送，保留大整数、签名和扩展字段。普通文本、工具定义、结果和请求参数仍使用 SDK 类型；不修改 SDK 或公共 Message。测试经过真实 Create 编码与 HTTP 委托验证，后续入站仍须捕获原始 steps，不能只序列化 SDK 解码对象。
- 原生 payload 为 `{steps, request_hash}`；摘要绑定 system、历史前缀、工具、端点及密钥，payload 不存凭据。完整状态同协议、同实例且公共正文 / 调用一致时才回放；其他情况用公共字段重建。匹配上下文中的损坏或不支持的原生 step 报错。thought summary 可缺省或为空，保留其原文，签名不能为空。
- 与 Claude 不同，[Google 要求切换模型仍重传原 thought](https://ai.google.dev/gemini-api/docs/thinking#thought-signatures)，由后端处理兼容性。因此摘要不绑定 model、推理等级或输出上限；同一服务实例切 Gemini 模型仍可回放，跨协议或端点 / 密钥变化则丢弃原生状态。摘要只是本地一致性检查，不替代服务端签名验证。
- 按内置精确 ID 校验推理等级和输出上限，用户配置可进一步收紧范围，不能扩大已验证能力；未知 ID 可发普通请求，但显式推理配置报错。2.5 Flash-Lite 保持默认关闭，显式开启才选 low；其他型号不把 minimal 或 thinking_summaries=none 当成关闭。Interactions 没有 thinking_budget / disabled 字段，不能照搬 GenerateContent 的 budget=0。
- 当前 [v1 GenerationConfig](https://ai.google.dev/api/interactions-api-v1) 不提供 temperature；虽然 SDK 仍保留已弃用字段，显式设置温度会报错，不静默丢弃。MaxTokens 映射 max_output_tokens：已知模型取已配置的更小上限，未知模型默认 4096。ToolChoice 映射 auto / none / required 与指定函数，拒绝未声明函数和未知字段。

## 第三单元实现结果

- 新增 Adapter.Generate：构造公共 Request 的 SDK 参数，显式 stream=false / Accept: application/json，使用已有实例的 SDK client 和完整端点。一次调用只发一次请求，不执行函数或启动自动工具循环。
- 已安装 SDK 的 ConsumeRawBody 会关闭网络 Body 并把原文放回 HTTPMeta.Response.Body；Generate 读取这份原文后关闭，保留完整 steps，不从 SDK 已解码的 map / step 重编码。收到意外 SSE 时关闭事件流并报错，不等待后续事件或挂起连接。
- 复用原生 step 校验，归一化全部交错文本、文本思考摘要和客户端函数；图片思考摘要仅随原生状态保存，不作正文。拒绝未支持的服务端工具、非文本 model_output、缺失签名及 step 错误。任一函数无效则不返回部分工具组；本包校验出的参数错误使用公共 tool_args_invalid 可重试标记，SDK 在返回响应前拒绝的 schema 错误则保留其错误链，不依赖错误字符串猜测具体字段。
- 只有 completed 和带有效客户端函数的 requires_action 成功；二者的 Complete=true，只有前者 TurnComplete=true。incomplete（包括达到输出上限）、failed、cancelled 分别报错，本阶段不把截断正文或工具作为成功结果交付；非终态、未知状态、诊断 errors 及状态与函数不符同样失败，不生成可回放状态。
- usage 按[官方 v1 定义和示例](https://ai.google.dev/api/interactions-api-v1)归一：completion_tokens=total_output_tokens+total_thought_tokens，缓存是输入子集，tool-use 保留为明细；服务端 total_tokens 原样保留，不重复累加这些明细。缺省统计不伪造，非法计数及缓存超过输入报错；公共数字继续使用 float64。
- HTTP 错误转换保留状态、原生 code / message、Retry-After 与可重试标记；也覆盖代理返回的 numeric code / status、HTML 和损坏错误 JSON。签名类 400 不清状态、不自动重试，context / 读取 / 传输错误保留错误链。
- 本地真实 SDK 闭环覆盖：Generate → 公共 Message JSON 往返 → 工具结果 → 切 Gemini 模型 → 第二次 Generate，核对原始 steps、签名、扩展及大整数；另覆盖整组工具失败、usage、异常 HTTP、取消与连接关闭。仍未注册根包或更改上层，真实服务验收留待有凭据时执行。

## 第四单元实现结果

- 新增 Adapter.Stream，以现有 SDK client 调用一次 Create，显式 stream=true / Accept: text/event-stream。正文、思考和函数参数实时发布，channel 发送和网络读取受调用方 context 控制，取消可释放连接和阻塞生产方。
- 生成的 SDK 事件对象不保留已知类型的原文，因此从同一 HTTP Body 使用 SDK 公开 types/stream.NewEventStream 读取 ServerEvent.Data；不再发请求，不自行实现 SSE 解帧，不修改 SDK 或共享传输。原 Create stream 负责关闭网络 Body，借用读取器的 Close 不重复关闭底层连接。
- 按连续 step 索引建组，允许不同函数交错增量 / 停止；保留 step.start 的初始正文、摘要及扩展。参数按 JSON 字符串累计，stop 时验证对象；完整初始参数与后续碎片冲突时明确失败，不猜测拼法。未知 step / delta、重复或未闭合 step 均报错。
- [官方 thought_signature](https://ai.google.dev/gemini-api/docs/thinking#streaming-with-thinking) 是 thought 结束前的完整签名，按原值保存，拒绝拼接重复签名或追加签名后的摘要。summary 允许缺省 / 空数组，图片摘要仅保留原文；文本摘要作为 EventThinking 交付。
- completed / requires_action 的状态事件或完成事件提供终态依据，核对 interaction ID 和所有 step 已闭合；其他失败状态及 error 事件不交付成功收尾。finish 复用 convertResponse，顺序发送最终 usage、ResponseState、整组 ToolCallEnd。终态附带完整 steps 时优先保留其原文，但必须与已发布正文、摘要、调用及签名一致。
- SDK 公开读取器使用空 sentinel，兼容合法终态后的正常 EOF 和可选 [DONE]；未收到终态、格式损坏、终态后的非法事件或真实读取错误仍失败。不会通过忽略 io.ErrUnexpectedEOF 掩盖断流。usage 的累计快照逐字段覆盖，缺省保留，忽略 step_usage 明细，不跨事件相加。
- 新增子包 testdata/interactions-tools.sse 和来源说明，均为本地合成资料，非真实抓包。测试经实际 Create、SDK 生成的事件解码器和公开 SSE 读取器，覆盖实时事件、工具组失败、状态 JSON 往返和模型切换续接、终态原文一致性、SSE 格式 / UTF-8、HTTP 错误、取消与连接关闭。Generate 保持现状，根包注册仍留在第五单元。
