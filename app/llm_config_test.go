package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestLLMConfigSaveReloadAndFailures(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	a := setupChatRoutingTest(t, "https://example.invalid")
	view := llm.LLMConfigView{Providers: []llm.ProviderView{
		{AdapterName: adapter.ProtocolOpenAIChat, Name: "relay", ProviderType: "openai", Source: "custom",
			APIKey: "old-key", ChatURL: "https://example.invalid/v1/chat/completions", CustomModels: []llm.ModelInfo{{ID: "configured-model"}}},
		{AdapterName: adapter.ProtocolAnthropicMessages, Name: "relay", ProviderType: "anthropic", Source: "custom",
			ChatURL: "https://example.invalid/custom/v8/messages", CustomModels: []llm.ModelInfo{{ID: "unconfigured-model"}}},
	}}
	path := config.LLMConfigPath()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, a.SaveLLMConfig(view))
	beforeFile, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(beforeFile), "old-key", "configuration must remain encrypted")
	stored, err := llm.LoadUserConfig(path)
	require.NoError(t, err)
	require.Len(t, stored.Adapters[adapter.ProtocolOpenAIChat].Providers, 1)
	require.Equal(t, "old-key", stored.Adapters[adapter.ProtocolOpenAIChat].Providers[0].APIKey)
	require.Len(t, stored.Adapters[adapter.ProtocolAnthropicMessages].Providers, 1)
	require.Empty(t, stored.Adapters[adapter.ProtocolAnthropicMessages].Providers[0].APIKey)
	require.Equal(t, view.Providers[1].ChatURL, stored.Adapters[adapter.ProtocolAnthropicMessages].Providers[0].ChatURL)
	loaded, err := a.GetLLMConfig()
	require.NoError(t, err)
	var custom []llm.ProviderView
	for _, provider := range loaded.Providers {
		if provider.Source == "custom" {
			custom = append(custom, provider)
		}
	}
	require.Len(t, custom, 2)
	require.Greater(t, len(loaded.Providers), len(custom), "the settings view must also include unconfigured builtins")
	models := a.GetModels()
	require.Len(t, models, 1, "a provider without an API key is saved but not active")
	require.Equal(t, adapter.ProtocolOpenAIChat, models[0].AdapterName)
	require.Equal(t, "relay", models[0].ProviderName)
	require.Equal(t, "configured-model", models[0].Model.ID)
	beforeRuntime := a.llmClient.Providers()
	for _, failure := range []string{"invalid view", "adapter construction", "file write"} {
		t.Run(failure, func(t *testing.T) {
			candidate := view
			candidate.Providers = slices.Clone(view.Providers)
			candidate.Providers[0].APIKey = "replacement-key"
			switch failure {
			case "invalid view":
				candidate.Providers[0].ProviderType = "unsupported"
			case "adapter construction":
				candidate.Providers[1].APIKey = "replacement-key"
				candidate.Providers[1].ChatURL += "#invalid-fragment"
			case "file write":
				blockedHome := filepath.Join(t.TempDir(), "not-a-directory")
				require.NoError(t, os.WriteFile(blockedHome, []byte("blocked"), 0600))
				t.Setenv("HOME", blockedHome)
				t.Setenv("USERPROFILE", blockedHome)
			}
			require.Error(t, a.SaveLLMConfig(candidate))
			afterFile, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, beforeFile, afterFile)
			require.Equal(t, beforeRuntime, a.llmClient.Providers())
		})
	}
	// Client 尚未初始化时仍允许保存配置，供后续初始化加载。
	a.llmClient = nil
	view.Providers[0].APIKey = "saved-without-client"
	require.NoError(t, a.SaveLLMConfig(view))
	stored, err = llm.LoadUserConfig(path)
	require.NoError(t, err)
	require.Equal(t, "saved-without-client", stored.Adapters[adapter.ProtocolOpenAIChat].Providers[0].APIKey)
}

func TestProviderProbesUseExplicitProtocolAndType(t *testing.T) {
	type request struct {
		method string
		path   string
		header http.Header
	}
	requests := make(chan request, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requests <- request{r.Method, r.URL.Path, r.Header.Clone()}:
		default:
			t.Error("unexpected extra probe request")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"test rejection"}}`)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"test rejection"}}`)
	}))
	t.Cleanup(server.Close)
	a := &App{ctx: context.Background()}
	for _, tt := range []struct {
		protocol     adapter.Protocol
		providerType string
		endpoint     string
		models       string
		header       string
	}{
		{adapter.ProtocolOpenAIChat, "mimo", "/chat/v1/chat/completions", "/chat/v1/models", "api-key"},
		{adapter.ProtocolAnthropicMessages, "anthropic", "/claude/v8/messages", "/claude/v8/models", "x-api-key"},
	} {
		t.Run(string(tt.protocol), func(t *testing.T) {
			input := llm.ProviderProbeInput{AdapterName: tt.protocol, ProviderName: "my-relay", ProviderType: tt.providerType,
				ChatURL: server.URL + tt.endpoint, APIKey: "probe-key"}
			_, err := a.DiscoverModels(input)
			require.Error(t, err)
			result, err := a.TestConnection(TestConnectionInput{ProviderProbeInput: input, ModelID: "test-model"})
			require.NoError(t, err)
			require.Equal(t, input.ChatURL, result.URL)
			require.NotEmpty(t, result.Warning)
			for _, want := range []request{{method: http.MethodGet, path: tt.models}, {method: http.MethodPost, path: tt.endpoint}} {
				select {
				case got := <-requests:
					require.Equal(t, want.method, got.method)
					require.Equal(t, want.path, got.path)
					require.Equal(t, "probe-key", got.header.Get(tt.header))
					require.Empty(t, got.header.Get("Authorization"))
				default:
					t.Fatal("probe did not reach the selected adapter endpoint")
				}
			}
		})
	}
	require.Empty(t, requests)
}
