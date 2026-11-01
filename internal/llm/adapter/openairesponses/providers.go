package openairesponses

import "github.com/sigpanic/goink/internal/llm/adapter"

// builtin 是内置实例，按 Name 合并用户覆盖；行为类型不充当实例索引。
var builtin = adapter.ProviderMap{
	"ChatGPT": {
		Name:         "ChatGPT",
		ProviderType: "openai",
		ChatURL:      "https://api.openai.com/v1/responses",
		PlatformURL:  "https://platform.openai.com",
		HelpText:     "在 OpenAI Platform 创建 API Key。中转服务请填写完整的 Responses 接口地址，保留所需路径和查询参数。",
		// 2026-10-23：规格和推理等级按精确模型 ID 查证，首项为 Goink 默认等级。
		// https://developers.openai.com/api/docs/models/gpt-6-astra
		// https://developers.openai.com/api/docs/models/gpt-6.1-sol
		// https://developers.openai.com/api/docs/models/gpt-6-luna
		// https://developers.openai.com/api/docs/models/gpt-6-sol
		// https://developers.openai.com/api/docs/models/gpt-5.6-sol
		// https://developers.openai.com/api/docs/models/gpt-5.6-terra
		// https://developers.openai.com/api/docs/models/gpt-5.6-luna
		// https://developers.openai.com/api/docs/models/gpt-5.6-cyber
		Models: []adapter.ModelInfo{
			{
				ID:               "gpt-6-astra",
				Name:             "GPT-6 Astra",
				ContextWindow:    1_050_000,
				MaxOutputTokens:  128_000,
				SupportsThinking: true,
				ReasoningLevels:  []string{"medium", "low", "high", "xhigh", "max"},
				SupportsVision:   true,
			},
			{
				ID:               "gpt-6.1-sol",
				Name:             "GPT-6.1 Sol",
				ContextWindow:    1_050_000,
				MaxOutputTokens:  128_000,
				SupportsThinking: true,
				ReasoningLevels:  []string{"medium", "low", "high", "xhigh", "max"},
				SupportsVision:   true,
			},
			{
				ID:               "gpt-6-luna",
				Name:             "GPT-6 Luna",
				ContextWindow:    1_050_000,
				MaxOutputTokens:  128_000,
				SupportsThinking: true,
				ReasoningLevels:  []string{"medium", "none", "low", "high", "xhigh", "max"},
				SupportsVision:   true,
			},
			{
				ID:               "gpt-6-sol",
				Name:             "GPT-6 Sol",
				ContextWindow:    1_050_000,
				MaxOutputTokens:  128_000,
				SupportsThinking: true,
				ReasoningLevels:  []string{"medium", "none", "low", "high", "xhigh", "max"},
				SupportsVision:   true,
			},
			{
				ID:               "gpt-5.6-sol",
				Name:             "GPT-5.6 Sol",
				ContextWindow:    1_050_000,
				MaxOutputTokens:  128_000,
				SupportsThinking: true,
				ReasoningLevels:  []string{"medium", "none", "low", "high", "xhigh", "max"},
				SupportsVision:   true,
			},
			{
				ID:               "gpt-5.6-terra",
				Name:             "GPT-5.6 Terra",
				ContextWindow:    1_050_000,
				MaxOutputTokens:  128_000,
				SupportsThinking: true,
				ReasoningLevels:  []string{"medium", "none", "low", "high", "xhigh", "max"},
				SupportsVision:   true,
			},
			{
				ID:               "gpt-5.6-luna",
				Name:             "GPT-5.6 Luna",
				ContextWindow:    1_050_000,
				MaxOutputTokens:  128_000,
				SupportsThinking: true,
				ReasoningLevels:  []string{"medium", "none", "low", "high", "xhigh", "max"},
				SupportsVision:   true,
			},
			// Cyber 需 Daybreak 单独开通；官方未公布 effort 枚举，不预填推理等级。
			{
				ID:               "gpt-5.6-cyber",
				Name:             "GPT-5.6 Cyber",
				ContextWindow:    400_000,
				MaxOutputTokens:  128_000,
				SupportsThinking: true,
				SupportsVision:   true,
			},
		},
	},
}

// BuiltinProviders 返回内置实例的深拷贝；运行时构造不隐式启用这些实例。
func BuiltinProviders() adapter.ProviderMap {
	providers := make(adapter.ProviderMap, len(builtin))
	for name, p := range builtin {
		providers[name] = p.Clone()
	}
	return providers
}
