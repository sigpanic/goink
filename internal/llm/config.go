package llm

import (
	"fmt"
	"math"
	"net/url"
	"slices"
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

// ProviderMap 按实例 Name 索引单个协议下的公共配置。
type ProviderMap = map[string]adapter.Provider

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

// Merge 按协议和 Name 组装有 APIKey 的运行时实例，不按 ProviderType 继承实例配置。
// 返回值是运行时快照，不可直接反写为用户配置，否则会固化内置默认值。
func Merge(user *UserLLMConfig) (map[adapter.Protocol]ProviderMap, error) {
	result, err := mergeConfig(user, builtinProviders())
	if err != nil {
		return nil, err
	}
	for protocol, providers := range result {
		for name, p := range providers {
			if p.APIKey == "" {
				delete(providers, name)
			}
		}
		if len(providers) == 0 {
			delete(result, protocol)
		}
	}
	return result, nil
}

// mergeConfig 合并全部内置实例和用户配置，供设置视图与运行时过滤共用。
func mergeConfig(user *UserLLMConfig, defaults map[adapter.Protocol]ProviderMap) (map[adapter.Protocol]ProviderMap, error) {
	if user == nil {
		user = emptyUserConfig()
	}
	if err := validateUserConfig(user, defaults); err != nil {
		return nil, err
	}
	result := make(map[adapter.Protocol]ProviderMap, len(defaults))
	for protocol, builtins := range defaults {
		providers := make(ProviderMap, len(builtins))
		for name, p := range builtins {
			providers[name] = p.Clone()
		}
		result[protocol] = providers
	}
	for protocol, group := range user.Adapters {
		for _, p := range group.Providers {
			result[protocol][p.Name] = mergeProvider(defaults[protocol][p.Name], p)
		}
	}
	return result, nil
}

func mergeProvider(preset, user adapter.Provider) adapter.Provider {
	if preset.Name == "" {
		return user.Clone()
	}
	p := preset.Clone()
	p.APIKey = user.APIKey
	if user.ChatURL != "" {
		p.ChatURL = user.ChatURL
	}
	if user.Temperature != nil {
		p.Temperature = floatPtr(*user.Temperature)
	}
	seen := make(map[string]bool, len(p.Models)+len(user.Models))
	for _, model := range p.Models {
		seen[model.ID] = true
	}
	for _, model := range user.Models {
		if seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		p.Models = append(p.Models, model.Clone())
	}
	return p
}

// LLMConfigView 是 GetLLMConfig 返回的完整配置视图，合并内置实例和用户配置。
type LLMConfigView struct {
	Providers []ProviderView `json:"providers"`
}

// ProviderView 是单个 provider 的前端展示视图。
type ProviderView struct {
	Key           string           `json:"key"` // 由协议和 Name 派生，仅供 UI 使用，不反解析调用目标
	AdapterName   adapter.Protocol `json:"adapter_name"`
	ProviderType  string           `json:"provider_type"`
	Name          string           `json:"name"`
	ChatURL       string           `json:"chat_url"`
	APIKey        string           `json:"api_key"`
	PlatformURL   string           `json:"platform_url"`
	HelpText      string           `json:"help_text"`
	Temperature   *float64         `json:"temperature"`
	Source        string           `json:"source"` // "builtin" | "custom"
	BuiltinModels []ModelInfo      `json:"builtin_models"`
	CustomModels  []ModelInfo      `json:"custom_models"`
}

func emptyUserConfig() *UserLLMConfig {
	return &UserLLMConfig{Version: ConfigVersion, Adapters: make(map[adapter.Protocol]AdapterConfig)}
}

// ValidateUserConfig 只校验新格式，不推断协议、实例名称或厂商类型。
func ValidateUserConfig(user *UserLLMConfig) error {
	return validateUserConfig(user, builtinProviders())
}

func validateUserConfig(user *UserLLMConfig, defaults map[adapter.Protocol]ProviderMap) error {
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

// BuildConfigView 使用同一合并规则生成设置视图，并展示尚未配置的全部内置实例。
func BuildConfigView(user *UserLLMConfig) (*LLMConfigView, error) {
	defaults := builtinProviders()
	resolved, err := mergeConfig(user, defaults)
	if err != nil {
		return nil, err
	}
	saved := make(map[adapter.Protocol]ProviderMap)
	if user != nil {
		for protocol, group := range user.Adapters {
			saved[protocol] = make(ProviderMap, len(group.Providers))
			for _, p := range group.Providers {
				saved[protocol][p.Name] = p
			}
		}
	}
	view := &LLMConfigView{Providers: []ProviderView{}}
	for protocol, providers := range resolved {
		for name, p := range providers {
			preset, builtin := defaults[protocol][name]
			source := "custom"
			if builtin {
				source = "builtin"
			}
			view.Providers = append(view.Providers, providerView(protocol, p, preset, saved[protocol][name].Models, source))
		}
	}
	sort.Slice(view.Providers, func(i, j int) bool { return view.Providers[i].Key < view.Providers[j].Key })
	return view, nil
}

func providerView(protocol adapter.Protocol, p, preset adapter.Provider, customModels []adapter.ModelInfo, source string) ProviderView {
	p = p.Clone()
	return ProviderView{
		Key: providerViewKey(protocol, p.Name), AdapterName: protocol,
		Name: p.Name, ProviderType: p.ProviderType,
		ChatURL: p.ChatURL, APIKey: p.APIKey, Temperature: p.Temperature,
		PlatformURL: p.PlatformURL, HelpText: p.HelpText, Source: source,
		BuiltinModels: cloneModels(preset.Models), CustomModels: cloneModels(customModels),
	}
}

func providerViewKey(adapterName adapter.Protocol, providerName string) string {
	return url.PathEscape(string(adapterName)) + "/" + url.PathEscape(providerName)
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

func cloneModels(models []ModelInfo) []ModelInfo {
	models = slices.Clone(models)
	for i := range models {
		models[i] = models[i].Clone()
	}
	return models
}
