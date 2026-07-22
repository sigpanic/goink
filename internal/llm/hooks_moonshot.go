package llm

import "strings"

// moonshotBuildRequest 适配 Kimi 开放平台与标准 OpenAI 格式的差异：
// - temperature 等参数 K2/K3 系列均有固定值，传入其他值会报错，需移除
// - K2 系列：不使用 reasoning_effort，通过 thinking.type 控制思考
//   - k2.6/k2.5 不传 thinking → 服务端默认开启，无需干涉
//   - kimi-k2.7-code 始终思考，不应传入 thinking 参数
//
// - K3 系列：始终思考，使用顶层 reasoning_effort 控制（当前仅支持 "max"）
//   - 不要使用 K2.x 的 thinking 参数（官方明确禁止）
//   - reasoning_effort 保留（由 buildPayload 根据 ReasoningLevels 注入）
//   - max_tokens 官方已 deprecated，rename 为 max_completion_tokens
func moonshotBuildRequest(payload map[string]any) map[string]any {
	delete(payload, "temperature") // K2/K3 temperature 均固定

	model, _ := payload["model"].(string)

	if strings.HasPrefix(model, "kimi-k3") {
		// K3 分支：禁用 K2.x 的 thinking 参数，保留 reasoning_effort
		delete(payload, "thinking")
		// max_tokens 官方已 deprecated，rename 为 max_completion_tokens
		if mt, ok := payload["max_tokens"]; ok {
			payload["max_completion_tokens"] = mt
			delete(payload, "max_tokens")
		}
		return payload
	}

	// K2 系列分支：不使用 reasoning_effort，按模型处理 thinking
	delete(payload, "reasoning_effort")
	if strings.HasPrefix(model, "kimi-k2.7-code") {
		delete(payload, "thinking")
	}

	return payload
}
