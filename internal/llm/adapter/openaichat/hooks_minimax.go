package openaichat

// minimaxBuildRequest 适配 MiniMax 与标准 OpenAI 格式的差异：
// - reasoning_split 将思考内容从 <think> 标签分离到 reasoning_content 字段
// - thinking.type 从 "enabled" 映射为 MiniMax 用的 "adaptive"
func minimaxBuildRequest(payload map[string]any) map[string]any {
	payload["reasoning_split"] = true

	if thinking, ok := payload["thinking"].(map[string]string); ok && thinking["type"] == "enabled" {
		thinking["type"] = "adaptive"
	}

	return payload
}
