# v1.3.0 — 多 Provider 协议适配（任意切换 + 同族真保留 + 跨族静默降级）

> 本文档为 v1.3.0 多 provider 适配的**修订版**，替代初稿的"三族原生适配 + ToAPIFormat 翻译"方向。
> 修订原因见 [§1.3](#13-本版相对初稿的修正)。

## v2 修订说明（2026-07-25）

本次 v2 修订基于新一轮调研结论对原文档进行勘误与补充，关键变更：

- **§1.1 根因修正为三层**：Bug C 不只是"中转站加严校验"，根源在 DeepSeek V4 官方协议要求 `reasoning_content` 回传，叠加中转站加严 + Goink 塞空字符串策略错误
- **§1.2 业界调研补充**：DeepSeek V4（2026-04-24 发布）官方协议约束，旧模型 deepseek-chat / deepseek-reasoner 于 2026-07-24 下线
- **§2.1 任意切换立论降级**：从"用户无感"降级为"协议层可行 / 语义层有损 / 用户可能感知质量下降"
- **§2.3 跨族策略改为"目标要求明文才塞明文"**：修正原"明文 thinking 跨族都传"的错误策略，避免对要求加密的目标 provider 塞无意义明文
- **§2.4 静默降级规则修正**：中转站空 reasoning_content 校验失败时塞非空内容重试（原"清空重试"在 DeepSeek V4 场景会更失败）
- **新增 GPT reasoning summary opt-in 缺口**：GPT 默认不返回 summary，请求时需 opt-in `reasoning.summary`，否则 GPT 消息没明文 thinking 可塞给其他 provider
- **新增 §3.5 CanSwitchToModel 切换前校验层**：复用现有 `CountMessagesTokens` / `ProviderModel` / `loadAPIMessages`，按 `ContextWindow - MaxOutputTokens` 预检
- **新增 §6.x 上下文窗口超限 / compress 阈值 bug**：发现 Goink `agent.go:194` 的 80% 阈值用 `ContextWindow` 没扣 `MaxOutputTokens`，可能导致超过输入预算但不触发压缩
- **核心原则**：永远塞非空内容（避免 Bug C 重演），宁可语义错位也不空字符串

## 1. 背景与动机

### 1.1 v1.2.0 issue #26 Bug C 根因

用户 log（`/tmp/snorlax_goink.log`）显示：

- message[74] 用 `xiangliang/gpt-5.3-codex`（GPT 中转）生成
- GPT 模型返回 `reasoning` 字段（OpenAI 标准），不返回 `reasoning_content`（DeepSeek 扩展）
- Goink 的 [stream.go:325](file:///home/nianhe/projects/todo/internal/llm/stream.go#L325) **只解析 `reasoning_content`，不解析 `reasoning`** → `ThinkingContent=""`
- 用户切换到 `向量引擎/deepseek-v4-flash`（DeepSeek 中转），Goink 把历史消息回传
- [session/types.go:79-81](file:///home/nianhe/projects/todo/internal/session/types.go#L79-L81) 在 `ThinkingContent=""` 且含 `tool_calls` 时设 `payload["reasoning_content"]=""`
- DeepSeek V4 官方协议要求 `reasoning_content` 在 tool_calls 场景必须回传（见下文三层根因 #1），中转站按协议加严校验拒绝空字符串 → 报 400

**v2 三层根因修正**（原文只归咎于"中转站加严校验"是片面的）：

1. **DeepSeek V4 官方协议要求 reasoning_content 完整回传**（2026-04-24 发布，[官方文档](https://api-docs.deepseek.com/zh-cn/guides/thinking_mode) 原文："当模型进行了工具调用的轮次，在后续所有请求中，必须完整回传 reasoning_content 给 API"）。这是协议层硬约束，不是中转站自定义。openclaw 实测发现 DeepSeek 官方 API 后端只校验字段存在性、不校验内容（空字符串也能过），但协议文档措辞加严，中转站据此实现严格校验。旧模型 deepseek-chat / deepseek-reasoner 已于 2026-07-24 下线，所有用户强制迁 V4，此约束不可回避。
2. **中转站按 DeepSeek 协议文档加严校验**（`vectorengine.cn` 行为）：协议文档说"必须完整回传"，中转站按字面要求非空。DeepSeek 官方后端宽松（接受空字符串）与协议文档措辞加严之间存在不一致，中转站倾向于按文档加严。
3. **Goink 塞空字符串的策略错误**（[session/types.go:79-81](file:///home/nianhe/projects/todo/internal/session/types.go#L79-L81)）：`ThinkingContent=""` 时设 `payload["reasoning_content"]=""`——既不满足"完整回传"的协议要求，也撞中转站非空校验。**正确策略应永远塞非空内容**（源 thinking 明文或占位符），宁可语义错位也不空字符串。

**根本问题**：Goink 只支持 OpenAI Chat Completions + DeepSeek 协议同构的字段集，不支持 OpenAI 的 `reasoning` 字段，更不支持 Anthropic / Gemini 的异构协议；并且在 thinking 缺失时采用了错误的"塞空字符串"策略，叠加 DeepSeek V4 新协议约束与中转站加严校验，三方共同触发 Bug C。

### 1.2 业界调研结论

| 框架 | 统一抽象 | 加密 token 处理 | 跨模型切换 |
|---|---|---|---|
| LangChain | ❌ 拒绝基类统一，要求 provider-specific 子类 | 各自处理 | 不处理 |
| LiteLLM | ✅ 统一为 `message.reasoning_content` 字符串 | 透传 | 自动转换（有损） |
| OpenAI SDK | N/A | 客户端必须原样回传 `reasoning.state` / `reasoning_details[].encrypted_content` | N/A |
| Anthropic SDK | N/A | `thinking_blocks[].signature` 必须回传 | N/A |
| Vercel AI SDK | ✅ 流式 `reasoning-delta` + `providerOptions` 下钻 | 保留加密 payload | provider 各自处理 |
| Trae / OpenRouter / one-api | ✅ 对外统一 OpenAI，内部网关翻译 | **跨模型直接丢弃**，thinking 仅前端显示 | 任意切换，但 thinking 跨模型不保留 |

**业界共识**：

1. 加密 token（OpenAI `encrypted_content` / Anthropic `signature`）必须原样回传，客户端无法解密
2. 跨 provider 切换时加密 token **必然丢失**，协议根本不兼容，无解
3. 业界主流是"provider-specific 适配层"而非"基类统一"
4. vLLM 已从 `reasoning_content` 改名为 `reasoning`，向 OpenAI 字段名靠拢
5. **Trae / OpenRouter 等"任意切换"产品的真相**：thinking 只用于前端展示，不参与跨模型 LLM 上下文。用户感知不到 thinking 丢失，但新模型实际看不到老模型的推理过程

**DeepSeek V4 协议约束（v2 补充，2026-07-25 调研）**：

| 维度 | 约束 | 说明 |
|---|---|---|
| 发布时间 | 2026-04-24 | DeepSeek V4 正式发布 |
| 旧模型下线 | 2026-07-24 | deepseek-chat / deepseek-reasoner 下线，所有用户强制迁 V4 |
| tool_calls 场景 | reasoning_content 必须回传 | 官方文档原文："当模型进行了工具调用的轮次，在后续所有请求中，必须完整回传 reasoning_content 给 API"。缺失或为空会触发 400 `The reasoning_content in the thinking mode must be passed back to the API` |
| 官方后端实际行为 | 只校验字段存在性 | openclaw 实测：DeepSeek 官方 API 后端只校验 `reasoning_content` 字段存在、不校验内容，空字符串能过 400 校验。但中转站按协议文档措辞加严校验（要求非空）才会拒绝空字符串 |
| 跨族影响 | 明文 thinking 必须塞非空 | 任意源 → DeepSeek V4（tool_calls）时，Goink 必须塞非空 `reasoning_content`（源 thinking 明文或占位符），不能塞空字符串 |

来源：[DeepSeek 官方文档 thinking_mode](https://api-docs.deepseek.com/zh-cn/guides/thinking_mode)

**Goink 的 hook 机制（hooks_minimax.go / hooks_qwen.go / hooks_moonshot.go）是业界主流做法**，与 LobeChat 的 `model-runtime` 类似。当前缺失：GPT / Anthropic / Gemini 的 hook。

### 1.3 本版相对初稿的修正

| 维度 | 初稿方向 | 修订方向 | 修订理由 |
|---|---|---|---|
| 分类维度 | 按"协议族"分（OpenAI 兼容族 / Anthropic 族 / Gemini 族） | 按"thinking 可互通性"分四族 | GPT 虽是 OpenAI 协议，但 thinking 加密，与国产明文族不通；按协议分族会把 GPT 错误塞进明文族 |
| 切换策略 | 隐含"按族限制切换" | **任意切换 + 同族真保留 + 跨族静默降级** | 用户看不懂加密 token 丢失，只看到"不能切=软件垃圾"；限制切换是产品死局 |
| GPT 工作量 | "加 `delta.reasoning` 解析即可"（标为可选） | 必须做 `reasoning` + `reasoning_details` 加密 token 完整链路 + 过期兜底 + **reasoning summary opt-in** | 光解析明文只治标（ThinkingContent 不为空），GPT 跨轮 thinking 保留必须靠加密 token；且 GPT 默认不返回 summary，必须请求时 opt-in `reasoning.summary: "auto"` 才能拿到明文 thinking 塞给其他 provider |
| Gemini 适配 | 走原生 `:streamGenerateContent`，写完整翻译层 | 走 OpenAI 兼容端点 + `extra_content.google.thought_signature` 持久化 | 实测兼容端点吐 `extra_content.google.thought_signature`（不是 `reasoning_content` 明文），需持久化 signature 才能跨轮；比原生协议翻译层省事，中转站友好 |
| Claude `max_tokens` | 默认 4096 | 取 ModelInfo.MaxOutputTokens | thinking 开启后 budget_tokens 与输出共享 max_tokens，4096 会被推理 token 吃光 |
| signature 过期 | "v1.3.0 不做" | 必须做"清空 thinking_blocks 重试一次"兜底 | 不做的话 Claude thinking 跨轮必撞 400，对话卡死 |
| 测试 | 依赖真实 API | SSE 录制回放为主，不需要国外卡 | 用户无国外卡；协议适配代码的测试不需要真连网 |

## 2. 核心决策：任意切换 + 同族真保留 + 跨族静默降级

### 2.1 为什么不限制切换

**v2 立论降级**（原文称"用户无感"过于乐观，修正为"协议层可行 / 语义层有损 / 用户可能感知质量下降"）：

- **协议层任意切换可行**：DeepSeek V4 官方只校验 `reasoning_content` 字段存在性、不校验内容，塞非空内容能过；其他族目标忽略或重新推理，不会卡死对话
- **语义层有损**：thinking 跨族是假保留，加密 token 丢、推理连续性断裂，新模型实际看不到老模型的推理过程
- **用户可能感知到质量下降**：跨族切换后模型可能答非所问或重复推理，**不再是"用户无感"**；但仍优于限制切换（行业硬伤，Trae 也做不到）
- **产品决策**：用户心智模型是"随便切"，"不能切"会被判定为软件垃圾；Trae / OpenRouter / Cherry Studio 都允许任意切换，Goink 限制切换等于主动让出竞争力
- **跨 provider 加密 token 丢失是行业天花板**，Trae 也做不到，Goink 不必为此背锅

**Goink 的差异化卖点**改为：**同族内 thinking 真保留**（如 DeepSeek↔Qwen 切换 thinking 完整延续），这点 Trae 不一定做。跨族是行业硬伤，谁都没解。

### 2.2 四族分类（按 thinking 可互通性）

| 族 | 成员 | thinking 跨轮保留机制 | 族内切换 | 跨族切换 |
|---|---|---|---|---|
| **国产明文族** | DeepSeek / Qwen / Doubao / GLM / Kimi / MiMo / MiniMax | `reasoning_content` 明文 | ✅ thinking 完整保留 | 明文可传，加密 token 丢 |
| **GPT 加密族** | GPT-5 / o 系 | `reasoning` 对象 + `reasoning_details[].encrypted_content` / `reasoning.state` | ✅ 靠加密 token | 加密 token 必丢 |
| **Claude 签名族** | Claude | `thinking_blocks[].signature` | ✅ 靠 signature | signature 必丢 |
| **Gemini 签名族** | Gemini 3 / 2.5 系 | thought part + `thought_signature`（function calling 场景**强制要求**回传，丢就 400） | ✅ 靠 `thought_signature` | signature 必丢，跨族用 `skip_thought_signature_validator` 兜底 |

**分类原则**：thinking 能不能跨模型互通，取决于加密 token 的可移植性，而不是协议是否同构。GPT 与 DeepSeek 同为 OpenAI Chat Completions 协议，但 GPT 加密、DeepSeek 明文，thinking 跨过去会丢——所以 GPT 不能并入国产明文族。

**族归类的修正（v1.3.0 修订）**：初稿把 Gemini 归为"无跨轮族"是**事实错误**。实测 Gemini 3/2.5 系通过 `thought_signature` 支持跨轮 thinking 保留，且 Gemini 3 在 function calling 场景**强制要求**回传 signature（缺失会报 400 "Function call FC1 in the 1. content block is missing a thought_signature"）。正确归类为"Gemini 签名族"，与 Claude 签名族机制类似但 signature 字段不同（Gemini 附加在 part 上，Claude 在 thinking block 内）。

### 2.3 跨 provider 行为矩阵

| 切换路径 | 明文 thinking | 加密 token | LLM 上下文延续 | 用户感知 |
|---|---|---|---|---|
| 国产 → 国产 | 塞 `reasoning_content`（非空） | N/A | ✅ 延续 | 无感 |
| GPT → GPT | 塞 `reasoning`（仅显示，需 opt-in `reasoning.summary`） | ✅ 回传 `reasoning_details` | ✅ 延续 | 无感 |
| Claude → Claude | 塞 thinking block（仅显示） | ✅ 回传 `signature` | ✅ 延续 | 无感 |
| Gemini → Gemini | 塞 thought part（仅显示） | ✅ 回传 `thought_signature` | ✅ 延续 | 无感 |
| 国产 → GPT | **不塞**（GPT 不认明文 reasoning，忽略） | ❌ 无 | ❌ GPT 重新推理 | 前端能看到历史思考 |
| GPT → 国产（含 tool_calls） | **塞 GPT reasoning 摘要（非空）到 `reasoning_content`** | ❌ 丢 | ⚠️ 国产看到明文 thinking（GPT 摘要） | 前端能看到历史思考 |
| 国产 → Claude | **不塞**（Claude 不认无 signature 的 thinking block） | ❌ 丢 | ❌ Claude 重新推理 | 前端能看到历史思考 |
| GPT → Claude | **不塞**（Claude 不认无 signature 的 thinking block） | ❌ 丢 | ❌ Claude 忽略并重新推理 | 前端能看到历史思考 |
| Claude → 国产（含 tool_calls） | **塞 Claude thinking 文本（非空）到 `reasoning_content`** | ❌ 丢 | ⚠️ 国产看到明文 thinking | 前端能看到历史思考 |
| Claude → GPT | **不塞**（GPT 不认明文 reasoning） | ❌ 丢 | ❌ GPT 重新推理 | 前端能看到历史思考 |
| 任意 → Gemini（function calling） | **不塞明文，塞 `skip_thought_signature_validator` 占位** | ❌ 丢 | ⚠️ Gemini 绕过 400 校验，重新推理 | 前端能看到历史思考 |
| 任意 → Gemini（无 function calling） | 不强制，塞或不塞都行 | ❌ 丢 | ⚠️ 取决于目标 provider | 前端能看到历史思考 |
| Gemini → 任意 | 按目标 provider 规则决定（见上） | ❌ 丢 | ⚠️ 取决于目标 provider | 前端能看到历史思考 |

**关键修正（v2）**：原"明文 thinking 跨族都传"策略错误。修正原则：

1. **目标要求明文才塞明文**（DeepSeek V4 tool_calls 场景官方要求 `reasoning_content` 明文回传 → 塞源 thinking 明文：GPT 摘要 / Claude thinking 文本 / Gemini thought 文本，**非空**）
2. **目标要求加密不塞明文**（GPT 要求 `encrypted_content` / Claude 要求 `signature` / Gemini function calling 要求 `thought_signature`——这些目标会忽略或重新推理他族明文 thinking → 不塞明文，让目标重新推理）
3. **目标不强制（Gemini 无 function calling）**：塞或不塞都行
4. **永远塞非空内容**（避免 Bug C 重演，宁可语义错位也不空字符串）
5. **Gemini function calling 场景塞 `skip_thought_signature_validator` 占位**绕过 400

**关键**：明文 thinking 跨族"保留"是**假保留**——前端能看到历史思考文本，但只有"目标要求明文"的路径（如 → DeepSeek V4 tool_calls）才能让新模型看到，其他路径新模型重新推理。这是协议硬伤，业界无解。

**Gemini 跨族兜底特殊**：Gemini 3 在 function calling 场景强制校验 `thought_signature`，跨族（如 Claude → Gemini）时历史 functionCall part 没有 Gemini signature，会撞 400。dotnet-genai 代码揭示可用特殊字符串 `"skip_thought_signature_validator"` 塞进 `thought_signature` 字段绕过校验，这是官方逃生口。

### 2.4 静默降级规则

跨族切换时**不在 UI 提示**"丢失推理上下文"（用户看不懂），改为后端静默处理：

1. **正常路径**：按 §2.3 矩阵决定塞什么——目标要求明文才塞源 thinking 明文（非空），目标要求加密则不塞明文（让目标重新推理），Gemini function calling 塞 `skip_thought_signature_validator` 占位
2. **报错兜底**（v2 修正，原文"中转站空字段清空重试"在 DeepSeek V4 场景会更失败）：
   - 中转站校验失败（空 `reasoning_content` / 缺字段）→ 400 → **塞非空内容**（源 thinking 明文或占位符如 `" "`）重试一次。**不能清空**——DeepSeek V4 协议要求字段存在，清空了更不行
   - Claude signature 过期 / 缺失（同族内）→ 400 → 清空 thinking_blocks 重试一次（Claude 同族内有效）
   - GPT `reasoning.state` / `encrypted_content` 过期 → 400 → 清空 reasoning_details 重试一次
   - Gemini `thought_signature` 过期 / 缺失（同族内）→ 400 → 清空 `thought_signatures` 重试一次
   - Gemini function calling 跨族缺 signature → 400 → 塞 `skip_thought_signature_validator` 重试一次
3. **重试仍失败**：发 EventError，由 agent 层走常规重试逻辑

**关键修正**：原"清空该字段重试"只对 Claude / GPT 同族内加密 token 过期有效，对中转站校验失败 / DeepSeek V4 协议要求**不适用**（清空更违反协议）。中转站校验失败必须塞非空内容重试。

## 3. 架构设计

### 3.1 三层适配模型

```
┌─────────────────────────────────────────────────────────────┐
│  请求层（出站）                                              │
│  - ToAPIFormat(dstProvider): 单条消息格式转换（按四族翻译）  │
│  - BuildRequest: 顶层请求结构转换（Hook）                    │
│  - BuildHeaders: 请求头改造（Hook）                          │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│  响应层（入站）                                              │
│  - ParseStream: 解析流式响应（Hook，替代硬编码 parseSSE）    │
│  - ParseError: 解析非标准错误响应（Hook）                    │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│  持久化层（存储/回传）                                       │
│  - Message.Provider: 标记每条消息由哪个 provider 生成       │
│  - ExtraMetadata: 通用扩展槽（tool_calls / 加密 token 等）  │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 Hook vs 翻译的边界

| 适配场景 | 类型 | 说明 |
|---|---|---|
| 请求参数转换（如 thinking → enable_thinking） | Hook | 流程主体不变，局部改字段 |
| 请求头转换（如 MiMo 鉴权） | Hook | 流程主体不变 |
| 错误响应解析 | Hook | 流程主体不变 |
| GPT `reasoning` / `reasoning_details` 字段解析 | Hook | 协议同构，只加字段 |
| Claude 消息格式 / 流式事件转换 | 翻译 | 协议异构，整个 ParseStream 替换 |
| Gemini（若走原生）消息格式 / 流式事件转换 | 翻译 | 协议异构 |
| Gemini（若走兼容端点） | Hook | 协议同构，但需 Hook 解析 `extra_content.google.thought_signature` 并持久化到 ExtraMetadata |

**判定原则**：协议同构用 Hook，协议异构用翻译。**Gemini 优先走兼容端点以避免翻译**，但仍需 Hook 解析 thought_signature（不是零适配）。

### 3.3 ToAPIFormat 改造（按四族翻译）

**决策**：ToAPIFormat 加 `dstProvider` 参数，根据 `srcProvider`（消息自身的 Provider 字段）和 `dstProvider`（目标 provider）按四族规则转换。

**Message 表新增 Provider 字段**：

```go
type Message struct {
    // ... 现有字段 ...
    Provider string `gorm:"column:provider;not null;default:''" json:"provider"`
}
```

- 持久化时：从当前 provider 设置 `msg.Provider = providerName`
- ToAPIFormat 时：读 `m.Provider` 判断 srcProvider
- DB migration：加字段，历史消息默认空字符串（视为国产明文族）
- **历史消息特殊处理**：若历史消息的 Model 字段能推断出 GPT/Claude/Gemini（如 `gpt-*` / `claude-*` / `gemini-*`），migration 时回填 Provider；无法推断的视为国产族。**避免 Bug C 重演**（历史 GPT 消息被当国产族处理）

**ToAPIFormat 新签名**（保留现有 role 处理逻辑，新增 dstProvider 参数）：

```go
func (m *Message) ToAPIFormat(dstProvider string, logger *slog.Logger) map[string]any
```

**字段转换按四族规则**（v2 修正：按目标 provider 官方回传要求决定塞什么，而非"明文跨族都传"）：

| srcProvider | dstProvider | 明文 thinking 塞到 | 加密 token 处理 |
|---|---|---|---|
| 国产 | 国产（含 tool_calls） | `reasoning_content`（非空，源明文） | N/A |
| 国产 | GPT | **不塞**（GPT 不认明文 reasoning，忽略） | ❌ 不传 |
| 国产 | Claude | **不塞**（Claude 不认无 signature 的 thinking block） | ❌ 不传 |
| 国产 | Gemini（function calling） | **不塞明文，塞 `skip_thought_signature_validator` 占位** | ❌ 不传 |
| 国产 | Gemini（无 function calling） | 不强制 | ❌ 不传 |
| GPT | GPT | `reasoning`（仅显示，需 opt-in `reasoning.summary`） | ✅ `reasoning_details` 原样回传 |
| GPT | 国产（含 tool_calls） | `reasoning_content`（非空，塞 GPT reasoning 摘要） | ❌ 丢 |
| GPT | Claude | **不塞**（Claude 不认无 signature） | ❌ 丢 |
| GPT | Gemini（function calling） | **不塞明文，塞 `skip_thought_signature_validator` 占位** | ❌ 丢 |
| Claude | Claude | thinking block（仅显示） | ✅ `thinking_blocks[].signature` 原样回传 |
| Claude | 国产（含 tool_calls） | `reasoning_content`（非空，塞 Claude thinking 文本） | ❌ 丢 |
| Claude | GPT | **不塞**（GPT 不认明文 reasoning） | ❌ 丢 |
| Claude | Gemini（function calling） | **不塞明文，塞 `skip_thought_signature_validator` 占位** | ❌ 丢 |
| Gemini | Gemini | thought part（仅显示） | ✅ `thought_signature`（按 tool_call_id 索引）原样回传 |
| Gemini | 国产（含 tool_calls） | `reasoning_content`（非空，塞 Gemini thought 文本） | ❌ 丢 |
| Gemini | GPT | **不塞**（GPT 不认明文 reasoning） | ❌ 丢 |
| Gemini | Claude | **不塞**（Claude 不认无 signature） | ❌ 丢 |

**关键设计原则**（v2 修正）：

1. **按目标 provider 官方回传要求决定塞什么**：
   - dstProvider = 国产明文族（DeepSeek V4 等）+ 含 tool_calls 历史 → 塞源 thinking 明文到 `reasoning_content`（**非空**，DeepSeek V4 协议要求）
   - dstProvider = GPT → **不塞 reasoning 明文**（GPT 不认明文，重新推理，不报错）
   - dstProvider = Claude → **不塞 thinking block 明文**（Claude 不认无 signature 的，忽略）
   - dstProvider = Gemini + function calling → **塞 `skip_thought_signature_validator` 占位**（绕过 400）
   - dstProvider = Gemini + 无 function calling → 不强制
2. **加密 token 只在同族内回传**（`srcProvider` 与 `dstProvider` 同族时才从 ExtraMetadata 取）
3. **跨族切换自然丢失加密 token**（符合业界做法，静默处理）
4. **ExtraMetadata 永不删除**：DB 里保留所有 provider 的字段，给"切回来还能用"的机会
5. **永远塞非空内容**：任何必须塞的字段（如 DeepSeek V4 的 `reasoning_content`）都要塞非空，避免 Bug C 重演
6. **不需要 BuildMessages hook**：被 ToAPIFormat 取代

**ExtraMetadata 存储内容**：
- `tool_calls`：assistant 消息的工具调用数组（现有）
- `tool_call_id` / `tool_name`：tool 消息（现有）
- `display_text`：显示文本（现有，可选）
- `reasoning_details`：GPT 的加密 token（新增，族2 内回传）
- `thinking_blocks`：Claude 的加密 token（新增，族3 内回传）
- `thought_signatures`：Gemini 的加密 token（新增，族4 内回传，按 tool_call_id 索引；text part 的 signature 单独存一个 `text_thought_signature` 字段）

**调用方改动**：
- `app/chat.go` 的 `loadAPIMessages`：调用 `msg.ToAPIFormat(dstProvider, logger)` 传入当前会话 provider
- `internal/agent/compress.go` 的 `Compress`：同上
- 其他调用 ToAPIFormat 的地方：都需要传 dstProvider

### 3.4 Provider 抽象扩展

现有 [Provider 结构](file:///home/nianhe/projects/todo/internal/llm/types.go#L12-L23) 加 `ParseStream` 字段：

```go
type Provider struct {
    // ... 现有字段 ...
    // ParseStream 解析流式响应，把 provider 特有 SSE 翻译成 StreamEvent。
    // nil 表示用默认的 OpenAI SSE 解析器（parseSSE）。
    // Claude 必须实现。Gemini 走兼容端点时为 nil，但默认 parseSSE 需扩展解析 extra_content.google.thought_signature。
    ParseStream func(r io.Reader, ch chan<- StreamEvent) error
}
```

**不需要 BuildMessages**：消息格式转换由 `session.Message.ToAPIFormat(dstProvider)` 负责。
**不需要 ParseReasoningDetails**：加密 token 提取由 ParseStream 内部完成，直接存到 ExtraMetadata。

**三层职责划分**（正交，不重叠）：

| 层 | 函数 | 包 | 职责 |
|---|---|---|---|
| 单条消息 | `ToAPIFormat(dstProvider)` | session | 出站消息格式转换（按四族规则） |
| 顶层结构 | `BuildRequest(payload)` | llm | 顶层请求结构转换（messages vs contents、system 字段、generationConfig 包装） |
| 入站流 | `ParseStream(r, ch)` | llm | 入站流式响应解析（SSE → StreamEvent） |

### 3.5 切换前校验层（CanSwitchToModel）

**v2 新增**（2026-07-25 调研发现 Goink 切换模型时未做上下文窗口预检，可能灾难）。Goink 切换模型时如果当前会话 token 数超过目标模型可用输入窗口，会撞 400 或上下文截断（如 DeepSeek 1M 聊到 600K 切给 200K 模型）。

**Goink 现有基础设施**（已确认可用）：

- token 计数：[`internal/llm/token_counter.go`](file:///home/nianhe/projects/todo/internal/llm/token_counter.go) 的 `CountMessagesTokens`，用 `o200k_base` BPE 编码器，所有 provider 共用
- ModelInfo 字段：`ContextWindow` 和 `MaxOutputTokens` 已存在（[`internal/llm/types.go:28-36`](file:///home/nianhe/projects/todo/internal/llm/types.go#L28-L36)）
- compress 机制：80% 阈值触发，**不能指定目标 token 数**（不能直接复用做预检压缩）
- 切换模型入口：没有专门 `SwitchModel` binding，切换 = 下次发消息传不同 `ProviderName` + `ModelID`；前端 [`ChatPanel.tsx:946-958`](file:///home/nianhe/projects/todo/frontend/src/components/ChatPanel.tsx#L946-L958) `handleSelectModel`

**新增 binding**：

```go
type SwitchCheckResult struct {
    Allowed       bool   `json:"allowed"`
    CurrentTokens int    `json:"current_tokens"`
    InputBudget   int    `json:"input_budget"`   // ContextWindow - MaxOutputTokens
    Reason        string `json:"reason,omitempty"`
}

func (a *App) CanSwitchToModel(providerName, modelID string) (*SwitchCheckResult, error)
```

**接入点**：前端 `handleSelectModel` 在切换前调用 `CanSwitchToModel` 预检，超窗时弹出提示（"当前会话 N tokens 超过目标模型输入预算 M，将自动压缩/截断/开新会话/选更大模型"）。

**复用现有函数**：

- `CountMessagesTokens(messages)` 计算当前会话 token 数
- `ProviderModel(providerName, modelID)` 查目标模型 ModelInfo
- `loadAPIMessages(sessionID)` 取当前会话消息

**inputBudget 公式**：

```
inputBudget = ContextWindow - MaxOutputTokens
```

比 Goink 现有 compress 阈值（`ContextWindow * 0.8`）更保守，因为 compress 阈值没扣 `MaxOutputTokens`（见 §6.x compress 阈值 bug）。

**超窗处理策略**（推荐自动压缩 + 提示）：

| 策略 | 优点 | 缺点 |
|---|---|---|
| 自动压缩到 inputBudget 内 | 用户无感，对话延续 | 压缩有损，可能丢早期上下文 |
| 截断早期消息 | 简单 | 丢上下文 |
| 开新会话 | 干净 | 用户需手动迁移上下文 |
| 选更大模型 | 上下文完整 | 依赖用户有更大模型权限 |

推荐：**自动压缩 + 前端提示"已压缩 N 条早期消息以适配目标模型"**。压缩复用现有 compress 机制，但需要扩展支持"指定目标 token 数"（当前 compress 只能按 80% 阈值触发）。

## 4. 各族适配方案

### 4.1 国产明文族（0 改动）

**成员**：DeepSeek / Qwen / Doubao / GLM / Kimi / MiMo / MiniMax

**现状**：已工作。`reasoning_content` 明文互通，族内切换 thinking 完整保留。

**改动**：无。族内 provider 互相切换时，ToAPIFormat 透传 `reasoning_content`，无需加密 token。

### 4.2 GPT 加密族（完整链路）

**成员**：GPT-5 / o 系

**改动范围**：
- `internal/llm/stream.go`：加 `delta.reasoning` + `delta.reasoning_details` 解析
- `internal/llm/providers.go`：加 `"openai"` 内置 provider
- `internal/session/types.go`：ToAPIFormat 支持 `dstProvider="openai"`，回传 `reasoning_details`
- **请求侧 opt-in `reasoning.summary`**（v2 补充，见下文 reasoning summary opt-in）
- 过期兜底：agent 层捕获 GPT 加密 token 过期 400，清空 `reasoning_details` 重试一次

**关键**：不是"加几行解析"就够。GPT 跨轮 thinking 保留**必须**靠加密 token：

1. 解析 `delta.reasoning`（明文 summary）→ EventThinking（前端显示）
2. 解析 `delta.reasoning_details[].encrypted_content` / `reasoning.state`（加密）→ 存 ExtraMetadata
3. ToAPIFormat("openai") 时把 `reasoning_details` 原样塞回请求
4. 过期兜底：400 → 清空 reasoning_details 重试

**reasoning summary opt-in（v2 补充，2026-07-25 调研发现的缺口）**：

OpenAI 官方明确"不暴露 raw reasoning tokens"，`reasoning` 字段返回的是 summary（摘要），完整推理靠 `encrypted_content` 加密回传。**关键缺口**：GPT 默认**不返回 summary**，必须请求时显式 opt-in `reasoning.summary: "auto" | "detailed" | "concise"`。否则 GPT 消息根本没有明文 thinking 可塞给其他 provider（如 DeepSeek V4 tool_calls 场景需要的 `reasoning_content`）。

来源：[OpenAI Reasoning 文档](https://developers.openai.com/api/docs/guides/reasoning)

| 模式 | 行为 |
|---|---|
| 未 opt-in（默认） | 不返回 `reasoning` summary，只有 `encrypted_content` 加密 token |
| `reasoning.summary: "auto"` | 返回摘要（推荐，平衡成本与可读性） |
| `reasoning.summary: "detailed"` | 返回详细摘要（token 多） |
| `reasoning.summary: "concise"` | 返回简洁摘要 |

**Chat Completions API 限制**：Chat Completions API 对 reasoning 支持是**降级模式**——拿不到 `encrypted_content`，也拿不到结构化 summary 数组。完整形态需要 Responses API（v1.3.0 不做，见 §8.5）。在 Chat Completions 下，opt-in `reasoning.summary` 能拿到摘要字符串，足够塞给 DeepSeek V4 等需要明文 `reasoning_content` 的目标 provider。

**改动**：GPT provider 的 BuildRequest / 请求构造里加 `reasoning: {summary: "auto"}`（用户可配置）。前端 GPT 模型设置加"返回思考摘要"开关，默认开（否则 GPT→国产 tool_calls 切换时无明文可塞）。

**风险**：低。GPT 用标准 OpenAI Chat Completions 协议，BuildRequest 仅加 `reasoning.summary` opt-in 字段。

### 4.3 Claude 签名族（原生适配，最大头）

**成员**：Claude（Sonnet / Opus / Haiku）

**改动范围**：
- 新增 `internal/llm/hooks_anthropic.go`：实现 `anthropicBuildRequest` / `anthropicParseStream` / `anthropicParseError`
- `internal/llm/providers.go`：加 `"anthropic"` 内置 provider
- `internal/session/types.go`：ToAPIFormat 支持 `dstProvider="anthropic"`

**Anthropic Messages API 协议要点**：

| 维度 | OpenAI Chat Completions | Anthropic Messages |
|---|---|---|
| 端点 | `POST /v1/chat/completions` | `POST /v1/messages` |
| 认证 | `Authorization: Bearer` | `x-api-key` + **`anthropic-version: 2023-06-01`（必填）** |
| 顶层结构 | `{messages:[...], model, temperature, tools}` | `{system:..., messages:[...], max_tokens, model, tools}` |
| content 类型 | 字符串 | 数组 `[{type:"text",text:...}]` |
| thinking | `reasoning_content` 字符串 | `{type:"thinking",thinking:...,signature:...}` block |
| tool_calls | `{id,type:function,func:{name,arguments}}` | `{type:"tool_use",id,name,input}`（input 是对象，非字符串） |
| tool 结果 | `{role:"tool",content,tool_call_id}` | `{role:"user",content:[{type:"tool_result",tool_use_id,content}]}` |
| 流式事件 | `data: {choices:[{delta:{...}}]}` | `event: message_start/content_block_delta/message_stop`（命名事件，无 `[DONE]`） |
| 加密 token | `reasoning_details[].encrypted_content` | `thinking_blocks[].signature` |
| stop reason | `stop`/`tool_calls`/`length` | `end_turn`/`tool_use`/`max_tokens` |

**anthropicBuildRequest 职责**：
1. 提取 system 消息（OpenAI 放 messages[0]，Anthropic 单独字段）
2. tools 格式转换（`parameters` → `input_schema`，`tool_choice` 类型映射）
3. max_tokens 必填：取 ModelInfo.MaxOutputTokens（**不要 4096**，thinking 开启后会被推理 token 吃光）
4. thinking 参数：开 thinking 时设 `thinking: {type:"enabled", budget_tokens: ...}`

**anthropicParseStream 职责**：
1. 解析命名 SSE 事件：`message_start` / `content_block_start` / `content_block_delta` / `content_block_stop` / `message_delta` / `message_stop`
2. `text_delta` → EventContent
3. `thinking_delta` → EventThinking；`content_block_stop` 时提取 `signature` 存 ExtraMetadata
4. `input_json_delta` 累积成 tool_use.input（对象，不是字符串）
5. `message_delta` 的 `stop_reason` 映射到 finish 语义

**anthropicParseError 职责**：解析 Anthropic 错误体 `{type:"error",error:{type,message}}`

**过期兜底**：agent 层捕获 signature 过期 400，清空 thinking_blocks 重试一次。

**风险**：
- 工作量大（顶层结构、流式事件、tool_calls、tool 结果全部不同）
- Anthropic 的 tool 结果放在 user 消息里，与 OpenAI 的 tool 角色不同，ToAPIFormat 转换要小心
- thinking_blocks 在 tool-call 场景必须回传，否则 400
- **必须做过期兜底**，否则 signature 过期后对话卡死

**实现路径决策**：**不引入** anthropic-sdk-go，自研 `hooks_anthropic.go` 翻译层。理由见 §8.6。自研时可参考 anthropic-sdk-go 的 `packages/ssestream` 子包（SSE 解析）和 `message.go`（Accumulate 累积 signature / input_json 逻辑）作为协议解析参考实现，但不拖入依赖。

### 4.4 Gemini 签名族（兼容端点 + signature 持久化）

**成员**：Gemini 3 / 2.5 系（Pro / Flash / Flash Thinking）

**实测结论**（取代初稿的"待验证假设"）：Google 官方 OpenAI 兼容端点 `https://generativelanguage.googleapis.com/v1beta/openai/chat/completions` **不吐** `reasoning_content` 明文，而是吐 `extra_content.google.thought_signature`。因此 Gemini **不能并入国产明文族**，需要单独持久化 signature。但兼容端点复用 OpenAI SSE 解析器（parseSSE），比原生协议翻译层省事，且中转站友好。

**关键事实**（v1.3.0 修订，纠正初稿误判）：
- Gemini 3/2.5 系通过 `thought_signature` 支持跨轮 thinking 保留（不是"无跨轮族"）
- Gemini 3 在 function calling 场景**强制要求**回传 `thought_signature`，缺失会报 400 "Function call FC1 in the 1. content block is missing a thought_signature"
- 兼容端点吐的字段是 `extra_content.google.thought_signature`（非标 OpenAI 扩展字段），**不是** `reasoning_content`
- 跨 provider 切换时（如 Claude → Gemini）历史 functionCall part 没有 Gemini signature，可用特殊字符串 `"skip_thought_signature_validator"` 塞进 `thought_signature` 字段绕过校验

**三条路径工作量对比**：

| 路径 | 工作量 | 主要问题 |
|---|---|---|
| **兼容端点 + signature 持久化（推荐）** | ~1.1 天 | 中转站可能过滤 `extra_content` 非标字段，需兜底 |
| 集成 google.golang.org/genai SDK | ~3.8 天 | +35 个传递依赖（含 Google Cloud auth）；走 v1beta 原生端点，**中转站用户走不通**；SDK 自管 HTTP，绕过 Goink Provider hook 抽象 |
| 自研 hooks_gemini.go 原生翻译层 | ~3.1 天 | role 体系/parts/streamGenerateContent SSE 全异构；Google 频繁迭代 signature 规则；中转站走不通 |

**推荐路径：兼容端点 + signature 持久化**

**改动范围**：
- `internal/llm/stream.go`：parseSSE 加 `delta.extra_content.google.thought_signature` 解析（Gemini signature 附加在 part 上，兼容端点把它塞到 delta 的 extra_content 扩展字段）
- `internal/llm/providers.go`：加 `"google"` 内置 provider，ChatURL 指向 `https://generativelanguage.googleapis.com/v1beta/openai/chat/completions`
- `internal/session/types.go`：ToAPIFormat 支持 `dstProvider="google"`，回传 `thought_signatures`（按 tool_call_id 索引）
- 过期/缺失兜底：agent 层捕获 Gemini signature 过期/缺失 400，清空 `thought_signatures` 重试一次（与 Claude/GPT 同款路径）

**signature 持久化细节**：
- Gemini signature 附加在 part 上（functionCall part 或 text part 末尾），不是独立 block
- 多个 functionCall 时，只有第一个 part 有 signature；顺序 functionCall 时每个 step 的第一个 part 都有
- 存 ExtraMetadata 时按 `tool_call_id` 索引（与 GPT 的 `reasoning_details`、Claude 的 `thinking_blocks` 并列）
- text part 末尾的 signature 可选回传，不影响 400 但影响推理质量，推荐保留（单独存 `text_thought_signature` 字段）

**跨 provider 兜底**：
- 跨族切换到 Gemini 时（如 Claude → Gemini），历史 functionCall part 没有 Gemini signature
- 塞特殊字符串 `"skip_thought_signature_validator"` 到 `thought_signature` 字段，绕过 Gemini 3 校验
- 这是 dotnet-genai 代码揭示的官方逃生口，Google 后端识别该字符串后跳过校验

**中转站风险**：
- 国内中转站可能过滤 `extra_content` 非标字段（OpenAI 标准里没这个字段）
- 中转站过滤后 signature 在链路中丢失，跨轮 thinking 失效
- 兜底：检测 signature 丢失时降级为"单轮 thinking"，不回传 signature，让 Gemini 重新推理

**原生协议要点**（备选，若兼容端点在某中转站走不通）：

| 维度 | OpenAI | Gemini 原生 |
|---|---|---|
| 端点 | `POST /v1/chat/completions` | `POST /v1beta/models/{model}:streamGenerateContent?alt=sse` |
| 认证 | `Authorization: Bearer` | `x-goog-api-key` |
| 请求结构 | `{messages:[{role,content}]}` | `{contents:[{role,parts:[{text}]}],systemInstruction:...}` |
| role | `user`/`assistant`/`system`/`tool` | `user`/`model`/`function` |
| thinking | `reasoning_content` | `{thought:true,text:...}` part + `thought_signature` |
| tool_calls | `{id,type:function,func:{name,arguments}}` | `{functionCall:{name,args}}`（args 是对象） |
| tool 结果 | `{role:"tool",content,tool_call_id}` | `{role:"function",parts:[{functionResponse:{name,response}}]}` |
| 流式 | `data:` + `[DONE]` | `data:{candidates}` 无 sentinel |

**风险**：
- 兼容端点是 Google 维护的，比第三方翻译可靠，但 `extra_content` 字段非标，中转站兼容性需逐个实测
- `thinking_level` / `thinkingBudget` 精细控制在兼容端点可能不支持（需实测）
- Gemini Interactions API（有状态模式，省 signature 管理）Go SDK 还没暴露，未来可选

## 5. 实施阶段

### 5.1 优先级与工作量

| 优先级 | 任务 | 价值 | 工作量 | 依赖 |
|---|---|---|---|---|
| P0 | 国产族：现状已工作，不动 | — | 0 | — |
| P1 | GPT 族：reasoning + reasoning_details 完整链路 + 过期兜底 + reasoning summary opt-in | GPT 思考可见且跨轮保留，直接修 Bug C | 0.5 天 | DB migration |
| P2 | Claude 族：hooks_anthropic.go 原生适配 + signature 持久化 + 过期兜底 | Claude 思考可见且跨轮保留 | 2-3 天 | DB migration |
| P3 | Gemini 族：兼容端点 + thought_signature 持久化 + 跨族兜底 | Gemini 可用且 thinking 跨轮保留 | ~1.1 天 | DB migration |
| P4 | 跨族降级 UX：报错静默清空/塞非空重试 + DB migration（Provider 字段） | 防止跨族切换卡死 | 1 天 | P1/P2 |
| P5 | 测试：SSE 录制回放 | 验证正确性 | 1 天 | 各族完成 |
| P6（v2 新增） | CanSwitchToModel 实现 + 前端预检 UI + compress 扩展支持目标 token 数 | 防止上下文窗口超限灾难 | 1 天 | P1 |

**合计**：~6 天，不需要国外卡。

### 5.2 阶段拆分

**阶段 1：基础设施 + GPT 族 + CanSwitchToModel（P1 + P4 基础 + P6）**

- DB migration：Message 加 Provider 字段，历史消息按 Model 名回填
- Provider 加 ParseStream 字段，parseSSE 重命名为 `openAIParseStream` 作为默认
- stream.go：加 `delta.reasoning` + `delta.reasoning_details` 解析
- providers.go：加 `"openai"` 内置 provider
- ToAPIFormat：加 dstProvider 参数，支持 "openai" 回传 reasoning_details
- **GPT 请求构造加 `reasoning.summary: "auto"` opt-in**（v2 新增）
- 过期兜底：agent 层捕获 GPT 加密 token 400，清空重试
- **CanSwitchToModel 实现 + 前端 handleSelectModel 预检 UI**（v2 新增，见 §3.5）
- **compress 扩展支持指定目标 token 数**（v2 新增，用于超窗时自动压缩）
- **修复 `agent.go:194` compress 阈值未扣 MaxOutputTokens 的 bug**（v2 新增，见 §6.x）

**阶段 2：Claude 族（P2）**

- 新增 hooks_anthropic.go
- providers.go 加 `"anthropic"` 内置 provider
- ToAPIFormat 支持 "anthropic"
- signature 过期兜底

**阶段 3：Gemini 族（P3）**

- providers.go 加 `"google"` 内置 provider，ChatURL 指向兼容端点
- parseSSE 加 `delta.extra_content.google.thought_signature` 解析
- ToAPIFormat 支持 `"google"`，回传 thought_signatures（按 tool_call_id 索引）
- ExtraMetadata 加 `thought_signatures` / `text_thought_signature` 字段持久化
- signature 过期/缺失兜底：清空重试一次（与 Claude/GPT 同款路径）
- 跨族兜底：塞 `skip_thought_signature_validator` 绕过 Gemini 3 校验

**阶段 4：测试（P5）**

- SSE 录制回放：parseSSE（含 thought_signature 解析）/ anthropicParseStream
- 跨族切换降级测试
- 回归测试：现有 7 个 provider 行为不变

## 6. 风险与边界条件

### 6.1 跨族切换加密 token 丢失

**场景**：用户用 GPT 生成几轮消息（含 reasoning_details.encrypted_content），切换到 Claude 继续对话

**行为**（v2 修正，按 §2.3 矩阵对 Claude 不塞明文 thinking）：
- Goink 把历史消息回传给 Claude
- reasoning_details 无法转成 signature，**丢弃**
- **不塞明文 thinking block**（Claude 不认无 signature 的 thinking block，塞了也会被忽略）
- Claude 看不到 GPT 的推理上下文，重新开始推理
- 前端仍能看到 GPT 的历史思考文本（前端从消息持久化的 ThinkingContent 字段渲染，与回传 LLM 的内容独立）

**应对**：静默处理，不在 UI 提示。这是行业硬伤。

### 6.2 中转站校验差异

**场景**：用户用 `vectorengine.cn` 中转站调 DeepSeek，历史消息含 `reasoning_content=""`

**行为**：
- DeepSeek V4 官方协议要求 tool_calls 场景 `reasoning_content` 必须回传（见 §1.2），官方后端宽松（接受空字符串）
- vectorengine.cn 中转站按协议文档措辞加严校验，要求非空 → 报 400 `The reasoning_content in the thinking mode must be passed back to the API`

**应对**（v2 修正，原"清空该字段重试"在 DeepSeek V4 场景会更失败）：
- **静默降级：塞非空内容重试一次**（源 thinking 明文或占位符如 `" "`）。**不能清空**——DeepSeek V4 协议要求字段存在，清空了更不行
- 推荐用户用 DeepSeek 官方 API（`https://api.deepseek.com`，官方后端宽松）
- 不为中转站做特殊兼容（中转站行为不可预测）

### 6.3 ExtraMetadata 向后兼容

**场景**：v1.3.0 之前生成的消息 ExtraMetadata 没有 reasoning_details / thinking_blocks 字段

**行为**：
- ToAPIFormat 透传 ExtraMetadata，缺失字段不报错
- 回传时缺失字段就跳过，不强制补
- GPT 适配后，新消息会有 reasoning_details，老消息没有，跨模型切换时老消息的推理上下文丢失

**应对**：可接受（老消息本来就没有加密 token，无法重建）

### 6.4 Anthropic tool 结果消息转换

**场景**：OpenAI 用 `role:"tool"` 消息承载工具结果，Anthropic 用 `role:"user"` + `tool_result` block

**风险**：转换时可能破坏消息顺序（user 消息合并？tool_result 单独成消息？）

**应对**：参考 LiteLLM 的转换逻辑，按 tool_call_id 分组，每个 tool_use 对应一个 tool_result。多个 tool_result 合并到同一个 user 消息的多个 block。

### 6.5 Claude signature 过期

**场景**：Claude thinking_blocks 的 signature 有时效性（几分钟到几小时），过期后回传会 400

**应对**：
- agent 层捕获 400，清空 thinking_blocks 重试一次
- 重试仍失败则发 EventError
- **必须做**，否则 Claude thinking 跨轮必撞 400

### 6.6 GPT reasoning.state 过期

**场景**：同 6.5，GPT 的加密 token 也有时效性

**应对**：agent 层捕获 400，清空 reasoning_details 重试一次。

### 6.7 GenerateText 适配（遗漏补漏）

**现状**：[generate.go:77-90](file:///home/nianhe/projects/todo/internal/llm/generate.go#L77-L90) 的 GenerateText 也只解析 OpenAI choices 格式，初稿完全没提它。

**应对**：
- 确认 GenerateText 的调用方（compress / title 生成等）
- Claude 适配时，GenerateText 也要走 anthropicParseStream 或单独的非流式解析
- 若 GenerateText 仅用于国产族场景，可暂不适配 Claude

### 6.8 Gemini thought_signature 过期/缺失

**场景**：Gemini 3 在 function calling 场景强制校验 `thought_signature`，缺失或过期会撞 400 "Function call FC1 in the 1. content block is missing a thought_signature"

**子场景**：
- 同族（Gemini → Gemini）：signature 有时效性，过期后回传会 400
- 跨族（任意 → Gemini）：历史 functionCall part 没有 Gemini signature，必撞 400

**应对**：
- 同族过期：agent 层捕获 400，清空 `thought_signatures` 重试一次（与 Claude/GPT 同款路径）
- 跨族缺失：ToAPIFormat 时塞特殊字符串 `"skip_thought_signature_validator"` 到 `thought_signature` 字段，绕过 Gemini 3 校验（dotnet-genai 代码揭示的官方逃生口）
- **必须做**，否则 Gemini 跨轮 thinking + 跨族切换必撞 400

### 6.9 中转站过滤 extra_content 非标字段

**场景**：用户用国内中转站调 Gemini 兼容端点，中转站只透传 OpenAI 标准字段，把 `extra_content.google.thought_signature` 这种非标扩展字段过滤掉

**行为**：
- 兼容端点吐的 signature 在中转链路中丢失
- Goink 持久化的 thought_signatures 为空
- 跨轮 thinking 失效，Gemini 每轮重新推理

**应对**：
- 检测 signature 持续为空时，前端给用户提示"当前中转站不支持 Gemini thinking 跨轮，建议换官方端点或支持透传的中转站"
- 不为中转站做特殊兼容（中转站行为不可预测，与 §6.2 同款原则）
- 推荐用户用 Google 官方兼容端点（需梯子）

### 6.10 Gemini function calling 跨族缺 thought_signature（v2 新增）

**场景**：跨族切换到 Gemini（如 Claude → Gemini / GPT → Gemini / 国产 → Gemini），且历史消息含 functionCall part

**行为**：
- Gemini 3 在 function calling 场景强制校验 `thought_signature`，跨族历史 functionCall part 没有 Gemini signature
- 必撞 400 "Function call FC1 in the 1. content block is missing a thought_signature"
- 不像 GPT/Claude 那样"忽略他族 thinking 就行"，Gemini function calling 是硬校验，不绕过就 400

**应对**：
- ToAPIFormat（dstProvider=Gemini + 含 functionCall 历史）时，对每个缺 signature 的 functionCall part 塞特殊字符串 `"skip_thought_signature_validator"` 到 `thought_signature` 字段
- 这是 dotnet-genai 代码揭示的官方逃生口，Google 后端识别该字符串后跳过校验
- **必须做**，否则跨族切到 Gemini（含 tool_calls 历史）必撞 400，对话卡死

**与 §6.8 的区别**：§6.8 处理同族内 signature 过期（清空重试）；本节处理跨族缺 signature（塞占位符绕过）。

### 6.11 上下文窗口超限（v2 新增）

**场景**：用户在 DeepSeek V4（1M 上下文）会话聊到 600K tokens，切换到 GPT（200K 上下文）或 Claude（200K 上下文）继续对话

**行为**：
- Goink 切换模型 = 下次发消息传不同 ProviderName + ModelID，无预检
- 历史消息 600K tokens 回传给 200K 模型，撞 400 "context length exceeded" 或被中转站截断
- 用户可能误以为是模型 bug，实际是上下文窗口超限

**应对**：
- 新增 `CanSwitchToModel` binding（见 §3.5），前端 `handleSelectModel` 预检
- inputBudget = ContextWindow - MaxOutputTokens（比 Goink 现有 compress 阈值更保守）
- 超窗时自动压缩 / 截断 / 开新会话 / 选更大模型（推荐自动压缩 + 提示）
- 复用现有 `CountMessagesTokens`（o200k_base BPE，所有 provider 共用）/ `ProviderModel` / `loadAPIMessages`

**风险**：
- token 计数用 `o200k_base` 编码器，对非 GPT 模型有偏差（DeepSeek / Claude 各自 tokenizer 不同），但误差在 10% 内，预检够用
- compress 当前不能指定目标 token 数，需扩展（见 §6.12）

### 6.12 compress 阈值未扣 MaxOutputTokens（v2 新增，Goink 潜在 bug）

**场景**：Goink 现有 [`internal/agent/agent.go:194`](file:///home/nianhe/projects/todo/internal/agent/agent.go#L194) 的 compress 阈值用 `ContextWindow * 0.8`，没扣 `MaxOutputTokens`

**行为**：
- 例如 GPT-5 ContextWindow=200K，MaxOutputTokens=32K，实际可用输入预算 = 168K
- Goink 现有 compress 阈值 = 200K * 0.8 = 160K（未扣 MaxOutputTokens）
- 修正后 compress 阈值应为 = 168K * 0.8 = 134.4K
- 问题场景：会话 token 数在 (160K, 168K] 区间时，按现有 0.8 阈值（160K）算**未触发压缩**，但实际输入 168K + 输出 32K = 200K 已顶满 ContextWindow，留给输出的空间不足，可能撞长度上限或输出被截断
- 更糟的情况：若 MaxOutputTokens 占比更大（如 Claude thinking 模式 budget_tokens 占一半），现有阈值的偏差更严重

**应对**：
- 修正 compress 阈值：`triggerThreshold = (ContextWindow - MaxOutputTokens) * 0.8`
- 与 §3.5 的 `CanSwitchToModel` 用同一个 inputBudget 公式，保持一致
- 阶段 1 修复（见 §5.2）

**与 §6.11 的关系**：§6.11 是切换模型时的预检（一次性），本节是会话进行中的压缩阈值（持续）。两者都应基于 `ContextWindow - MaxOutputTokens`，而非 `ContextWindow`。

## 7. 测试策略

### 7.1 测试不需要国外卡

**核心洞察**：要测的是 Goink 的协议适配代码，不是 API 本身通不通。所以不需要真卡。

### 7.2 测试方案（按性价比排）

| 方案 | 覆盖范围 | 成本 |
|---|---|---|
| **SSE 录制回放（主方案）** | parseSSE（含 thought_signature 解析）/ anthropicParseStream 的协议解析 | 0 |
| **本地假 SSE 服务器** | 流中断 / 超时 / 畸形 JSON 等容错路径 | 0 |
| **Gemini 免费额度实测** | Gemini 兼容端点 thought_signature 字段 + 跨轮 thinking | 0（Google 账号即可） |
| **OpenRouter 免费模型** | 真实 GPT reasoning 字段流式 | 邮箱注册 + 支付宝 |
| **国内中转站** | OpenAI 兼容路径冒烟 | 现有 |

**SSE 录制回放数据来源**（免费）：
- Anthropic / OpenAI / Gemini 官方文档的 SSE 示例段
- `anthropic-sdk-python` / `openai-python` / `google-genai` SDK 的 test fixtures
- LiteLLM / Vercel AI SDK 的 mock 响应
- `simonw/research-llm-apis` 的完整事件序列

### 7.3 单元测试项

| 测试项 | 覆盖范围 |
|---|---|
| `TestStreamParse_OpenAIReasoning` | GPT 族：`delta.reasoning` 解析 |
| `TestStreamParse_OpenAIReasoningDetails` | GPT 族：`reasoning_details` 加密 token 持久化 |
| `TestToAPIFormat_Anthropic` | Claude 族：消息格式转换（OpenAI ↔ Anthropic） |
| `TestParseStream_Anthropic` | Claude 族：流式解析（命名事件 + thinking signature） |
| `TestToAPIFormat_Gemini` | Gemini 族：thought_signature 回传（按 tool_call_id 索引，兼容端点路径） |
| `TestStreamParse_GeminiThoughtSignature` | Gemini 族：`extra_content.google.thought_signature` 解析 + 持久化 |
| `TestToAPIFormat_ExtraMetadataPassthrough` | ExtraMetadata 透传不丢失 |
| `TestCrossModel_ReasoningDetailsLost` | 跨族切换加密 token 丢失（文档化行为） |
| `TestExpiryFallback_ClaudeSignature` | Claude signature 过期清空重试 |
| `TestExpiryFallback_GPTState` | GPT reasoning.state 过期清空重试 |
| `TestExpiryFallback_GeminiSignature` | Gemini thought_signature 过期/缺失清空重试 |
| `TestCrossProvider_GeminiSkipValidator` | 跨族切到 Gemini 时塞 `skip_thought_signature_validator` 绕过 400 |
| `TestMidRelay_EmptyReasoningFallback` | 中转站空 reasoning_content 校验降级（Bug C 路径，v2 修正：塞非空重试而非清空） |
| `TestMidRelay_ExtraContentFiltered` | 中转站过滤 extra_content 非标字段降级 |
| `TestCanSwitchToModel_WithinWindow`（v2 新增） | CanSwitchToModel：当前会话 token 数 < inputBudget 时返回 Allowed=true |
| `TestCanSwitchToModel_ExceedWindow`（v2 新增） | CanSwitchToModel：当前会话 token 数 > inputBudget 时返回 Allowed=false + Reason |
| `TestToAPIFormat_CrossFamilyNoReasoningForGPT`（v2 新增） | 跨族切到 GPT 时不塞明文 reasoning（验证 §2.3 矩阵：GPT 不认明文） |
| `TestToAPIFormat_CrossFamilyNoReasoningForClaude`（v2 新增） | 跨族切到 Claude 时不塞明文 thinking block（验证 §2.3 矩阵：Claude 不认无 signature） |
| `TestToAPIFormat_CrossFamilyReasoningForDeepSeek`（v2 新增） | 跨族切到 DeepSeek V4（含 tool_calls）时塞源 thinking 明文到 reasoning_content（非空，验证 §2.3 矩阵 + DeepSeek V4 协议要求） |
| `TestToAPIFormat_CrossFamilyGeminiSkipValidator`（v2 新增） | 跨族切到 Gemini（function calling）时塞 skip_thought_signature_validator 占位 |
| `TestCompress_ThresholdSubtractsMaxOutputTokens`（v2 新增） | compress 阈值修正：(ContextWindow - MaxOutputTokens) * 0.8，而非 ContextWindow * 0.8（验证 §6.12 bug 修复） |

### 7.4 集成测试

| 测试项 | 覆盖范围 |
|---|---|
| GPT 模型对话 + 工具调用 | 端到端验证 Bug C 修复 |
| Claude 模型对话 + 工具调用 + thinking_blocks | 端到端验证 Claude 适配 |
| Gemini 模型对话 + 工具调用 | 端到端验证 Gemini 适配 |
| 跨族切换（GPT → DeepSeek → Claude → Gemini） | 验证加密 token 丢失行为符合预期 + 静默降级 + skip_thought_signature_validator 兜底 |

### 7.5 回归测试

现有 7 个国产族 provider 行为必须不变：
- `go test ./internal/llm/...`
- `go test ./app/...`
- `go test ./internal/session/...`

### 7.6 现有测试缺口

[stream_test.go](file:///home/nianhe/projects/todo/internal/llm/stream_test.go) 当前只测了 parseRetryAfter / statusRetryable / parseDefaultError，**parseSSE 本身零覆盖**。阶段 1 应先建 SSE mock/回放测试基础设施。

## 8. 决策记录

### 8.1 为什么任意切换而非限制切换？

1. **产品决策优先**：用户看不懂加密 token 丢失，"不能切"会被判定软件垃圾
2. **竞品对齐**：Trae / OpenRouter / Cherry Studio 都允许任意切换
3. **行业硬伤**：跨 provider 加密 token 丢失是无解的，限制切换也救不回来
4. **差异化在别处**：Goink 的卖点是同族内 thinking 真保留，不是限制切换

### 8.2 为什么按"thinking 可互通性"分四族，而非按协议分三族？

1. **GPT 协议同构但 thinking 不通**：GPT 和 DeepSeek 同为 OpenAI Chat Completions，但 GPT 加密、DeepSeek 明文，按协议分族会把 GPT 错误塞进明文族
2. **用户核心诉求是 thinking 保留**：分类维度应贴合用户价值（思考可见 + 跨轮保留），而非实现细节（协议同构）
3. **族内行为一致**：同族内 thinking 互通规则一致，ToAPIFormat 按族分流更清晰

### 8.3 为什么 GPT 族工作量比初稿说的大？

1. **光解析明文只治标**：`delta.reasoning` 解析只让 ThinkingContent 不为空，但 GPT 跨轮 thinking 保留必须靠加密 token
2. **GPT 跨轮认加密 token**：请求侧 `reasoning` 字段不认明文，只认 `reasoning_details` / `reasoning.state`
3. **必须完整链路**：解析 → 持久化 → 回传 → 过期兜底，缺一不可
4. **reasoning summary 必须 opt-in**（v2 补充）：GPT 默认不返回 summary，必须请求时 opt-in `reasoning.summary: "auto"`，否则 GPT 消息没明文 thinking 可塞给 DeepSeek V4 等需要明文 `reasoning_content` 的目标 provider。这是初稿遗漏的工作量

### 8.4 为什么 Gemini 优先走兼容端点？

1. **省掉翻译层**：原生协议适配工作量大（role 体系、parts 结构、streamGenerateContent）
2. **Google 官方提供**：兼容端点是 Google 自己维护的，比第三方翻译可靠
3. **实测吐 signature 而非 reasoning_content**：兼容端点吐 `extra_content.google.thought_signature`，**不是零适配**，需要持久化 signature 才能跨轮；但比原生协议翻译层省事（~1.1 天 vs ~3.1 天）
4. **中转站友好**：兼容端点走 OpenAI 路径，国内中转站普遍代理；原生 v1beta 端点中转站基本不走
5. **复用现有 parseSSE**：兼容端点的 SSE 行格式与 OpenAI 同构，parseSSE 加 thought_signature 解析即可，无需独立 ParseStream hook

### 8.5 为什么不做 Responses API？

1. **工作量巨大**：Responses API 是有状态 API，需要完整重写会话管理
2. **Chat Completions 足够**：OpenAI 承诺无限期支持 Chat Completions
3. **本次目标**：解决 Bug C + 支持主流 provider，不追求 OpenAI 最新特性
4. **未来可选**：v1.4.0+ 可以考虑支持 Responses API

### 8.6 为什么不引入各厂商官方 SDK（anthropic-sdk-go / google.golang.org/genai）？

**决策**：v1.3.0 多 provider 适配**不引入** Anthropic / Google 官方 SDK，全部自研翻译层（Claude）+ 兼容端点解析（Gemini）。可借鉴 SDK 源码作为协议解析参考实现，但不拖入依赖。

**理由**：

1. **事件粒度不对齐**：SDK 给的是协议原生事件（Claude 的 `ContentBlockDeltaEvent`、Gemini 的 chunk），Goink 的 `StreamEvent`（`EventToolCallStart`/`EventToolCallEnd` 等）是业务抽象。agent 层依赖业务事件驱动 agent loop，**SDK 事件 → Goink 业务事件**这层翻译无论用不用 SDK 都要写。SDK 节省的只是底层 SSE 字节流解析（~150 行），净收益小。

2. **类型系统阻抗**：Goink 现有体系用 `map[string]any`（Provider hook、ToAPIFormat、buildPayload），SDK 用强类型 struct（`[]anthropic.MessageParam` / `[]genai.Content`）。引入 SDK 必须做 `map ↔ struct` 双向转换层（~1 天工作量），且每种消息角色 + 每种 content block 都要写转换代码。

3. **破坏 Provider 抽象一致性**：Goink 的 `Provider` 结构（`BuildRequest` / `BuildHeaders` / `ParseStream` / `ParseError`）假设"自己发 HTTP"。SDK 自管 HTTP（发请求 + 解析 + 重试），引入 SDK 后该 provider 走完全不同的代码路径，7 个国产 provider 走 hook、Claude/Gemini 走 SDK，心智成本高，未来加新 provider 时分裂加剧。

4. **依赖膨胀**：
   - anthropic-sdk-go 拖入 16 个传递依赖（AWS SDK for Bedrock、Google API for Vertex、gRPC、MCP SDK、OpenTelemetry）
   - google.golang.org/genai 拖入 35 个传递依赖（含 Google Cloud auth 系列库）
   - go.sum 膨胀几百 MB，CVE 扫描面变大，与 Goink 当前轻量依赖风格不符

5. **中转站生态不友好**：Gemini SDK 走 `v1beta/models/{model}:streamGenerateContent` 原生端点，国内中转站基本不代理这个路径，**用户用中转站走不通**。Goink 用户主要用国内中转站，这是硬约束。兼容端点 + 自研解析对中转站友好。

6. **工作量反而更大**：
   - Claude：自研 3.5 天 vs SDK 4.4 天（多出 map↔struct 转换 + 职责冲突处理 + 依赖审计 + 关 SDK 自动重试）
   - Gemini：兼容端点 1.1 天 vs SDK 3.8 天（多出依赖解决 + adapter + 中转站兼容 + GenerateText 适配）

7. **业界主流做法**：多 provider 适配框架（LiteLLM / Vercel AI SDK / LangChain / LobeChat model-runtime / one-api / OpenRouter）普遍**不引入各厂商 SDK**，自研统一 HTTP 层 + 按 provider 翻译。引入多个 SDK = N 套独立代码路径 + 依赖冲突。Goink 走派别 A 符合业界主流。

8. **SDK 自动重试干扰**：SDK 默认重试 429/5xx 2 次，与 Goink agent 层的 signature 过期清空重试逻辑叠加。需手动 `WithMaxRetries(0)` 关闭，否则重试行为不可控。

**例外**：如果未来 Goink 要支持 Anthropic Bedrock / Vertex 部署，或集成 MCP 协议，那时再考虑引入 SDK 的对应子包（`bedrock/` / `vertex/` / MCP 工具）。当前 v1.3.0 目标只是消息 API + thinking + tool_use，不需要。

**借鉴 SDK 源码**：虽然不引入 SDK 依赖，但自研翻译层时可参考 SDK 源码作为协议解析的最佳参考实现：
- `anthropic-sdk-go/packages/ssestream`：SSE 解析逻辑（Go 写的，可直接移植）
- `anthropic-sdk-go/message.go`：`Accumulate` 累积 signature / input_json 的逻辑
- `google-genai-go/models.go`：`GenerateContentStream` 实现参考

这样既能省踩坑，又不用拖入依赖。

## 9. 后续工作

- v1.3.0 完成后，评估是否支持 OpenAI Responses API
- 评估是否支持 Anthropic 的 adaptive thinking（Opus 4.6+，`thinking.type:"adaptive"` + `output_config.effort`）
- 评估是否支持 Gemini 的 thinking budget / thinking_level 精细控制（兼容端点是否支持需实测）
- 评估是否引入 Gemini Interactions API 有状态模式（`store:true` + `previous_interaction_id`，省 signature 管理；当前 Go SDK 未暴露，待官方支持后评估）
- 评估 `skip_thought_signature_validator` 在 Gemini 3 所有版本是否都生效（dotnet-genai 代码引用，官方文档未明确，需实测）
- 监控中转站兼容性（尤其 `extra_content` 非标字段透传情况），按需更新文档
- 考虑 Goink 差异化：同族内 thinking 真保留的 UI 提示（"切换到同族模型，推理上下文完整保留"）
