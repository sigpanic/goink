# v1.3.0 — 多 Provider 协议适配（GPT / Anthropic / Gemini）

## 背景与动机

### v1.2.0 issue #26 Bug C 根因

用户 log（`/tmp/snorlax_goink.log`）显示：

- message[74] 用 `xiangliang/gpt-5.3-codex`（GPT 中转）生成
- GPT 模型返回 `reasoning` 字段（OpenAI 标准），不返回 `reasoning_content`（DeepSeek 扩展）
- Goink 的 [stream.go:316](file:///home/nianhe/projects/todo/internal/llm/stream.go#L316) **只解析 `reasoning_content`，不解析 `reasoning`** → `ThinkingContent=""`
- 用户切换到 `向量引擎/deepseek-v4-flash`（DeepSeek 中转），Goink 把历史消息回传
- [types.go:79-81](file:///home/nianhe/projects/todo/internal/session/types.go#L79-L81) 在 `ThinkingContent=""` 且含 `tool_calls` 时设 `payload["reasoning_content"]=""`
- **DeepSeek 官方接受空字符串**，但中转站 `vectorengine.cn` 自己加了更严格的 schema 校验（Pydantic "required" 语义），要求非空 → 报 400

**根本问题**：Goink 只支持 OpenAI Chat Completions + DeepSeek 协议同构的字段集，不支持 OpenAI 的 `reasoning` 字段，更不支持 Anthropic / Gemini 的异构协议。

### 业界调研结论

| 框架 | 统一抽象 | 加密 token 处理 | 跨模型切换 |
|---|---|---|---|
| LangChain | ❌ 拒绝基类统一，要求 provider-specific 子类 | 各自处理 | 不处理 |
| LiteLLM | ✅ 统一为 `message.reasoning_content` 字符串 | 透传 | 自动转换（有损） |
| OpenAI SDK | N/A | 客户端必须原样回传 `reasoning.encrypted_content` | N/A |
| Anthropic SDK | N/A | `thinking_blocks[].signature` 必须回传 | N/A |
| Vercel AI SDK | ✅ 流式 `reasoning-delta` + `providerOptions` 下钻 | 保留加密 payload | provider 各自处理 |

**业界共识**：
1. 加密 token（OpenAI `encrypted_content` / Anthropic `signature`）必须原样回传，客户端无法解密
2. 跨 provider 切换时加密 token **必然丢失**，协议根本不兼容，无解
3. 业界主流是"provider-specific 适配层"而非"基类统一"
4. vLLM 已从 `reasoning_content` 改名为 `reasoning`，向 OpenAI 字段名靠拢

**Goink 的 hook 机制（hooks_minimax.go / hooks_qwen.go / hooks_moonshot.go）是业界主流做法**，与 LobeChat 的 `model-runtime` 类似。当前缺失：GPT / Anthropic / Gemini 的 hook。

## 设计目标

1. **解决 Bug C**：GPT 模型生成的消息不再出现 `ThinkingContent=""` 脏数据
2. **支持 OpenAI GPT 系列**：解析 `reasoning` 字段 + `reasoning_details` 加密 token 持久化
3. **支持 Anthropic Claude**：完整适配 Messages API（消息结构、流式事件、thinking_blocks）
4. **支持 Google Gemini**：完整适配 Generate Content API（contents/parts、流式 SSE）
5. **保持向后兼容**：不破坏现有 DeepSeek / Qwen / MiniMax / Moonshot / Doubao / GLM / MiMo 的行为
6. **职责分离**：session 包不依赖 llm 包的 provider 细节
7. **可扩展**：新增 provider 只改 llm 包，不改 session 包

**不在本次范围**：
- 跨模型切换时加密 token 重建（协议根本不兼容，业界无解，文档说明即可）
- 中转站校验差异处理（推荐用户用官方 API，文档说明 vectorengine.cn 等小众中转站的额外校验）
- Responses API 支持（OpenAI 推荐但工作量巨大，本次只支持 Chat Completions）

## 架构设计

### 三层适配模型

```
┌─────────────────────────────────────────────────────────────┐
│  请求层（出站）                                              │
│  - ToAPIFormat(dstProvider): 单条消息格式转换（改造，翻译）  │
│    - OpenAI/DeepSeek: 透传（不变）                           │
│    - Anthropic: {role,content} → {role,content:[{type,...}]}│
│    - Gemini: {role,content} → {role,parts:[{text,...}]}     │
│  - BuildRequest: 顶层请求结构转换（已有+语义扩展，Hook）     │
│  - BuildHeaders: 改造请求头（已有，Hook）                    │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│  响应层（入站）                                              │
│  - ParseStream: 解析流式响应（新增，翻译，替代硬编码 parseSSE）│
│    - OpenAI/DeepSeek: SSE data: {choices:[{delta:{...}}]}    │
│    - Anthropic: SSE event: message_start/content_block_delta │
│    - Gemini: SSE data: {candidates:[{content:{parts:[...]}}]}│
│  - ParseError: 解析非标准错误响应（已有，Hook）              │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│  持久化层（存储/回传）                                       │
│  - Message.Provider: 标记每条消息由哪个 provider 生成       │
│  - ExtraMetadata: 通用扩展槽（tool_calls / 加密 token 等）  │
└─────────────────────────────────────────────────────────────┘
```

### Hook vs 翻译的边界

| 适配场景 | 类型 | 修改范围 | 说明 |
|---|---|---|---|
| 请求参数转换（如 thinking→enable_thinking） | Hook | 函数级 | 流程主体不变，局部改字段 |
| 请求头转换（如 MiMo 鉴权） | Hook | 函数级 | 流程主体不变 |
| 错误响应解析 | Hook | 函数级 | 流程主体不变 |
| **GPT reasoning 字段解析** | Hook | 函数级（stream.go 加几行） | 协议同构，只加字段 |
| **Anthropic 消息格式转换** | 翻译 | ToAPIFormat("anthropic") | 协议异构，消息结构不同 |
| **Anthropic 流式解析** | 翻译 | 整个 ParseStream | 协议异构，SSE 事件类型不同 |
| **Gemini 消息格式转换** | 翻译 | ToAPIFormat("gemini") | 协议异构 |
| **Gemini 流式解析** | 翻译 | 整个 ParseStream | 协议异构 |

**判定原则**：协议同构用 Hook，协议异构用翻译。

### ToAPIFormat 改造方案（核心决策）

**决策**：ToAPIFormat 加 `dstProvider` 参数，根据 `srcProvider`（消息自身的 Provider 字段）和 `dstProvider`（目标 provider）直接转换格式。**不使用 BuildMessages hook**。

**理由**（推翻之前"不改"的决策）：
1. **逻辑集中**：转换逻辑在一个函数，不用分散到 ToAPIFormat + BuildMessages 两个地方
2. **性能更好**：少一层转换
3. **语义清晰**：ToAPIFormat 本来就是"转 API 格式"，根据目标 provider 转换是合理的
4. **调试容易**：一个问题在一个地方找
5. **业界先例**：LangChain 的 provider-specific Chat 类也是把转换逻辑放在消息层
6. **职责分离担心过度**：session 包本来就知道 OpenAI 格式（ToAPIFormat 现在就硬编码），加 dstProvider 参数并不违反职责分离

**Message 表新增 Provider 字段**：

```go
type Message struct {
    // ... 现有字段 ...
    Provider string `gorm:"column:provider;not null;default:''" json:"provider"` // 生成这条消息的 provider 名
}
```

- 持久化时：从当前 provider 设置 `msg.Provider = providerName`
- ToAPIFormat 时：读 `m.Provider` 判断 srcProvider
- DB migration：加字段，历史消息默认空字符串（视为 DeepSeek/OpenAI 兼容）

**ToAPIFormat 新签名**（保留现有 role 处理逻辑，新增 dstProvider 参数）：

```go
func (m *Message) ToAPIFormat(dstProvider string) map[string]any {
    srcProvider := m.Provider
    payload := map[string]any{
        "role":    m.Role,
        "content": m.Content,
    }

    // 解析 ExtraMetadata（通用扩展槽，存 tool_calls / tool_call_id / tool_name / 加密 token 等）
    var meta map[string]any
    if m.ExtraMetadata != "" {
        json.Unmarshal([]byte(m.ExtraMetadata), &meta)
    }

    if m.Role == "assistant" {
        // 1. 明文 thinking：跨 provider 都传，根据 dstProvider 塞到对应字段
        if m.ThinkingContent != "" {
            switch dstProvider {
            case "openai":
                payload["reasoning"] = m.ThinkingContent
            case "anthropic":
                // 转成 content 数组里的 thinking block
                payload["content"] = []map[string]any{
                    {"type": "thinking", "thinking": m.ThinkingContent},
                    {"type": "text", "text": m.Content},
                }
            default:
                // DeepSeek/Qwen 等用 reasoning_content
                payload["reasoning_content"] = m.ThinkingContent
            }
        }

        // 2. 加密 token：只当 srcProvider == dstProvider 时才回传（同 provider 内跨轮保留）
        if srcProvider == dstProvider && meta != nil {
            if encrypted, ok := meta[dstProvider]; ok {
                switch dstProvider {
                case "openai":
                    payload["reasoning_details"] = encrypted
                case "anthropic":
                    // 塞到 thinking block 的 signature 字段
                    // ...
                }
            }
        }

        // 3. tool_calls：根据 dstProvider 转换格式
        if tc, ok := meta["tool_calls"]; ok {
            switch dstProvider {
            case "anthropic":
                // 转成 tool_use block（在 content 数组里）
            case "gemini":
                // 转成 functionCall part
            default:
                // OpenAI/DeepSeek 格式：{id,type:function,function:{name,arguments}}
                payload["tool_calls"] = tc
            }
        }
    }

    if m.Role == "tool" {
        // tool 结果：根据 dstProvider 转换
        switch dstProvider {
        case "anthropic":
            // 转成 user 消息的 tool_result block
            payload["role"] = "user"
            payload["content"] = []map[string]any{
                {"type": "tool_result", "tool_use_id": meta["tool_call_id"], "content": m.Content},
            }
        case "gemini":
            // 转成 function 消息的 functionResponse part
            payload["role"] = "function"
            payload["content"] = map[string]any{
                "name":     meta["tool_name"],
                "response": m.Content,
            }
        default:
            // OpenAI/DeepSeek 格式
            payload["tool_call_id"] = meta["tool_call_id"]
            payload["name"] = meta["tool_name"]
        }
    }

    return payload
}
```

**保留现有 role 处理**：
- assistant：拼 reasoning + 加密 token + tool_calls
- tool：拼 tool_call_id + name（OpenAI 格式）或转成 tool_result block（Anthropic）或 functionResponse（Gemini）
- user/system：透传 role + content

**关键设计原则**：

1. **ThinkingContent（明文）跨 provider 都传**：根据 dstProvider 塞到对应字段（reasoning_content / reasoning / thinking block）
2. **加密 token 只在同 provider 内回传**：`srcProvider == dstProvider` 时才从 ExtraMetadata 取对应命名空间的加密 token
3. **跨 provider 切换自然丢失加密 token**：只传明文 thinking，不传加密 token（符合业界做法）
4. **ExtraMetadata 永不删除**：DB 里保留所有 provider 的字段，给"切回来还能用"的机会
5. **不需要 BuildMessages hook**：被 ToAPIFormat 取代

**ExtraMetadata 是通用扩展槽**（不只存加密 token）：

```json
// DeepSeek 生成的 assistant 消息（含 tool_calls）
{
  "tool_calls": [
    {"id":"call_a","type":"function","function":{"name":"read","arguments":"{}"}}
  ]
  // 无加密 token（DeepSeek 没有）
}

// GPT 生成的 assistant 消息（含 tool_calls + 加密 token）
{
  "tool_calls": [...],
  "openai": {
    "reasoning_details": [{...encrypted...}]
  }
}

// Claude 生成的 assistant 消息（含 tool_calls + 加密 token）
{
  "tool_calls": [...],  // 格式可能不同，需要 ToAPIFormat 转换
  "anthropic": {
    "thinking_blocks": [{...signature...}]
  }
}

// tool 消息（所有 provider 通用）
{
  "tool_call_id": "call_xxx",
  "tool_name": "read"
}
```

**ExtraMetadata 存储内容**：
- `tool_calls`：assistant 消息的工具调用数组（现有）
- `tool_call_id`：tool 消息的工具调用 ID（现有）
- `tool_name`：tool 消息的工具名称（现有）
- `display_text`：显示文本（现有，可选）
- `openai.reasoning_details`：GPT 的加密 token（新增）
- `anthropic.thinking_blocks`：Claude 的加密 token（新增）

**字段转换矩阵**：

| srcProvider → dstProvider | ThinkingContent（明文） | 加密 token | 行为 |
|---|---|---|---|
| deepseek → deepseek | 塞到 reasoning_content | N/A | 同 provider，原样 |
| openai → openai | 塞到 reasoning | ✅ reasoning_details | 同 provider，加密 token 保留 |
| anthropic → anthropic | 塞到 thinking block | ✅ thinking_blocks | 同 provider，加密 token 保留 |
| deepseek → openai | 塞到 reasoning | ❌ 不传 | 跨 provider，只传明文 |
| openai → deepseek | 塞到 reasoning_content | ❌ 不传 | 跨 provider，只传明文 |
| 任意 → gemini | 塞到 thought part | ❌ 不传 | Gemini 不支持跨轮 thinking |
| gemini → 任意 | 塞到对应字段 | N/A | Gemini 本来就没有加密 token |

**加密 token 时效性**：
- OpenAI `reasoning.encrypted` 和 Anthropic `signature` 都有时效性（几分钟到几小时）
- ExtraMetadata 永久保留，但切回原 provider 时加密 token 可能已过期
- 过期后 provider 会报错或忽略，Goink 捕获错误后清空对应字段重试（未来增强，v1.3.0 不做）

**调用方改动**：
- `app/chat.go` 的 `loadAPIMessages`：调用 `msg.ToAPIFormat(dstProvider)` 传入当前会话的 provider
- `internal/agent/compress.go` 的 `Compress`：同上
- 其他调用 ToAPIFormat 的地方：都需要传 dstProvider

## Provider 抽象扩展

### 现有 Provider 结构（[internal/llm/providers.go:12-23](file:///home/nianhe/projects/todo/internal/llm/providers.go#L12-L23)）

```go
type Provider struct {
    Name         string
    ChatURL      string
    APIKey       string
    PlatformURL  string
    HelpText     string
    Models       []ModelInfo
    Temperature  *float64
    BuildRequest func(payload map[string]any) map[string]any    // Hook
    BuildHeaders func(base map[string]string) map[string]string // Hook
    ParseError   func(body []byte) error                        // Hook
}
```

### 新增字段（仅 ParseStream，不需要 BuildMessages）

```go
type Provider struct {
    // ... 现有字段 ...

    // ParseStream 解析流式响应，把 provider 特有 SSE 翻译成 StreamEvent。
    // nil 表示用默认的 OpenAI SSE 解析器（parseSSE）。
    // Anthropic/Gemini 必须实现。
    ParseStream func(r io.Reader, ch chan<- StreamEvent) error
}
```

**不需要 BuildMessages**：消息格式转换由 `session.Message.ToAPIFormat(dstProvider)` 负责。
**不需要 ParseReasoningDetails**：加密 token 提取由 ParseStream 内部完成，直接存到 ExtraMetadata。

### 设计原则

1. **Hook 函数风格**：不引入 interface，符合 Goink 现有风格
2. **nil = 默认 OpenAI 行为**：现有 provider 不需要改，新 provider 按需实现
3. **三层职责划分**（正交，不重叠）：

| 层 | 函数 | 包 | 职责 | 改造程度 |
|---|---|---|---|---|
| **单条消息** | `ToAPIFormat(dstProvider)` | session | 出站消息格式转换（根据 srcProvider + dstProvider） | 现有 + 改造（加 dstProvider 参数 + role 处理保留） |
| **顶层结构** | `BuildRequest(payload)` | llm | 顶层请求结构转换（messages vs contents、system 字段、generationConfig 包装） | 现有 + **语义扩展**（从"局部改造"扩展为"完整请求体构建"） |
| **入站流** | `ParseStream(r, ch)` | llm | 入站流式响应解析（SSE → StreamEvent） | 新增 |

**三层职责的完整数据流**：

```
1. ToAPIFormat(dstProvider)：单条消息格式转换
   - 输入：session.Message + dstProvider
   - 输出：单条消息 map（目标 provider 的消息格式）
   - 职责：role/content/reasoning/tool_calls 的字段映射
   - 示例：{role:"assistant",content:"...",reasoning_content:"..."} → {role:"assistant",content:[{type:"text",text:...},{type:"thinking",thinking:...}]}

2. 组装 payload：把所有消息组装成 OpenAI 顶层结构
   - {messages:[...], model, temperature, tools, ...}

3. BuildRequest(payload)：顶层结构转换
   - OpenAI/DeepSeek/Qwen 等：nil（直接用 OpenAI 结构）或局部改造（如 thinking → enable_thinking）
   - Anthropic：提取 system 到独立字段、转换 tools 格式、补 max_tokens
   - Gemini：messages → contents、role 转换、generationConfig 包装

4. BuildHeaders：请求头改造（如 MiMo 鉴权）

5. 发送请求
```

**BuildRequest 的语义扩展**（重要）：
- **现有语义**：局部参数改造（如 Qwen 的 thinking → enable_thinking）
- **扩展后语义**：完整请求体构建（包括顶层结构转换）
- OpenAI/DeepSeek 等同构协议：BuildRequest 为 nil 或做局部改造
- Anthropic/Gemini 异构协议：BuildRequest 做完整顶层结构转换

**不需要 BuildMessages**：消息格式转换由 `session.Message.ToAPIFormat(dstProvider)` 负责，顶层结构由 `BuildRequest` 负责，两者正交。
**不需要 ParseReasoningDetails**：加密 token 提取由 ParseStream 内部完成，直接存到 ExtraMetadata。

## 各 Provider 适配方案

### 1. GPT 适配（Hook，最小改动）

**改动范围**：只改 [internal/llm/stream.go](file:///home/nianhe/projects/todo/internal/llm/stream.go)

**改动内容**：

在 [stream.go:316](file:///home/nianhe/projects/todo/internal/llm/stream.go#L316) 附近，`reasoning_content` 解析旁边加 `reasoning` 解析：

```go
// reasoning_content → EventThinking（DeepSeek / vLLM 旧版）
if reasoning, ok := delta["reasoning_content"].(string); ok && reasoning != "" {
    ch <- StreamEvent{Type: EventThinking, Data: reasoning}
}

// reasoning → EventThinking（OpenAI GPT-5 / o-series / vLLM 新版）
if reasoning, ok := delta["reasoning"].(string); ok && reasoning != "" {
    ch <- StreamEvent{Type: EventThinking, Data: reasoning}
}
```

**可选**（完整支持加密 token）：解析 `reasoning_details` 数组存到 ExtraMetadata，ToAPIFormat("openai") 回传时原样放回。**对 Bug C 不是必需的**，但能让 GPT 模型跨轮保留推理状态。

**provider 注册**：在 [providers.go](file:///home/nianhe/projects/todo/internal/llm/providers.go) 加 `"openai"` 内置 provider：

```go
"openai": {
    Name:        "OpenAI",
    ChatURL:     "https://api.openai.com/v1/chat/completions",
    PlatformURL: "https://platform.openai.com",
    HelpText:    "...",
    Models: []ModelInfo{
        {ID: "gpt-5.3-codex", ...},
        {ID: "gpt-5.2", ...},
        {ID: "o3", ...},
    },
    BuildRequest: nil, // GPT 用标准 OpenAI 格式
},
```

**风险**：无（只加字段解析，不破坏现有 provider）

### 2. Anthropic 适配（翻译，大改造）

**改动范围**：
- 新增 `internal/llm/hooks_anthropic.go`：实现 `anthropicBuildRequest`（顶层结构转换）+ `anthropicParseStream`（流式解析）
- 改 `internal/llm/stream.go`：把 parseSSE 抽象成可替换的 ParseStream
- 改 `internal/session/types.go`：ToAPIFormat 加 dstProvider 参数，支持 Anthropic 消息格式转换

**Anthropic Messages API 协议要点**：

| 维度 | OpenAI Chat Completions | Anthropic Messages |
|---|---|---|
| 端点 | `POST /v1/chat/completions` | `POST /v1/messages` |
| 顶层结构 | `{messages:[...], model, temperature, tools}` | `{system:..., messages:[...], max_tokens, model, tools}` |
| content 类型 | 字符串 | 数组 `[{type:"text",text:...}]` |
| thinking | `reasoning_content` 字符串 | `{type:"thinking",thinking:...,signature:...}` block |
| tool_calls | `{id,type:function,func:{name,arguments}}` | `{type:"tool_use",id,name,input}` |
| tool 结果 | `{role:"tool",content,tool_call_id}` | `{role:"user",content:[{type:"tool_result",tool_use_id,content}]}` |
| 流式事件 | `data: {choices:[{delta:{...}}]}` | `event: message_start/content_block_delta/...` |
| 加密 token | `reasoning_details[].reasoning.encrypted` | `thinking_blocks[].signature` |

**hooks_anthropic.go 实现**：

```go
// anthropicBuildRequest 把 OpenAI 顶层结构转换成 Anthropic 顶层结构
// 职责：提取 system、转换 tools 格式、补 max_tokens
// 注意：单条消息格式已由 ToAPIFormat("anthropic") 转换，这里只做顶层结构
func anthropicBuildRequest(payload map[string]any) map[string]any {
    // 1. 提取 system 消息（OpenAI 放在 messages[0]，Anthropic 单独字段）
    msgs := payload["messages"].([]map[string]any)
    if len(msgs) > 0 && msgs[0]["role"] == "system" {
        payload["system"] = msgs[0]["content"]
        payload["messages"] = msgs[1:]
    }

    // 2. tools 格式转换（OpenAI tools → Anthropic tools）
    if tools, ok := payload["tools"].([]any); ok {
        payload["tools"] = convertOpenAIToolsToAnthropic(tools)
    }

    // 3. max_tokens 必填（Anthropic 要求）
    if payload["max_tokens"] == nil {
        payload["max_tokens"] = 4096
    }

    return payload
}

// anthropicParseStream 解析 Anthropic 流式响应
func anthropicParseStream(r io.Reader, ch chan<- StreamEvent) error {
    // 解析 event: message_start / content_block_start / content_block_delta / content_block_stop / message_delta / message_stop
    // 转换成 StreamEvent（EventContent / EventThinking / EventToolCallStart / ...）
    // 提取 thinking_block.signature 存到 ExtraMetadata（通过 EventToolCallEnd 或新事件类型）
}
```

**加密 token 持久化**：
- anthropicParseStream 提取 `thinking_block.signature` 发出特殊 StreamEvent
- agent 层接收后存到 `ExtraMetadata.anthropic.thinking_blocks`
- ToAPIFormat("anthropic") 回传时从 ExtraMetadata 取，原样放回 thinking block 的 signature 字段

**风险**：
- 工作量大（顶层结构、流式事件、tool_calls、tool 结果全部不同）
- Anthropic 的 tool 结果放在 user 消息里，与 OpenAI 的 tool 角色不同，ToAPIFormat 转换要小心
- thinking_blocks 在 tool-call 场景必须回传，否则 400

### 3. Gemini 适配（翻译，大改造）

**改动范围**：
- 新增 `internal/llm/hooks_gemini.go`
- 复用 ParseStream 抽象

**Gemini Generate Content API 协议要点**：

| 维度 | OpenAI Chat Completions | Gemini Generate Content |
|---|---|---|
| 端点 | `POST /v1/chat/completions` | `POST /v1beta/models/{model}:streamGenerateContent` |
| 请求结构 | `{messages:[{role,content}]}` | `{contents:[{role,parts:[{text}]}],systemInstruction:...}` |
| role | `user`/`assistant`/`system`/`tool` | `user`/`model`/`function` |
| content 类型 | 字符串 | parts 数组 `[{text:...}]` |
| thinking | `reasoning_content` | `{thought:true,text:...}` part（Gemini 2.5+） |
| tool_calls | `{id,type:function,func:{name,arguments}}` | `{functionCall:{name,args}}` |
| tool 结果 | `{role:"tool",content,tool_call_id}` | `{role:"function",parts:[{functionResponse:{name,response}}]}` |
| 流式事件 | `data: {choices:[{delta:{...}}]}` | `data: {candidates:[{content:{parts:[...]}}]}` |
| 加密 token | 无 | 无（Gemini 不支持跨轮 thinking 保留） |

**hooks_gemini.go 实现**：

```go
// geminiBuildRequest 把 OpenAI 顶层结构转换成 Gemini 顶层结构
// 职责：messages → contents、提取 systemInstruction、generationConfig 包装
// 注意：单条消息格式已由 ToAPIFormat("gemini") 转换，这里只做顶层结构
func geminiBuildRequest(payload map[string]any) map[string]any {
    // 1. 提取 system 消息（OpenAI 放在 messages[0]，Gemini 放 systemInstruction）
    // 2. messages → contents（顶层字段改名）
    // 3. role 转换：assistant → model，tool → function
    // 4. temperature/max_tokens 等包装到 generationConfig
}

// geminiParseStream 解析 Gemini 流式响应
func geminiParseStream(r io.Reader, ch chan<- StreamEvent) error {
    // 解析 candidates[].content.parts[]
    // 转换成 StreamEvent（EventContent / EventThinking / EventToolCallStart / ...）
}
```

**风险**：
- Gemini 的 role 体系不同（user/model/function vs user/assistant/system/tool）
- Gemini 的 tool 结果用 functionResponse，需要把 OpenAI 的 tool_call_id 映射到 function name
- Gemini 2.5+ 的 thinking 是 `thought:true` 的 part，与 OpenAI/Anthropic 都不同

## 实施阶段

### 阶段 1：GPT 适配（v1.2.1 或 v1.3.0 子项，最小改动）

**目标**：解决 Bug C，让 GPT 模型生成的消息不再出现 `ThinkingContent=""` 脏数据

**改动**：
- `internal/llm/stream.go`：加 `delta.reasoning` 字符串解析（几行代码）
- `internal/llm/providers.go`：加 `"openai"` 内置 provider
- 可选：加 `delta.reasoning_details` 数组解析，存到 `ExtraMetadata.reasoning_details`

**测试**：
- 单元测试：mock GPT 流式响应，验证 `EventThinking` 事件正确触发
- 集成测试：用 GPT 模型生成消息，切换到 DeepSeek，验证不再触发 Bug C

**预估工作量**：小（1-2 个文件，几十行代码）

### 阶段 2：ParseStream 抽象（v1.3.0 基础设施）

**目标**：把 stream.go 的硬编码 parseSSE 抽象成 Provider.ParseStream hook

**改动**：
- `internal/llm/types.go`：Provider 加 `ParseStream` 字段
- `internal/llm/stream.go`：把 parseSSE 重命名为 `openAIParseStream`，作为默认实现
- `internal/llm/generate.go`：调用 `provider.ParseStream` 而非直接调 parseSSE

**测试**：
- 现有 provider 行为不变（回归测试）
- 新增 provider 可以注入自定义 ParseStream

**预估工作量**：中（重构，但不改行为）

### 阶段 3：Anthropic 适配（v1.3.0 主体）

**目标**：完整支持 Anthropic Claude 模型

**改动**：
- 新增 `internal/llm/hooks_anthropic.go`：实现 `anthropicBuildRequest`（顶层结构转换）/ `anthropicParseStream`（流式解析）/ `anthropicParseError`
- `internal/llm/providers.go`：加 `"anthropic"` 内置 provider
- `internal/session/types.go`：ToAPIFormat 加 dstProvider 参数，支持 Anthropic 消息格式转换
- `internal/llm/encrypt.go`：可能需要扩展（Anthropic signature 不同于 OpenAI encrypted_content）

**测试**：
- 单元测试：消息格式转换（OpenAI ↔ Anthropic），覆盖 ToAPIFormat("anthropic") + anthropicBuildRequest
- 集成测试：Claude 模型对话 + 工具调用 + thinking_blocks 持久化

**预估工作量**：大

### 阶段 4：Gemini 适配（v1.3.0 主体）

**目标**：完整支持 Google Gemini 模型

**改动**：
- 新增 `internal/llm/hooks_gemini.go`：实现 `geminiBuildRequest`（顶层结构转换）/ `geminiParseStream` / `geminiParseError`
- `internal/llm/providers.go`：加 `"google"` 内置 provider
- 复用 ToAPIFormat("gemini") + ParseStream 抽象

**测试**：
- 单元测试：消息格式转换（OpenAI ↔ Gemini），覆盖 ToAPIFormat("gemini") + geminiBuildRequest
- 集成测试：Gemini 模型对话 + 工具调用

**预估工作量**：大

## 风险与边界条件

### 1. 跨模型切换时加密 token 丢失

**场景**：用户用 GPT 生成几轮消息（含 `reasoning_details.encrypted`），切换到 DeepSeek 继续对话

**行为**：
- Goink 把历史消息回传给 DeepSeek
- `reasoning_details.encrypted` 无法转成 `reasoning_content`，**丢弃**
- DeepSeek 看不到 GPT 的推理上下文，重新开始推理

**应对**：
- 文档说明跨模型切换的协议根本不兼容
- UI 可加提示："切换模型会丢失推理上下文"
- 不做任何自动重建（业界无解）

### 2. 中转站校验差异

**场景**：用户用 `vectorengine.cn` 中转站调 DeepSeek，历史消息含 `reasoning_content=""`

**行为**：
- DeepSeek 官方接受空字符串
- vectorengine.cn 中转站自己加了 Pydantic "required" 校验，可能要求非空 → 报 400

**应对**：
- 文档说明中转站校验差异
- 推荐用户用 DeepSeek 官方 API（`https://api.deepseek.com`）
- 不为中转站做特殊兼容（中转站行为不可预测）

### 3. ExtraMetadata 向后兼容

**场景**：v1.3.0 之前生成的消息 `ExtraMetadata` 没有 `reasoning_details` / `thinking_blocks` 字段

**行为**：
- ToAPIFormat 透传 `ExtraMetadata`，缺失字段不报错
- ToAPIFormat 回传时缺失字段就跳过，不强制补
- GPT 适配后，新消息会有 `reasoning_details`，老消息没有，跨模型切换时老消息的推理上下文丢失

**应对**：可接受（老消息本来就没有加密 token，无法重建）

### 4. Anthropic tool 结果消息转换

**场景**：OpenAI 用 `role:"tool"` 消息承载工具结果，Anthropic 用 `role:"user"` + `tool_result` block

**风险**：转换时可能破坏消息顺序（user 消息合并？tool_result 单独成消息？）

**应对**：参考 LiteLLM 的转换逻辑，按 tool_call_id 分组，每个 tool_use 对应一个 tool_result

### 5. Gemini role 体系差异

**场景**：OpenAI 的 `assistant` 对应 Gemini 的 `model`，`tool` 对应 `function`

**风险**：role 转换错误会导致 Gemini 报 400

**应对**：单元测试覆盖所有 role 转换组合

## 测试策略

### 单元测试

| 测试项 | 覆盖范围 |
|---|---|
| `TestStreamParse_OpenAIReasoning` | GPT 适配：`delta.reasoning` 解析 |
| `TestStreamParse_OpenAIReasoningDetails` | GPT 适配：`reasoning_details` 数组持久化 |
| `TestToAPIFormat_Anthropic` | Anthropic 消息格式转换（OpenAI ↔ Anthropic） |
| `TestParseStream_Anthropic` | Anthropic 流式解析 |
| `TestToAPIFormat_Gemini` | Gemini 消息格式转换（OpenAI ↔ Gemini） |
| `TestParseStream_Gemini` | Gemini 流式解析 |
| `TestToAPIFormat_ExtraMetadataPassthrough` | ExtraMetadata 透传不丢失 |
| `TestCrossModel_ReasoningDetailsLost` | 跨模型切换加密 token 丢失（文档化行为） |

### 集成测试

| 测试项 | 覆盖范围 |
|---|---|
| GPT 模型对话 + 工具调用 | 端到端验证 Bug C 修复 |
| Claude 模型对话 + 工具调用 + thinking_blocks | 端到端验证 Anthropic 适配 |
| Gemini 模型对话 + 工具调用 | 端到端验证 Gemini 适配 |
| 跨模型切换（GPT → DeepSeek → Claude） | 验证加密 token 丢失行为符合预期 |

### 回归测试

现有 provider（DeepSeek / Qwen / MiniMax / Moonshot / Doubao / GLM / MiMo）的行为必须不变：
- `go test ./internal/llm/...`
- `go test ./app/...`
- `go test ./internal/session/...`

## 决策记录

### 为什么 ToAPIFormat 改造（推翻之前"不改"的决策）？

1. **逻辑集中**：转换逻辑在一个函数，不用分散到 ToAPIFormat + BuildMessages 两个地方
2. **性能更好**：少一层转换
3. **语义清晰**：ToAPIFormat 本来就是"转 API 格式"，根据目标 provider 转换是合理的
4. **业界先例**：LangChain 的 provider-specific Chat 类也是把转换逻辑放在消息层
5. **职责分离担心过度**：session 包本来就知道 OpenAI 格式（ToAPIFormat 现在就硬编码），加 dstProvider 参数并不违反职责分离

详见上文"ToAPIFormat 改造方案（核心决策）"章节。

### 为什么 GPT 适配用 Hook 而不是翻译？

1. **协议同构**：GPT 用的就是 OpenAI Chat Completions 格式
2. **改动最小**：只加 `reasoning` 字段解析
3. **风险最低**：不破坏现有 provider
4. **直接解决 Bug C**：让 ThinkingContent 不再为空

### 为什么 Anthropic/Gemini 用翻译？

1. **协议异构**：消息结构、流式事件、tool_calls 格式全不同
2. **Hook 不够**：局部改字段无法解决结构差异
3. **必须替换流程主体**：parseSSE 整个替换成 anthropicParseStream / geminiParseStream

### 为什么不做 Responses API？

1. **工作量巨大**：Responses API 是有状态 API，需要完整重写会话管理
2. **Chat Completions 足够**：OpenAI 承诺无限期支持 Chat Completions
3. **本次目标**：解决 Bug C + 支持主流 provider，不追求 OpenAI 最新特性
4. **未来可选**：v1.4.0+ 可以考虑支持 Responses API

## 后续工作

- v1.3.0 完成后，评估是否支持 OpenAI Responses API
- 评估是否支持 Anthropic 的 adaptive thinking（Opus 4.6+）
- 评估是否支持 Gemini 的 thinking budget 控制
- 监控中转站兼容性，按需更新文档
