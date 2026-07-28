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

func TestMimoBuildHeaders(t *testing.T) {
	input := map[string]string{"Authorization": "Bearer sk-mimo-123"}
	result := mimoBuildHeaders(input)
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
