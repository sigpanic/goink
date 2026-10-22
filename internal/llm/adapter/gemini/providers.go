package gemini

import "github.com/sigpanic/goink/internal/llm/adapter"

// builtin 是内置实例而非类型模板；配置层按 Name 合并覆盖，密钥和温度默认留空。
var builtin = adapter.ProviderMap{
	"Google": {
		Name:         "Google",
		ProviderType: "google",
		ChatURL:      "https://generativelanguage.googleapis.com/v1/interactions",
		PlatformURL:  "https://aistudio.google.com/apikey",
		HelpText:     "在 Google AI Studio 创建 API Key。中转服务需支持 Interactions，并填写完整接口地址。",
		// 2026-10-22：精确模型 ID、输入 / 输出限制和能力来自官方模型页。
		// https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash
		// 其余规格见 /docs/models/<模型 ID>；支持范围及推理等级另见：
		// https://ai.google.dev/gemini-api/docs/interactions-overview#supported-models-agents
		// https://ai.google.dev/gemini-api/docs/thinking#controlling-thinking
		// 首项为默认推理等级；2.5 Pro / Flash 未公布固定默认等级，Goink 选择 medium。
		// 2.5 Flash-Lite 默认关闭推理，low 是显式开启后的首选等级，由后续请求层区分。
		// Argon 的窗口为输入输出合计，输出上限取当前前台调用的 64K，不启用 long decode。
		Models: []adapter.ModelInfo{
			{
				ID:               "gemini-4-argon",
				Name:             "Gemini 4 Argon",
				ContextWindow:    1_048_576,
				MaxOutputTokens:  65_536,
				SupportsThinking: true,
				ReasoningLevels:  []string{"medium", "low", "high"},
				SupportsVision:   true,
			},
			{
				ID:               "gemini-3.8-flash",
				Name:             "Gemini 3.8 Flash",
				ContextWindow:    1_048_576,
				MaxOutputTokens:  65_536,
				SupportsThinking: true,
				ReasoningLevels:  []string{"medium", "low", "high"},
				SupportsVision:   true,
			},
			{
				ID:               "gemini-3.6-flash",
				Name:             "Gemini 3.6 Flash",
				ContextWindow:    1_048_576,
				MaxOutputTokens:  65_536,
				SupportsThinking: true,
				ReasoningLevels:  []string{"medium", "minimal", "low", "high"},
				SupportsVision:   true,
			},
			{
				ID:               "gemini-3.5-flash-lite",
				Name:             "Gemini 3.5 Flash-Lite",
				ContextWindow:    1_048_576,
				MaxOutputTokens:  65_536,
				SupportsThinking: true,
				ReasoningLevels:  []string{"minimal", "low", "medium", "high"},
				SupportsVision:   true,
			},
			{
				ID:               "gemini-3.1-pro-preview",
				Name:             "Gemini 3.1 Pro Preview",
				ContextWindow:    1_048_576,
				MaxOutputTokens:  65_536,
				SupportsThinking: true,
				ReasoningLevels:  []string{"high", "low", "medium"},
				SupportsVision:   true,
			},
			{
				ID:               "gemini-3.1-flash-lite",
				Name:             "Gemini 3.1 Flash-Lite",
				ContextWindow:    1_048_576,
				MaxOutputTokens:  65_536,
				SupportsThinking: true,
				ReasoningLevels:  []string{"minimal", "low", "medium", "high"},
				SupportsVision:   true,
			},
			{
				ID:               "gemini-3-flash-preview",
				Name:             "Gemini 3 Flash Preview",
				ContextWindow:    1_048_576,
				MaxOutputTokens:  65_536,
				SupportsThinking: true,
				ReasoningLevels:  []string{"high", "minimal", "low", "medium"},
				SupportsVision:   true,
			},
			{
				ID:               "gemini-2.5-pro",
				Name:             "Gemini 2.5 Pro",
				ContextWindow:    1_048_576,
				MaxOutputTokens:  65_536,
				SupportsThinking: true,
				ReasoningLevels:  []string{"medium", "low", "high"},
				SupportsVision:   true,
			},
			{
				ID:               "gemini-2.5-flash",
				Name:             "Gemini 2.5 Flash",
				ContextWindow:    1_048_576,
				MaxOutputTokens:  65_536,
				SupportsThinking: true,
				ReasoningLevels:  []string{"medium", "low", "high"},
				SupportsVision:   true,
			},
			{
				ID:               "gemini-2.5-flash-lite",
				Name:             "Gemini 2.5 Flash-Lite",
				ContextWindow:    1_048_576,
				MaxOutputTokens:  65_536,
				SupportsThinking: true,
				ReasoningLevels:  []string{"low", "medium", "high"},
				SupportsVision:   true,
			},
		},
	},
}

// BuiltinProviders 返回独立快照；本包不会在 New 时自动激活内置实例。
func BuiltinProviders() adapter.ProviderMap {
	providers := make(adapter.ProviderMap, len(builtin))
	for name, p := range builtin {
		providers[name] = p.Clone()
	}
	return providers
}
