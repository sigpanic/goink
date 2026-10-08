package app

import (
	"context"
	"net/http"
	"sync"
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

func TestShutdownTimeoutPreservesMCPDependencies(t *testing.T) {
	a := setupTestApp(t)
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	a.mcpServer = mcpserver.New(a.registry, a.db, func(ctx context.Context) (mcpserver.CurrentNovel, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-release
		return mcpserver.CurrentNovel{}, ctx.Err()
	}, a.logger, nil)
	endpoint, err := a.mcpServer.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		unblock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, a.mcpServer.Stop(ctx))
	})
	callCtx, cancelCall := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelCall()
	client, err := mcpclient.Connect(callCtx, mcpclient.Config{
		Transport: mcpclient.TransportStreamableHTTP, Endpoint: endpoint.URL,
		HTTPClient: &http.Client{Transport: mcpTestAuthTransport{token: endpoint.Token}},
	})
	require.NoError(t, err)
	defer client.Close()
	defer unblock()
	callDone := make(chan struct{})
	go func() {
		defer close(callDone)
		_, _ = client.CallTool(callCtx, "get_current_novel", nil)
	}()
	select {
	case <-started:
	case <-callCtx.Done():
		t.Fatal("MCP 调用未开始")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelShutdown()
	a.OnShutdown(shutdownCtx)
	select {
	case <-canceled:
	default:
		t.Fatal("应用关停未取消 MCP 调用")
	}
	sqlDB, err := a.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.PingContext(context.Background()), "MCP 调用未退出时不应关闭数据库")

	unblock()
	stopCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStop()
	require.NoError(t, a.mcpServer.Stop(stopCtx))
	select {
	case <-callDone:
	case <-callCtx.Done():
		t.Fatal("MCP 客户端调用未结束")
	}
	a.OnShutdown(context.Background())
	require.Error(t, sqlDB.PingContext(context.Background()), "MCP 调用退出后应正常释放数据库")
}
