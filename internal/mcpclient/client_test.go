package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func protocolTestServer(waitStarted chan<- struct{}, releaseWait <-chan struct{}) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "goink-mcpclient-test", Version: "1.0.0"}, &mcp.ServerOptions{PageSize: 1})
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
	}
	server.AddTool(&mcp.Tool{Name: "echo", Description: "回显文本", InputSchema: schema}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: args.Text}},
			StructuredContent: map[string]any{"echo": args.Text},
		}, nil
	})
	server.AddTool(&mcp.Tool{Name: "fail", InputSchema: schema}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "工具失败"}}}, nil
	})
	server.AddTool(&mcp.Tool{Name: "wait", InputSchema: schema}, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if waitStarted != nil {
			close(waitStarted)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-releaseWait:
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "已结束等待"}}}, nil
		}
	})
	return server
}

func TestMCPServerProcess(t *testing.T) {
	if os.Getenv("GOINK_MCP_TEST_SERVER") != "1" {
		return
	}
	if err := protocolTestServer(nil, nil).Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestStdioClient(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := Connect(ctx, Config{
		Transport: TransportStdio,
		Command:   os.Args[0],
		Args:      []string{"-test.run=^TestMCPServerProcess$"},
		Env:       map[string]string{"GOINK_MCP_TEST_SERVER": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	assertToolListAndCall(t, ctx, client)
	if err := client.Close(); err != nil {
		t.Fatalf("关闭 stdio MCP 会话失败: %v", err)
	}
}

func TestStreamableHTTPClient(t *testing.T) {
	waitStarted := make(chan struct{})
	releaseWait := make(chan struct{})
	server := protocolTestServer(waitStarted, releaseWait)
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{JSONResponse: true}))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := Connect(ctx, Config{Transport: TransportStreamableHTTP, Endpoint: httpServer.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	defer close(releaseWait)

	assertToolListAndCall(t, ctx, client)

	callCtx, cancelCall := context.WithCancel(ctx)
	callDone := make(chan error, 1)
	go func() {
		_, err := client.CallTool(callCtx, "wait", nil)
		callDone <- err
	}()
	select {
	case <-waitStarted:
	case <-ctx.Done():
		t.Fatalf("等待工具调用启动超时: %v", ctx.Err())
	}
	cancelCall()
	select {
	case err := <-callDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("取消调用返回 %v，期望 context.Canceled", err)
		}
	case <-ctx.Done():
		t.Fatalf("取消调用未及时结束: %v", ctx.Err())
	}
}

func TestStreamableHTTPSessionlessClient(t *testing.T) {
	server := protocolTestServer(nil, nil)
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := Connect(ctx, Config{Transport: TransportStreamableHTTP, Endpoint: httpServer.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if got := client.session.InitializeResult().ProtocolVersion; got != "2026-07-28" {
		t.Fatalf("协商协议版本为 %q，期望 2026-07-28", got)
	}
	assertToolListAndCall(t, ctx, client)
}

func assertToolListAndCall(t *testing.T, ctx context.Context, client *Client) {
	t.Helper()
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 3 {
		t.Fatalf("tools/list 分页后得到 %d 个工具，期望 3 个", len(tools))
	}
	foundEcho := false
	for _, tool := range tools {
		if tool.Name == "echo" {
			foundEcho = true
			if tool.Description != "回显文本" || tool.InputSchema == nil {
				t.Fatalf("echo 工具元数据丢失: %+v", tool)
			}
		}
	}
	if !foundEcho {
		t.Fatal("tools/list 未返回 echo 工具")
	}

	result, err := client.CallTool(ctx, "echo", map[string]any{"text": "你好"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("echo 返回异常: %+v", result)
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok || content.Text != "你好" {
		t.Fatalf("echo 文本内容异常: %+v", result.Content)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["echo"] != "你好" {
		t.Fatalf("echo 结构化内容异常: %+v", result.StructuredContent)
	}

	failure, err := client.CallTool(ctx, "fail", nil)
	if err != nil {
		t.Fatalf("工具错误被误当成传输错误: %v", err)
	}
	if !failure.IsError {
		t.Fatalf("工具错误标记丢失: %+v", failure)
	}
}

func TestConnectRejectsInvalidConfig(t *testing.T) {
	for _, config := range []Config{
		{Transport: TransportStdio},
		{Transport: TransportStreamableHTTP, Endpoint: "file:///tmp/mcp"},
		{Transport: TransportStdio, Command: "test", Env: map[string]string{"BAD=KEY": "value"}},
		{Transport: Transport("unknown")},
	} {
		_, err := Connect(context.Background(), config)
		if err == nil || !strings.Contains(err.Error(), "MCP") {
			t.Fatalf("配置 %+v 返回错误 %v", config, err)
		}
	}
}
