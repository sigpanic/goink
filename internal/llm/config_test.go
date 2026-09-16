package llm

import (
	"testing"
)

func TestDerefOrZero_Nil(t *testing.T) {
	if v := derefOrZero(nil); v != 0 {
		t.Errorf("expected 0, got %f", v)
	}
}

func TestDerefOrZero_Value(t *testing.T) {
	f := 3.14
	if v := derefOrZero(&f); v != 3.14 {
		t.Errorf("expected 3.14, got %f", v)
	}
}

func TestModelExists_True(t *testing.T) {
	models := []ModelInfo{
		{ID: "deepseek-v4-pro"},
		{ID: "deepseek-v4-flash"},
	}
	if !modelExists(models, "deepseek-v4-flash") {
		t.Error("should find existing model")
	}
}

func TestModelExists_False(t *testing.T) {
	models := []ModelInfo{{ID: "deepseek-v4-pro"}}
	if modelExists(models, "glm-5") {
		t.Error("should not find missing model")
	}
}

func TestMerge_UserKeyOverridesBuiltin(t *testing.T) {
	builtin := map[string]Provider{
		"ds": {
			Name:    "DeepSeek",
			ChatURL: "https://api.deepseek.com/v1",
			Models:  []ModelInfo{{ID: "ds-pro", Name: "DeepSeek Pro"}},
		},
	}
	user := &UserLLMConfig{
		Providers: []Provider{
			{Name: "ds", APIKey: "sk-xxx"},
		},
	}

	result := Merge(builtin, user)
	p, ok := result["ds"]
	if !ok {
		t.Fatal("expected ds provider")
	}
	if p.APIKey != "sk-xxx" {
		t.Errorf("APIKey not set: %s", p.APIKey)
	}
	if p.ChatURL != "https://api.deepseek.com/v1" {
		t.Errorf("ChatURL should fallback to builtin: %s", p.ChatURL)
	}
}

func TestMerge_CustomModelAppended(t *testing.T) {
	builtin := map[string]Provider{
		"ds": {
			Name:    "DeepSeek",
			ChatURL: "https://api.deepseek.com/v1",
			Models:  []ModelInfo{{ID: "ds-pro", Name: "DeepSeek Pro"}},
		},
	}
	user := &UserLLMConfig{
		Providers: []Provider{
			{
				Name:   "ds",
				APIKey: "sk-xxx",
				Models: []ModelInfo{{ID: "my-custom", Name: "Custom"}},
			},
		},
	}

	result := Merge(builtin, user)
	p := result["ds"]
	if len(p.Models) != 2 { // 1 builtin + 1 custom
		t.Errorf("expected 2 models, got %d", len(p.Models))
	}
}

func TestMerge_CustomModelDedup(t *testing.T) {
	builtin := map[string]Provider{
		"ds": {
			Models: []ModelInfo{{ID: "ds-pro", Name: "DeepSeek Pro"}},
		},
	}
	user := &UserLLMConfig{
		Providers: []Provider{
			{
				Name:   "ds",
				APIKey: "sk-xxx",
				Models: []ModelInfo{{ID: "ds-pro", Name: "duplicate"}}, // same ID as builtin
			},
		},
	}

	result := Merge(builtin, user)
	if len(result["ds"].Models) != 1 {
		t.Errorf("duplicate model should be skipped, got %d", len(result["ds"].Models))
	}
}

func TestMerge_CustomProvider(t *testing.T) {
	builtin := map[string]Provider{}
	user := &UserLLMConfig{
		Providers: []Provider{
			{
				Name:    "custom-provider",
				ChatURL: "https://my.api.com/v1",
				APIKey:  "sk-custom",
				Models:  []ModelInfo{{ID: "model1", Name: "Model 1"}},
			},
		},
	}

	result := Merge(builtin, user)
	p, ok := result["custom-provider"]
	if !ok {
		t.Fatal("expected custom provider")
	}
	if p.ChatURL != "https://my.api.com/v1" {
		t.Errorf("ChatURL: %s", p.ChatURL)
	}
}

