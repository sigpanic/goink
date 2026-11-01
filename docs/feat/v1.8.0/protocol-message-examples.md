# LLM 协议消息速查：同一个工具调用如何组织

> 2026-10-16 修订。用最小示例理解协议，配合 [接入设计](./llm-protocol-design.md) 和 [实施路线](./commit-roadmap.md) 阅读。本版本顺序为公共适配层基建 → OpenAI Chat 适配器 → Claude Messages 适配器 → Gemini GenerateContent 适配器；支持同会话模型切换，允许丢弃不兼容推理状态。Interactions 仅作协议对照，留到后续。

## 1. Google 官方现在推荐哪个 API

官方当前推荐 **Interactions API** 用于新项目，GenerateContent 仍受支持，另外也提供 OpenAI Chat Completions 兼容接口。它们是不同请求格式，不能只替换 URL。[Interactions 概览](https://ai.google.dev/gemini-api/docs/interactions-overview)、[OpenAI 兼容接口](https://ai.google.dev/gemini-api/docs/openai)。

| 协议 | 典型 POST 路径 | 历史容器 | 系统提示 | 工具结果 |
|---|---|---|---|---|
| Chat Completions，Goink 已实现 | `/v1/chat/completions` | `messages` | `role: system` | `role: tool` + `tool_call_id` |
| Claude Messages | `/v1/messages` | `messages` | 顶层 `system` | user 内的 `tool_result` |
| Gemini GenerateContent | `/v1beta/models/{model}:generateContent` | `contents` | `systemInstruction` | part 内的 `functionResponse` |
| Gemini Interactions | `/v1beta/interactions` | `input`，支持 steps | `system_instruction` | `function_result` + `call_id` |

表中 Interactions 路径采用官网概览示例；具体 API 版本随选定 SDK 和目标端点核验。GenerateContent 流式调用改用 `:streamGenerateContent?alt=sse`，其他三种一般设置 `stream: true`。

## 2. 所有协议都完成同一件事

```text
用户：查询角色张三
  → 模型请求 get_characters({"search":"张三"})
  → Goink Registry 校验并执行工具
  → Goink 返回角色结果
  → 模型回答用户
```

工具名和参数来自现有 [get_characters](../../../internal/mcp_tools/character_tools.go)。下面 JSON 都表示“工具已执行，准备请求模型继续回答”；第一次请求只带系统提示、用户输入和工具定义，模型调用记录由首次响应提供。`<model>` 是占位符，schema / 工具结果做了简化，时间前缀、thinking 和签名暂不展示；生产回放必须保留 API 要求的原始推理状态。

### 2.1 Chat Completions：assistant 发起调用，tool 返回结果

```json
{
  "model": "<model>",
  "stream": true,
  "tools": [{
    "type": "function",
    "function": {
      "name": "get_characters",
      "description": "按名称查询角色",
      "parameters": {"type": "object", "properties": {"search": {"type": "string"}}}
    }
  }],
  "messages": [
    {"role": "system", "content": "你是网文写作助手。"},
    {"role": "user", "content": "查询角色张三"},
    {"role": "assistant", "content": "", "tool_calls": [{
      "id": "call_7",
      "type": "function",
      "function": {"name": "get_characters", "arguments": "{\"search\":\"张三\"}"}
    }]},
    {"role": "tool", "tool_call_id": "call_7",
     "content": "{\"success\":true,\"data\":{\"characters\":[{\"id\":7,\"name\":\"张三\"}]}}"}
  ]
}
```

- `tools` 是可用工具的定义；`assistant.tool_calls` 是模型本次具体要调用什么；`role=tool` 是程序执行后的结果。
- `arguments` 是 **JSON 字符串**；`tool_calls[].id` 与结果的 `tool_call_id` 一一对应。下一次请求带上调用和结果，不能只发送工具结果。
- Goink 当前由 [Registry.OpenAI](../../../internal/mcp_tools/base.go) 生成工具定义，[ToAPIFormat](../../../internal/session/types.go) 导出消息，[buildToolCalls / resultJSON](../../../internal/agent/safety.go) 组织调用和结果。当前还会附加 `reasoning_content`、tool 的 `name` 等字段，本文示例省略。

[OpenAI 官方消息定义](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)。

### 2.2 Claude：工具调用和结果放进 content 数组

```json
{
  "model": "<model>", "max_tokens": 4096, "stream": true,
  "system": "你是网文写作助手。",
  "tools": [{
    "name": "get_characters", "description": "按名称查询角色",
    "input_schema": {"type": "object", "properties": {"search": {"type": "string"}}}
  }],
  "messages": [
    {"role": "user", "content": "查询角色张三"},
    {"role": "assistant", "content": [{
      "type": "tool_use", "id": "call_7", "name": "get_characters",
      "input": {"search": "张三"}
    }]},
    {"role": "user", "content": [{
      "type": "tool_result", "tool_use_id": "call_7",
      "content": "{\"success\":true,\"data\":{\"characters\":[{\"id\":7,\"name\":\"张三\"}]}}"
    }]}
  ]
}
```

`input` 是对象。工具结果使用 user 下的 `tool_result`，紧接原 assistant 消息；并行调用的结果集中放在下一条 user 消息里。若响应还有 text / thinking / redacted_thinking，需保留其原始顺序及状态。[Messages](https://platform.claude.com/docs/en/api/messages/create)、[工具结果规则](https://platform.claude.com/docs/en/agents-and-tools/tool-use/handle-tool-calls)。

### 2.3 Gemini GenerateContent：model + parts

```json
{
  "systemInstruction": {"parts": [{"text": "你是网文写作助手。"}]},
  "tools": [{"functionDeclarations": [{
    "name": "get_characters", "description": "按名称查询角色",
    "parameters": {"type": "object", "properties": {"search": {"type": "string"}}}
  }]}],
  "contents": [
    {"role": "user", "parts": [{"text": "查询角色张三"}]},
    {"role": "model", "parts": [{"functionCall": {
      "name": "get_characters", "args": {"search": "张三"}
    }}]},
    {"role": "user", "parts": [{"functionResponse": {
      "name": "get_characters",
      "response": {"success": true, "data": {"characters": [{"id": 7, "name": "张三"}]}}
    }}]}
  ]
}
```

模型名放在 URL，assistant 对应 `model`；`args`、`response` 都是对象。这是单工具示例；实际响应有调用 ID 或 `thoughtSignature` 时也必须保存和回传，不能只复制 name / args。多个调用应按原始顺序与 ID / 位置配对。[GenerateContent 官方定义](https://ai.google.dev/api/generate-content)。

### 2.4 Gemini Interactions：使用有类型的 steps

```json
{
  "model": "<model>", "store": false, "stream": true,
  "system_instruction": "你是网文写作助手。",
  "tools": [{
    "type": "function", "name": "get_characters", "description": "按名称查询角色",
    "parameters": {"type": "object", "properties": {"search": {"type": "string"}}}
  }],
  "input": [
    {"type": "user_input", "content": [{"type": "text", "text": "查询角色张三"}]},
    {"type": "function_call", "id": "call_7", "name": "get_characters",
     "arguments": {"search": "张三"}},
    {"type": "function_result", "call_id": "call_7", "name": "get_characters",
     "result": {"success": true, "data": {"characters": [{"id": 7, "name": "张三"}]}}}
  ]
}
```

这里用 `store=false` 和全量 steps 展示 Goink 自己管理历史的方式。响应中读取 `steps`，普通回答对应 `model_output`；模型只返回生成的内容，客户端继续保留用户输入。若采用服务端状态，则使用 `previous_interaction_id` 加新的工具结果 / 用户输入，不需要重复发送旧历史；配置仍按每次请求要求提供。两种方式都需要保留完整原生响应状态。[Interactions 字段](https://ai.google.dev/api/interactions-api)、[会话方式](https://ai.google.dev/gemini-api/docs/interactions-overview)。

## 3. SSE 最后都转换成 Goink 的事件

SSE 是响应逐步到达的格式；上面的 messages / contents / input 是下一次请求需要的历史。不能把收到的每个片段直接当成一条完整历史消息。

| 协议 | 收到的工具信息 | Goink 适配器负责 |
|---|---|---|
| Chat Completions | `choices[].delta.tool_calls[]` 的 name / id / arguments | 按 index 累积参数字符串 |
| Claude | `content_block_start(tool_use)`、`input_json_delta` | 累积对应 block 的 input JSON |
| GenerateContent | `candidates[].content.parts[].functionCall` | 提取调用，保留 parts 和签名；不能假定总有参数增量 |
| Interactions | `step.start(function_call)`、`step.delta(arguments)`、`step.stop` | 按 step 累积调用并识别请求的完成 / 等待工具状态 |

最后统一产生 `EventToolCallStart / Arguments / End`；文本产生 `EventContent`，思考摘要产生 `EventThinking`，用量和错误各自转换。**一次响应完整收尾、工具参数全部校验通过后才发可执行的 End**，工具仍由 Goink 执行。适配器还输出内部 `EventResponseState` 保存来源、完成状态及可选原生内容；它在可执行的 End 之前到达。原生推理状态不从显示用文本反向猜。[Chat SSE](https://developers.openai.com/api/reference/resources/chat/subresources/completions/streaming-events)、[Interactions 流式示例](https://ai.google.dev/gemini-api/docs/migrate-to-interactions)。

数据库继续使用统一的 Role / Content / ThinkingContent 和工具 metadata；新增来源及原生状态保存到 `ExtraMetadata.llm_state`，无需按协议增加消息表。上面的 JSON 是各适配器出站生成的格式，Session 保持统一存储。OpenAI Chat 也作为适配器实现，内部复用现有 HTTP、SSE 和厂商 hooks。

三个实现分别封装在 `internal/llm/adapter/{openaichat,anthropic,gemini}` 子包，每个适配器持有自己的 providers，providers.go 和厂商 hooks / 规则归入各自子包。调用按 `adapterName → providerName → model` 选择：Client 按协议取适配器，Goink Request 携带 ProviderName、Model、messages / tools / options，适配器再构造实际 HTTP / SDK 请求。例如同名 gemini provider 可分别配置在 openai_chat 与 gemini_generate_content 下，按调用时明确指定的协议使用。

公共 `Stream` 返回 Goink 事件，`Generate` 返回统一完整结果；来源 / 原生状态在后续持久化单元补齐。GenerateText 提取正文，CallTool 共用流式包装，业务层执行工具。当前只做公共适配层基建，现有 OpenAI 草稿保持原状，不纳入当前暂存和验收。

## 4. 已取得的 SSE 有什么用，能否换成我们的工具

有用。现有 13 条 SSE / 108 帧涵盖文本、工具参数分片、工具后的回复等；Anthropic 还有含 thinking / signature 的官方构造 fixture。Google 部分来自官方录制的响应分段，重新封装为 SSE，不保留原始网络字节和时间。本轮只验证了 JSON / 参数 / 边界，没有跑 SDK → Goink 集成测试。

工具名由应用定义，不要求一定是原样本的 `get_weather` 或 `customDivide`。例如可将 Gemini 工具样本改写为下面的测试帧：

```text
data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"get_characters","args":{"search":"张三"}}}]},"finishReason":"STOP"}]}

```

这是基于协议改写的测试数据，不是真实模型对 Goink 工具的录制。它可以验证 SDK 能读出调用，适配器能转换为 Goink event，Registry 能找到并执行 get_characters；同一个协议不需要为每个工具再录一次流。

改写时保留原始文件和来源，在派生样本中同步修改工具定义、名称、参数、调用 ID、工具结果及预期请求。参数分片可以重新切开，但拼起来必须仍是合法 JSON；用量改成测试期望值，不把旧录制 token 数当成改写后的真实用量。优先用只读工具和隔离测试数据库。

签名按不透明数据保存。改写历史后，可以测试签名字段是否被完整传递，但不能据此认定服务端验签通过。现有包还缺 Gemini thoughtSignature、Interactions 专用流和若干失败场景，要分别补样本。[Anthropic 样本来源](https://github.com/anthropics/anthropic-sdk-go/tree/bdcae90baad7d21949fc694f779377f62ff68631/toolrunner/testdata/cassettes)、[Google 样本来源](https://github.com/googleapis/js-genai/tree/1693f7cce750e5e9c3615e5a90b4602029cb0979/test/system/recordings)。样本已迁入 [internal/llm/testdata/sse](../../../internal/llm/testdata/sse/README.md)，随附来源、许可证和校验结果，尚未接入自动化回放测试。

## 5. 本版本的难度

**完整交付属于中等偏大；普通消息和工具调用的格式转换本身属于中等。** 两种 SDK 承担 HTTP / SSE 解析，适配器负责上面的结构对应，业务工具不需要按协议各写一份。

| 工作 | 判断 |
|---|---|
| 单个协议的文本、工具定义、调用 / 结果转换、event 映射 | 中等，边界明确，可用回放样本逐项验证 |
| 多轮历史、完整签名保存、重启后继续 | 中等偏大，必须验证序列化和回传没有丢字段 |
| 同会话切换协议 / 模型 | 保留正文与工具信息，按目标协议舍弃不兼容推理；需要工具 ID 映射、来源 / epoch 与目标容量预检 |
| 工具执行中取消 / 重试、压缩后续聊 | 难点，既要满足协议，又要避免漏配结果或重复执行 |
| 同时覆盖两种 Gemini 协议、不同中转站和型号能力 | 增加适配与验证工作，不能仅靠一次连接测试确认 |

实现时先独立验收公共适配层的协议标识、含 ProviderName 的请求、统一结果 / 事件和能力接口；下一步才迁移 OpenAI，将 providers、hooks 和运行时组装内聚到其子包，并接通三级调用和后续状态 / 切换能力；随后完成 Claude 的工具闭环、多轮 / 重启、压缩 / 取消 / 重试、设置和 OpenAI 双向切换；最后完成 Gemini 的同等能力及三协议互切。先保留现有 messages 和工具业务逻辑，按协议逐个验收完整产品链路。

## 6. 同一会话切换时发生什么

例如 OpenAI Chat → Claude → Gemini → Claude：

1. 在空闲且工具组完整的边界按 adapterName、providerName、model 选择目标，后端检查配置和目标容量；有运行中的调用或审批时先完成收尾。
2. Claude 适配器将现有正文、工具调用和结果转成 Messages；来源不兼容的 thinking / signature 从本次请求移除，数据库和前端历史保留。
3. 切到 Gemini 时生成 contents / parts；有需要导入的外部工具轨迹时，使用官方允许的导入标记或经过验证的完整文本投影，正常 Gemini 工具循环继续保留真实签名。[官方 FAQ](https://ai.google.dev/gemini-api/docs/generate-content/thought-signatures#faqs)
4. 切回 Claude 后仍可在原会话继续，但不自动复活已经失效的旧推理链。后端保存来源和推理状态 epoch，使重启后的处理一致；普通正文和工具结果继续作为历史使用，历史工具不会重新执行。

目标窗口较小时先按实际输出预算、system 和 tools 计算可用容量，必要时执行按目标容量压缩。对于目标强制要求、源历史无法提供的真实推理字段，适配器将受影响历史组转换成可读上下文并验证端点兼容性；不能靠伪造推理内容保证请求通过。完整规则和失败边界见接入设计第 4、7 节。
