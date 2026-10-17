package llm

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

const ConfigVersion = 2

// UserLLMConfig 是用户 LLM 配置的持久化格式（加密 JSON）。
// 只保存用户配置；内置模型由具体适配器提供，自定义模型需完整填写信息。
type UserLLMConfig struct {
	Version  int                                `json:"version"`
	Adapters map[adapter.Protocol]AdapterConfig `json:"adapters"`
}

// AdapterConfig 按协议保存用户修改和自定义实例，Name 在所属协议内唯一。
type AdapterConfig struct {
	Providers []adapter.Provider `json:"providers"`
}

// AvailableModel 是前端下拉列表的模型选项。
type AvailableModel struct {
	Key              string // "deepseek/deepseek-v4-pro"
	ModelID          string // "deepseek-v4-pro"（独立字段，不依赖 Key 拆分）
	ProviderKey      string // "deepseek"（小写 key，与 ProviderView.Key 一致，供 ProviderIcon 匹配）
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
			if up.Temperature != nil {
				p.Temperature = up.Temperature
			} else {
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

func emptyUserConfig() *UserLLMConfig {
	return &UserLLMConfig{Version: ConfigVersion, Adapters: make(map[adapter.Protocol]AdapterConfig)}
}

// ValidateUserConfig 只校验新格式，不推断协议、实例名称或厂商类型。
func ValidateUserConfig(user *UserLLMConfig) error {
	return validateUserConfig(user, builtinProviders())
}

func validateUserConfig(user *UserLLMConfig, defaults map[adapter.Protocol]map[string]adapter.Provider) error {
	if user == nil {
		return fmt.Errorf("LLM config must not be nil")
	}
	if user.Version != ConfigVersion {
		return fmt.Errorf("unsupported LLM config version: %d; expected %d", user.Version, ConfigVersion)
	}
	if user.Adapters == nil {
		return fmt.Errorf("LLM config adapters must be an object")
	}
	for protocol, group := range user.Adapters {
		builtins, ok := defaults[protocol]
		if !ok {
			return fmt.Errorf("unsupported adapter: %s", protocol)
		}
		seen := make(map[string]bool, len(group.Providers))
		for _, p := range group.Providers {
			if p.Name == "" || strings.TrimSpace(p.Name) != p.Name {
				return fmt.Errorf("provider name must not be empty or have surrounding whitespace in adapter %s", protocol)
			}
			if seen[p.Name] {
				return fmt.Errorf("duplicate provider: %s/%s", protocol, p.Name)
			}
			seen[p.Name] = true
			_, builtin := builtins[p.Name]
			if builtin {
				if p.ProviderType != "" {
					return fmt.Errorf("builtin provider %s/%s must omit provider_type", protocol, p.Name)
				}
			} else if err := validateProviderType(protocol, p.ProviderType); err != nil {
				return fmt.Errorf("provider %s/%s: %w", protocol, p.Name, err)
			}
			if p.ChatURL != "" || !builtin {
				parsed, err := url.Parse(p.ChatURL)
				if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
					return fmt.Errorf("provider %s/%s requires an absolute HTTP(S) chat_url", protocol, p.Name)
				}
			}
			if p.Temperature != nil && (math.IsNaN(*p.Temperature) || math.IsInf(*p.Temperature, 0)) {
				return fmt.Errorf("provider %s/%s temperature must be finite", protocol, p.Name)
			}
			models := make(map[string]bool, len(p.Models))
			for _, model := range p.Models {
				if strings.TrimSpace(model.ID) == "" {
					return fmt.Errorf("provider %s/%s model id must not be empty", protocol, p.Name)
				}
				if models[model.ID] {
					return fmt.Errorf("provider %s/%s has duplicate model: %s", protocol, p.Name, model.ID)
				}
				models[model.ID] = true
			}
		}
	}
	return nil
}

// BuildConfigView 合并内置模板和用户配置，生成前端可用的完整视图。
func BuildConfigView(user *UserLLMConfig) *LLMConfigView {
	view := &LLMConfigView{}

	// 内置 provider：从 Builtin 取模板，从 user 取 key、URL 和自定义模型
	for key, bp := range Builtin {
		var apiKey, chatURL string
		var customModels []ModelInfo
		temperature := derefOrZero(bp.Temperature)
		for _, up := range user.Providers {
			if up.Name == key {
				apiKey = up.APIKey
				if up.ChatURL != "" {
					chatURL = up.ChatURL
				}
				customModels = up.Models
				if up.Temperature != nil {
					temperature = *up.Temperature
				}
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
			Temperature:   temperature,
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
// 只保留有 APIKey 的 provider，无 key 的不写入（内置模板由 Merge 自动提供）。
//
// 保存用户填的 URL 原值，不再 normalizeURL：
//   - 保存前前端已通过 TestConnection 验证 URL，并把探测到的正确 URL 回写到 pv.ChatURL
//   - 二次 normalizeURL 会破坏已验证的 URL（测试和保存不一致）
//   - 内置 provider 用户未改 URL 时留空，由 Merge 用 bp.ChatURL 兜底
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
			p.ChatURL = pv.ChatURL
		} else if bp, ok := Builtin[pv.Key]; ok && pv.ChatURL != bp.ChatURL {
			p.ChatURL = pv.ChatURL
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
				ModelID:          m.ID,
				ProviderKey:      name,
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
