package mcp_tools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/web"
	"github.com/stretchr/testify/require"
)

type webSearchFunc func(context.Context, string) (*web.SearchResult, error)

func (f webSearchFunc) Search(ctx context.Context, query string) (*web.SearchResult, error) {
	return f(ctx, query)
}

type webFetchFunc func(context.Context, string) (*web.FetchResult, error)

func (f webFetchFunc) Fetch(ctx context.Context, url string) (*web.FetchResult, error) {
	return f(ctx, url)
}

type webToolTransport func(*http.Request) (*http.Response, error)

func (f webToolTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestWebSearchToolUsesInjectedSearcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg := testMCPRegistry(t, &WebSearchTool{})
	for _, items := range [][]web.SearchItem{
		{},
		{{Title: "来源", URL: "https://example.com", Summary: "摘要", Date: "2026-11-01T00:00:00Z", Truncated: true}},
	} {
		calls := 0
		result := reg.Execute(ctx, "web_search", []byte(`{"query":"如何查询资料"}`), ToolContext{
			WebSearch: webSearchFunc(func(got context.Context, query string) (*web.SearchResult, error) {
				calls++
				require.Equal(t, ctx, got)
				require.Equal(t, "如何查询资料", query)
				return &web.SearchResult{Query: query, Results: items}, nil
			}),
		}, nil)
		require.True(t, result.Success, result.Error)
		require.Equal(t, 1, calls)
		require.Equal(t, "如何查询资料", result.Data["query"])
		require.Equal(t, items, result.Data["results"])
		require.NotContains(t, result.Data, "summary")
	}
}

func TestWebToolsValidateParametersBeforeCallingDependencies(t *testing.T) {
	reg := testMCPRegistry(t, &WebSearchTool{})
	reg.Register(&WebFetchTool{})
	tc := ToolContext{
		WebSearch: webSearchFunc(func(context.Context, string) (*web.SearchResult, error) {
			t.Fatal("invalid arguments reached search provider")
			return nil, nil
		}),
		RemoteFetch: webFetchFunc(func(context.Context, string) (*web.FetchResult, error) {
			t.Fatal("invalid arguments reached fetch provider")
			return nil, nil
		}),
	}
	for _, test := range []struct{ name, args string }{
		{"web_search", `{}`},
		{"web_search", `{"query":""}`},
		{"web_search", `{"prompt":"旧参数"}`},
		{"web_search", `{"query":"` + strings.Repeat("中", 129) + `"}`},
		{"web_fetch", `{"url":"https://example.com","mode":"invalid"}`},
		{"web_fetch", `{"url":"ftp://example.com","mode":"remote"}`},
		{"web_fetch", `{"url":"https://example.com/` + strings.Repeat("a", 8192) + `","mode":"remote"}`},
	} {
		result := reg.Execute(context.Background(), test.name, []byte(test.args), tc, nil)
		require.False(t, result.Success, test.args)
		require.Equal(t, ErrKindArgs, result.ErrKind, result.Error)
	}
}

func TestWebToolSchemasDescribeQueryAndFetchModes(t *testing.T) {
	var schema struct {
		Required   []string                  `json:"required"`
		Properties map[string]map[string]any `json:"properties"`
	}
	require.NoError(t, json.Unmarshal((&WebSearchTool{}).JSONSchema(), &schema))
	require.Contains(t, schema.Required, "query")
	require.NotContains(t, schema.Properties, "prompt")
	require.Equal(t, float64(128), schema.Properties["query"]["maxLength"])
	schema.Required = nil
	require.NoError(t, json.Unmarshal((&WebFetchTool{}).JSONSchema(), &schema))
	require.Contains(t, schema.Required, "url")
	require.NotContains(t, schema.Required, "mode")
	require.Equal(t, "local", schema.Properties["mode"]["default"])
	require.ElementsMatch(t, []any{"local", "remote"}, schema.Properties["mode"]["enum"])
}

