package app

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestLoadLLMClientFallsBackOnAdapterConstructionFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	a := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	user := &llm.UserLLMConfig{Version: llm.ConfigVersion, Adapters: map[adapter.Protocol]llm.AdapterConfig{
		adapter.ProtocolAnthropicMessages: {Providers: []adapter.Provider{{
			Name: "relay", ProviderType: "anthropic", APIKey: "stored-key",
			ChatURL: "https://example.invalid/v8/messages#invalid-fragment",
		}}},
	}}
	path := config.LLMConfigPath()
	require.NoError(t, llm.SaveUserConfig(path, user))
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	client, err := a.loadLLMClient()
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Empty(t, llm.Models(client.Providers()))
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
