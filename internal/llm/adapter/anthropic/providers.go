package anthropic

import "github.com/sigpanic/goink/internal/llm/adapter"

// builtin 是内置实例而非类型模板；配置层按 Name 合并用户覆盖，APIKey 默认留空。
var builtin = adapter.ProviderMap{
	"Claude": {
		Name:         "Claude",
		ProviderType: "anthropic",
		ChatURL:      "https://api.anthropic.com/v1/messages",
		PlatformURL:  "https://platform.claude.com",
		HelpText:     "在 Claude Console 创建 API Key。中转服务请填写完整的 Messages 接口地址，保留服务商要求的路径和查询参数。",
		// 2026-10-19：模型规格与 effort 取自官方文档；首项为默认推理等级。
		// https://platform.claude.com/docs/en/models/overview
		// https://platform.claude.com/docs/en/build-with-claude/effort
		// 温度保持未指定；具体 thinking、effort 及工具选择限制由后续请求构建处理。
		Models: []adapter.ModelInfo{
			{
				ID:               "claude-opus-5-5",
				Name:             "Claude Opus 5.5",
				ContextWindow:    1_000_000,
				MaxOutputTokens:  128_000,
				SupportsThinking: true,
				ReasoningLevels:  []string{"medium", "low", "high", "xhigh", "max"},
				SupportsVision:   true,
			},
			{
				ID:               "claude-sonnet-5-5",
				Name:             "Claude Sonnet 5.5",
				ContextWindow:    1_000_000,
				MaxOutputTokens:  128_000,
				SupportsThinking: true,
				ReasoningLevels:  []string{"high", "low", "medium", "xhigh", "max"},
				SupportsVision:   true,
			},
			{
				ID:               "claude-haiku-5-5",
				Name:             "Claude Haiku 5.5",
				ContextWindow:    1_000_000,
				MaxOutputTokens:  128_000,
				SupportsThinking: true,
				ReasoningLevels:  []string{"medium", "low", "high", "xhigh", "max"},
				SupportsVision:   true,
			},
			{
				ID:               "claude-fable-5-1",
				Name:             "Claude Fable 5.1",
				ContextWindow:    1_000_000,
				MaxOutputTokens:  128_000,
				SupportsThinking: true,
				ReasoningLevels:  []string{"high", "low", "medium", "xhigh", "max"},
				SupportsVision:   true,
			},
		},
	},
}

// BuiltinProviders 返回内置实例的独立副本，供根包配置层合并；本包不自行补入默认实例。
func BuiltinProviders() adapter.ProviderMap {
	providers := make(adapter.ProviderMap, len(builtin))
	for name, p := range builtin {
		providers[name] = p.Clone()
	}
	return providers
}
