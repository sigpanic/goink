package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/mcpclient"
	"github.com/sigpanic/goink/internal/mcpserver"
	"github.com/stretchr/testify/require"
)

type mcpTestAuthTransport struct{ token string }

func (t mcpTestAuthTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	copy.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(copy)
}

func TestMCPServerUsesCurrentNovelWithRegisteredTool(t *testing.T) {
	a := setupTestApp(t)
	created, err := a.CreateNovel(CreateNovelInput{Title: "MCP 测试小说"})
	require.NoError(t, err)
	require.NoError(t, a.SetActiveNovel(SetActiveNovelInput{NovelID: created.ID}))

	server := mcpserver.New(a.registry, a.db, a.currentNovel, a.logger, []string{"get_chapter_list"})
	endpoint, err := server.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, server.Stop(ctx))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mcpclient.Connect(ctx, mcpclient.Config{
		Transport:  mcpclient.TransportStreamableHTTP,
		Endpoint:   endpoint.URL,
		HTTPClient: &http.Client{Transport: mcpTestAuthTransport{token: endpoint.Token}},
	})
	require.NoError(t, err)
	defer client.Close()

	result, err := client.CallTool(ctx, "get_chapter_list", nil)
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Equal(t, "暂无章节。", result.StructuredContent.(map[string]any)["content"])

	require.NoError(t, a.SetActiveNovel(SetActiveNovelInput{NovelID: 0}))
	result, err = client.CallTool(ctx, "get_chapter_list", nil)
	require.NoError(t, err)
	require.True(t, result.IsError)
}
