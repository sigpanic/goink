# v1.2.0 — 对话中断错误反馈修复（issue #26）

## 背景与动机

issue #26（2026-07-19）用户反馈"对话总是被中断"：

- 用户 A（Snorlax-bit）："这几天全是对话中断，我用的官方的 api"（DeepSeek 官方）
- 用户 B（chenjiangcheng0928-web）："对话总是会中断"、"总是被中断是什么原因？"

现象描述（来自用户 B 截图）：

1. AI 正在思考闪烁中
2. 显示红色"对话被中断"
3. 用户再次发送"接着上面的思考继续"
4. AI 继续思考然后输出一段话
5. 接着又显示"对话被中断"

**不是用户手动停止**。开发者本地用同一个 DeepSeek 模型测试"一点事情没有"。

## 根因定位（多层叠加）

### 1. 后端 FriendlyError 吞原文

[internal/agent/errors.go:12-32](file:///home/nianhe/projects/todo/internal/agent/errors.go) 的 `FriendlyError` 只按 StatusCode 粗分类，**丢光服务商原文**：

| StatusCode | 显示文案 |
|---|---|
| 401 | "API Key 无效..." |
| 403 | "API Key 无权限" |
| 429 | "请求过于频繁..." |
| 5xx | "AI 服务暂时不可用..." |
| default（含 400/404/413） | "对话出错，请重试" |

服务商真实消息（`model not found` / `context length exceeded` / `invalid api key: sk-xxx`）全部被替换。

[app/chat.go:185](file:///home/nianhe/projects/todo/app/chat.go) 又走一次 `FriendlyError`，原始信息第二次被吞。

### 2. 后端 EventType 设计偏离实现

[internal/session/types.go:46](file:///home/nianhe/projects/todo/internal/session/types.go) 字段注释里规划了 4 种 EventType：

```
"compression" | "interrupt" | "error" | ""
```

但 [app/chat.go:165-170](file:///home/nianhe/projects/todo/app/chat.go) 实现时只用了 3 种：

| EventType | 触发条件 |
|---|---|
| `system_interrupted` | 所有非 `context.Canceled` 错误（含 HTTP 4xx/5xx） |
| `user_stopped` | 仅 `context.Canceled` |
| `compression` | 上下文压缩边界标记 |

**`interrupt` 和 `error` 被合并成 `system_interrupted` 一个值** —— 设计与实现不一致。

开发者语义期望：

- **中断**：后端主动取消（死循环检测、用户停止、MaxTurns 触发）
- **失败**：服务商错误（HTTP 4xx/5xx、网络错误、流式断开）

实际后端"主动中断"代码路径：

| 场景 | 代码路径 | 返回 error | 最终 EventType |
|---|---|---|---|
| 用户点停止 | `CancelChat` → `ctx.Done` → `agent.go:417` return `ctx.Err()` | `context.Canceled` | `user_stopped` |
| 死循环检测 | `safety.go:72` 注入 `<system-reminder>` 警告，**不 break** | 无 | 无（继续跑） |
| MaxTurns 触发（当前 50） | `agent.go:174` for 循环正常结束 → `agent.go:419` return `nil` | `nil` | **走成功路径，不写 EventType** |
| HTTP 4xx/5xx | `llm.EventError` → `agent.go:349` return `event.Error` | 非 nil，非 Canceled | `system_interrupted` ✗ |
| 流式网络异常 | 同上 | 同上 | `system_interrupted` ✗ |

后端**完全没有区分**"主动中断"和"错误失败"——除了 `context.Canceled` 单独走 `user_stopped`，其他所有错误都被打包进 `system_interrupted`。

### 3. 前端 ChatPanel.tsx catch 块状态覆盖 bug（核心 BUG）

[frontend/src/components/chat/ChatPanel.tsx:813-818](file:///home/nianhe/projects/todo/frontend/src/components/chat/ChatPanel.tsx) catch 块对 `await app.Chat()` 的所有 reject **一视同仁地**设为 `'interrupted'`，**丢失了 'failed' 状态和 errorMessage**。

完整路径（HTTP 错误为什么显示成"对话被中断"）：

| 步骤 | 文件:行号 | 行为 |
|---|---|---|
| 1 | stream.go:108-121 | DeepSeek 返回 HTTP 4xx/5xx → 发 `StreamEvent{Type: EventError}` |
| 2 | agent.go:339-349 | `emit(AgentEvent{Type: EventError})` 推前端 + `return AgentLoopResult{...}, event.Error` |
| 3 | ChatPanel.tsx:274-281 | 前端收到 EventError → status='failed' + errorMessage=真实错误（如"请求过于频繁"） |
| 4 | chat.go:165-186 | 后端用 `FriendlyError(runErr)` 包成 error 返回给前端 |
| 5 | `await app.Chat()` reject | — |
| 6 | **ChatPanel.tsx:813-818** | **catch 块无脑把 status 覆盖为 'interrupted'，丢失了 'failed' 状态和 errorMessage** |
| 7 | ChatPanel.tsx:1039-1045 | UI 渲染 'interrupted' 时只显示固定文案"对话被中断" |

**同一个 HTTP 错误，触发了两次状态写入**：第一次正确地写了 `failed` + errorMessage，第二次错误地覆盖成 `interrupted` 把前者冲掉。

### 4. 前端 interrupted 渲染不读 errorMessage

[ChatPanel.tsx:1039-1045](file:///home/nianhe/projects/todo/frontend/src/components/chat/ChatPanel.tsx) 只渲染 `t('chat.chatInterrupted')` 固定文案，**不读 errorMessage 字段**。即便后端给了具体原因也显示不出来。

### 5. 前端 failed 状态是死代码

[ChatPanel.tsx:1032](file:///home/nianhe/projects/todo/frontend/src/components/chat/ChatPanel.tsx) 已经写了 `failed` 的红色错误框 UI 分支，但 `rebuildTurns`（[types.ts:94-108](file:///home/nianhe/projects/todo/frontend/src/components/chat/types.ts)）**永远不会产出 `'failed'` 状态**——为"错误"预留的 UI 通道从未被后端激活。

### 6. agent 层无重试逻辑（Retryable 死代码）

[internal/agent/agent.go:339-349](file:///home/nianhe/projects/todo/internal/agent/agent.go) 收到 EventError 直接 `return AgentLoopResult{...,event.Error}`，无任何重试。

但 transport 层 [internal/llm/stream.go](file:///home/nianhe/projects/todo/internal/llm/stream.go) 把以下场景全部标记为 `Retryable: true`：

| 场景 | 文件:行号 |
|---|---|
| 网络错误（reset/EOF） | stream.go:99-104 |
| HTTP 4xx/5xx | stream.go:108-121 |
| SSE 中断 | stream.go:338-344 |
| 空响应 | stream.go:370-376 |

Grep `Retryable` 在整个 `internal/agent/` 目录下**无任何匹配** —— `Retryable` 字段目前是死代码。

DeepSeek 官方 API 在高峰期频繁返回 429/503/connection reset，每次都被 `FriendlyError` 显示"AI 服务暂时不可用，请稍后重试"后整段结束。开发者本地网络稳定不触发，于是"本地没问题"。

### 7. 流式请求无超时

- [internal/llm/stream.go:29-33](file:///home/nianhe/projects/todo/internal/llm/stream.go) `http.Client{Timeout: 0}`，注释写"由 ctx 控制"
- [app/chat.go:47](file:///home/nianhe/projects/todo/app/chat.go) `ctx, cancel := context.WithCancel(a.ctx)` —— 只 cancel 无 deadline

DeepSeek 思考模型长推理时可能出现"连接保持但服务端不吐 chunk"，对话无限期挂起，用户视角等同中断。

### 8. 不是 i18n 回归

git blame 证实 [ChatPanel.tsx:1039-1045](file:///home/nianhe/projects/todo/frontend/src/components/chat/ChatPanel.tsx) 的 i18n 改造（commit `aa71996` "feat: internationalization..."）**只是文案替换**，未改变"interrupted 不读 errorMessage"行为。该 bug 从 commit `5849d25b`（2026-06-14 "fix: persist tool results on interrupt"）引入 interrupted 状态时就已经存在。

### 9. partial tool_calls 丢失导致上下文缺失（但不破坏协议）

**之前的错误判断**：曾认为 partial 消息含 reasoning_content 被传回 DeepSeek 违反协议。经核对官方文档后确认**这个判断完全错误**。

**DeepSeek 协议对 reasoning_content 的精确要求**（官方文档原文）：

> "Between two `user` messages, if the model **did not perform a tool call**, the intermediate assistant's `reasoning_content` does not need to participate in the context concatenation. **If passed to the API in subsequent turns, it will be ignored.**"
>
> "Between two `user` messages, if the model **performed a tool call**, the intermediate assistant's `reasoning_content` **must** participate in the context concatenation and **must be passed back to the API** in all subsequent user interaction turns."
>
> "If your code does not correctly pass back reasoning_content, the API will return a 400 error."

即：
- **纯对话场景**：reasoning_content 不需要回传，回传会被**忽略**（不报错）
- **工具调用场景**：reasoning_content **必须**回传，不回传触发 400

**Goink 当前行为**（[types.go:62-66](file:///home/nianhe/projects/todo/internal/session/types.go) `ToAPIFormat` 把 reasoning_content 拼入 payload）**完全符合协议**：纯对话场景被忽略，工具调用场景必需回传。

**真正的实际缺陷**：partial tool_calls 在流式中断时被丢弃，导致**上下文缺失**（不是协议破坏）。

| 场景 | partial tool_calls 处理 | 协议破坏 | 上下文缺失 |
|---|---|---|---|
| 成功完成 | 保存 + 补 tool 消息（[agent.go:370-381](file:///home/nianhe/projects/todo/internal/agent/agent.go)） | 否 | 否 |
| EventError 断流 | **丢弃**（[agent.go:345-348](file:///home/nianhe/projects/todo/internal/agent/agent.go) `appendMsg(..., nil, ...)`） | **否** | **是**（LLM 看不到上次尝试调过工具，可能重复调用） |
| ctx 取消 | flush + 补 tool 消息 | 否 | 取决于时序 |

**为什么 partial tool_calls 被丢弃**：

1. [agent.go:345-348](file:///home/nianhe/projects/todo/internal/agent/agent.go) 流式中断时 `appendMsg(..., nil, ...)`，extra 参数传 nil
2. parseSSE 在 [stream.go:216-221](file:///home/nianhe/projects/todo/internal/llm/stream.go) 维护局部累积器 `accumulated`，agent 层看不到
3. [stream.go:338-344](file:///home/nianhe/projects/todo/internal/llm/stream.go) `scanner.Err()` 分支只发 EventError，**不会 flush 已累积的 tool_calls**
4. 即使 tool_calls JSON 已完整累积在 `accumulated` 中，也会随 EventError 一起丢弃
5. flushInterruptedTools（[agent.go:208](file:///home/nianhe/projects/todo/internal/agent/agent.go)）只能 drain 已发出的 EventToolCallEnd，但 scanner.Err 时根本不发

**用户截图场景重新归因**：

用户描述"思考闪烁→中断→AI 输出一段话→又中断"对应 **EventError 断流场景**：

- **不是协议破坏**（partial 不含 tool_calls，传给 DeepSeek 没问题）
- **是 DeepSeek 服务器抖动 / 网络瞬态错误 / 限流**
- 加上前端 catch 块状态覆盖 + interrupted 不显示 errorMessage + agent 层无重试，导致用户看到"对话被中断"且无法看到真实原因

## 设计目标

1. **让用户看到真实失败原因**：HTTP 4xx/5xx 时显示服务商原文（如"model not found"），而不是笼统的"对话被中断"
2. **语义对齐**：`system_interrupted` 表示后端主动打断（死循环检测/MaxTurns 触发，未来扩展），`error` 表示服务商错误（HTTP 4xx/5xx/网络错误），`user_stopped` 表示用户手动停止（独立，无意外）
3. **可恢复错误自动重试**：DeepSeek 高峰期 429/503/connection reset 自动指数退避重试（仅重试 LLM 调用本身，不重试 tool 执行），不直接终止对话
4. **可观测性**：关键节点补加日志，下次出现 bug 能从日志快速定位
5. **复用现有基础设施**：v1.2.0 已经引入 `apperr` 错误码体系（[docs/feat/v1.2.0/error-code-system.md](file:///home/nianhe/projects/todo/docs/feat/v1.2.0/error-code-system.md)），定义了 `CodeLLMRateLimited` / `CodeLLMServerError` 等 LLM 错误码和 `codeFromLLMAPIError` 映射函数，但设计目标"只在新增 API 上启用，旧 API 保持原签名"——所以 Chat API 没用上。本次修复让 Chat API 启用 apperr

**不在本次修复范围**（独立 issue 跟进）：

- partial tool_calls 丢失导致 LLM 重复调用同一工具：需要 parseSSE 在 scanner.Err 时 flush accumulated，agent 层暴露累积器。改动较大，建议作为独立 issue 跟进，不阻塞本次 issue #26 修复

## 修复方案

### P0：让用户看到真实失败原因（最小改动）

**后端**：

- [internal/agent/errors.go:12-32](file:///home/nianhe/projects/todo/internal/agent/errors.go) `FriendlyError` 在 `*APIError` 分支末尾拼接 `apiErr.Message`，例如 `"请求失败（HTTP 413）：context length exceeded"`。`context.Canceled` 仍返回空串走 `user_stopped` 路径

**前端**：

- [ChatPanel.tsx:813-818](file:///home/nianhe/projects/todo/frontend/src/components/chat/ChatPanel.tsx) catch 块保护 `'failed'` 状态：`if (t.status === 'failed' || t.status === 'stopped') return t` —— 已经被 EventError 正确设置的 `'failed'` 不被覆盖
- [ChatPanel.tsx:1039-1045](file:///home/nianhe/projects/todo/frontend/src/components/chat/ChatPanel.tsx) interrupted 渲染分支改成 `turn.errorMessage || t('chat.chatInterrupted')`，参考 failed 分支（1032-1038）的写法

### P1：后端 EventType 区分中断和失败（不改名，只新增 error）

[internal/session/types.go:46](file:///home/nianhe/projects/todo/internal/session/types.go) 字段是 GORM 普通 TEXT 列，**无 CHECK 约束**，加新值不破坏 schema。`system_interrupted` 字符串在代码库**只出现 2 处**（写入端 [app/chat.go:167](file:///home/nianhe/projects/todo/app/chat.go)、读取端 [types.ts:102](file:///home/nianhe/projects/todo/frontend/src/components/chat/types.ts)）。

**不改名 `system_interrupted`**，保持向后兼容。历史数据里 `system_interrupted` 行既包含 HTTP 错误也包含真·中断，没有可靠判据回溯区分，**不迁移、只向前区分**。老数据维持 `system_interrupted` → 前端显示 `interrupted`，对用户而言"对话被中断"是模糊但合理的展示。

| 场景 | EventType |
|---|---|
| `context.Canceled`（用户点停止） | `user_stopped` |
| HTTP 4xx/5xx / 流错误 / 网络错误 / 首字节超时 | `error`（新增） |
| 死循环检测 / MaxTurns 触发 / 后端主动中断 | `system_interrupted`（语义回归"主动打断"） |

[app/chat.go:165-170](file:///home/nianhe/projects/todo/app/chat.go) 增加判断：

- `errors.Is(runErr, context.Canceled)` → `user_stopped`
- `errors.As(runErr, &apiErr)` 且 `apiErr != nil` → `error`
- 其他 → `system_interrupted`

前端 [types.ts:94-108](file:///home/nianhe/projects/todo/frontend/src/components/chat/types.ts) `rebuildTurns` 增加 `event_type === 'error'` → `status = 'failed'` 分支，激活 [ChatPanel.tsx:1032](file:///home/nianhe/projects/todo/frontend/src/components/chat/ChatPanel.tsx) 已有的 failed UI。

### P2：agent 层可恢复错误重试

业界共识是 **tool 已执行不回滚，只重试 LLM 调用本身**。

| 框架 | LLM 调用重试 | Tool 调用重试 | 副作用处理 |
|---|---|---|---|
| OpenAI Agents SDK | opt-in，指数退避 + jitter，policy DSL 区分错误类型 | 不内置重试 tool 执行 | 只在 model 层重试，tool 已执行不回滚 |
| LangGraph | RetryPolicy(max=3-4, backoff=2.0)，只重试 ConnectionError + 5xx | ToolRetry middleware | tool 副作用转错误消息回灌 LLM，不自动重试已副作用化的 tool |
| Anthropic SDK / Claude Code | 自动重试 429/529，**必读 Retry-After header** | tool 层不重试 | Claude Code 显示错误前已自动重试多轮 |
| LangChain | ModelRetry middleware 指数退避 | ToolRetry middleware | 强调"瞬态异常才重试，永久异常直接退出" |

**关于"AI 创建角色成功后能不能看到不再创建"**：这是 LLM 自身上下文能力问题，不是重试问题。Goink 的 `appendMsg` 已经把 `create_character` 的成功结果（含角色 ID）写入 `opts.Messages`，LLM 下一轮必然看到。重试 LLM 调用（messages 不变）也不会丢失这个信息。真正的风险是"LLM 上下文太长被压缩后丢失角色信息"——这是 `Compress` 的责任，与重试无关。

**P2 策略**：

1. **重试范围**：仅在 `EventError` 分支（[agent.go:339-349](file:///home/nianhe/projects/todo/internal/agent/agent.go)）重试 LLM 调用本身（messages 不变），不重试 tool 执行
2. **重试条件**：`errors.As(event.Error, &apiErr)` 且 `apiErr.Retryable == true`（429/408/5xx/网络错误/首字节超时）。**不重试** 401/403/400/413（参数/鉴权类永久错误）
3. **退避**：指数退避 + jitter，3 次封顶。建议 200ms → 500ms → 1.2s。若服务商返回 `Retry-After` header 优先遵循（Anthropic/Claude 关键要求）
4. **重试时跳过 appendMsg**：345-348 行已有 partial 持久化保护。重试时清空 `responseBuffer` / `thinkingBuffer`，避免重试后内容重复累加
5. **MaxTurns 不变**：重试不消耗 MaxTurns 计数（`loopCount++` 放在重试成功后，或重试不递增）
6. **失败兜底**：3 次重试全失败 → 走现有 `return ... event.Error`，由 `app/chat.go` 写入 `EventType='error'`

### P3：流式请求加超时（用 ResponseHeaderTimeout，绝不用 http.Client.Timeout）

**修订**：原方案提"给 http.Client 加 180s 超时"是错的。`http.Client.Timeout` 是**整体超时**（DNS + 连接 + 首字节 + body 读取全部时间），会切断 DeepSeek 思考模型长输出 60s+，**绝对不能加**。

| 配置 | 含义 | 是否可行 |
|---|---|---|
| `http.Client.Timeout` | 整体超时（含 body 读取） | **不行**，DeepSeek 思考模型长输出会被强行截断 |
| `http.Transport.ResponseHeaderTimeout` | **首字节超时**（从请求发出到收到响应头，不含 body 读取） | **可行**，正是开发者要的 |
| `http.Transport.DialContext` | TCP 连接超时（如 10s） | 可行 |

**修订后的 P3 方案**：

- [internal/llm/stream.go:29-33](file:///home/nianhe/projects/todo/internal/llm/stream.go) 给 `http.Client` 配置自定义 `Transport`，设置 `ResponseHeaderTimeout`（建议 60s，首字节超时）+ `DialContext`（10s TCP 连接超时）
- `http.Client.Timeout` 保持 `0`（不限制整体超时），body 读取由 `ctx` 控制
- 首字节超时触发时返回 `APIError{StatusCode: 0, Retryable: true, Message: "服务器响应超时"}`，由 P2 重试逻辑处理

**前端显示**：

- 首字节超时走 `error` EventType（不是 `system_interrupted`），因为这是服务商不可达/超时，属于"失败"而非"主动中断"
- 走 `error` → `status='failed'` → 显示 `errorMessage`（`FriendlyError` 设为"服务器响应超时，请检查网络或服务商状态"）
- **无需新增 i18n key**，复用 failed 分支

**错误区分点**（[stream.go](file:///home/nianhe/projects/todo/internal/llm/stream.go) 现有结构）：

- `c.http.Do(req)` 返回 err → 连接失败 / 首字节超时（**新增**）
- `resp.StatusCode >= 400`（[stream.go:109](file:///home/nianhe/projects/todo/internal/llm/stream.go)）→ HTTP 4xx/5xx 错误
- `scanner.Err()` 返回非 nil（[stream.go:338](file:///home/nianhe/projects/todo/internal/llm/stream.go)）→ 流中途中断（body 读取阶段，**不算超时**）

### 配套：MaxTurns 50 → 100

[internal/agent/agent.go:174](file:///home/nianhe/projects/todo/internal/agent/agent.go) `MaxTurns=50` 改为 `MaxTurns=100`。

理由：

- 复杂任务（多步工具调用 + 长上下文压缩 + 多轮思考）可能超过 50
- 触发后是 break + return nil error，无副作用，目前走成功路径不写 EventType
- 100 是合理上限，避免真正死循环
- 死循环检测（safety.go）本身只注入提醒不 cancel，提高 MaxTurns 影响有限

## 加日志方案

当前日志情况：

| 位置 | 是否打日志 | 内容 |
|---|---|---|
| [app/chat.go:166](file:///home/nianhe/projects/todo/app/chat.go) | **是** | `a.logger.Error("对话失败", "err", runErr)` —— 含 `[StatusCode] Message` 原文 |
| [agent.go:339-349](file:///home/nianhe/projects/todo/internal/agent/agent.go) EventError 分支 | **否** | 完全不打 logger |
| [stream.go](file:///home/nianhe/projects/todo/internal/llm/stream.go) HTTP 4xx/5xx / 网络错误 / SSE 中断 / 空响应 | **否** | 只通过 channel 传 APIError |
| stream.go JSON 解析失败 / tool index 缺失 / arguments 无效 | 是（Warn） | 局部问题日志 |
| agent loop 开始 / 收到事件 / 结束 | **否** | 关键节点无日志 |

**补加日志位置**：

| 位置 | 日志内容 | 级别 |
|---|---|---|
| agent.go:339-349（EventError 分支） | `"agent event error"`, `err`, `retryable`, `status_code` | Warn |
| stream.go:108-121（HTTP 4xx/5xx） | `"llm http error"`, `status`, `body` 摘要 | Warn |
| stream.go:99-104（网络错误） | `"llm network error"`, `err` | Warn |
| stream.go:338-344（SSE 中断） | `"sse stream interrupted"`, `err` | Warn |
| stream.go:370-376（空响应） | `"empty sse response"` | Warn |

日志输出位置：`~/Goink/goink.log`（[internal/logger/logger.go:41](file:///home/nianhe/projects/todo/internal/logger/logger.go)），10MB 轮转保留 3 份，Debug 级别，文本格式，同时写 stderr。

用户上报 bug 时让用户上传 `~/Goink/goink.log`，搜 `对话失败` / `agent event error` / `llm http error` 关键字即可看到真实 `[StatusCode] Message`。

## 实施计划

| 步骤 | 内容 | 优先级 |
|---|---|---|
| 1 | 前端 ChatPanel.tsx catch 块保护 'failed' 状态 + interrupted 读 errorMessage | P0 |
| 2 | 后端 FriendlyError 保留 apiErr.Message 原文 | P0 |
| 3 | 后端 EventType 增加 `error` 类型（不改名 system_interrupted），区分中断和失败 | P1 |
| 4 | 前端 rebuildTurns 增加 error → failed 映射 | P1 |
| 5 | agent 层加可恢复错误重试（指数退避 + 最大次数 3，遵循 Retry-After header） | P2 |
| 6 | 流式请求加 http.Transport.ResponseHeaderTimeout（60s 首字节超时）+ DialContext（10s） | P3 |
| 7 | 关键节点加日志（agent EventError / stream HTTP错误 / SSE中断） | 配套 |
| 8 | MaxTurns 50 → 100 | 配套 |

**实施优先级建议**：P0 → P1 → P3 → P2 → 配套。P0/P1 是用户可见改进，先改最显眼；P3 改动小且独立；P2 最复杂放最后；配套日志和 MaxTurns 顺手做。

## 测试覆盖

- FriendlyError 对 `*APIError{StatusCode: 413, Message: "context length exceeded"}` 返回含原文的字符串
- FriendlyError 对 `context.Canceled` 返回空串
- 后端 EventType 选择：HTTP 4xx → `error`；`context.Canceled` → `user_stopped`；其他真·中断 → `system_interrupted`
- 前端 rebuildTurns：`event_type='error'` → `status='failed'`；`event_type='system_interrupted'` → `status='interrupted'`
- agent 重试：模拟 `Retryable=true` 错误，验证 3 次重试后退出
- agent 重试：模拟 `Retryable=false`（401）错误，验证直接退出不重试
- agent 重试：模拟带 `Retry-After` header 的 429，验证遵循 header 等待
- agent 重试：模拟 `Retryable=true` 错误，验证重试时清空 responseBuffer / thinkingBuffer，避免重试后内容重复累加
- stream 首字节超时：模拟 ResponseHeaderTimeout 触发，验证返回 `Retryable=true` 错误且不切断已开始的 body 读取
- 端到端：DeepSeek 流式中断 → partial 保存（不含 tool_calls）→ 用户"接着上面的思考继续" → 验证不触发 400

## 不做的事

- 不为现有所有 Wails API 切换到 apperr，仅 Chat API 评估启用
- 不引入新的错误码注册机制，沿用 apperr 的 const 定义
- 不为 FriendlyError 加 i18n，ErrMsg 沿用后端原文
- 不修改死循环检测逻辑（safety.go），保持只注入提醒不 cancel
- **不改名 `system_interrupted` 字符串**（保持向后兼容），仅调整其语义边界
- **不迁移历史 `system_interrupted` 数据**（无可靠判据回溯区分），只向前区分
- **不用 `http.Client.Timeout`**（整体超时会切断 DeepSeek 思考模型长输出）
- **不修改 partial 消息的 ToAPI 标记**（DeepSeek 协议允许 reasoning_content 回传，纯对话场景被忽略，工具调用场景必需，Goink 当前行为正确）
- **不修复 partial tool_calls 丢失**（独立 issue 跟进，需要 parseSSE 在 scanner.Err 时 flush accumulated + agent 层暴露累积器，改动较大）

## 风险与权衡

1. **前端 catch 块状态保护可能漏边界**：需要确认 EventError 事件和 Chat reject 的时序。若 EventError 先到，前端正确设置 `failed`，catch 块跳过；若 EventError 后到（极端时序），catch 先设 `interrupted`，EventError 再设 `failed`，最终正确。两种时序都需测试
2. **agent 重试不担心副作用**：业界共识是"tool 已执行不回滚，只重试 LLM 调用本身"。Goink 的 `appendMsg` 已经把 tool 成功结果写入 opts.Messages，LLM 下一轮必然看到，重试 LLM 调用（messages 不变）不会丢失信息，也不会重复执行 tool。真正的风险是上下文被 `Compress` 压缩后丢失 tool 结果，与重试无关
3. **首字节超时设置不当可能误杀**：60s 是首字节超时（不含 body 读取），DeepSeek 思考模型即使长思考也会很快返回首个 SSE chunk（reasoning_content 增量），60s 足够覆盖网络抖动和服务商排队。**注意**：DeepSeek-reasoner 长推理期间会持续吐 reasoning_content chunk，不算首字节超时
4. **MaxTurns 提高可能掩盖死循环**：50→100 后死循环检测更晚触发。但死循环检测（safety.go）本身只注入提醒不 cancel，影响有限
5. **EventType 新增 `error` 的向后兼容**：前端老版本读到 `event_type='error'` 会走 default 分支（保持 `streaming`/`done`），不会显示错误 UI，但也不会显示"对话被中断"。建议前端版本对齐发布
6. **重试 Retry-After header 兼容性**：DeepSeek 是否返回 Retry-After header 需实测。Anthropic/Claude 必返回，OpenAI 部分场景返回。若无 header 则按指数退避兜底
7. **partial tool_calls 丢失的遗留风险**（独立 issue 跟进）：用户流式中断后下次继续，LLM 看不到上次尝试调过工具，可能重复调用同一工具。本次不修复，作为独立 issue 跟进

## 关键文件速查

| 文件 | 行号 | 作用 |
|---|---|---|
| [internal/agent/errors.go](file:///home/nianhe/projects/todo/internal/agent/errors.go) | 12-32 | FriendlyError 吞原文（P0） |
| [internal/agent/agent.go](file:///home/nianhe/projects/todo/internal/agent/agent.go) | 174 | MaxTurns=50（配套改 100） |
| [internal/agent/agent.go](file:///home/nianhe/projects/todo/internal/agent/agent.go) | 339-349 | EventError 无重试（P2） |
| [internal/llm/stream.go](file:///home/nianhe/projects/todo/internal/llm/stream.go) | 29-33 | http.Client 无超时（P3 改 Transport） |
| [internal/llm/stream.go](file:///home/nianhe/projects/todo/internal/llm/stream.go) | 99-104, 108-121, 338-344, 370-376 | APIError.Message 来源 + Retryable 标志 |
| [internal/llm/types.go](file:///home/nianhe/projects/todo/internal/llm/types.go) | 36-44 | APIError 结构 + Retryable 字段 |
| [internal/session/types.go](file:///home/nianhe/projects/todo/internal/session/types.go) | 46 | EventType 字段（P1） |
| [app/chat.go](file:///home/nianhe/projects/todo/app/chat.go) | 47 | ctx 只 cancel 无 deadline |
| [app/chat.go](file:///home/nianhe/projects/todo/app/chat.go) | 165-186 | EventType 选择 + FriendlyError 二次包装（P1） |
| [frontend/src/components/chat/ChatPanel.tsx](file:///home/nianhe/projects/todo/frontend/src/components/chat/ChatPanel.tsx) | 274-281 | EventError → failed + errorMessage |
| [frontend/src/components/chat/ChatPanel.tsx](file:///home/nianhe/projects/todo/frontend/src/components/chat/ChatPanel.tsx) | 780-795 | 订阅时序窗口（推测） |
| [frontend/src/components/chat/ChatPanel.tsx](file:///home/nianhe/projects/todo/frontend/src/components/chat/ChatPanel.tsx) | 813-818 | catch 块状态覆盖 BUG（P0 核心） |
| [frontend/src/components/chat/ChatPanel.tsx](file:///home/nianhe/projects/todo/frontend/src/components/chat/ChatPanel.tsx) | 1032-1045 | failed/interrupted UI 渲染分支（P0） |
| [frontend/src/components/chat/types.ts](file:///home/nianhe/projects/todo/frontend/src/components/chat/types.ts) | 94-108 | rebuildTurns EventType → status 映射（P1） |
| [frontend/src/i18n/locales/zh-CN.json](file:///home/nianhe/projects/todo/frontend/src/i18n/locales/zh-CN.json) | 125 | `chatInterrupted` 文案定义 |
| [internal/apperr/apperr.go](file:///home/nianhe/projects/todo/internal/apperr/apperr.go) | — | 已有 LLM 错误码体系可复用 |

## 修订历史

- **v1（初稿）**：识别 8 层根因，提出 P0-P3 + 配套方案
- **v2（修订）**：增加根因 #9（误判 reasoning_content 破坏协议），新增 P1.5（partial ToAPI=false）
- **v3（最终）**：核对 DeepSeek 官方文档后确认 v2 的根因 #9 完全错误。DeepSeek 协议是：纯对话场景 reasoning_content 回传被忽略，工具调用场景必须回传。Goink 当前行为完全符合协议。删除根因 #9 错误判断和 P1.5 错误方案。重新归因为 partial tool_calls 丢失导致上下文缺失（但不破坏协议），作为独立 issue 跟进。本次修复聚焦 P0-P3 + 配套方案
- **v4（P1 实施记录）**：实施 P1 方案 C。后端 EventType 新增 `error` 类型，前端 rebuildTurns 加 `error` → `failed` 映射，app/chat.go 增加 EventType 选择逻辑（context.Canceled → user_stopped / apiErr 存在 → error / 其他 → system_interrupted）。session/types.go EventType 字段注释更新为 5 种值
- **v5（P3 实施记录）**：实施 P3 流式超时方案。internal/llm/stream.go 引入 newHTTPClient() 工厂函数，配置 http.Transport.ResponseHeaderTimeout=60s + DialContext=10s，http.Client.Timeout 保持 0。首字节超时触发时返回 APIError{StatusCode:0, Retryable:true, Message:"服务器响应超时（首字节超过 60s）"}，由 P2 重试逻辑处理
- **v6（配套实施记录）**：实施配套方案。agent.go MaxTurns 50→100；agent.go EventError 分支加 Warn 日志（含 status_code/retryable 字段）；stream.go HTTP 4xx/5xx 分支加 Warn 日志（body 截断 500 字符）；stream.go SSE 中断 / 空响应分支加 Warn 日志；stream.go 网络错误分支扩展为超时和非超时两类日志
- **v7（配套补充）**：app/chat.go L161 主对话 MaxTurns 50→100；app/chat.go L392 压缩路径 `MaxTurns: 50` 删除（Grep 确认 compress.go 不读 opts.MaxTurns，Compress 走 GenerateText 单次调用不进入 agent loop，原字段是死代码）
