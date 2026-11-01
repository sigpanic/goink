# v1.8.0 — 协议适配器、Claude / Gemini 接入与同会话切换

> 2026-10-22 更新：已确认 Google 原生接入改用 Interactions；以 [Gemini 子包新路线](../../../internal/llm/adapter/gemini/design.md)为准。本文下方的 GenerateContent、parts 和旧阶段状态保留为历史方案，不再作为本轮实现依据。

> 2026-10-16 修订。已确认采用协议适配器、官方 SDK 和同会话模型切换；不兼容的推理状态允许在出站时丢弃。实现顺序：先完成公共 adapter 基建，再迁移 OpenAI Chat，之后实现 Claude Messages、Gemini GenerateContent。当前只验收公共适配层，已改动的 OpenAI 草稿保持原状。配套 [commit-roadmap.md](./commit-roadmap.md)。

## 1. 目标与范围

使用 Anthropic、Google 官方 Go SDK 接入 Claude Messages 和 Gemini GenerateContent。SDK 负责原生请求和响应解析，Goink 适配器负责出站转换、`StreamEvent` 映射及原生响应状态的保存与回放。工具执行、审批、重试策略和会话管理继续由 Goink 负责。

本版本最终交付主 Agent / 子 Agent 对话、工具调用、标题生成、摘要压缩、连接测试、模型发现、重启续聊和同会话模型切换。当前阶段只明确公共 adapter 契约及职责，不迁移 OpenAI、不接入原生 SDK、不改造上层调用。后续 OpenAI 作为第一个具体适配器，收拢现有 providers、HTTP、SSE 和 hooks，再接两个原生适配器。

同一 session 可以切换 provider、model 和已支持的协议，保留用户输入、正文、工具调用与结果。切换在当前请求和工具组收尾后的空闲边界生效；进行中的主 / 子 Agent 使用已捕获的配置快照。不兼容的 thinking、signature 等推理状态可以从目标请求中移除，数据库原始记录和前端历史继续保留；切回原模型也不承诺恢复旧推理链。不同模型、账户或中转站的兼容性须分别验证，不承诺跨模型推理状态无损。

Google Interactions、Bedrock / Vertex 等部署和厂商托管 Agent 留到后续。每种新协议通过新增适配器扩展；现有服务商则按协议复用适配器和厂商参数规则。

