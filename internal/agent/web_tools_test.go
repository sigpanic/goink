package agent

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/web"
	"github.com/stretchr/testify/require"
)

type agentWebTransport func(*http.Request) (*http.Response, error)

func (f agentWebTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestBuildWebToolsUsesCurrentDeepSeekConfiguration(t *testing.T) {
	a := &Agent{}
	searcher, fetcher := a.buildWebTools()
	require.Nil(t, searcher)
	require.Nil(t, fetcher)
	client, err := llm.NewClient(&llm.UserLLMConfig{Version: llm.ConfigVersion, Adapters: map[adapter.Protocol]llm.AdapterConfig{}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	a.llm = client
	searcher, fetcher = a.buildWebTools()
	require.Nil(t, searcher)
	require.Nil(t, fetcher)
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	for _, key := range []string{"first-key", "updated-key"} {
		require.NoError(t, client.Reload(&llm.UserLLMConfig{Version: llm.ConfigVersion, Adapters: map[adapter.Protocol]llm.AdapterConfig{
			adapter.ProtocolOpenAIChat: {Providers: []adapter.Provider{{Name: "DeepSeek", APIKey: key}}},
		}}))
		searcher, fetcher = a.buildWebTools()
		require.IsType(t, &web.DeepSeekClient{}, searcher)
		require.Same(t, searcher, fetcher)
		calls := 0
		http.DefaultTransport = agentWebTransport(func(req *http.Request) (*http.Response, error) {
			calls++
			require.Equal(t, "api.deepseek.com", req.URL.Host)
			require.Equal(t, "Bearer "+key, req.Header.Get("Authorization"))
			body := `{"results":[]}`
			if req.URL.Path == "/v1/tools/fetch" {
				body = `{"title":"来源","content":"正文"}`
			} else {
				require.Equal(t, "/v1/tools/search", req.URL.Path)
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		_, err := searcher.Search(context.Background(), "查询")
		require.NoError(t, err)
		_, err = fetcher.Fetch(context.Background(), "https://example.com/article")
		require.NoError(t, err)
		require.Equal(t, 2, calls)
	}
	require.NoError(t, client.Reload(&llm.UserLLMConfig{Version: llm.ConfigVersion, Adapters: map[adapter.Protocol]llm.AdapterConfig{}}))
	searcher, fetcher = a.buildWebTools()
	require.Nil(t, searcher)
	require.Nil(t, fetcher)
}
