# Goink 原生协议 SSE 回放样本

2026-10-13 获取，2026-10-15 迁入 `internal/llm/testdata/sse/`。当前包含 13 条 SSE 响应和 1 个 HTTP 404 响应。全部来自官方公开 SDK 测试资料；整理和迁入均没有调用真实模型 API，也没有读取本地 API Key。本目录只包含回放资料，尚未接入 SDK 或 Goink 自动化测试。

样本按供应商分目录；各自的许可证和来源清单随样本保存。`validation.json` 中的路径相对于本目录，各来源清单中的路径相对于清单所在目录。

```text
sse/
├── README.md
├── validation.json
├── anthropic/    # 6 条 SSE、来源清单、MIT 许可证
└── google/       # 7 条 SSE、响应分段 JSON、404 响应、来源清单、Apache 2.0 许可证
```

## 来源与转换

| 来源 | 固定 revision | 样本 |
|---|---|---|
| [Anthropic Go SDK](https://github.com/anthropics/anthropic-sdk-go/tree/bdcae90baad7d21949fc694f779377f62ff68631/toolrunner/testdata/cassettes) | `bdcae90baad7d21949fc694f779377f62ff68631` | 从官方 VCR cassette 提取 3 条 SSE body |
| [Anthropic Go SDK 测试 fixture](https://github.com/anthropics/anthropic-sdk-go/tree/bdcae90baad7d21949fc694f779377f62ff68631/lib/betafallback/testdata/fallbackstream) | 同上 | 直接取得 3 条 SDK 编写的 `.sse` fixture |
| [Google JS SDK 系统测试录制](https://github.com/googleapis/js-genai/tree/1693f7cce750e5e9c3615e5a90b4602029cb0979/test/system/recordings) | `1693f7cce750e5e9c3615e5a90b4602029cb0979` | 7 条 SSE 和 1 个 HTTP 错误响应 |

Anthropic cassette 中的响应字符串按 YAML 定义提取，保留内容和事件排列，不带请求头。SDK 编写的 fixture 不是本次真实 API 录制。

Google 官方录制保存的是 `response.bodySegments` JSON 对象，未保存原始 SSE 字节或 TCP 分片。每个 `.sse` 由原始 JSON 分段按顺序转换为 `data: <JSON>\n\n`，不增加 `[DONE]`，不更改 JSON 字段值。每条响应还保留包含状态码、原始分段和修改声明的 `.json`。这些样本可以测试 SDK 解析和后续 Goink event 转换，但不能验证原始空白、传输字节边界或时间特征。

Google 的错误样本是 HTTP 404 JSON，保留为 `google/gemini-error-01.json`，不能按 200 成功 SSE 返回。

Google 每个 `.sse` 的头部注释行和每个响应 `.json` 的 `_fixture_notice` 字段记录 Goink 的提取、重封装修改、上游来源和许可证。SSE 注释不属于 `data:` 事件；JSON 的声明在 `body_segments` 外，不属于 API 响应，回放时仅使用 `status_code` 和 `body_segments`。Google 来源清单和校验记录中的文件大小、SHA-256 均包含这些声明。

每条样本的来源文件、固定链接、源文件 SHA-256、响应序号、样本 SHA-256、状态及事件 / 分段摘要见：

- [anthropic-sources.json](./anthropic/anthropic-sources.json)
- [google-sources.json](./google/google-sources.json)

## 回放入口

| 文件 | 覆盖内容 |
|---|---|
| [anthropic-tool_runner_next_streaming-01.sse](./anthropic/anthropic-tool_runner_next_streaming-01.sse) | 文本、ping、工具声明、JSON 参数增量、usage 和正常结束 |
| [anthropic-tool_runner_streaming_all-01.sse](./anthropic/anthropic-tool_runner_streaming_all-01.sse) | 第一轮工具调用 |
| [anthropic-tool_runner_streaming_all-02.sse](./anthropic/anthropic-tool_runner_streaming_all-02.sse) | 同份 cassette 中工具结果后的最终文本回复 |
| [anthropic-fallback-end-01.sse](./anthropic/anthropic-fallback-end-01.sse) | SDK 编写的最终文本 fixture |
| [anthropic-fallback-tooluse-01.sse](./anthropic/anthropic-fallback-tooluse-01.sse) | SDK 编写的工具调用 fixture |
| [anthropic-refusal-01.sse](./anthropic/anthropic-refusal-01.sse) | SDK 编写的 refusal fixture，包含 thinking / thinking_delta / signature_delta；它的测试签名不能作为真实服务端验签证据 |
| [gemini-text-01.sse](./google/gemini-text-01.sse) | 11 个响应分段、文本输出和 usage |
| [gemini-tools-01.sse](./google/gemini-tools-01.sse)、[gemini-tools-02.sse](./google/gemini-tools-02.sse) | 同份录制中的工具响应；本样本的工具参数为完整对象，不能据此假定参数一定增量下发 |
| [gemini-tool-loop-01.sse](./google/gemini-tool-loop-01.sse)、[gemini-tool-loop-02.sse](./google/gemini-tool-loop-02.sse) | 工具调用与工具执行后的文本回复；不使用 SDK 的自动执行功能 |
| [gemini-generation-config-01.sse](./google/gemini-generation-config-01.sse)、[gemini-chat-config-01.sse](./google/gemini-chat-config-01.sse) | 带调用配置的公开测试录制响应 |
| [gemini-error-01.json](./google/gemini-error-01.json) | HTTP 404 状态和错误 JSON |

后续实现时，用 `httptest.Server` 按样本记录的 HTTP 状态响应。SSE 样本设置 `Content-Type: text/event-stream`，将文件 body 写入 response；错误样本返回 JSON。实际 SDK 使用假 key 和本地 BaseURL，从这条完整 HTTP 路径解析，再由适配器映射为 Goink `StreamEvent`。

同一源文件的响应序号保留了多次调用顺序，但当前目录没有保存完整请求或生成 Goink 工具结果。后续多轮测试应从官方源资料提取安全的预期请求字段并定义本地工具结果，不能仅轮流返回文件就宣称历史回传正确。

## 当前验证与缺口

已逐帧检查 SSE data JSON、响应边界、工具参数 JSON 和本地来源校验和；验证结果见 [validation.json](./validation.json)。本次没有安装 SDK 或执行 Goink event 映射测试。

本组的 Anthropic refusal fixture 含 thinking 和 signature 增量，可用于签名字段的解析、保存与回传测试；当前没有 redacted_thinking，也没有 Gemini thoughtSignature。不覆盖签名在真实服务端是否有效。已确认 Google Go SDK 的回放读取依赖 `GOOGLE_GENAI_REPLAYS_DIRECTORY`，本次检查的该仓库公开文件目录没有配套响应录制，所以转用了官方 JS SDK 的同协议录制数据。

签名、同名并行调用、半途断流、取消、非法参数和末尾空文本 part 等案例还需补充。可按官方协议构造并明确标注测试样本，验证解析、保存和回传；真正的服务端验签仍需使用相同模型、账户及历史前缀的真实请求烟测。公开录制中的旧模型和 SDK fixture 不代表当前所有型号或中转站行为。

## 许可

Anthropic SDK 资料随 [ANTHROPIC-LICENSE.txt](./anthropic/ANTHROPIC-LICENSE.txt) 保留 MIT 许可。Google SDK 资料随 [GOOGLE-LICENSE.txt](./google/GOOGLE-LICENSE.txt) 保留 Apache 2.0 许可。迁入时已保留来源、校验信息和许可说明；后续按实际选定 SDK 版本复核样本。
