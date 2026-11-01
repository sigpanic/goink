//go:build cgo && upgrade

package upgrade_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/llm/adapter"
)

func assertLLMConfig(t *testing.T, dir string) {
	t.Helper()
	got, err := llm.LoadUserConfig(filepath.Join(dir, ".goink", "llm_config.enc"))
	if err != nil {
		t.Fatal("load upgraded LLM config:", err)
	}
	zero, temperature := 0.0, 0.7
	want := &llm.UserLLMConfig{
		Version: 2,
		Adapters: map[adapter.Protocol]llm.AdapterConfig{
			adapter.ProtocolOpenAIChat: {Providers: []adapter.Provider{
				{
					Name: "DeepSeek", APIKey: "fixture-builtin-key",
					ChatURL: "https://builtin.example/v8/chat/completions?region=cn", Temperature: &zero,
					Models: []adapter.ModelInfo{{
						ID: "fixture-deepseek", Name: "内置服务商自定义模型",
						ContextWindow: 65536, MaxOutputTokens: 8192,
						SupportsThinking: true, ReasoningLevels: []string{"low", "high"}, SupportsVision: true,
					}},
				},
				{
					Name: "升级中转站", ProviderType: "openai", APIKey: "fixture-relay-key",
					ChatURL: "https://relay.example/custom/chat/completions?tenant=fixture", Temperature: &temperature,
					Models: []adapter.ModelInfo{{
						ID: "vendor/model", Name: "中转模型", ContextWindow: 32768, MaxOutputTokens: 4096,
					}},
				},
			}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("upgraded LLM config differs from the historical seed contract")
	}
}
