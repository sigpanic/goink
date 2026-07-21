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

## 用户日志实证分析（2026-07-21）

issue #26 用户 Snorlax-bit 上传了完整 `goink.log`（8065 行，4.9MB），覆盖 2026-07-12 ~ 2026-07-17 共 6 天使用记录。本次实证分析从前述 P0-P3 + 配套方案之外**新发现 3 个独立 bug**，均为代码缺陷，与已实施的 P0-P3 修复正交。

### 错误类型分布统计

| 错误类型 | 出现次数 | 性质 | 与本次修复方案关系 |
|---|---|---|---|
| `[400] Duplicate value for 'tool_call_id'` | 3 次 | **代码 bug（已修复）** | 正交，已修复（commit 3961850） |
| `[400] messages[74].reasoning_content is required for thinking tool-call history` | 1 次 | **代码 bug（根因已定位）** | 由 v1.3.0 多 Provider 适配解决（见 [v1.3.0 文档](file:///home/nianhe/projects/todo/docs/feat/v1.3.0/multi-provider-adapter.md)） |
| `tool panicked tool=read panic="slice bounds out of range [48:10]"` | 3 次（[48:10]/[89:15]/[54:40]） | **代码 bug（已修复）** | 正交，已修复 |
| `[403] chat pre-consumed quota failed` | 6 次 | 用户 API 余额不足 | 非代码问题， FriendlyError 改进后用户能看到原文 |
| `[0] 流式响应为空，服务商可能不支持流式请求或返回了非标准格式` | 3 次 | 服务商问题 | P3 首字节超时不覆盖此场景，可观察 |
| `context canceled` | 2 次 | 用户主动取消 | 正常行为 |
| `压缩摘要生成失败` | 1 次 | 切模型导致 ctx 取消 | 正常行为 |

### Bug A：Duplicate tool_call_id（P0，对话彻底中断根因之一）

**现象**：DeepSeek 返回 `[400] Duplicate value for 'tool_call_id' of call_a9hDhfmLkboHAaoTtIsHKedq in message[74]`，明确指出 message[74] 的 `tool_calls` 数组里 `call_a9hDhfmLkboHAaoTtIsHKedq` 出现了两次。

**根因**：GPT（`xiangliang/gpt-5.3-codex` 中转）在单次响应中给不同 `index` 的 parallel tool_call 偶发分配了相同 `id`（GPT 服务端偶发行为，非 Goink bug），Goink 全链路**无任何 id 去重逻辑**，重复 id 被原样持久化到 DB，切换到 DeepSeek 时被 400 拒绝（DeepSeek 比 GPT 更严格地校验 tool_call_id 唯一性）。

**全链路无去重的 8 个环节**：

| 步骤 | 文件:行号 | 行为 |
|---|---|---|
| 1. 累积 | [internal/llm/stream.go:343-356](file:///home/nianhe/projects/todo/internal/llm/stream.go#L343-L356) | parseSSE 按 `index` 键化累积到 `accumulated[idx]`，**不按 id 去重**；两个不同 index 的 slot 可以共存同一 id |
| 2. 发射 | [internal/llm/stream.go:397-418](file:///home/nianhe/projects/todo/internal/llm/stream.go#L397-L418) | 流结束后遍历 `accumulated` 所有 slot，**每个非空 slot 独立发一个 EventToolCallEnd**，重复 id 被发两次 |
| 3. 收集 | [internal/agent/agent.go:261-339](file:///home/nianhe/projects/todo/internal/agent/agent.go#L261-L339) | `toolOutputs = append(toolOutputs, ...)`，**不去重** |
| 4. 构造 | [internal/agent/safety.go:57-70](file:///home/nianhe/projects/todo/internal/agent/safety.go#L57-L70) | buildToolCalls 直接遍历输出 `[{id, type, function:{name, arguments}}]`，**不去重** |
| 5. 持久化 | [internal/agent/agent.go:429-440](file:///home/nianhe/projects/todo/internal/agent/agent.go#L429-L440) | assistant 消息的 `ExtraMetadata.tool_calls = buildToolCalls(toolOutputs)`，**原样写入 DB** |
| 6. 读出 | [internal/session/store.go](file:///home/nianhe/projects/todo/internal/session/store.go) GetMessagesForAPI | 按 `to_api=true AND version=?` 查询，**无任何过滤** |
| 7. 转格式 | [internal/session/types.go:62-101](file:///home/nianhe/projects/todo/internal/session/types.go#L62-L101) | ToAPIFormat `payload["tool_calls"] = meta["tool_calls"]`，**直接透传** |
| 8. 序列化 | [app/chat.go](file:///home/nianhe/projects/todo/app/chat.go) loadAPIMessages | 遍历调用 `ToAPIFormat()`，**不做任何校验** |

**关键证据**：

- 用户日志 7505 行：`time=2026-07-17T16:16:45.904+08:00 level=ERROR source=D:/a/goink/goink/app/chat.go:166 msg=对话失败 err="[400] Duplicate value for 'tool_call_id' of call_a9hDhfmLkboHAaoTtIsHKedq in message[74]"`
- 用户日志 7524 行：同一 id `call_a9hDhfmLkboHAaoTtIsHKedq` 在 message[74] 重复出现，且用户连续重试 3 次都失败（7505/7524/7676 三次报错完全相同）
- 用户日志 2955 行显示同一 turn（287）的 assistant 消息 `extra_metadata.tool_calls` 数组中确实存在 3 个 tool_call（call_00/01/02），证明 parallel tool calls 场景存在

**v9 实证补充（2026-07-21 深度分析）**：

用户日志 7312 行的 INSERT INTO `messages` SQL 完整记录了 turn 340 主 agent 的 assistant 消息，其 `extra_metadata.tool_calls` 数组含 **10 个元素**（5 个 id 各出现 2 次），`tool_displays` 也是 10 个（前 5 和后 5 完全相同）：

| 序号 | name | id | arguments |
|---|---|---|---|
| 1 / 6 | get_preferences | call_a9hDhfmLkboHAaoTtIsHKedq | `{}` |
| 2 / 7 | get_timeline | call_57Oj0jV1enUC69KUkzqMtU2p | `{"category":"foreshadowing","current_chapter":22,"page":1,"size":50,"status":"pending"}` |
| 3 / 8 | get_story_arcs | call_7i4aZ4m8bpLpffqHSTVjFO40 | `{"arc_type":"main","current_chapter":22,"page":1,"size":50,"status":"active"}` |
| 4 / 9 | get_reader_perspective | call_5Np3mdLIvJO66l6ZJfdWWuKx | `{}` |
| 5 / 10 | run_subagent | call_3YoEbnwTzHm2Od0s9HMlk1YR | `{"agent_type":"review","instruction":"请对当前小说..."}` |

前 5 个和后 5 个的 id、name、arguments **完全相同**。这证明 GPT（`xiangliang/gpt-5.3-codex` 中转）在一次流式响应里发了 10 个不同 index 的 tool_calls，其中 index 0-4 和 index 5-9 的内容完全一致。

**执行时间线**（主 agent 在一个 streamLoop 迭代内执行了 10 次 tool，跨度 130 秒）：

| 时间 | tool | elapsed | 备注 |
|---|---|---|---|
| 16:06:31.609 | get_preferences | 0ms | 第 1 次 |
| 16:06:31.633 | get_timeline | 24ms | 第 1 次 |
| 16:06:31.637 | get_story_arcs | 3ms | 第 1 次 |
| 16:06:31.639 | get_reader_perspective | 1ms | 第 1 次 |
| 16:06:31.639 | run_subagent | 52288ms | 第 1 次（阻塞 52s） |
| 16:07:23.928 | get_preferences | 0ms | **第 2 次（重复）** |
| 16:07:23.930 | get_timeline | 2ms | **第 2 次（重复）** |
| 16:07:23.933 | get_story_arcs | 2ms | **第 2 次（重复）** |
| 16:07:23.934 | get_reader_perspective | 1ms | **第 2 次（重复）** |
| 16:07:23.935 | run_subagent | 78467ms | **第 2 次（重复，阻塞 78s）** |

run_subagent 被执行两次（日志 7212 + 7309 行），每次都启动了完整的子 agent（子 agent 内部又调了 get_preferences 等工具，产生 7194-7204 和 7281-7291 的 review agent 消息）。这证明 tool 确实被执行了两次，不只是 toolOutputs 被重复追加。

**排除其他路径**：

- **排除 retry**：搜索 `agent retrying llm call` / `agent event error` / `sse stream interrupted` 在 turn 340 范围内无任何匹配，没有触发 agent.go:358-396 的 retry 逻辑
- **排除 subagent 合并**：RunSubAgent（agent.go:91-122）创建独立的 subOpts.Messages，子 agent 的消息不会合并到主 agent 的 toolOutputs
- **排除 ctx 取消/partial**：没有 context.Canceled，没有 EventError

**与最新两个 commit 的关系**：

| commit | 是否引入此 bug | 说明 |
|---|---|---|
| `86bf424` fix(chat): clear partial segments on retry via clear_from_seq | **否** | 只在 `EventRetrying` 事件中加 `ClearFromSeq` 字段，前端用于清空 partial segments；**未触及** parseSSE 累积、toolOutputs 处理、buildToolCalls、appendMsg、ToAPIFormat 任何一环 |
| `1da9e8b` fix(chat): route subagent EventRetrying by sub_task_id | **否** | 纯前端路由修复，**完全未触及后端** |

这是项目从一开始就没有 tool_call_id 去重逻辑的**固有问题**，由 DeepSeek 自身在 parallel tool calls 中偶发分配重复 id 触发。

**修复方案**（v9 已实施方案 1 + 方案 3）：

1. **源头去重**（✅ 已实施）：[internal/llm/stream.go:397-430](file:///home/nianhe/projects/todo/internal/llm/stream.go#L397-L430) 流结束后遍历 `accumulated` 发射 `EventToolCallEnd` 之前，维护 `seenToolIDs map[string]bool`，若 `acc.id` 已存在则 `continue` 并 log warn。这样 DeepSeek 发的重复 id 在源头就被去重，只发 5 次 `EventToolCallEnd`，主 agent 只执行 5 次 tool（避免 run_subagent 被执行两次浪费 130s）
2. **构造时去重**（❌ 不推荐）：在 [internal/agent/safety.go:57-70](file:///home/nianhe/projects/todo/internal/agent/safety.go#L57-L70) buildToolCalls 输出前按 `id` 去重。方案 1 实施后 toolOutputs 不会有重复 id，方案 2 是死代码。且方案 2 不能避免 tool 副作用（run_subagent 已经执行两次，去重只能避免持久化重复，不能挽回已浪费的资源）
3. **兜底防御**（✅ 已实施）：[app/chat.go loadAPIMessages](file:///home/nianhe/projects/todo/app/chat.go) 出口加防御性校验。遍历所有消息时维护 `validToolCallIDs` 和 `seenToolMsgIDs` 两个 map：assistant 消息去重 `tool_calls`（保留首次出现的 id）；tool 消息跳过 orphan（无对应 tool_call）+ 跳过重复（每个 tool_call_id 只保留一条）。防止 DB 中已有的历史脏数据（如用户 message[74]）再次发给 DeepSeek 触发 400

**已知未覆盖路径**（v10 评估，方案 D 不修）：

[internal/agent/compress.go:105-113](file:///home/nianhe/projects/todo/internal/agent/compress.go#L105-L113) 压缩完成后重新加载新 version 消息时，直接调 `GetMessagesForAPI` + `ToAPIFormat`，**绕过 loadAPIMessages 去重防御**（代码注释"与 Chat() 走同一条路径"与实现不一致）。若 retainMessages 保留了含重复 tool_call_id 的脏数据到新 version，下次 Run 循环调 LLM 会触发 400 Duplicate tool_call_id。

| 压缩环节 | 入口 | messages 来源 | 防御 |
|---|---|---|---|
| 手动压缩调 LLM | `CompressContext` → `loadAPIMessages` | DB → 去重 | ✅ 已防御 |
| 自动压缩调 LLM | `agent.Run` → `a.Compress` → `generateSummary` | `opts.Messages`（来自 Chat 的 loadAPIMessages） | ✅ 已防御 |
| 压缩后重新加载 | `compress.go:105-113` | `GetMessagesForAPI` + `ToAPIFormat` | ⚠️ 未防御 |

**触发条件较窄**：需同时满足 (1) Bug A 产生脏数据（已修，新版本不会触发）(2) 脏数据被 retainMessages 保留到新 version（保留最近 15 条 user 消息开始的所有消息，含对应 assistant+tool）。鉴于 Bug A 源头已修，新版本不会产生新脏数据，此漏洞只对历史已污染 DB 有影响，且 loadAPIMessages 在 Chat 入口已兜底，**接受不修（方案 D）**。

### Bug B：read 工具切片越界 panic（P0，AI 反复调用失败）

**现象**：日志显示 3 次 panic：

| 时间 | panic 信息 | 推算参数 |
|---|---|---|
| 2026-07-13 09:04:37 | `slice bounds out of range [48:10]` | start_line=49, end_line=10 |
| 2026-07-15 11:04:19 | `slice bounds out of range [89:15]` | start_line=90, end_line=15 |
| 2026-07-16 08:04:28 | `slice bounds out of range [54:40]` | start_line=55, end_line=40 |

**根因**：AI 调用 read 工具时传入了 `start_line > end_line` 的参数，代码无校验直接切片 `lines[start-1 : end]`，导致越界 panic。

**确切位置**：[internal/mcp_tools/rw_tools.go:638](file:///home/nianhe/projects/todo/internal/mcp_tools/rw_tools.go#L638) `selected := lines[start-1 : end]`

**参数处理流程**（以 start_line=49, end_line=10 为例）：

| 行号 | 代码 | 结果 |
|------|------|------|
| 619 | `start := a.StartLine` | `start = 49` |
| 620-622 | `if start == 0 { start = 1 }` | 不触发，`start = 49` |
| 623 | `end := a.EndLine` | `end = 10` |
| 624-626 | `if end == 0 { end = 2000 }` | 不触发，`end = 10` |
| 628-629 | `lines := strings.Split(...)`; `totalLines := len(lines)` | 假设 `totalLines = 50` |
| 631-633 | `if start > totalLines` → 49 > 50? **否**，不报错 | 跳过 |
| 634-636 | `if end > totalLines` → 10 > 50? **否**，`end` 保持 10 | 跳过 |
| **638** | `selected := lines[start-1 : end]` → `lines[48:10]` | **PANIC** |

**对比 edit 工具**：[rw_tools.go:311-313](file:///home/nianhe/projects/todo/internal/mcp_tools/rw_tools.go#L311-L313) 和 [:508](file:///home/nianhe/projects/todo/internal/mcp_tools/rw_tools.go#L508) 都有 `start > end` 校验，**read 工具漏了**。

**次要问题**：[internal/mcp_tools/base.go:224-233](file:///home/nianhe/projects/todo/internal/mcp_tools/base.go#L224-L233) panic recover 后返回通用错误 `"服务器内部错误，请稍后重试"`，AI 看不到真实原因（"start_line 不能大于 end_line"），倾向用相同参数重试，形成 panic 循环。

**为什么 validator 拦不住**：[rw_tools.go:576-581](file:///home/nianhe/projects/todo/internal/mcp_tools/rw_tools.go#L576-L581) ReadArgs 的 validate tag 只校验单字段下界（`min=1`/`min=0`），`go-playground/validator` 的 struct tag **无法表达跨字段约束**（start_line <= end_line）。

**修复方案选择**（已实施方案 A）：

| 方案 | 实现 | 优点 | 缺点 |
|---|---|---|---|
| **A. 校验返回业务错误**（推荐） | rw_tools.go:631 之前加 `if start > end { return 业务错误 }` | 与 edit 工具一致；AI 看到真实原因后能修正参数；字段语义清晰 | AI 偶发犯错时调用失败一次 |
| B. 容忍自动交换 | `if start > end { start, end = end, start }` | 用户能得到结果 | AI 不知道自己错了，下次还可能这样传；start_line/end_line 字段名暗示起点终点，自动交换语义模糊；与 edit 工具行为不一致 |

**推荐方案 A**，理由：

1. 与 edit 工具保持一致（edit 已有 `start_line 不能大于 end_line` 校验）
2. 让 AI 看到真实错误，下次调用时修正
3. 字段语义清晰，避免歧义
4. 已有现成模式可复用

**配套增强**（可选）：

1. 在 [rw_tools.go:700-713](file:///home/nianhe/projects/todo/internal/mcp_tools/rw_tools.go#L700-L713) `readDescription` 和 ReadArgs 字段 description 中明确写"start_line 必须 <= end_line"
2. 仿照 [search_replace_test.go:555](file:///home/nianhe/projects/todo/internal/mcp_tools/search_replace_test.go#L555) `TestLineRangeReplace_StartAfterEnd`，新增 `TestReadTool_StartAfterEnd` 覆盖该分支

### Bug C：reasoning_content 缺失（P1，DeepSeek 兼容性，根因已定位）

**现象**：`[400] messages[74].reasoning_content is required for thinking tool-call history (request id: 20260717161627994112599q0IgJL6N)`

**背景**：用户在 16:16:02 切换到 `deepseek/deepseek-v4-flash + reasoning_effort=high` 模型后立即触发。DeepSeek 协议要求 thinking 模式下的 tool-call 历史 message 必须带 `reasoning_content` 字段（详见本文档根因 #9 的协议引用）。

**根因已定位**（详见 [v1.3.0 多 Provider 适配文档](file:///home/nianhe/projects/todo/docs/feat/v1.3.0/multi-provider-adapter.md)）：

1. message[74] 用 `xiangliang/gpt-5.3-codex`（GPT 中转）生成
2. GPT 模型返回 `reasoning` 字段（OpenAI 标准），不返回 `reasoning_content`（DeepSeek 扩展）
3. Goink 的 [internal/llm/stream.go:316](file:///home/nianhe/projects/todo/internal/llm/stream.go#L316) **只解析 `reasoning_content`，不解析 `reasoning`** → `ThinkingContent=""` 被存入 DB
4. 用户切换到 `向量引擎/deepseek-v4-flash`（DeepSeek 中转），Goink 把历史消息回传
5. [internal/session/types.go:79-81](file:///home/nianhe/projects/todo/internal/session/types.go#L79-L81) 在 `ThinkingContent=""` 且含 `tool_calls` 时设 `payload["reasoning_content"]=""`
6. **DeepSeek 官方接受空字符串**，但中转站 `vectorengine.cn` 自己加了更严格的 schema 校验（Pydantic "required" 语义），要求非空 → 报 400

**修复方案**（v1.3.0 阶段 1 GPT 适配）：在 [internal/llm/stream.go:316](file:///home/nianhe/projects/todo/internal/llm/stream.go#L316) 附近加 `delta.reasoning` 字符串解析，让 GPT 模型生成的消息不再出现 `ThinkingContent=""` 脏数据。详见 [v1.3.0 文档](file:///home/nianhe/projects/todo/docs/feat/v1.3.0/multi-provider-adapter.md) 阶段 1。

**v1.2.0 不修**：Bug C 根因是 GPT 协议适配缺失，属于 v1.3.0 多 Provider 适配范围，v1.2.0 不处理。

### 其他非 bug 发现

1. **用户余额不足**（6 次 403）：用户 API 余额 ＄0.42 / 需要 ＄0.47 等场景。P0 修复 FriendlyError 保留原文后，用户能看到"预扣费额度失败"等真实原因，体验改善
2. **流式响应为空**（3 次）：可能是用户切换的"向量引擎/little"等第三方服务商不支持流式或返回非标准格式。P3 首字节超时不覆盖此场景（首字节已收到，但 body 为空），可考虑在 [stream.go:370-376](file:///home/nianhe/projects/todo/internal/llm/stream.go#L370-L376) 空响应分支加更详细日志
3. **read 工具 panic 后 AI 行为**：日志 2955-2961 行显示 panic 后 AI 仍然成功调用了另外两个 read（call_01/call_02），说明 panic recover 机制有效，但 AI 对失败的工具调用不知道原因，可能影响后续推理质量

### 新增修复优先级

| 优先级 | Bug | 修复位置 | 是否阻塞 |
|---|---|---|---|
| **P0** | Bug A（tool_call_id 去重） | stream.go + safety.go + loadAPIMessages | 阻塞，对话彻底中断无法恢复 |
| **P0** | Bug B（read 工具越界校验） | rw_tools.go:631 | 非阻塞但高频，**已修复**（方案 A） |
| **P1** | Bug C（reasoning_content 缺失） | v1.3.0 stream.go 加 reasoning 解析 | 阻塞 thinking 模式 + tool-call 场景，由 v1.3.0 多 Provider 适配解决 |

## 修订历史

- **v1（初稿）**：识别 8 层根因，提出 P0-P3 + 配套方案
- **v2（修订）**：增加根因 #9（误判 reasoning_content 破坏协议），新增 P1.5（partial ToAPI=false）
- **v3（最终）**：核对 DeepSeek 官方文档后确认 v2 的根因 #9 完全错误。DeepSeek 协议是：纯对话场景 reasoning_content 回传被忽略，工具调用场景必须回传。Goink 当前行为完全符合协议。删除根因 #9 错误判断和 P1.5 错误方案。重新归因为 partial tool_calls 丢失导致上下文缺失（但不破坏协议），作为独立 issue 跟进。本次修复聚焦 P0-P3 + 配套方案
- **v4（P1 实施记录）**：实施 P1 方案 C。后端 EventType 新增 `error` 类型，前端 rebuildTurns 加 `error` → `failed` 映射，app/chat.go 增加 EventType 选择逻辑（context.Canceled → user_stopped / apiErr 存在 → error / 其他 → system_interrupted）。session/types.go EventType 字段注释更新为 5 种值
- **v5（P3 实施记录）**：实施 P3 流式超时方案。internal/llm/stream.go 引入 newHTTPClient() 工厂函数，配置 http.Transport.ResponseHeaderTimeout=60s + DialContext=10s，http.Client.Timeout 保持 0。首字节超时触发时返回 APIError{StatusCode:0, Retryable:true, Message:"服务器响应超时（首字节超过 60s）"}，由 P2 重试逻辑处理
- **v6（配套实施记录）**：实施配套方案。agent.go MaxTurns 50→100；agent.go EventError 分支加 Warn 日志（含 status_code/retryable 字段）；stream.go HTTP 4xx/5xx 分支加 Warn 日志（body 截断 500 字符）；stream.go SSE 中断 / 空响应分支加 Warn 日志；stream.go 网络错误分支扩展为超时和非超时两类日志
- **v7（配套补充）**：app/chat.go L161 主对话 MaxTurns 50→100；app/chat.go L392 压缩路径 `MaxTurns: 50` 删除（Grep 确认 compress.go 不读 opts.MaxTurns，Compress 走 GenerateText 单次调用不进入 agent loop，原字段是死代码）
- **v8（用户日志实证分析）**：issue #26 用户上传完整 goink.log（8065 行，2026-07-21）。实证发现 3 个独立 bug（与 P0-P3 正交）：Bug A — Duplicate tool_call_id（GPT parallel tool calls 偶发分配相同 id + Goink 全链路 8 个环节无去重，导致对话彻底中断）；Bug B — read 工具切片越界 panic（AI 传 start_line > end_line 时 rw_tools.go:638 无校验直接切片，对比 edit 工具已有校验）；Bug C — reasoning_content 缺失（thinking 模式 + tool-call 场景触发 DeepSeek 400）。新增"用户日志实证分析"章节，含错误分布统计、全链路去重缺失分析、修复方案对比、优先级建议。所有修复均未动代码，待用户决定方案
- **v9（Bug A 深度分析 + 实施方案 1+3）**：深度分析 turn 340 的 7312 行 INSERT 实证，确认 GPT（xiangliang/gpt-5.3-codex 中转）在一次响应里发了 10 个不同 index 的 tool_calls（index 0-4 和 index 5-9 的 id/name/arguments 完全相同），导致主 agent 在一个 streamLoop 迭代内执行 10 次 tool（含 run_subagent 被执行两次共 130s）。排除 retry/subagent 合并/ctx 取消等其他路径。实施方案 1（stream.go:397-430 源头去重，维护 seenToolIDs map 跳过重复 id）+ 方案 3（app/chat.go loadAPIMessages 出口防御，assistant 去重 tool_calls + tool 消息跳过 orphan/重复）。方案 2（buildToolCalls 构造时去重）不推荐，因方案 1 实施后为死代码且不能避免 tool 副作用。go build + go test ./internal/... ./app/... 全部通过
- **v10（compress 路径评估 + 测试补充）**：评估压缩路径的 tool_call_id 去重覆盖情况。手动压缩入口（CompressContext）走 loadAPIMessages 已防御；自动压缩的 LLM 调用（generateSummary）用 opts.Messages（来自 Chat 的 loadAPIMessages）已防御；压缩后重新加载（compress.go:105-113）绕过防御，但触发条件窄（需 Bug A 脏数据被 retainMessages 保留），且 Bug A 源头已修，**接受不修（方案 D）**。新增 app/chat_test.go 单元测试覆盖 loadAPIMessages 的 5 个场景（正常无重复 / assistant.tool_calls 重复 id 去重 / orphan tool 消息跳过 / 重复 tool 消息只保留首次 / 跨 turn 不误判），go test 全部通过
- **v11（Bug C 根因定位 + 指向 v1.3.0）**：v1.3.0 多 Provider 适配文档已完成根因分析，Bug C 的根因不是 ToAPIFormat 未透传 reasoning_content，而是 stream.go:316 只解析 reasoning_content 不解析 reasoning，导致 GPT 模型生成的消息 ThinkingContent="" 被存入 DB，后续 ToAPIFormat 把空 ThinkingContent 透传成 reasoning_content=""，被中转站 vectorengine.cn 严格 schema 校验拒绝。更新 Bug C 章节（L529-546）为"根因已定位"，错误分布表（L381）和优先级表（L560）状态同步更新，指向 v1.3.0 文档。Bug C 由 v1.3.0 阶段 1 GPT 适配解决（stream.go 加 delta.reasoning 解析），v1.2.0 不修
- **v12（Bug A 根因订正 + Bug B 状态更新）**：根据日志 L7101-L7112 实证，turn 340（16:06:31）用的模型是 `xiangliang/gpt-5.3-codex`（GPT 中转），不是 DeepSeek。订正 Bug A 根因：重复 tool_call_id 是 GPT 偶发分配的，不是 DeepSeek；DeepSeek 只是后来拒绝接收含重复 id 的 message[74] 时报 400（DeepSeek 比 GPT 更严格地校验 tool_call_id 唯一性）。更新 L392/L425 根因描述 + v8/v9 修订历史。Bug B（read 工具越界）确认已修复（rw_tools.go:631-633 已加 `if start > end` 校验，返回业务错误 `"start_line(xxx) 不能大于 end_line(xxx)"`），更新错误分布表（L382）、Bug B 章节（L504/L510）、优先级表（L559）为已修复状态