func TestMerge_UserTemperatureOverridesBuiltin(t *testing.T) {
	builtin := map[string]Provider{
		"ds": {
			Name:        "DeepSeek",
			Temperature: floatPtr(0.7),
		},
	}
	user := &UserLLMConfig{
		Providers: []Provider{
			{Name: "ds", APIKey: "sk-xxx", Temperature: floatPtr(1.0)},
		},
	}

	result := Merge(builtin, user)
	p, ok := result["ds"]
	if !ok {
		t.Fatal("expected ds provider")
	}
	if p.Temperature == nil || *p.Temperature != 1.0 {
		t.Errorf("expected user temperature 1.0, got %v", p.Temperature)
	}
}

func TestMerge_BuiltinTemperatureFallback(t *testing.T) {
	builtin := map[string]Provider{
		"ds": {
			Name:        "DeepSeek",
			Temperature: floatPtr(0.7),
		},
	}
	user := &UserLLMConfig{
		Providers: []Provider{
			{Name: "ds", APIKey: "sk-xxx"},
		},
	}

	result := Merge(builtin, user)
	p, ok := result["ds"]
	if !ok {
		t.Fatal("expected ds provider")
	}
	if p.Temperature == nil || *p.Temperature != 0.7 {
		t.Errorf("expected builtin temperature 0.7, got %v", p.Temperature)
	}
}

func TestMerge_UserURLOverridesBuiltin(t *testing.T) {
	builtin := map[string]Provider{
		"ds": {Name: "DeepSeek", ChatURL: "https://builtin.api/v1"},
	}
	user := &UserLLMConfig{
		Providers: []Provider{
			{Name: "ds", APIKey: "sk-xxx", ChatURL: "https://user.api/v1"},
		},
	}

	result := Merge(builtin, user)
	p := result["ds"]
	if p.ChatURL != "https://user.api/v1" {
		t.Errorf("expected user URL, got %s", p.ChatURL)
	}
}

func TestMerge_CopiesBuiltinHeaderHooks(t *testing.T) {
	builtin := map[string]Provider{
		"ds": {
			Name: "DeepSeek",
			BuildRequest: func(map[string]any) map[string]any {
				return nil
			},
			BuildHeaders: func(*CallOptions, map[string]string) map[string]string {
				return nil
			},
			ParseError: func([]byte) error { return nil },
		},
	}
	user := &UserLLMConfig{
		Providers: []Provider{{Name: "ds", APIKey: "sk-xxx"}},
	}

	result := Merge(builtin, user)
	p := result["ds"]
	if p.BuildRequest == nil || p.BuildHeaders == nil || p.ParseError == nil {
		t.Error("builtin hooks should be copied to merged provider")
	}
}

