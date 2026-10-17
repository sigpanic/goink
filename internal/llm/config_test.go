package llm

import (
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/stretchr/testify/require"
)

func testUserConfig(providers ...adapter.Provider) *UserLLMConfig {
	return &UserLLMConfig{
		Version: ConfigVersion,
		Adapters: map[adapter.Protocol]AdapterConfig{
			adapter.ProtocolOpenAIChat: {Providers: providers},
		},
	}
}

func testBuiltinConfig() map[adapter.Protocol]ProviderMap {
	return map[adapter.Protocol]ProviderMap{
		adapter.ProtocolOpenAIChat: {
			"MiMo": {
				Name: "MiMo", ProviderType: "mimo", ChatURL: "https://official.example/v1/chat/completions",
				Temperature: floatPtr(1), PlatformURL: "https://official.example", HelpText: "help",
				Models: []adapter.ModelInfo{{ID: "model-a", Name: "Official", ReasoningLevels: []string{"high"}}},
			},
			"未配置": {Name: "未配置", ProviderType: "openai", ChatURL: "https://unused.example"},
		},
	}
}

func TestMerge_InstancesShareOnlyType(t *testing.T) {
	defaults := testBuiltinConfig()
	user := testUserConfig(
		adapter.Provider{Name: "MiMo", APIKey: "official-key", Temperature: floatPtr(0),
			Models: []adapter.ModelInfo{{ID: "model-b"}, {ID: "model-a", Name: "Cannot override official"}}},
		adapter.Provider{Name: "中转站1", ProviderType: "mimo", ChatURL: "https://relay.example/v1/chat/completions",
			APIKey: "relay-key", Models: []adapter.ModelInfo{{ID: "model-c"}}},
	)
	merged, err := mergeConfig(user, defaults)
	require.NoError(t, err)
	group := merged[adapter.ProtocolOpenAIChat]
	require.Len(t, group, 3, "全量合并保留未配置的内置项")
	require.Equal(t, defaults[adapter.ProtocolOpenAIChat]["未配置"], group["未配置"])
	official := group["MiMo"]
	require.Equal(t, "mimo", official.ProviderType)
	require.Equal(t, defaults[adapter.ProtocolOpenAIChat]["MiMo"].ChatURL, official.ChatURL)
	require.Equal(t, 0.0, *official.Temperature)
	require.Equal(t, "official-key", official.APIKey)
	require.Equal(t, []adapter.ModelInfo{
		{ID: "model-a", Name: "Official", ReasoningLevels: []string{"high"}}, {ID: "model-b"},
	}, official.Models)
	relay := group["中转站1"]
	require.Equal(t, "https://relay.example/v1/chat/completions", relay.ChatURL)
	require.Nil(t, relay.Temperature, "自定义实例不能继承同 Type 内置实例的温度")
	require.Empty(t, relay.PlatformURL)
	require.Empty(t, relay.HelpText)
	require.Equal(t, []adapter.ModelInfo{{ID: "model-c"}}, relay.Models)
	require.Equal(t, "", user.Adapters[adapter.ProtocolOpenAIChat].Providers[0].ChatURL, "不能将完整配置反写到原始修改量")
}

func TestMerge_BuiltinDefaultsRemainLive(t *testing.T) {
	defaults := testBuiltinConfig()
	user := testUserConfig(adapter.Provider{Name: "MiMo", APIKey: "key", Models: []adapter.ModelInfo{{ID: "model-b"}}})
	first, err := mergeConfig(user, defaults)
	require.NoError(t, err)
	require.Equal(t, 1.0, *first[adapter.ProtocolOpenAIChat]["MiMo"].Temperature)
	preset := defaults[adapter.ProtocolOpenAIChat]["MiMo"]
	preset.ChatURL = "https://updated.example/v1/chat/completions"
	preset.Models = []adapter.ModelInfo{{ID: "model-a2"}}
	defaults[adapter.ProtocolOpenAIChat]["MiMo"] = preset
	second, err := mergeConfig(user, defaults)
	require.NoError(t, err)
	require.Equal(t, preset.ChatURL, second[adapter.ProtocolOpenAIChat]["MiMo"].ChatURL)
	require.Equal(t, []adapter.ModelInfo{{ID: "model-a2"}, {ID: "model-b"}}, second[adapter.ProtocolOpenAIChat]["MiMo"].Models)
	require.Equal(t, []adapter.ModelInfo{{ID: "model-b"}}, user.Adapters[adapter.ProtocolOpenAIChat].Providers[0].Models)
	user.Adapters[adapter.ProtocolOpenAIChat].Providers[0].ChatURL = "https://override.example/v1/chat/completions"
	third, err := mergeConfig(user, defaults)
	require.NoError(t, err)
	require.Equal(t, "https://override.example/v1/chat/completions", third[adapter.ProtocolOpenAIChat]["MiMo"].ChatURL)
}

