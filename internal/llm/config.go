package llm

import (
	"sort"
	"strings"
)

// normalizeURL 补全 https:// 协议头，并确保路径以 /chat/completions 结尾。
// 修复两个 bug：
//   - Bug A：末尾斜杠导致 /chat/completions 重复拼接（如 https://x.com/v1/chat/completions/
//     会被旧逻辑拼成 https://x.com/v1/chat/completions/chat/completions，触发中转站 404）
//   - Bug B：裸域名（如 https://x.com）不补 /v1，导致请求落到 https://x.com/chat/completions
//     被中转站返回 404 HTML，parseSSE 找不到 data 行，最终触发 empty sse response
func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	// 先去末尾斜杠，避免 HasSuffix 误判（修复 Bug A）
	raw = strings.TrimRight(raw, "/")

	// 已以 /chat/completions 结尾，不再追加
	if strings.HasSuffix(raw, "/chat/completions") {
		return raw
	}

	// 拆解 scheme://host/path，仅在 path 为空（裸域名）时补 /v1
	// 修复 Bug B：用户填 https://x.com（裸域名）时补成 https://x.com/v1/chat/completions
	// 用户填 https://x.com/v1 或 https://x.com/api/openai 等带路径的，不擅自补 /v1（避免误伤自定义路径）
	if idx := strings.Index(raw, "://"); idx >= 0 {
		hostPart := raw[idx+3:]
		if !strings.Contains(hostPart, "/") {
			return raw + "/v1/chat/completions"
		}
	}

	return raw + "/chat/completions"
}

// UserLLMConfig 是用户 LLM 配置的持久化格式（加密 JSON）。
// 一个 provider 一个 key，内置模型随代码自动更新，自定义模型需完整填写信息。
type UserLLMConfig struct {
	Providers []Provider `json:"providers"`
}

// AvailableModel 是前端下拉列表的模型选项。
type AvailableModel struct {
	Key              string // "deepseek/deepseek-v4-pro"
	ProviderName     string // "DeepSeek"
	ModelName        string // "DeepSeek V4 Pro"
	ContextWindow    int
	MaxOutputTokens  int
	SupportsThinking bool
	ReasoningLevels  []string
	SupportsVision   bool
}