func TestMerge_EmptyUserConfig(t *testing.T) {
	result := Merge(map[string]Provider{"ds": {Name: "DeepSeek"}}, &UserLLMConfig{})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestBuildConfigView_HasBuiltins(t *testing.T) {
	// 用真实的 Builtin 测试
	user := &UserLLMConfig{
		Providers: []Provider{
			{Name: "deepseek", APIKey: "sk-test"},
		},
	}

	view := BuildConfigView(user)
	if len(view.Providers) < 3 {
		t.Errorf("expected at least 3 providers (3 builtins), got %d", len(view.Providers))
	}

	// deepseek 应该有 key
	found := false
	for _, pv := range view.Providers {
		if pv.Key == "deepseek" {
			found = true
			if pv.APIKey != "sk-test" {
				t.Errorf("APIKey not passed through: %s", pv.APIKey)
			}
			if pv.Source != "builtin" {
				t.Errorf("deepseek should be builtin")
			}
		}
	}
	if !found {
		t.Error("deepseek not found in view")
	}
}

func TestBuildConfigView_CustomProvider(t *testing.T) {
	user := &UserLLMConfig{
		Providers: []Provider{
			{
				Name:    "my-api",
				ChatURL: "https://example.com/v1",
				APIKey:  "sk-custom",
				Models:  []ModelInfo{{ID: "m1", Name: "Model 1"}},
			},
		},
	}

	view := BuildConfigView(user)
	found := false
	for _, pv := range view.Providers {
		if pv.Key == "my-api" {
			found = true
			if pv.Source != "custom" {
				t.Errorf("my-api should be custom, got %s", pv.Source)
			}
			if pv.BuiltinModels != nil {
				t.Error("custom provider should have nil BuiltinModels")
			}
		}
	}
	if !found {
		t.Error("custom provider not found in view")
	}
}

func TestBuildConfigView_UserTemperatureOverridesBuiltin(t *testing.T) {
	user := &UserLLMConfig{
		Providers: []Provider{
			{Name: "deepseek", APIKey: "sk-test", Temperature: floatPtr(1.0)},
		},
	}

	view := BuildConfigView(user)
	found := false
	for _, pv := range view.Providers {
		if pv.Key == "deepseek" {
			found = true
			if pv.Temperature != 1.0 {
				t.Errorf("expected user temperature 1.0, got %v", pv.Temperature)
			}
		}
	}
	if !found {
		t.Error("deepseek not found in view")
	}
}

func TestBuildConfigView_BuiltinTemperatureFallback(t *testing.T) {
	user := &UserLLMConfig{
		Providers: []Provider{
			{Name: "deepseek", APIKey: "sk-test"},
		},
	}

	view := BuildConfigView(user)
	found := false
	for _, pv := range view.Providers {
		if pv.Key == "deepseek" {
			found = true
			// deepseek 内置默认温度 0.7
			if pv.Temperature != 0.7 {
				t.Errorf("expected builtin temperature 0.7, got %v", pv.Temperature)
			}
		}
	}
	if !found {
		t.Error("deepseek not found in view")
	}
}

func TestBuildConfigView_UserURLOverridesBuiltin(t *testing.T) {
	user := &UserLLMConfig{
		Providers: []Provider{
			{Name: "deepseek", APIKey: "sk-test", ChatURL: "https://user.api/v1"},
		},
	}

	view := BuildConfigView(user)
	found := false
	for _, pv := range view.Providers {
		if pv.Key == "deepseek" {
			found = true
			if pv.ChatURL != "https://user.api/v1" {
				t.Errorf("expected user URL, got %s", pv.ChatURL)
			}
		}
	}
	if !found {
		t.Error("deepseek not found in view")
	}
}

func TestBuildConfigView_EmptyConfigShowsAllBuiltins(t *testing.T) {
	view := BuildConfigView(&UserLLMConfig{})
	if len(view.Providers) != len(Builtin) {
		t.Errorf("expected %d builtin providers, got %d", len(Builtin), len(view.Providers))
	}
}

func TestBuildConfigView_SortedByKey(t *testing.T) {
	view := BuildConfigView(&UserLLMConfig{})
	for i := 1; i < len(view.Providers); i++ {
		if view.Providers[i-1].Key > view.Providers[i].Key {
			t.Errorf("providers not sorted by key: %s > %s", view.Providers[i-1].Key, view.Providers[i].Key)
		}
	}
}

func TestToUserConfig_OnlyProvidersWithKey(t *testing.T) {
	view := &LLMConfigView{
		Providers: []ProviderView{
			{Key: "deepseek", APIKey: "sk-xxx", Source: "builtin"},
			{Key: "zhipu", APIKey: "", Source: "builtin"},
			{Key: "custom", APIKey: "sk-yyy", Source: "custom", ChatURL: "https://custom.api"},
		},
	}

	user := view.ToUserConfig()
	if len(user.Providers) != 2 {
		t.Fatalf("expected 2 providers (with keys), got %d", len(user.Providers))
	}
	// zhipu 无 key 应被跳过
	for _, p := range user.Providers {
		if p.Name == "zhipu" {
			t.Error("zhipu should be skipped (no key)")
		}
	}
}

func TestToUserConfig_BuiltinURLOnlySavedWhenChanged(t *testing.T) {
	bp, ok := Builtin["deepseek"]
	if !ok {
		t.Fatal("deepseek builtin not found")
	}
	view := &LLMConfigView{
		Providers: []ProviderView{
			{Key: "deepseek", APIKey: "sk-a", Source: "builtin", ChatURL: bp.ChatURL},
			{Key: "zhipu", APIKey: "sk-b", Source: "builtin", ChatURL: "https://changed.api/v1"},
		},
	}

	user := view.ToUserConfig()
	for _, p := range user.Providers {
		switch p.Name {
		case "deepseek":
			if p.ChatURL != "" {
				t.Errorf("unchanged builtin URL should not be saved, got %q", p.ChatURL)
			}
		case "zhipu":
			if p.ChatURL != "https://changed.api/v1" {
				t.Errorf("changed builtin URL should be saved, got %q", p.ChatURL)
			}
		}
	}
}

func TestToUserConfig_SavesTemperature(t *testing.T) {
	view := &LLMConfigView{
		Providers: []ProviderView{
			{Key: "deepseek", APIKey: "sk-xxx", Source: "builtin", Temperature: 1.0},
		},
	}

	user := view.ToUserConfig()
	if len(user.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(user.Providers))
	}
	p := user.Providers[0]
	if p.Temperature == nil || *p.Temperature != 1.0 {
		t.Errorf("expected temperature 1.0 saved, got %v", p.Temperature)
	}
}

