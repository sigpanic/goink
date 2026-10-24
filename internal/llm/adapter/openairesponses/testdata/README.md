# Responses SSE 测试资料

`responses-tools.sse` 是按官方事件格式手工构造的本地 fixture，不是真实 API 录制，不含凭据、用户内容或有效推理密文。

回放经过固定版本 OpenAI Go SDK 的 SSE 读取器和本包 `Stream`，覆盖正文及 refusal、思考摘要、空 reasoning、交错工具参数、`call_id` 关联、usage 和完成状态。工具逆序结束，最终仍按 output 索引交付。reasoning 的 `added` 密文故意不完整，第一条 reasoning 的终态省略密文，验证回传使用真正收到的 `output_item.done`。

事件依据：[OpenAI Responses streaming events](https://developers.openai.com/api/reference/resources/responses/streaming-events)。fixture 默认以正常 EOF 结束；测试另覆盖 `[DONE]`、截断和读取错误。