func TestMerge_DeepCopiesAllMutableFields(t *testing.T) {
	defaults := testBuiltinConfig()
	user := testUserConfig(
		adapter.Provider{Name: "MiMo", Models: []adapter.ModelInfo{{ID: "extra", ReasoningLevels: []string{"low"}}}},
		adapter.Provider{Name: "custom", ProviderType: "mimo", ChatURL: "https://relay.example", Temperature: floatPtr(0.5),
			Models: []adapter.ModelInfo{{ID: "custom", ReasoningLevels: []string{"max"}}}},
	)
	merged, err := mergeConfig(user, defaults)
	require.NoError(t, err)
	official := merged[adapter.ProtocolOpenAIChat]["MiMo"]
	*official.Temperature = 99
	official.Models[0].ReasoningLevels[0] = "changed"
	official.Models[1].ReasoningLevels[0] = "changed"
	custom := merged[adapter.ProtocolOpenAIChat]["custom"]
	*custom.Temperature = 99
	custom.Models[0].ReasoningLevels[0] = "changed"
	require.Equal(t, 1.0, *defaults[adapter.ProtocolOpenAIChat]["MiMo"].Temperature)
	require.Equal(t, "high", defaults[adapter.ProtocolOpenAIChat]["MiMo"].Models[0].ReasoningLevels[0])
	require.Equal(t, "low", user.Adapters[adapter.ProtocolOpenAIChat].Providers[0].Models[0].ReasoningLevels[0])
	require.Equal(t, 0.5, *user.Adapters[adapter.ProtocolOpenAIChat].Providers[1].Temperature)
	require.Equal(t, "max", user.Adapters[adapter.ProtocolOpenAIChat].Providers[1].Models[0].ReasoningLevels[0])
}

func TestMerge_EmptyConfigDoesNotEnableBuiltins(t *testing.T) {
	defaults := testBuiltinConfig()
	all, err := mergeConfig(nil, defaults)
	require.NoError(t, err)
	require.Equal(t, defaults, all)
	merged, err := Merge(nil)
	require.NoError(t, err)
	require.Empty(t, merged)
	merged, err = Merge(emptyUserConfig())
	require.NoError(t, err)
	require.Empty(t, merged)
}

