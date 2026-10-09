package mcpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sigpanic/goink/internal/mcpconfig"
	"github.com/stretchr/testify/require"
)

func TestManagerPermissionsAndCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := protocolTestServer(started, release)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	var requests atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer http-secret" {
			http.Error(w, "http-secret", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	manager := NewManager()
	defer manager.Close(ctx)
	config := mcpconfig.Connection{ID: "one", Transport: string(TransportStreamableHTTP), Endpoint: httpServer.URL}
	manager.Configure(config, mcpconfig.Secrets{BearerToken: "http-secret"})
	_, err := manager.CallTool(ctx, "one", "echo", nil)
	require.Error(t, err)
	require.Zero(t, requests.Load(), "禁用时不得隐式连接")
	list, err := manager.Refresh(ctx, "one")
	require.NoError(t, err)
	require.Len(t, list, 3)
	list[0].Name = "modified"
	require.NotEqual(t, "modified", manager.Tools("one")[0].Name)
	config.Enabled = true
	config.AllowedTools = map[string]bool{"echo": true, "wait": true}
	manager.SetAccess(config)
	require.Len(t, manager.Tools("one"), 3, "切换许可应保留管理员工具列表")
	result, err := manager.CallTool(ctx, "one", "echo", map[string]any{"text": "hello"})
	require.NoError(t, err)
	require.Equal(t, "hello", result.StructuredContent.(map[string]any)["echo"])
	_, err = manager.CallTool(ctx, "one", "fail", nil)
	require.Error(t, err)
	done := make(chan error, 1)
	go func() { _, err := manager.CallTool(ctx, "one", "wait", nil); done <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("等待调用未开始")
	}
	queuedCtx, cancelQueued := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancelQueued()
	queuedDone := make(chan error, 1)
	go func() { _, err := manager.Refresh(queuedCtx, "one"); queuedDone <- err }()
	select {
	case err := <-queuedDone:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("排队中的取消请求不应等待慢工具释放连接")
	}
	config.Enabled = false
	manager.SetAccess(config)
	select {
	case err := <-done:
		require.Error(t, err)
	case <-ctx.Done():
		t.Fatal("禁用没有取消旧连接")
	}
	_, err = manager.CallTool(ctx, "one", "echo", nil)
	require.Error(t, err)
	manager.Remove("one")
	require.Empty(t, manager.Tools("one"))
	_, err = manager.Refresh(ctx, "one")
	require.Error(t, err)
}

func TestManagerCloseCancelsAllConnections(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	manager := NewManager()
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, manager.Close(closeCtx))
	})
	done := make(chan error, 2)
	for _, id := range []string{"one", "two"} {
		started := make(chan struct{})
		release := make(chan struct{})
		server := protocolTestServer(started, release)
		httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true}))
		defer httpServer.Close()
		defer close(release)
		config := mcpconfig.Connection{
			ID: id, Transport: string(TransportStreamableHTTP), Endpoint: httpServer.URL,
			Enabled: true, AllowedTools: map[string]bool{"wait": true},
		}
		manager.Configure(config, mcpconfig.Secrets{})
		if id == "two" {
			manager.SetAccess(config)
		}
		go func() {
			_, err := manager.CallTool(ctx, id, "wait", nil)
			done <- err
		}()
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("等待调用未开始")
		}
	}
	closeCtx, cancelClose := context.WithTimeout(ctx, 8*time.Second)
	defer cancelClose()
	closed := make(chan error, 1)
	go func() { closed <- manager.Close(closeCtx) }()
	select {
	case <-manager.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("关闭 Manager 没有立即取消顶层上下文")
	}
	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-closeCtx.Done():
		t.Fatal("关闭 Manager 没有完成连接清理")
	}
	for range 2 {
		select {
		case err := <-done:
			require.Error(t, err)
		case <-closeCtx.Done():
			t.Fatal("关闭 Manager 没有取消所有连接请求")
		}
	}
	_, err := manager.Refresh(ctx, "one")
	require.Error(t, err)
}

func TestManagerStdioAndRedactedErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	manager := NewManager()
	defer manager.Close(ctx)
	manager.Configure(mcpconfig.Connection{
		ID: "stdio", Transport: string(TransportStdio), Command: os.Args[0],
		Args: []string{"-test.run=^TestMCPServerProcess$"}, Enabled: true, AllowedTools: map[string]bool{"echo": true},
	}, mcpconfig.Secrets{Env: map[string]string{"GOINK_MCP_TEST_SERVER": "1"}})
	list, err := manager.Refresh(ctx, "stdio")
	require.NoError(t, err)
	require.Len(t, list, 3)
	result, err := manager.CallTool(ctx, "stdio", "echo", map[string]any{"text": "stdio"})
	require.NoError(t, err)
	require.False(t, result.IsError)

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "private-token private-env", http.StatusUnauthorized)
	}))
	defer httpServer.Close()
	manager.Configure(mcpconfig.Connection{ID: "bad", Transport: string(TransportStreamableHTTP), Endpoint: httpServer.URL}, mcpconfig.Secrets{BearerToken: "private-token"})
	_, err = manager.Refresh(ctx, "bad")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private-token")
	require.NotContains(t, manager.Status("bad").LastError, "private-env")
	require.False(t, manager.Status("bad").Connected)
	_, err = manager.Refresh(ctx, "stdio")
	require.NoError(t, err, "一个来源失败不能影响其他来源")
}

func TestManagerRejectsCredentialRedirect(t *testing.T) {
	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Store(true)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	manager := NewManager()
	defer manager.Close(ctx)
	manager.Configure(mcpconfig.Connection{ID: "redirect", Transport: string(TransportStreamableHTTP), Endpoint: source.URL}, mcpconfig.Secrets{BearerToken: "secret"})
	_, err := manager.Refresh(ctx, "redirect")
	require.Error(t, err)
	require.False(t, leaked.Load())
}
