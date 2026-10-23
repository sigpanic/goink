# Interactions SSE 回放

`interactions-tools.sse` 是 Goink 编写的本地合成样本，不是真实服务抓包，不含真实凭据、签名或小说内容。字段依据 [Interactions v1](https://ai.google.dev/api/interactions-api-v1) 和 [thinking 流式示例](https://ai.google.dev/gemini-api/docs/thinking#streaming-with-thinking)。

覆盖初始正文 / 摘要、增量文本、独立签名、无文本的图片摘要、交错的同名函数、JSON 参数分片、扩展字段、大整数、累计 usage 和 requires_action 终态。测试会通过已安装 SDK 的 Create、生成的事件解码器及公开 SSE 读取器校验格式。

旧 `internal/llm/testdata/sse/google/` 是 GenerateContent 资料，不能用作 Interactions 的协议样本。