func TestValidateUserConfig_RejectsInvalidNewConfig(t *testing.T) {
	valid := adapter.Provider{Name: "relay", ProviderType: "mimo", ChatURL: "https://relay.example"}
	tests := []struct {
		name string
		cfg  *UserLLMConfig
		want string
	}{
		{"nil", nil, "must not be nil"},
		{"old version", &UserLLMConfig{}, "version"},
		{"unknown version", &UserLLMConfig{Version: 3}, "version"},
		{"no adapters", &UserLLMConfig{Version: ConfigVersion}, "adapters"},
		{"unsupported adapter", &UserLLMConfig{Version: ConfigVersion, Adapters: map[adapter.Protocol]AdapterConfig{"unknown": {}}}, "unsupported adapter"},
		{"empty name", testUserConfig(adapter.Provider{ProviderType: "mimo", ChatURL: valid.ChatURL}), "name"},
		{"whitespace name", testUserConfig(adapter.Provider{Name: " relay", ProviderType: "mimo", ChatURL: valid.ChatURL}), "name"},
		{"duplicate name", testUserConfig(valid, valid), "duplicate provider"},
		{"custom type required", testUserConfig(adapter.Provider{Name: "relay", APIKey: "key", ChatURL: valid.ChatURL}), "provider type"},
		{"unknown type", testUserConfig(adapter.Provider{Name: "relay", ProviderType: "unknown", ChatURL: valid.ChatURL}), "provider type"},
		{"builtin type not persisted", testUserConfig(adapter.Provider{Name: "MiMo", ProviderType: "mimo"}), "must omit provider_type"},
		{"builtin type cannot change", testUserConfig(adapter.Provider{Name: "MiMo", ProviderType: "openai"}), "must omit provider_type"},
		{"custom URL required", testUserConfig(adapter.Provider{Name: "relay", ProviderType: "mimo"}), "chat_url"},
		{"relative URL", testUserConfig(adapter.Provider{Name: "relay", ProviderType: "mimo", ChatURL: "/v1/chat/completions"}), "chat_url"},
		{"empty model id", testUserConfig(adapter.Provider{Name: "MiMo", Models: []adapter.ModelInfo{{}}}), "model id"},
		{"duplicate models", testUserConfig(adapter.Provider{Name: "MiMo", Models: []adapter.ModelInfo{{ID: "x"}, {ID: "x"}}}), "duplicate model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { require.ErrorContains(t, ValidateUserConfig(tt.cfg), tt.want) })
	}
}

func TestBuildConfigView_ToUserConfigPreservesOverrides(t *testing.T) {
	user := testUserConfig(
		adapter.Provider{Name: "MiMo", APIKey: "official-key", Temperature: floatPtr(0),
			Models: []adapter.ModelInfo{{ID: "extra", ReasoningLevels: []string{"high"}}}},
		adapter.Provider{Name: "中转站1", ProviderType: "mimo", ChatURL: "https://relay.example", APIKey: "relay-key",
			Models: []adapter.ModelInfo{{ID: "relay-only"}}},
	)
	view, err := BuildConfigView(user)
	require.NoError(t, err)
	count := len(builtinProviders()[adapter.ProtocolOpenAIChat])
	require.Len(t, view.Providers, count+1)
	for i, pv := range view.Providers {
		if i > 0 {
			require.Less(t, view.Providers[i-1].Key, pv.Key)
		}
		if pv.Name == "MiMo" {
			require.Equal(t, "builtin", pv.Source)
			require.Equal(t, "mimo", pv.ProviderType)
			require.NotEmpty(t, pv.ChatURL)
			require.NotEmpty(t, pv.BuiltinModels)
			require.Len(t, pv.CustomModels, 1)
		}
		if pv.Name == "中转站1" {
			require.Equal(t, "custom", pv.Source)
			require.Empty(t, pv.BuiltinModels)
			require.Nil(t, pv.Temperature)
		}
	}
	saved, err := view.ToUserConfig()
	require.NoError(t, err)
	require.Equal(t, ConfigVersion, saved.Version)
	require.Len(t, saved.Adapters, 1)
	require.ElementsMatch(t, user.Adapters[adapter.ProtocolOpenAIChat].Providers, saved.Adapters[adapter.ProtocolOpenAIChat].Providers,
		"不能将内置URL/模型反写到用户配置；零温度必须保留")
	for _, pv := range view.Providers {
		if pv.Name == "MiMo" {
			pv.CustomModels[0].ReasoningLevels[0] = "changed in view"
			pv.BuiltinModels[0].Name = "changed in view"
		}
	}
	require.Equal(t, "high", user.Adapters[adapter.ProtocolOpenAIChat].Providers[0].Models[0].ReasoningLevels[0])
	for _, p := range saved.Adapters[adapter.ProtocolOpenAIChat].Providers {
		if p.Name == "MiMo" {
			require.Equal(t, "high", p.Models[0].ReasoningLevels[0])
		}
	}
	reopened, err := BuildConfigView(user)
	require.NoError(t, err)
	for _, pv := range reopened.Providers {
		if pv.Name == "MiMo" {
			require.NotEqual(t, "changed in view", pv.BuiltinModels[0].Name)
		}
	}
}

func TestToUserConfig_PreservesOverridesWithoutAPIKey(t *testing.T) {
	user := testUserConfig(
		adapter.Provider{Name: "MiMo", ChatURL: "https://override.example", Temperature: floatPtr(0),
			Models: []adapter.ModelInfo{{ID: "extra"}}},
		adapter.Provider{Name: "relay", ProviderType: "mimo", ChatURL: "https://relay.example",
			Models: []adapter.ModelInfo{{ID: "relay-model"}}},
	)
	view, err := BuildConfigView(user)
	require.NoError(t, err)
	saved, err := view.ToUserConfig()
	require.NoError(t, err)
	require.ElementsMatch(t, user.Adapters[adapter.ProtocolOpenAIChat].Providers, saved.Adapters[adapter.ProtocolOpenAIChat].Providers)
	restored, err := BuildConfigView(saved)
	require.NoError(t, err)
	require.Equal(t, view, restored)
	runtime, err := Merge(saved)
	require.NoError(t, err)
	require.Empty(t, runtime, "无 APIKey 的配置可以编辑和保存，但不能进入运行时")
}

func TestToUserConfig_DefaultsStayOmitted(t *testing.T) {
	view, err := BuildConfigView(nil)
	require.NoError(t, err)
	unchanged, err := view.ToUserConfig()
	require.NoError(t, err)
	require.Empty(t, unchanged.Adapters)
	for i := range view.Providers {
		if view.Providers[i].Name == "MiMo" {
			view.Providers[i].APIKey = "key"
		}
	}
	saved, err := view.ToUserConfig()
	require.NoError(t, err)
	providers := saved.Adapters[adapter.ProtocolOpenAIChat].Providers
	require.Len(t, providers, 1, "未修改的内置项不写入")
	require.Equal(t, "MiMo", providers[0].Name)
	require.Empty(t, providers[0].ProviderType)
	require.Empty(t, providers[0].ChatURL)
	require.Nil(t, providers[0].Temperature)
	require.Empty(t, providers[0].Models)
}

func TestBuildConfigView_KeepsUserModelsShadowedByBuiltin(t *testing.T) {
	preset := builtinProviders()[adapter.ProtocolOpenAIChat]["MiMo"]
	require.NotEmpty(t, preset.Models)
	userModel := adapter.ModelInfo{ID: preset.Models[0].ID, Name: "用户原来追加的定义", ReasoningLevels: []string{"low"}}
	user := testUserConfig(adapter.Provider{Name: "MiMo", APIKey: "key", Models: []adapter.ModelInfo{userModel}})
	merged, err := Merge(user)
	require.NoError(t, err)
	require.Equal(t, preset.Models[0], merged[adapter.ProtocolOpenAIChat]["MiMo"].Models[0], "运行时仍以官方定义为准")
	view, err := BuildConfigView(user)
	require.NoError(t, err)
	for _, pv := range view.Providers {
		if pv.Name == "MiMo" {
			require.Equal(t, []ModelInfo{userModel}, pv.CustomModels)
		}
	}
	saved, err := view.ToUserConfig()
	require.NoError(t, err)
	require.Equal(t, user, saved, "设置页普通保存不能悄悄删除被新内置模型遮蔽的用户记录")
}

func TestToUserConfig_ValuesEqualToDefaultsFollowBuiltin(t *testing.T) {
	preset := builtinProviders()[adapter.ProtocolOpenAIChat]["MiMo"]
	user := testUserConfig(adapter.Provider{
		Name:   preset.Name,
		APIKey: "key", ChatURL: preset.ChatURL, Temperature: floatPtr(*preset.Temperature),
	})
	view, err := BuildConfigView(user)
	require.NoError(t, err)
	saved, err := view.ToUserConfig()
	require.NoError(t, err)
	p := saved.Adapters[adapter.ProtocolOpenAIChat].Providers[0]
	require.Empty(t, p.ChatURL)
	require.Nil(t, p.Temperature, "设置页保存相对默认值的差量，等值字段归一为继承")
	updated := preset.Clone()
	updated.ChatURL = "https://updated.example/v1/chat/completions"
	updated.Temperature = floatPtr(0.3)
	merged, err := mergeConfig(saved, map[adapter.Protocol]ProviderMap{
		adapter.ProtocolOpenAIChat: {preset.Name: updated},
	})
	require.NoError(t, err)
	require.Equal(t, updated.ChatURL, merged[adapter.ProtocolOpenAIChat][preset.Name].ChatURL)
	require.Equal(t, 0.3, *merged[adapter.ProtocolOpenAIChat][preset.Name].Temperature)
}

func TestToUserConfig_RejectsConflictsForEmptyOverrides(t *testing.T) {
	view, err := BuildConfigView(nil)
	require.NoError(t, err)
	view.Providers = append(view.Providers, view.Providers[0])
	_, err = view.ToUserConfig()
	require.ErrorContains(t, err, "duplicate provider")
	view = &LLMConfigView{Providers: []ProviderView{{AdapterName: adapter.ProtocolOpenAIChat, Name: "MiMo", ProviderType: "mimo", Source: "custom"}}}
	_, err = view.ToUserConfig()
	require.ErrorContains(t, err, "source")
	view = &LLMConfigView{Providers: []ProviderView{{AdapterName: adapter.ProtocolOpenAIChat, Name: "renamed builtin", ProviderType: "mimo", Source: "builtin"}}}
	_, err = view.ToUserConfig()
	require.ErrorContains(t, err, "source")
}

func TestModels_UsesProtocolAndInstanceName(t *testing.T) {
	providers := map[adapter.Protocol]ProviderMap{
		adapter.ProtocolOpenAIChat: {
			"中转站1": {Name: "中转站1", ProviderType: "mimo", Models: []adapter.ModelInfo{{ID: "m", Name: "Model", ReasoningLevels: []string{"high"}}}},
		},
		adapter.ProtocolAnthropicMessages: {
			"中转站1": {Name: "中转站1", ProviderType: "anthropic", Models: []adapter.ModelInfo{{ID: "m", Name: "Model"}}},
		},
	}
	list := Models(providers)
	require.Len(t, list, 2)
	require.Equal(t, "anthropic_messages/%E4%B8%AD%E8%BD%AC%E7%AB%991/m", list[0].Key)
	require.Equal(t, "openai_chat/%E4%B8%AD%E8%BD%AC%E7%AB%991/m", list[1].Key)
	require.Equal(t, "中转站1", list[1].ProviderName)
	require.Equal(t, "mimo", list[1].ProviderKey)
	list[1].ReasoningLevels[0] = "changed"
	require.Equal(t, "high", providers[adapter.ProtocolOpenAIChat]["中转站1"].Models[0].ReasoningLevels[0])
	require.Empty(t, Models(nil))
}

func TestModels_KeysEscapeEachNameAndModelSegment(t *testing.T) {
	providers := map[adapter.Protocol]ProviderMap{
		adapter.ProtocolOpenAIChat: {
			"a/b": {Name: "a/b", ProviderType: "openai", Models: []adapter.ModelInfo{{ID: "c"}}},
			"a":   {Name: "a", ProviderType: "openai", Models: []adapter.ModelInfo{{ID: "b/c"}}},
		},
	}
	list := Models(providers)
	require.Len(t, list, 2)
	require.Equal(t, "openai_chat/a/b%2Fc", list[0].Key)
	require.Equal(t, "a", list[0].ProviderName)
	require.Equal(t, "b/c", list[0].ModelID)
	require.Equal(t, "openai_chat/a%2Fb/c", list[1].Key)
	require.Equal(t, "a/b", list[1].ProviderName)
	require.Equal(t, "c", list[1].ModelID)
	require.NotEqual(t, providerViewKey(adapter.ProtocolOpenAIChat, "a/b"), providerViewKey(adapter.ProtocolOpenAIChat, "a%2Fb"))
}