Google 已推荐新项目使用 Interactions，同时仍完整支持 GenerateContent；Interactions 也支持 `store=false`，并非必须采用服务端会话。本次先使用 GenerateContent，是为了控制接入范围并覆盖已有原生端点；未来将 Interactions 作为独立协议适配器评估，不复用 GenerateContent 的 wire 格式。[官方说明](https://ai.google.dev/gemini-api/docs/interactions-overview)。

## 2. 改造前调用链与旧调查的继承

| 位置 | 当前行为 | 本次需要补齐 |
|---|---|---|
| `internal/llm/types.go` | Provider 提供请求、请求头、错误 hook；`StreamEvent` 没有最终原生状态 | 明确协议、运行时适配入口和响应状态事件 |
| `internal/llm/stream.go` | 按 Provider.ChatURL 发请求；payload / SSE 按 OpenAI 格式处理 | 公共基建验收后，接通 adapter → providerName → model 路由并迁移 OpenAI |
| `internal/llm/generate.go` | 非流式调用只读 `choices[].message.content` | 标题和压缩摘要也经过协议适配器 |
| `internal/session/types.go` | `ToAPIFormat` 导出 OpenAI 形状的 map | 携带内部响应状态，但不生成厂商原生格式 |
| `internal/agent/agent.go` | 流结束后根据工具执行结果重建 assistant 消息 | 保存完整响应状态，保证原生工具声明不被重建丢失 |
| `internal/agent/compress.go` | 从 map 反向提取少数 metadata 字段 | 明确新上下文版本的原生状态处理 |
| `internal/llm/test_connection.go` / `discover.go` | OpenAI URL 探测、SSE 检查及 `/models` 解析 | 共用协议配置、鉴权、端点和解析入口 |
| `app/chat.go` / `chat_api.go` | 每次请求可以传不同 provider/model | 接通切换预检、当前选择恢复和目标上下文投影 |

[v1.3.0 旧调查](../v1.3.0/multi-provider-adapter.md)继续作为需求与风险依据。采用 SDK 是实现路线变化，同会话切换是独立的产品目标；本版保留后者，并继承以下设计：

- 同会话允许切换，接受推理连续性有损；显示用 thinking 与模型实际接收的推理状态分开处理。
- 同协议内仍存在厂商差异，继续使用相应 hooks / 参数规则；异构协议由专门适配器转换。
- 切换前检查目标输入预算，压缩支持指定目标容量，预留本次输出预算和估算余量。
- SSE 回放与本地假服务为主要测试手段；标题、摘要、工具结果分组和中转字段差异均纳入验收。

以下旧结论需要修正或按具体端点重新核验：

- SDK 事件和 Goink 事件不同，因此需要适配器；这不意味着 SDK 收益仅限于减少 SSE 行解析代码。
- 现有 `messages []map[string]any`、`tools []map[string]any` 和调用参数需要转换；手写原生 HTTP 同样需要协议语义转换。
- SDK 的 HTTP client、自定义端点和重试可以配置，不能以“自管 HTTP”直接判定与 Goink 冲突。
- 依赖增量应按选定版本实测；旧稿的 `go.sum` 几百 MB、开发天数和行业普遍做法没有足够依据。
- 中转端点是否支持原生协议与是否使用 SDK 是两个维度。仅代理 OpenAI 路径的中转服务无法因此获得原生协议能力。
- 不假定签名有固定有效期，不采用“任意签名 400 都清空重试”的通用策略；具体模型、账户和上下文编辑限制应按官方规则处理。
- 不按“四族”直接承诺推理状态互通。兼容性同时涉及协议、模型、账户、历史前缀和厂商扩展；旧稿 GPT reasoning 字段需区分官方端点与中转扩展后再决定支持范围。

## 3. 协议适配器与 SDK 边界

### 3.1 SDK 选择与生命周期

- Claude：`github.com/anthropics/anthropic-sdk-go`，使用 Messages 调用及流式累积能力。[Go SDK 文档](https://platform.claude.com/docs/en/cli-sdks-libraries/sdks/go)。
- Gemini：`google.golang.org/genai`，使用 Models 的 GenerateContent / GenerateContentStream。HTTP client 和 BaseURL 可配置。[官方仓库](https://github.com/googleapis/go-genai)、[配置实现](https://github.com/googleapis/go-genai/blob/main/client.go)。

已在 `go.mod` / `go.sum` 锁定 Anthropic SDK `v1.81.0`、Google SDK `v1.75.0`，均要求 Go 1.24，项目继续使用 Go 1.25.0。当前依赖差异为 `go.mod` 增加 24 行、`go.sum` 增加 127 行；SDK 及生产包已构建通过。SDK 尚未被业务代码导入，依赖暂显示为 indirect；实现前不通过 tidy 移除它们。后续升级以确切版本和实际构建结果为依据。

各具体适配器在组装其 providers 时，为需要的 provider 创建 SDK client 并随配置快照使用。热重载由具体适配器构建新的 providers 快照并原子替换，请求先按 providerName 捕获所需配置与 SDK client，进行中的请求继续使用旧快照；不在每个流片段创建 client，不让热重载修改请求正在读取的数据。显式传入用户配置的密钥、端点及后端，防止环境变量悄悄改变 provider 选择。

向 SDK 注入现有 HTTP client / transport，保留代理、TCP 连接超时、首字节超时和 context 取消。长流不设置短的整体 `http.Client.Timeout`。SDK 特有超时设置也必须检查，避免覆盖现有长推理行为。

业务调用的重试归 Goink 管理。Anthropic 设置 `WithMaxRetries(0)`；Google 的单次请求次数根据锁定版本的重试配置明确关闭并测试，不混用不同 API 的重试选项。不使用 SDK 的 Tool Runner / 自动工具执行、托管 Agent 或自动会话循环。[Anthropic 重试和超时配置](https://platform.claude.com/docs/en/cli-sdks-libraries/sdks/go#retries)。

### 3.2 顶层适配器与三级选择

**适配器管理一种协议下的 providers。调用顺序固定为 adapter → providerName → model。** adapter 名称使用协议标识，不再增加与协议并列的选择维度。一个具体适配器实例持有自己的 providers 集合，内部再按 providerName 选择地址、密钥、模型能力、厂商规则及 SDK client。

```text
调用方：adapterName + providerName + model + Goink messages / tools / options
                                │
                        internal/llm.Client
                                │ adapters[adapterName]，按协议选择
          ┌─────────────────────┼────────────────────────┐
          │                     │                        │
  OpenAIChatAdapter   AnthropicMessagesAdapter   GeminiGenerateContentAdapter
  providers           providers                  providers
  ├─ deepseek         ├─ anthropic               ├─ gemini
  ├─ gemini           └─ relay-b                 └─ relay-c
  └─ relay-a
          │                     │                        │
          └─────────────────────┼────────────────────────┘
                                │ Goink StreamEvent / Response
                            Agent / Session
                        工具执行、审批、重试、存储
```

图中的 provider 名称用于说明作用域，不代表某个端点已经验证可用。同一逻辑服务商可出现在多个适配器下，例如 `openai_chat / gemini` 与 `gemini_generate_content / gemini`；两者分别配置该协议的端点、鉴权、模型及规则。适配器之间不互相查找或共享可变的运行配置。

顶层注册表的目标形状为 `map[adapter.Protocol]adapter.ProtocolAdapter`，映射项是协议名到协议级适配器。各实现内部持有以 `providerName` 索引的 providers 集合；provider 名称保持 `providerName`，不改称 providerKey。providerName 在所属 adapter 内唯一，完整调用目标由 adapterName、providerName、model 共同确定。

以下为公共调用契约，Request / Response 均为 Goink 类型，SDK 类型留在具体子包：

```go
type ProtocolAdapter interface {
    Providers() map[string]Provider
    Stream(ctx context.Context, req Request) <-chan StreamEvent
    Generate(ctx context.Context, req Request) (Response, error)
    DiscoverModels(ctx context.Context, providerName string) ([]ModelInfo, error)
    TestConnection(ctx context.Context, providerName, model string) (TestConnectionResult, error)
}

type Request struct {
    ProviderName string
    Model        string
    Messages     []map[string]any
    Tools        []map[string]any
    Options      *CallOptions
}
```

公共 `Provider` 只定义共用配置，不包含 SDK client 或协议 hooks。具体子包的运行时 Provider 匿名嵌入 `adapter.Provider`，再增加本协议需要的字段；具体 Adapter 自己持有 `providers map[string]本包Provider`。同名 providerName 可分别存在于不同协议适配器中。

不提供公共 Base：匿名嵌入用于组合公共字段，不会使具体 Provider 与 `adapter.Provider` 成为同一个类型，二者的 map 也不能直接互换。各具体适配器自行实现 Providers / Stream / Generate / DiscoverModels / TestConnection；Providers 只提取公共配置并返回独立副本，不暴露运行时 hooks 或 SDK client。构造和查询时的可变配置隔离由具体实现负责，包含模型数组、推理等级和温度指针；热重载创建新实例，让进行中的请求继续使用旧快照。

上层入口的目标签名示意如下，具体接线在公共基建验收后进行：

```go
func (c *Client) ChatStream(
    ctx context.Context,
    adapterName adapter.Protocol,
    providerName, model string,
    messages, tools []map[string]any,
    opts *CallOptions,
) <-chan StreamEvent
```

Client 按 adapterName 查找具体适配器，再组装带 ProviderName 和 Model 的 Request 交给它；适配器按 ProviderName 查找自身的 provider，再解析 Model。Request 中不必重复携带已由顶层选定的 adapterName。协议、provider 或模型选择失败均须明确报错，不从模型名推测协议，不跨适配器回退查找同名 provider。适配器按已声明的模型配置处理能力，未知能力与未知模型的策略在模型配置阶段明确。

`Request` 承载调用上下文，实际 HTTP / SDK 请求由适配器内部构造。`Response` 是统一的完整结果，包含正文、显示用思考、工具调用、可选 usage 和停止原因；来源及不透明的原生回放状态在后续状态持久化单元补齐。显示用思考不替代原生状态。

`Stream` 输出 Goink 事件，初始请求失败和流中失败统一通过 EventError 表达，生产方负责关闭 channel。提前结束消费时调用方取消 context 或排空流；发送事件也需响应取消，避免阻塞。完整工具组校验和最终状态顺序见第 5 节。`Generate` 调用对应协议的非流式 API，并将结果转换为 Goink Response，不强制通过消费 Stream 模拟非流式。两个方法共用该适配器的 provider 解析、历史投影、参数、鉴权与错误规则。

`DiscoverModels(ctx, providerName)` 和 `TestConnection(ctx, providerName, model)` 直接属于 ProtocolAdapter，不再拆成 ModelDiscoverer / ConnectionTester 两个能力接口；调用方均先选择 adapter，具体实现使用自己持有的 provider 配置与鉴权。发现不受支持时明确返回错误，允许手动配置模型；连接探测按对应协议的最小合法请求实现。Providers 返回各适配器的配置副本，根层据此汇总展示；公共层不提供配置存取 CRUD 或 SDK 私有接口。

`ChatStream`、`GenerateText`、`CallTool` 的业务用途保留，其调用签名后续统一显式指定 adapterName、providerName、model。GenerateText 从 Generate 的完整结果提取正文；CallTool 共用流式包装；Agent 负责对话循环、工具执行、审批和业务重试。允许分阶段改底层签名、后续再接上层，阶段间编译失败如实报告，不为维持旧签名改变已确定的架构。

本阶段的边界：仅公共 adapter 标识、Request / Response / 事件、Provider 及统一接口；实际持有 providers 的结构体在各具体协议子包中实现。不注册真实协议适配器，不迁移 OpenAI 模板或旧 llm.Provider，不改 OpenAI 实现和 Client。已存在的 OpenAI 迁移草稿保留在工作区，不纳入本次提交或验收。

### 3.3 继续使用当前 messages

`RunOptions.Messages` 暂时保留 `[]map[string]any`。它当前采用 OpenAI 形状，不应称为已经完全中立的消息模型。适配器根据 role、content、tool_calls、tool_call_id 和原生状态转换为 SDK 请求。

原生消息状态通过明确的内部保留键传递，例如 `_goink_llm_state`。该键只用于内存桥接，由 `ToAPIFormat` 从 `ExtraMetadata.llm_state` 导出；反向转换时还原到同一 metadata 槽。所有出站实现必须移除此类内部键，既不发送给 OpenAI 服务，也不当作 SDK extra fields。

厂商角色转换、工具结果分组和参数限制在 llm 适配器完成；Session 只负责保存、提取内部状态，不增加 `ToAPIFormat(dstProvider)` 来生成多套 wire 格式。每个适配器实现“Goink 消息 → 本协议”，复用公共的来源识别、工具组校验和 ID 映射辅助逻辑，避免为每一对协议编写互转器。实际出站字段由目标适配器白名单构造，不能因 map 形状相近就透传其他厂商扩展。

### 3.4 存储保持与需要修改的链路

保留现有 `messages` 表、Role / Content / ThinkingContent、工具 metadata、Version、ToAPI / ToFrontend 和 append-only 语义。新增状态放入现有 ExtraMetadata 扩展槽，无需新增消息来源数据库列。所有新 assistant 消息统一记录来源，OpenAI Chat 也适用；旧消息缺少来源时按未知处理，不根据模型名猜测回填。

存储结构复用不代表读写代码无需改动：Agent 保存完整响应状态，Session 导出 / 还原内部状态槽，压缩副本携带来源并处理状态失效，前端查询移除内部状态。各协议的工具结果仍存为统一 tool 消息，出站时再转换成对应 wire 结构。OpenAI 适配器无需为了接口统一额外复制一份完整响应 JSON。

### 3.5 子包内聚与依赖方向

目标目录按适配器组织。具体协议的 providers.go、厂商规则和运行时 provider 组装均归其子包；根目录 config.go 只负责用户配置存取与汇总，不保留集中承载全部服务商规则的 providers.go。

```text
internal/llm/
├── stream.go / factory.go       # 以后接线：按 adapterName 路由和创建协议级实例
├── config.go                   # 用户配置存取与适配器配置汇总
├── generate.go / calltool.go    # 共用业务包装，以后接入三级目标
├── types.go                    # 上层类型过渡，绑定兼容在接线时处理
├── adapter/
│   ├── adapter.go / types.go    # 当前范围：协议标识、公共接口与输入输出
│   ├── provider.go              # 当前范围：可由具体 Provider 嵌入的公共配置
│   ├── openaichat/              # 后续迁移
│   │   ├── adapter.go          # 持有本协议的 providers 集合
│   │   ├── providers.go        # 服务商模板及本协议下的 provider 组装
│   │   └── hooks_* / stream / generate / discover / test_connection
│   ├── anthropic/              # providers、SDK clients、请求 / 事件 / 状态转换
│   └── gemini/                 # providers、SDK clients、请求 / 事件 / 状态转换
└── testdata/sse/                # 保留已提交资料的位置
```

Go 导入方向为 `llm → 具体适配器子包 → adapter`，llm 同时直接依赖公共契约。公共 adapter 包不导入具体实现；各实现不反向导入 llm、Agent、Session 或 app，也不相互导入。根层创建入口按协议组装适配器，不遍历每个 provider 创建一个顶层适配器。

providers 的公共配置字段按语义复用，SDK client、协议专用选项及函数 hooks 留在具体子包，不参与用户配置 JSON 序列化。根层将用户配置按协议交给各适配器；内置模板匹配、provider_type 规则选择、配置合并和 provider 查找在对应子包内完成。根层不根据 providerName 匹配 OpenAI hooks，也不接触 Claude / Gemini 的私有参数。

新增同协议服务商只扩展该子包的 providers / 厂商规则；新增协议增加实现子包，再接入顶层注册入口。通用来源识别、工具组检查等仅在确有复用时加入公共包。模型发现、连接测试与实际生成均从同一适配器中按 providerName 获取同一份配置规则。

公共类型保持 SDK 无关，字段一致时复用，不按层复制相同 DTO。原有 StreamEvent、CallOptions 等上层过渡方式在接线时决定；Wails 导出的 ModelInfo、TestConnectionResult 应保持原绑定类型身份，必要时复用公共字段并在边界转换。这些兼容工作不扩展当前公共基建阶段的范围。

测试按职责归属：公共包检查契约与类型可编译；协议子包测试自己的 providers、hooks、转换及 SDK 回放；根层接线时验证按 adapterName 路由，同名 providerName 在不同适配器中可分别选择。Agent / Session 的生命周期测试在后续状态单元完成。

## 4. Provider 配置、模型切换与容量预检

### 4.1 Provider 配置

增加可持久化 `protocol`：

| 值 | 含义 |
|---|---|
| `openai_chat` | 当前 OpenAI Chat Completions 兼容调用链 |
| `anthropic_messages` | Anthropic Messages SDK |
| `gemini_generate_content` | Google GenerateContent SDK |

调用目标按 adapterName → providerName → model 组织，adapterName 使用上表的协议标识。providerName 是所属适配器内的 provider 配置名称，另有展示名和服务商类型 `provider_type`；同一类型允许配置多个不同 providerName，同名 providerName 可存在于不同适配器。设置流程先明确协议，再在该适配器中选择服务商模板或通用模板，填写名称、地址、密钥和模型。模板、配置合并和 hooks / 参数规则由对应子包处理，自定义与内置走相同链路，不依赖用户填写的名称猜测服务商类型。

协议不根据模型名字猜测。配置接线阶段处理旧配置缺字段的迁移，将其归入 `openai_chat`；旧内置服务商按现有明确匹配关系保留 hooks，未知自定义实例使用通用 OpenAI 配置。配置视图、保存转换、热重载、连接测试和模型发现共用具体适配器的解析结果；业务调用显式传入协议，缺失或不支持的协议不静默回退。原生协议模板在该协议阶段完整验收后开放。

复用现有 `chat_url` 配置入口，按协议解释和校验：OpenAI 保留现有完整聊天端点语义；原生协议接受 SDK 可使用的 API 根地址，并支持把明确的完整端点规范化为该根地址。保留中转路径前缀、版本及查询约束，不靠截到 host 的方式修正 URL。模型发现、连接测试和实际调用必须使用相同规范化结果；Google 的模型与调用方法路径由 SDK 构造。SDK 无法表达的任意自定义完整路由直接提示不支持，不增加一套隐藏的手写原生传输。

Claude 需要预留可选 workspace ID。官方当前同时支持 Bearer 和旧 `x-api-key`；未限定 workspace 的身份关联密钥可能要求 workspace header。使用 SDK 对应配置，不从供应商名称推断密钥类型。[官方鉴权说明](https://platform.claude.com/docs/en/manage-claude/authentication)。

模型能力需要覆盖 thinking 模式、可关闭性、可用 effort / budget、采样参数、输出上限和工具能力。先复用现有 ModelInfo 字段，确有不同语义再增加能力字段；不把所有厂商控制都硬塞成同一个 bool。内置模型使用经过核验的能力配置，自定义模型不凭名字宣称支持某项能力。

### 4.2 同会话切换的边界

会话使用最近一次接受的 adapterName / providerName / model 作为当前选择，允许下一轮更换。`Session.Model` 继续用于展示；当前完整选择可存入 `Session.ExtraMetadata.llm_selection` 供重启和 UI 恢复，不构成固定绑定。字段更新保留 ExtraMetadata 的其他键；旧会话没有该字段仍可选择任意已实现协议。

1. 仅在请求与工具组完成收尾后接受切换。流式生成、工具执行、审批等待和压缩期间保持当前快照；后端也检查并发执行状态，不能只靠禁用 UI 保证。
2. 接受目标前检查配置、模型能力、工具配对、目标历史投影和输入预算。普通拒绝发生在分配新 turn / 写用户消息之前；用户确认执行的压缩单独按第 7 节事务提交，后续失败不伪装成未压缩。
3. Chat 和手动 CompressContext 共用检查与投影能力，App 只接线；切换不要求创建新 session。UI 预检用于解释是否需要压缩，实际请求仍由后端重新检查。
4. 主 Agent 切换不改写已有子 Agent 的历史。运行中的子 Agent 保持自己的快照与推理状态，新启动的子 Agent 继承当前运行选择。
5. provider 地址、密钥或 workspace 改变也进入兼容性检查；相同 adapterName / providerName 不代表仍是相同账户或后端。

### 4.3 目标历史投影与推理状态丢弃

每次请求从当前版本的统一消息和来源状态生成目标协议历史，投影不修改原始消息。正常兼容续聊尽量回放完整原生内容；跨模型 / 协议转换时，对不兼容或来源未知的状态，使用正文和工具信息构造合法历史，移除无法复用的推理块、signature 及其他厂商私有字段。显示用 thinking 继续保留在数据库和前端，默认不转成目标模型的推理输入。旧 OpenAI 消息的常规续聊保持已验证兼容行为，进入原生协议时按外部历史处理。

| 历史情况 | 出站策略 |
|---|---|
| 来源、模型、后端及前缀兼容，状态完整 | 回放适配器确认可用的原生状态 |
| 跨协议 / 不兼容模型 / 需要转换的旧消息来源未知 | 保留正文、调用参数和结果，丢弃不兼容推理状态 |
| 工具 ID 格式不符合目标要求 | 为当前请求稳定映射，调用与结果同步转换，数据库 ID 保留 |
| 目标要求外部工具轨迹附带特殊标记 | 仅使用该协议明确支持的导入规则，见第 6 节 |
| 目标强制要求真实推理字段，外部历史无法提供 | 将受影响的完整历史组确定性渲染为带角色、工具名、参数和结果的普通上下文文本；保留顺序和含义，不重复执行工具 |

最后一项是目标适配器的显式兼容策略，须按实际端点验证；必要时覆盖完整的历史前缀，避免留下仍需签名的结构化片段。不统一填充空值或伪造源模型推理。DeepSeek 等端点对带 tools 的历史推理字段有专门要求，须作为 OpenAI 适配器的厂商规则和切换验收案例，不能假定删除字段即可接受。[DeepSeek thinking 规则](https://api-docs.deepseek.com/guides/thinking_mode/)

推理状态丢弃后继续在同一会话完成对话；普通正文和工具信息转换失败应报告具体原因，不能被当作可丢弃的推理状态。保留原始状态用于审计，不承诺切回模型后自动恢复。

为避免重启或切回时重新插入已经失效的推理链，在会话 metadata 中维护单调递增的 `reasoning_epoch`，新响应状态记录生成时的 epoch。当前仍可回放的状态首次因切换或前缀变化被舍弃时推进边界，后续仅回放当前 epoch 内经过兼容性校验的状态；历史中已有退役状态不导致每轮重复推进。进入实际执行时持久化边界，单纯 UI 选中或预检失败不改变它；已开始的请求失败也不自动恢复旧边界。子 Agent 独立维护自己的边界。

### 4.4 容量预检与按目标压缩

继承旧稿 `CanSwitchToModel` 的思路，预检输入包含 session、目标 provider / model 和本次选项。估算目标投影后的完整上下文，包含 system、工具定义、保留历史及新输入；不把不透明 signature 当普通文本直接计数。UI 阶段给出估算，正式请求以实际输入重新计算。

对于输入输出共享窗口的模型，采用 `inputBudget = ContextWindow - 本次输出预算 - 估算余量`。输出预算按实际请求选项和模型限制解析，不一律预留型号允许的最大输出；独立输入窗口的模型按其能力定义处理。统一 tokenizer 只用于估算，不宣称对所有厂商误差固定；窗口未知时明确显示未知，并提供手动配置入口。

自动压缩阈值基于可用输入预算，沿用 80% 提前触发比例；切到较小窗口时，压缩支持指定目标容量，完成后重新检查。超窗时由用户选择执行压缩或更换模型；没有成功生成合规上下文之前不发送超窗请求，也不静默截断历史。压缩失败保留原上下文，后续规则见第 7 节。

## 5. 完整响应状态与事件顺序

### 5.1 状态保存

在 `Message.ExtraMetadata.llm_state` 中保存有版本的 SDK 无关封装：来源 protocol / provider / model、reasoning_epoch、完整响应标志、必要的停止原因，以及可选的原始 assistant content 或 parts。所有适配器记录来源；原生数组以协议所属的 JSON 保存，只有对应适配器解释内部字段。Claude 状态还记录生成该响应时的请求前缀指纹，用于发现 system、tools 或此前历史被改写。字段结构在内部一处声明，避免只提取 signature 后丢掉位置和块类型。

前缀指纹在适配器完成原生转换后计算，仅覆盖签名相关的 system、工具定义和该响应之前的原生消息，不包括密钥、HTTP header、后续追加消息或 usage。回放时对同一历史边界重新计算，保持数组顺序并采用稳定 JSON 表达；指纹只用于本地识别编辑，不能代替服务端验签。不能直接对当前全量 payload 做哈希，否则每次追加新用户消息都会被误判为前缀改变。压缩副本明确标记旧 Claude thinking 不再回放，不通过重写指纹伪装状态仍有效。

`Content`、`ThinkingContent`、`tool_calls`、`tool_displays` 继续作为当前统一消息和前端展示字段。原生状态经来源、epoch 和前缀校验后优先恢复原始 assistant 内容；不兼容时走第 4.3 节的有损投影，不能把重新拼接的展示文本、工具数组冒充原始 signed blocks。保存前核对统一工具 ID / 参数与原生声明一致；若去重或过滤导致不一致，不能静默发送互相矛盾的两份历史。

SDK 累积对象序列化后必须做真实 SDK 请求类型的 round-trip 测试，验证字段、空块、签名和数组顺序都保留。SDK 增加未知块类型时，不因显示层暂不识别而过滤原始状态；仅推理状态不兼容可按规则舍弃，无法安全转换的正文、工具声明等业务内容返回明确错误。旧 OpenAI 消息的常规续聊保留已验证兼容行为，缺失来源不作为猜测或伪造原生签名的依据。

API 消息查询仍只读取当前 version 且 `to_api=true` 的消息；前端查询继续使用 `to_frontend`，完整审计保留原始记录。前端响应的 metadata 投影移除 `llm_state`，保留现有显示所需字段；不为此新建重复的 Message DTO。历史加载须保持确定顺序，验证 created_at 并列和重启回放的次序，不以全局 map 遍历顺序构造 signed blocks。

### 5.2 流式事件

| 适配器响应信息 | Goink 事件 / 状态 |
|---|---|
| 普通文本增量 | `EventContent` |
| 可显示的思考摘要增量 | `EventThinking` |
| 工具名称、ID 已确定 | 一次 `EventToolCallStart` |
| 参数增量 | `EventToolCallArguments`，保持现有累计文本语义 |
| 最终 usage | 一次 `EventUsage`，已归一化 |
| 完整响应及状态 | 新增内部 `EventResponseState`；OpenAI 也记录来源 / 完成状态，不转为前端文本事件 |
| 全部工具参数校验成功 | 按原生顺序发送 `EventToolCallEnd` |
| 请求、流读取、参数错误 | `EventError`，保留现有 APIError 语义 |

`EventResponseState` 携带来源、epoch、停止原因及可选原生内容，在最终 usage 之后、所有可执行 `EventToolCallEnd` 之前发送；协议未提供 usage 时不伪造统计。整个适配器流正常结束、所有工具声明均完整且参数通过验证后才进入该收尾阶段；任一参数非法都不发任何可执行结束事件，继续沿用 `tool_args_invalid` 提醒和重试行为。第一步封装 OpenAI 适配器时先保持既有事件行为，再在公共状态阶段增加该事件。

Agent 收到状态后按当前 stream 保存到临时槽；最终 assistant 落库时与显示字段一起写入。无工具的最终回复和只有原生状态的完整回复也必须保存状态。重试开始时同时清空文本、thinking 和原生状态，不能把失败尝试的 signature 混入下一次响应。Agent 只处理通用完成状态和保存动作，不解析厂商块或判断协议兼容性。

HTTP 200 内的协议 error、异常 EOF、输出截断、拒绝和零产出要按实际 finish / stop 信息区分。截断或错误响应不得仅因参数字符串恰好构成有效 JSON 就执行工具。用量可按确实收到的最终值记录，但不能发多次累计事件。

### 5.3 取消和执行中断

所有 channel 发送感知 ctx 取消，退出时关闭 SDK stream，避免消费方停止后 goroutine 卡在发送。当前 `flushInterruptedTools` 使用非阻塞排干，不能把它当作原生响应一定完整的保证。

原生流成功收尾时，状态事件已给出完整工具声明；后续工具执行因审批拒绝、取消或失败中断，必须为每个原始调用生成一个执行结果或“未执行 / 已中断”结果，再保存完整 assistant 和对应结果，不能只重建已执行的 tool_calls。这属于 Agent 层收尾，SDK 不执行工具。

如果 SDK 流尚未完整结束，保留已有可见文本作为 partial 展示和审计；新增原生路径的该 partial 不作为下一次可回放的完整 assistant 消息，明确 `to_api=false`。不伪造缺失签名、不保留悬空工具调用、不改变现有 OpenAI 路径的 partial 行为。

## 6. 按实现顺序接入三个适配器

### 6.1 OpenAI Chat：公共基建验收后迁移

公共基建验收后，再收拢 `stream.go` / `generate.go` 的请求与响应处理、连接测试、模型发现，以及现有 providers / hooks。服务商模板和运行时组装迁到 `adapter/openaichat`，适配器持有 providers，调用按 providerName 和 model 解析；根层按 openai_chat 选择该适配器。先验收完整调用链及三级选择，再引入来源状态、切换投影和容量检查。当前工作区已有的单 provider 实例草稿保持原状，后续按本节设计调整，不在公共基建单元中处理。

对同协议不同厂商分别验证 reasoning 字段、thinking 参数和工具历史要求。完整保存当前端点确实返回、后续需要回传的推理状态；缺少原始推理时按第 4.3 节构造兼容历史，不统一生成非空占位思考。这个阶段完成后形成后两个 SDK 适配器遵循的调用与事件契约。

### 6.2 Claude Messages：第二个适配器

出站将稳定的头部 system 消息转换到顶层 system；普通 user / assistant 转换为原生内容，tools.parameters 转为 input_schema。工具参数的 OpenAI JSON 字符串转换为 input 对象，tool 消息转换成 user 下的 tool_result。连续的工具结果按对应 assistant 调用组装，工具结果先于同条消息中的其他文本，不改变助手原始内容块顺序。[Messages](https://platform.claude.com/docs/en/api/messages/create)、[工具结果规则](https://platform.claude.com/docs/en/agents-and-tools/tool-use/handle-tool-calls)。

SDK 流累积完整 content，保留 text、tool_use、thinking、signature、redacted_thinking 和空 thinking 块。显示层只取可显示文本；重启后恢复完整数组。不能依赖工具执行后的统一文本重建 signed content。[工具与 thinking 工作流](https://platform.claude.com/docs/en/build-with-claude/thinking-tool-workflows)。

thinking 按模型能力配置：优先支持 adaptive 和 output_config.effort；旧型号手动 budget 模式只有在测试覆盖的能力配置下开放，不统一发送 enabled。输出上限、temperature 和 forced tool choice 也按能力校验。当前 CallTool 使用提示词约束工具的方式继续保留。[手动 thinking](https://platform.claude.com/docs/en/build-with-claude/extended-thinking)、[参数限制](https://platform.claude.com/docs/en/build-with-claude/thinking)。

对于其他协议或不兼容模型产生的历史，以统一正文、调用和结果重建 Messages，不携带外部 thinking / signature。只有推理块且没有可用业务内容的历史消息不生成空 assistant 占位。正常续聊保持完整原生内容；切换、压缩和前缀变化后的处理见第 7 节。Claude 阶段同时验收与 OpenAI Chat 双向切换，完成后再开始 Gemini。

### 6.3 Gemini GenerateContent：第三个适配器

出站把 system 转为 systemInstruction，assistant 转为 model，消息内容转为 parts。工具声明使用 SDK 支持的 JSON Schema 字段，验证当前 Registry schema 的兼容子集；不先把所有 schema 降成字符串。工具调用 input 转为 functionCall.args，工具结果转为 functionResponse 的对象；非对象或纯文本结果采用稳定包装，保持结果含义和错误信息。[GenerateContent 参考](https://ai.google.dev/api/generate-content)。

原生调用 ID 存在时原样保留；不存在时为 Goink 工具执行生成稳定本地 ID，并记录它与原始调用位置的对应关系，不把本地 ID 冒充原生响应字段。同名并行工具通过位置 / ID 配对，不能只用函数名匹配。先收录全部调用，再回传整组结果。

完整保存 parts，包括 thought 标志、thoughtSignature 与空文本签名片段。不能把所有签名合成一个消息级字符串，也不能因文本为空提前结束流。正常同模型回放必须保留真实签名；跨模型导入的工具历史若落入目标校验范围，按官方 FAQ 使用 `skip_thought_signature_validator`，仅用于投影生成的外部轨迹，不覆盖真实 Gemini 响应中的签名，也不回写数据库。这是导入标记，不代表原推理被恢复；目标中转不支持时采用第 4.3 节的完整文本投影并单独验证。[签名规则与导入 FAQ](https://ai.google.dev/gemini-api/docs/generate-content/thought-signatures#faqs)。

thinkingLevel / thinkingBudget、includeThoughts 和是否可关闭按型号能力映射；不同时发送互相冲突的控制，也不把 ThinkingEnabled=false 解释成所有型号都可关闭推理。响应中的可显示 thought 是摘要，不能作为原生推理状态替代品。[Thinking 配置](https://ai.google.dev/gemini-api/docs/generate-content/thinking)。

## 7. 压缩与签名限制

同会话切换和上下文压缩都可能使旧推理状态失效。Claude 部分新模型还检查 system、tools 和此前历史前缀；同厂商、同 provider 也不代表所有 signed blocks 均可互通。[Preserved thinking](https://platform.claude.com/docs/en/build-with-claude/preserved-thinking)。

v180 使用以下策略：

1. 正常工具循环使用同一份 system、tools 和调用配置快照，按顺序追加消息；读取原生状态时先验证来源、reasoning_epoch 和实际目标兼容性。
2. 原生上下文压缩只能在工具调用与结果配对完整的边界实施。切割保留区时不得从一组工具结果中间开始。自动压缩与手动压缩都通过同一套协议规则处理。
3. 摘要先通过目标适配器投影历史并生成；按第 4.4 节为摘要调用本身检查容量，必要时分段摘要，不能把超窗历史直接发送给小窗口目标。压缩支持指定目标容量，成功后沿用新 active_version + 新消息副本的事务，并原子保存新的选择及必要的 epoch 边界；失败保留原上下文和版本，不删除或更新旧审计消息。
4. Claude 客户端压缩改变前缀后，新版本去除保留副本中的旧 thinking / redacted_thinking，继续保留文本和完整工具配对；不把已经失效的签名复制后当作可回放状态。旧型号手动 thinking 的首块规则需要专项测试，未验证的组合不开放，不塞占位 thinking。
5. Gemini 的保留历史按原始 parts 和工具配对处理，不把 Claude 的前缀校验规则当作 Gemini 的规则，也不默认清空所有签名。保留状态在新版本副本中的读写须通过请求回放测试。
6. 前缀指纹发现 tool 集合等其他前缀发生变化时，对受影响的推理链采用协议允许的整体丢弃规则并推进 epoch。保留完整业务内容和工具配对，在原会话继续；不得从中间随意移除 thinking 后又复活旧块，不无限循环清空重试。不能构造合法请求时报告具体未支持的组合，不把新建会话作为正常切换流程。

不自动启用 drop_block beta 作为长期方案。以后若采用服务端 compaction / context editing 或 binding controls，必须单独评估模型和中转端点支持；本版本明确接受客户端压缩后旧 Claude 推理状态的损失。

## 8. 辅助调用、错误与 usage

GenerateText 走相同适配器、配置解析和出站投影，提供有界输出预算并提取普通文本，供标题和摘要使用。不因原生 provider 缺少 OpenAI choices 而报 empty response。标题生成不改写主会话当前选择或 epoch；摘要读取同一会话的历史，但不把摘要请求的签名加入主对话。

连接测试使用真实适配器发送最小请求，验证对应协议能完成解析。原生协议不复用 OpenAI 的 URL 试探和 max_tokens=1 策略；按模型最小合法配置选择输出预算，区分“鉴权 / 路由可能正确”与“生成成功”，不会把限制响应伪装成实际成功。测试结果回写的 URL 与保存、发现、调用一致。

模型发现使用 SDK 对应的 Models API 并处理分页。Google 过滤不支持 GenerateContent 的模型；原始名称转成 SDK 可接受的模型标识。缺失的 context / output / thinking 信息沿用已验证模板或留未知，不能从展示名推断能力。发现失败允许手动配置模型，不暗中退回另一协议。

各适配器将错误转换为 `APIError`，保留 HTTP 状态、可重试性、Retry-After 和足够的错误类型。协议参数、历史不兼容和签名错误不走一般网络退避；限流和暂时性服务错误沿用现有 Agent 策略。协议专用恢复仅作用于明确识别、允许恢复的错误，在尚未执行工具时重新投影；不能把全部 400 标成可重试，也不能因切换重新执行历史工具。

usage 在 llm 层转换为现有统计口径，使用一个共用归一化入口：输入、输出、总量、缓存和推理明细。Claude 输入缓存与未缓存字段的包含关系、Gemini thoughts 与 candidates 的包含关系必须按原生定义核算，避免漏算或重复计入；一次请求仅记录一次最终用量。继续匹配当前 map 数字类型，测试主对话、session 展示及 activity 累计结果。[Claude usage](https://platform.claude.com/docs/en/api/messages/create)、[Gemini UsageMetadata](https://ai.google.dev/api/generate-content#UsageMetadata)。

## 9. 验证与发布边界

公共基建先独立构建并检查契约，不导入具体协议。后续主要使用 `httptest` 返回 HTTP / SSE，让 OpenAI 现有传输或真正的 SDK 和适配器经过完整调用链。先验证按协议路由及 provider 作用域，再验收 OpenAI，之后完成 Claude 的完整功能与切换，最后验收 Gemini 及三协议互切。样例不含真实密钥、用户小说内容或其他敏感数据。测试不限于 mock 出几个 SDK event。

| 验证组 | 必须覆盖 |
|---|---|
| 配置 / 适配器 | adapterName → providerName → model 三级选择、同名 provider 在不同协议下隔离、未知 adapter / provider 不跨协议回退、旧配置迁入 OpenAI、provider_type / 实例名称分离、内置 / 自定义复用规则、发现 / 测试 / 对话共用配置、路径前缀、凭据、热重载、长流取消 |
| OpenAI 首阶段 | 现有 HTTP / SSE / hooks 收拢前后行为一致，文本 / 工具 / 错误、标题、摘要、CallTool、连接测试、模型发现均走适配器 |
| Claude | 文本 / thinking / tool_use 交错、redacted / 空 thinking、signature 累积、并行工具、工具结果次序 |
| Gemini | thought / text / functionCall、无原生 ID、同名并行调用、单独签名片段、最后空文本 part |
| 执行边界 | 完整流才执行、任一非法参数全拒绝、截断、HTTP 200 内 error、取消 / 审批拒绝时全组结果配对 |
| 历史 / 压缩 | 同模型多轮、序列化后重启、保留顺序、按目标容量压缩成功与失败、旧状态失效、副本与原审计分离、子 Agent 隔离 |
| 模型切换 | 三协议六个有向切换方向、同协议不同模型 / provider、旧消息未知来源、带并行工具历史、不兼容状态不外发、原正文 / 工具信息保留、历史工具不重复执行、切回及重启不复活旧推理链 |
| 切换边界 / 容量 | 运行与审批中拒绝切换、后端预检、UI 选择不改 epoch、已有退役状态不重复推进、配置失效、目标窗口缩小、输出预算 / tools / system 计入、预检失败不分配 turn / 写用户消息、压缩成功后独立事务语义 |
| 辅助入口 / usage | 标题、摘要、CallTool、连接测试、发现分页、缺失模型能力、usage 不重复累计 |
| 回归 | 当前 OpenAI 请求 hook、headers、tool_args_invalid、partial 和切换行为保持可用；无内部 metadata 外发 |

代码阶段从仓库根目录执行 `go build ./...`、`go test ./...`，按 Go 变更执行 lint；前端阶段进入 frontend 执行 build / lint / test。涉及 DTO 的阶段通过 Wails 命令重新生成绑定。当前单元只验证公共 adapter 包与文档。用户允许分阶段签名改造造成工作区编译失败；具体协议和上层尚未接线时记录失败位置，不改冻结草稿来凑整仓通过。上层接线阶段再恢复生产包完整验证。

SDK 版本已锁定，内置型号能力、外部历史导入规则和目标中转站路径仍需实施阶段核验。官方直连烟测仅在有可用凭据时进行，并报告具体模型 / 端点与未验证项；本轮没有真实 API 互通证据。模拟测试通过不代表所有中转站、账户或模型均兼容。

## 10. 实现状态与当前边界

- 设计文档基线已提交（`d38f6ed1`）；本单元修订顶层 adapter、providers 归属、providerName 命名及实施范围。
- 当前只验收 `internal/llm/adapter` 的公共 Provider、含 ProviderName 的 Request、统一结果 / 事件，以及包含生成 / 发现 / 探测的统一接口。公共层不提供 Base；实际持有 providers、配置快照隔离、原 llm.Provider 的迁移、根层注册表和具体协议接线属于后续单元。
- 工作区此前已产生单 provider 绑定式 OpenAI 子包、Client / factory、配置引用和测试迁移草稿；用户要求保持现状，本次不继续修改、不回滚、不暂存这些文件，也不将其标记为新设计已完成。
- 本单元提交公共 adapter 基建及配套设计文档；SDK 依赖和上述具体实现草稿继续保留为未暂存改动。
- 公共包通过 build / test / vet / lint 检查；当前没有具体适配器行为测试，providers 持有与快照隔离测试随具体实现补充。工作区完整 LLM 目录仍受冻结草稿的旧接口及未接线影响，本单元不修复其编译错误。此前旧方案的生产包 / race / lint 通过记录不作为本次三级路由的验收证据。
- Anthropic `v1.81.0`、Google `v1.75.0` 已安装，依赖改动未提交；SSE 资料已按供应商分目录提交（`e1bfb0a9`）。原生 SDK 业务接入、来源状态、同会话切换和 UI 尚未实现，未生成绑定；真实模型 API 烟测未进行。
- 总体顺序为公共基建 → OpenAI → Claude → Gemini；每个实现单元完成后等待 review。既存 dev_test 编译问题仍单独记录，旧调查保留。
