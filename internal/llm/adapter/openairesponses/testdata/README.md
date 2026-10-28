# Responses SSE 测试资料

`responses-tools.sse` 是按官方事件格式手工构造的本地 fixture，不是真实 API 录制，不含凭据、用户内容或有效推理密文。

回放经过固定版本 OpenAI Go SDK 的 SSE 读取器和本包 `Stream`，覆盖正文及 refusal、思考摘要、空 reasoning、交错工具参数、`call_id` 关联、usage 和完成状态。工具逆序结束，最终仍按 output 索引交付。reasoning 的 `added` 密文故意不完整，第一条 reasoning 的终态省略密文，验证回传使用真正收到的 `output_item.done`。

事件依据：[OpenAI Responses streaming events](https://developers.openai.com/api/reference/resources/responses/streaming-events)。fixture 默认以正常 EOF 结束；测试另覆盖 `[DONE]`、截断和读取错误。

## 无状态真实录制

`rig/` 保存 Rig 项目的第三方 HTTP 录制，固定提交 `96e53f41eb8a489b58bd00fb6212dbb75a0750c1`。原始 YAML 和 [MIT 许可](rig/LICENSE.txt)逐字节保留，固定来源及 SHA256 见 [sources.json](rig/sources.json)。录制方式见上游的[回放说明](https://github.com/0xPlaygrounds/rig/blob/96e53f41eb8a489b58bd00fb6212dbb75a0750c1/tests/README.md)；没有调用模型 API 或读取真实密钥。

[responses-stateless-tools.yaml](rig/responses-stateless-tools.yaml) 包含两轮 `/v1/responses` SSE：`gpt-5-mini` 在 `store=false` 下生成 reasoning 和两个无参工具调用；下一轮保留同一组工具，携带完整原生历史、密文和两个 `function_call_output`，返回 `The labels are crimson-harbor and silver-orchard.`。两轮都请求 `include: ["reasoning.encrypted_content"]`，不使用 `previous_response_id` 或服务端 conversation。

`recorded_stream_test.go` 经真实 SDK 和 Goink Stream 回放原始响应，检查出站无状态参数、include、工具定义及下一轮历史。首轮消息经过公共 Message JSON 往返，工具结果故意逆序提供；出站仍须按录制的调用顺序配对。测试还验证原始 call_id、正文、usage、完整工具组和 `output_item.done` 原文的保存与回传。

这份录制中，reasoning 在 `added` 时的密文不完整；`item.done` 和最终 `response.completed` 中的密文也不同。原始第二轮请求使用 `item.done` 的密文，与官方文档建议一致。测试保留这些差异，不能为通过校验而改写录制。Goink 允许终态 reasoning 的密文字段不同或缺失，仍校验其类型和其他字段，回放始终使用已完成的 `item.done`。

与原请求的差异：Goink 将顶层 `instructions` 表达为 input 中的 system 消息，用户文本使用 SDK 的字符串简写；工具结果省略可选的 `status=completed`。这些内容按语义对照，原生 reasoning / function_call 则逐项对比完整 JSON。Goink 额外显式发送 `background=false` 和默认输出上限；保留录制的原模型及工具参数，不宣称请求字节完全一致。回放只证明离线转换和历史续接，不验证原密文在当前服务端仍然有效。
