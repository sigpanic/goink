package openaichat

// qwenBuildRequest 将标准 OpenAI thinking 参数转换为 Qwen 的 enable_thinking。
// Qwen 非流式请求开启 enable_thinking 会报错，因此非流式时显式关闭。
func qwenBuildRequest(payload map[string]any) map[string]any {
	thinkingType := ""
	if t, ok := payload["thinking"].(map[string]string); ok {
		thinkingType = t["type"]
	}
	delete(payload, "thinking")
	delete(payload, "reasoning_effort")

	isStream, _ := payload["stream"].(bool)

	switch thinkingType {
	case "enabled":
		if isStream {
			payload["enable_thinking"] = true
		} else {
			payload["enable_thinking"] = false
		}
	case "disabled":
		payload["enable_thinking"] = false
	}

	return payload
}
