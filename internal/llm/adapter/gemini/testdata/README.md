# Interactions SSE 回放

`interactions-tools.sse` 是 Goink 编写的本地合成样本，不是真实服务抓包，不含真实凭据、签名或小说内容。字段依据 [Interactions v1](https://ai.google.dev/api/interactions-api-v1) 和 [thinking 流式示例](https://ai.google.dev/gemini-api/docs/thinking#streaming-with-thinking)。

覆盖初始正文 / 摘要、增量文本、独立签名、无文本的图片摘要、交错的同名函数、JSON 参数分片、扩展字段、大整数、累计 usage 和 requires_action 终态。测试会通过已安装 SDK 的 Create、生成的事件解码器及公开 SSE 读取器校验格式。

旧 `internal/llm/testdata/sse/google/` 是 GenerateContent 资料，不能用作 Interactions 的协议样本。

## 真实录制回放

`rig/` 保存 Rig 项目发布的第三方 HTTP 录制，固定上游提交 `6eff50a2fd95de3a505000a0e39d2893d83c6f4d`。原始 YAML 和 [MIT 许可](rig/LICENSE.txt)逐字节保留，来源链接及 SHA256 见 [sources.json](rig/sources.json)。来源及录制方式见上游的[回放说明](https://github.com/0xPlaygrounds/rig/blob/6eff50a2fd95de3a505000a0e39d2893d83c6f4d/tests/README.md)；这些不是 Goink 自行调用 Google 得到的抓包。

新增的无状态非流式录制单独固定提交 `96e53f41eb8a489b58bd00fb6212dbb75a0750c1`；来源清单中的文件级 commit / retrieval_date 覆盖默认值，已有 SSE 资料不变。

| 文件 | 内容 |
| --- | --- |
| [interactions-tool-loop.yaml](rig/interactions-tool-loop.yaml) | 两份 HTTP 请求 / 响应：`add(17, 25)` 的原始调用 ID、参数增量、签名及 requires_action；工具结果提交后返回正文 `42` 和 completed。 |
| [interactions-signature-only.yaml](rig/interactions-signature-only.yaml) | 禁用思考摘要时仍返回独立 thought_signature，随后正文 `11`、usage 和 completed。 |
| [interactions-stateless-unary.yaml](rig/interactions-stateless-unary.yaml) | 两轮 `store=false`、`stream=false` 的原始 JSON 响应，均省略 `id`；第一轮回答 `289`，第二轮携带完整历史和原始签名回答 `290`。 |

`recorded_stream_test.go` 使用已有 YAML 依赖直接读取每个响应的 SSE body，经过真实 SDK Create、生成的事件解码器和 Goink Stream；不替换工具名、调用 ID、签名、正文或 token 计数。测试验证正文 / 工具 / 终态、真实 usage 的归一化，以及工具响应 → 公共 Message JSON 往返 → 原生 steps 和签名重传 → 第二轮 Stream。

录制请求属于 `/v1beta/interactions`，未记录 Api-Revision。测试分别向本地替身的 `/v1beta/interactions` 与 `/v1/interactions` 发请求，明确使用当前 SDK v1.75.0 及其 Api-Revision: 2026-05-20，验证端点透传与已录制响应的解码兼容；不证明真实 v1 服务与录制环境完全等价，也不能离线验证签名在上游仍然有效。

原始请求中的 `temperature`、`store=true` 或省略 store、`thinking_summaries=none` 不作为 Goink 出站模板。Goink 测试请求沿用当前公共消息 / 工具契约和无状态策略，显式 store=false、background=false，不发送 previous_interaction_id；工具结果按公共文本结果编码。因此这是原始响应回放和 Goink 续接测试，不是原始请求字节完全匹配或真实服务验收。原始 YAML 不因测试通过与否而修改。

## 无状态非流式回归

`recorded_generate_test.go` 通过真实 SDK Create 和 Goink Generate 回放原始 JSON，验证缺失 ID、正文、思考摘要、包含 thought 的 usage、完整终态和原生 steps。第一轮结果经过公共 Message JSON 往返后继续第二轮；两轮出站 input 逐项与录制请求对比，防止签名、正文或历史顺序改变。

请求保持原模型和 `max_output_tokens=2048`，显式 `store=false`、`stream=false`、`background=false`，不发送 `previous_interaction_id`。录制中的历史型号 `gemini-3-flash-preview` 已不在当前内置思考策略中，因此不强行重建原请求的 `thinking_level=low` / `thinking_summaries=auto`。本地请求使用当前 `/v1/interactions` 及 SDK revision，原录制来自 `/v1beta/interactions`；测试验证这些原始响应的转换和无状态历史续接，不证明当前型号、生成配置和线上 v1 完全等价，也不把 JSON 改装成 SSE。