func TestWebFetchExplicitRemoteMode(t *testing.T) {
	reg := testMCPRegistry(t, &WebFetchTool{})
	calls := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := reg.Execute(ctx, "web_fetch", []byte(`{"url":"https://example.com/article","mode":"remote"}`), ToolContext{
		RemoteFetch: webFetchFunc(func(got context.Context, url string) (*web.FetchResult, error) {
			calls++
			require.Equal(t, ctx, got)
			require.Equal(t, "https://example.com/article", url)
			return &web.FetchResult{URL: url, Title: "文章", Text: "正文", Mode: web.RemoteFetch, Truncated: true}, nil
		}),
	}, nil)
	require.True(t, result.Success, result.Error)
	require.Equal(t, 1, calls)
	require.Equal(t, web.RemoteFetch, result.Data["mode"])
	require.Equal(t, true, result.Data["truncated"])
	require.Equal(t, "正文", result.Data["text"])
}

func TestWebFetchDefaultsToLocalWithoutAutomaticRemoteFallback(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	reg := testMCPRegistry(t, &WebFetchTool{})
	remoteCalls := 0
	tc := ToolContext{RemoteFetch: webFetchFunc(func(context.Context, string) (*web.FetchResult, error) {
		remoteCalls++
		return nil, errors.New("remote must only run when requested")
	})}
	for _, test := range []struct {
		name, mode string
		status     int
	}{
		{"default", "", http.StatusOK},
		{"explicit local", `,"mode":"local"`, http.StatusOK},
		{"local failure", "", http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			localCalls := 0
			http.DefaultTransport = webToolTransport(func(req *http.Request) (*http.Response, error) {
				localCalls++
				require.Equal(t, "203.0.113.1", req.URL.Hostname())
				body := "<html><head><title>文章</title></head><body><article><p>" + strings.Repeat("这是正常的文章内容。", 50) + "</p></article></body></html>"
				return &http.Response{StatusCode: test.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			result := reg.Execute(context.Background(), "web_fetch", []byte(`{"url":"https://203.0.113.1/article"`+test.mode+`}`), tc, nil)
			require.Equal(t, 1, localCalls)
			require.Zero(t, remoteCalls)
			if test.status == http.StatusOK {
				require.True(t, result.Success, result.Error)
				require.Equal(t, web.LocalFetch, result.Data["mode"])
				require.Equal(t, false, result.Data["truncated"])
			} else {
				require.False(t, result.Success)
				require.Equal(t, ErrKindSystem, result.ErrKind)
			}
		})
	}
}

func TestWebToolsMissingConfiguration(t *testing.T) {
	reg := testMCPRegistry(t, &WebSearchTool{})
	reg.Register(&WebFetchTool{})
	for _, test := range []struct{ name, args string }{
		{"web_search", `{"query":"查询"}`},
		{"web_fetch", `{"url":"https://example.com","mode":"remote"}`},
	} {
		result := reg.Execute(context.Background(), test.name, []byte(test.args), ToolContext{}, nil)
		require.False(t, result.Success)
		require.Equal(t, ErrKindBusiness, result.ErrKind)
		require.Contains(t, result.Error, "配置 DeepSeek")
	}
}

func TestWebToolsDistinguishRemoteRejectionsFromFailures(t *testing.T) {
	reg := testMCPRegistry(t, &WebSearchTool{})
	reg.Register(&WebFetchTool{})
	for _, status := range []int{400, 401, 402, 403, 404, 413, 429, 500} {
		dependencyError := &web.APIError{StatusCode: status, Message: "raw dependency error"}
		tc := ToolContext{
			WebSearch: webSearchFunc(func(context.Context, string) (*web.SearchResult, error) { return nil, dependencyError }),
			RemoteFetch: webFetchFunc(func(context.Context, string) (*web.FetchResult, error) {
				return nil, dependencyError
			}),
		}
		for _, test := range []struct{ name, args string }{
			{"web_search", `{"query":"查询"}`},
			{"web_fetch", `{"url":"https://example.com","mode":"remote"}`},
		} {
			result := reg.Execute(context.Background(), test.name, []byte(test.args), tc, nil)
			require.False(t, result.Success)
			if status == 429 || status == 500 {
				require.Equal(t, ErrKindSystem, result.ErrKind)
				require.Contains(t, result.Error, "raw dependency error")
			} else {
				require.Equal(t, ErrKindBusiness, result.ErrKind)
				require.NotContains(t, result.Error, "raw dependency error")
			}
		}
	}
}