// Builtin 内置 provider 模板。APIKey 留空，运行时由用户配置注入。
func floatPtr(f float64) *float64 { return &f }
func derefOrZero(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// Merge 合并内置模板和用户配置，返回组装完成的 Provider map。
// 内置模型全量加入（随代码版本），用户自定义模型去重追加。只产出用户配置过的 Provider。
func Merge(builtin map[string]Provider, user *UserLLMConfig) map[string]Provider {
	result := make(map[string]Provider, len(user.Providers))

	for _, up := range user.Providers {
		bp, isBuiltin := builtin[up.Name]

		p := Provider{Name: up.Name, ChatURL: up.ChatURL, APIKey: up.APIKey}

		if isBuiltin {
			if p.ChatURL == "" {
				p.ChatURL = bp.ChatURL
			}
			if p.Temperature == nil {
				p.Temperature = bp.Temperature
			}
			p.BuildRequest = bp.BuildRequest
			p.BuildHeaders = bp.BuildHeaders
			p.ParseError = bp.ParseError

			// 内置模型全量加入（随代码版本自动更新）
			p.Models = append([]ModelInfo{}, bp.Models...)
		}

		// 用户自定义模型去重追加
		for _, um := range up.Models {
			if !modelExists(p.Models, um.ID) {
				p.Models = append(p.Models, um)
			}
		}

		result[up.Name] = p
	}

	return result
}

// LLMConfigView 是 GetLLMConfig 返回的完整配置视图，合并内置模板和用户配置。
type LLMConfigView struct {
	Providers []ProviderView `json:"providers"`
}

// ProviderView 是单个 provider 的前端展示视图。
type ProviderView struct {
	Key           string      `json:"key"`            // provider 标识符，如 "deepseek"
	Name          string      `json:"name"`           // 显示名称
	ChatURL       string      `json:"chat_url"`       // API 端点
	APIKey        string      `json:"api_key"`        // 用户配置的 key，空表示未配置
	PlatformURL   string      `json:"platform_url"`   // 注册平台链接
	HelpText      string      `json:"help_text"`      // 注册引导文字
	Temperature   float64     `json:"temperature"`    // 默认创意度 0~2
	Source        string      `json:"source"`         // "builtin" | "custom"
	BuiltinModels []ModelInfo `json:"builtin_models"` // 内置模型，自定义 provider 为 nil
	CustomModels  []ModelInfo `json:"custom_models"`  // 用户添加的自定义模型
}

// BuildConfigView 合并内置模板和用户配置，生成前端可用的完整视图。
func BuildConfigView(user *UserLLMConfig) *LLMConfigView {
	view := &LLMConfigView{}

	// 内置 provider：从 Builtin 取模板，从 user 取 key、URL 和自定义模型
	for key, bp := range Builtin {
		var apiKey, chatURL string
		var customModels []ModelInfo
		for _, up := range user.Providers {
			if up.Name == key {
				apiKey = up.APIKey
				if up.ChatURL != "" {
					chatURL = up.ChatURL
				}
				customModels = up.Models
				break
			}
		}
		if chatURL == "" {
			chatURL = bp.ChatURL
		}
		view.Providers = append(view.Providers, ProviderView{
			Key:           key,
			Name:          bp.Name,
			ChatURL:       chatURL,
			APIKey:        apiKey,
			PlatformURL:   bp.PlatformURL,
			HelpText:      bp.HelpText,
			Temperature:   derefOrZero(bp.Temperature),
			Source:        "builtin",
			BuiltinModels: append([]ModelInfo{}, bp.Models...),
			CustomModels:  append([]ModelInfo{}, customModels...),
		})
	}

	// 自定义 provider：不在 Builtin 中的用户 provider
	for _, up := range user.Providers {
		if _, isBuiltin := Builtin[up.Name]; !isBuiltin {
			view.Providers = append(view.Providers, ProviderView{
				Key:           up.Name,
				Name:          up.Name,
				ChatURL:       up.ChatURL,
				APIKey:        up.APIKey,
				Temperature:   derefOrZero(up.Temperature),
				Source:        "custom",
				BuiltinModels: nil,
				CustomModels:  append([]ModelInfo{}, up.Models...),
			})
		}
	}

	sort.Slice(view.Providers, func(i, j int) bool {
		return view.Providers[i].Key < view.Providers[j].Key
	})

	return view
}

// ToUserConfig 将前端视图转换回可持久化的 UserLLMConfig。
// ToUserConfig 将前端视图转换回可持久化的 UserLLMConfig。
// 只保留有 APIKey 的 provider，无 key 的不写入（内置模板由 Merge 自动提供）。
func (v *LLMConfigView) ToUserConfig() *UserLLMConfig {
	providers := make([]Provider, 0, len(v.Providers))
	for _, pv := range v.Providers {
		if pv.APIKey == "" {
			continue
		}
		p := Provider{
			Name:   pv.Key,
			APIKey: pv.APIKey,
			Models: append([]ModelInfo{}, pv.CustomModels...),
		}
		if pv.Source != "builtin" {
			p.ChatURL = normalizeURL(pv.ChatURL)
		} else if bp, ok := Builtin[pv.Key]; ok && pv.ChatURL != bp.ChatURL {
			p.ChatURL = normalizeURL(pv.ChatURL)
		}
		p.Temperature = floatPtr(pv.Temperature)
		providers = append(providers, p)
	}
	return &UserLLMConfig{Providers: providers}
}

// Models 从 Provider map 中提取前端下拉列表。
func Models(providers map[string]Provider) []AvailableModel {
	var list []AvailableModel
	for name, p := range providers {
		for _, m := range p.Models {
			list = append(list, AvailableModel{
				Key:              name + "/" + m.ID,
				ProviderName:     p.Name,
				ModelName:        m.Name,
				ContextWindow:    m.ContextWindow,
				MaxOutputTokens:  m.MaxOutputTokens,
				SupportsThinking: m.SupportsThinking,
				ReasoningLevels:  m.ReasoningLevels,
				SupportsVision:   m.SupportsVision,
			})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].ProviderName != list[j].ProviderName {
			return list[i].ProviderName < list[j].ProviderName
		}
		return list[i].ModelName < list[j].ModelName
	})
	return list
}

func modelExists(models []ModelInfo, id string) bool {
	for i := range models {
		if models[i].ID == id {
			return true
		}
	}
	return false
}
