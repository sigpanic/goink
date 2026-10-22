# Google Interactions 适配器实施路线

> 2026-10-22。第一单元已实现，待 review；生成、历史转换、发现 / 探测和注册尚未实现。本文替代旧总设计中的 Gemini GenerateContent 实施路线。本阶段继续自底向上，仅改 `internal/llm/`，每个单元完成后等待 review，不自动暂存或提交。

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
| 4. 流式生成 | `feat(llm): stream Gemini interaction steps` | stream*.go、协议样本和测试。真实 SDK SSE、step 累积、参数和签名分片、终态及错误、取消释放连接；任一工具损坏不交付部分可执行组。旧 Google testdata 是 GenerateContent，不能当作 Interactions 正确性证据。 |
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