func TestModels_Extraction(t *testing.T) {
	providers := map[string]Provider{
		"ds": {
			Name: "DeepSeek",
			Models: []ModelInfo{
				{ID: "ds-pro", Name: "DeepSeek Pro", ContextWindow: 1_000_000},
			},
		},
	}

	list := Models(providers)
	if len(list) != 1 {
		t.Fatalf("expected 1 model, got %d", len(list))
	}
	if list[0].Key != "ds/ds-pro" {
		t.Errorf("Key: expected ds/ds-pro, got %s", list[0].Key)
	}
	if list[0].ProviderName != "DeepSeek" {
		t.Errorf("ProviderName: got %s", list[0].ProviderName)
	}
}

func TestModels_SortedByProviderThenModel(t *testing.T) {
	providers := map[string]Provider{
		"zz": {
			Name: "Zeta",
			Models: []ModelInfo{
				{ID: "m1", Name: "Alpha"},
			},
		},
		"aa": {
			Name: "Alpha",
			Models: []ModelInfo{
				{ID: "m3", Name: "Beta"},
				{ID: "m2", Name: "Alpha"},
			},
		},
	}

	list := Models(providers)
	if len(list) != 3 {
		t.Fatalf("expected 3 models, got %d", len(list))
	}
	// 按 ProviderName 排序：Alpha 在前；同 provider 内按 ModelName：Alpha 再 Beta
	want := []string{"Alpha/Alpha", "Alpha/Beta", "Zeta/Alpha"}
	for i, m := range list {
		key := m.ProviderName + "/" + m.ModelName
		if key != want[i] {
			t.Errorf("order[%d]: expected %s, got %s", i, want[i], key)
		}
	}
}

func TestModels_Empty(t *testing.T) {
	if list := Models(nil); len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

func TestMimoBuildHeaders(t *testing.T) {
	input := map[string]string{"Authorization": "Bearer sk-mimo-123"}
	result := mimoBuildHeaders(nil, input)
	if apiKey, ok := result["api-key"]; !ok || apiKey != "sk-mimo-123" {
		t.Errorf("api-key: got %q", result["api-key"])
	}
	if _, exists := result["Authorization"]; exists {
		t.Error("Authorization should be removed")
	}
	// Go map 是引用类型，原 map 也会被修改
	if _, exists := input["api-key"]; !exists {
		t.Error("api-key should exist in original map (go map is by reference)")
	}
}
