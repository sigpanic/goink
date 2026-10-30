# Claude / Responses 公共流式回放

从仓库根目录运行：

```sh
go test ./internal/llm/adapter -run 'Test(Recorded|GoinkToolReplay)' -count=1
```

公共回放测试位于 `internal/llm/adapter/*recorded*_test.go` 和 `goink_tool_stream_test.go`，使用外部 `adapter_test` 包调用两套真实 SDK 与 Goink 适配器。HTTP transport 只返回本地数据，不访问模型、读取 API Key、执行工具或修改小说。每次 body Read 最多返回 7 字节，覆盖事件、JSON 转义及中文 UTF-8 在读取边界被拆开的情况；这不是原录制的网络分片。

## 原始来源

各文件的固定提交链接、SHA256、资料类型及许可证见 `sources.json`。上游数据与许可逐字节保存；`TestRecordedStreamSources` 核对校验和并拒绝未登记的资料。五组资料于 2026-10-30 获取，不能视为对当前所有模型或中转站的在线验证。

| 目录 | 内容 | 回放覆盖 |
| --- | --- | --- |
| `llm/` | Simon Willison LLM 的两轮 VCR cassette，响应为 gzip 二进制 YAML | GPT-5.5，`store=false`、`stream=true`、非空 multiply 参数的 11 个增量、结果 `2869461` 回传及最终正文；请求包含 encrypted_content include，但这组响应没有 reasoning item |
| `grafana/` | Grafana AI SDK 的 reasoning-tool-use conformance 录制，含配置、实际请求快照、两轮事件 JSONL | o4-mini，`store=false`、`summary=auto`、Portland 工具参数、密文续接、第二轮 74 个推理摘要增量、usage |
| `vercel/` | Vercel AI SDK 的 encrypted-content provider 事件 fixture | GPT-5.1 Codex Max，四段 response、32 个摘要增量、三次带参数的 calculator 调用、added/done 不同的密文及最后正文 |
| `pydantic/` | Pydantic AI 的两轮 cassetter 录制 | Sonnet 5.5，adaptive + summarized、effort=max，signed thinking、无参工具、结果回传后继续 thinking 与回答 |
| `goodall/` | Goodall 的两轮原始请求 JSON 和 SSE；上游标注于 2026-10-28 录制 | Sonnet 5.5，adaptive + summarized、effort=high，非空 get_weather 参数、签名、缓存输入及 thinking token usage、续接正文 |

LLM、Grafana、Vercel 保留 Apache 2.0 许可，Grafana 同时保留上游 NOTICE。Pydantic AI 和 Goodall 保留 MIT 许可。

## 请求对照与已知差异

有完整请求的录制验证工具定义、结果 ID/正文以及下一轮历史；每轮 assistant 消息经过公共 Message JSON 保存/恢复，再走适配器请求构造。原生 reasoning/content 必须按收到的完成状态回传；签名及密文不出现在展示文本中。Claude 的正文和摘要还核对固定 SHA256，usage 与录制值核对，包含缓存与 thinking 明细。

- Responses 每轮都核对 Goink 实际出站的 `store=false`、`background=false`、`stream=true`、`include=["reasoning.encrypted_content"]`，且不得使用 `previous_response_id` 或 conversation。Goink 将 instructions 表达为 system 消息；文本可能使用 SDK 字符串简写。
- 真实录制保留原模型 ID。Goink 对不在当前内置策略中的历史模型不强塞 effort/summary，所以 LLM/Grafana/Vercel 回放不宣称请求逐字节相同；派生场景另使用当前内置 GPT-6.1 Sol 并断言 `summary=auto`。默认输出上限也可能与旧录制不同。
- Grafana 保存的是逐行事件 JSON，测试增加 SSE `data:` 封装。其工具结果是 JSON 对象，Goink 使用内容相同的 JSON 文本；Goink 保留完整 output_item.done，包括上游续接请求省略的 content、function item id 等可选字段。首轮 reasoning 密文与上游第二轮请求一致。
- Vercel 文件包含四段完成的响应事件，但没有原始请求。测试按 response.created 拆成四次独立响应，补入本地 calculator 结果 19、57、570；这只能证明 Goink 的离线转换和续接，不能冒充完整 HTTP 录制。文件记录 summary=detailed，上游当前测试构造 summary=auto，两者不混同。
- Pydantic 的 URI 带 `?beta=true`，其录制 hooks 会过滤 `anthropic-*` 等头部并规范化部分 Unicode。不能从资料中证明原 beta 头部为空。回放走 Goink 配置的标准 Messages 路径。
- Goodall 原请求含顶层 `cache_control`，Goink 当前没有该设置；回放保留响应缓存 usage，未宣称发送了缓存配置。
- 两份 Claude 上游续接请求省略 tool_use.caller，响应含 `caller={"type":"direct"}`。测试严格对照上游实际回传的字段，同时要求 Goink 保留收到的完整原生块；不为对齐请求删 caller 或重写签名。

## Goink read/edit 派生场景

`TestGoinkToolReplayReadOutlineAndEdit` 在内存中借用 Vercel / Goodall 的事件排列，构造 read 正文 → read 大纲 → edit 全文 → 最终中文回复。字段按当前 ReadArgs/EditArgs 取值，schema 只保留用于本场景的字段，不复制无关描述。测试不调用真实工具实现。

派生时替换工具名、参数、结果、最终正文和必要的工具/消息 ID，工具参数覆盖 include_lines、行范围、中文长正文、换行、引号及反斜线。参数按原事件数量分片，7 字节 HTTP 读取再独立拆分 UTF-8。真实签名/密文换为 `synthetic-offline-*` 占位，原始录制文件保持不动；派生场景不能作为服务端验签证据。

每轮核对累计参数、完整 JSON、call_id/tool_use_id、工具结果文本、原生历史和消息 JSON 恢复。`TestGoinkToolReplayTruncatedEditCannotExecute` 删除 edit 响应的协议终态，要求报告错误且不发布可执行调用或完成状态。token 数沿用原事件，仅原始录制测试核验其记录值，不将派生 token 数解释为修改后请求的真实消耗。

## 与已有测试的分工

公共入口还直接读取已有 `openairesponses/testdata/rig/responses-stateless-tools.yaml` 和 `internal/llm/testdata/sse/anthropic/*.sse`，不复制这些文件。老 Claude 目录既有真实录制，也有官方 SDK 手工 fixture，沿用各自来源说明。这里统一 SDK 读取、usage/state/工具发布顺序及续接检查；原协议包的非法参数、取消、未知事件、HTTP 错误和细节回归仍留在各包。

本组验证范围是 HTTP/SSE → SDK → 适配器 → Message JSON → 下一轮请求，不包含 Agent、数据库、Wails UI 或真实服务端签名验证。
