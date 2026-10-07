package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sigpanic/goink/internal/mcp_tools"
	"github.com/sigpanic/goink/internal/mcpclient"
)

type testNovelTool struct {
	name    string
	failure bool
}

func (t *testNovelTool) Name() string                     { return t.name }
func (t *testNovelTool) Description() string              { return "读取当前小说的测试工具" }
func (t *testNovelTool) Category() mcp_tools.ToolCategory { return mcp_tools.CategoryNovelManagement }
func (t *testNovelTool) JSONSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (t *testNovelTool) ExposeToLLM() bool { return true }
func (t *testNovelTool) NewArgs() any      { return &struct{}{} }
func (t *testNovelTool) Execute(_ context.Context, _ any, tc mcp_tools.ToolContext) (*mcp_tools.ToolResult, error) {
	if t.failure {
		return &mcp_tools.ToolResult{Success: false, Error: "小说不可读", Data: map[string]any{"reason": "测试原因"}}, nil
	}
	return &mcp_tools.ToolResult{Success: true, Data: map[string]any{"novel_id": tc.NovelID, "content": "测试内容"}}, nil
}

type bearerTransport struct {
	token  string
	origin string
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.Header.Set("Authorization", "Bearer "+t.token)
	if t.origin != "" {
		copy.Header.Set("Origin", t.origin)
	}
	return http.DefaultTransport.RoundTrip(copy)
}

func TestServerProtocolAndCurrentNovel(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := mcp_tools.NewRegistry(logger)
	registry.Register(&testNovelTool{name: "read_novel"})
	registry.Register(&testNovelTool{name: "failed_novel", failure: true})
	registry.Register(&testNovelTool{name: "blocked_tool"})
	var activeID atomic.Int64
	activeID.Store(1)
	server := New(registry, nil, func(context.Context) (CurrentNovel, error) {
		id := activeID.Load()
		if id == 0 {
			return CurrentNovel{}, ErrNoCurrentNovel
		}
		return CurrentNovel{ID: id, Title: "测试小说"}, nil
	}, logger, []string{"read_novel", "failed_novel"})
	endpoint, err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Stop(ctx); err != nil {
			t.Errorf("停止 MCP server 失败: %v", err)
		}
	}()
	if !strings.HasPrefix(endpoint.URL, "http://127.0.0.1:") || endpoint.Token == "" {
		t.Fatalf("本地端点不符合预期: %+v", endpoint)
	}
	if _, err := server.Start(); err == nil {
		t.Fatal("重复启动 MCP server 应被拒绝")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if client, err := mcpclient.Connect(ctx, mcpclient.Config{Transport: mcpclient.TransportStreamableHTTP, Endpoint: endpoint.URL}); err == nil {
		_ = client.Close()
		t.Fatal("未携带令牌的客户端不应连接成功")
	}
	if client, err := mcpclient.Connect(ctx, mcpclient.Config{
		Transport:  mcpclient.TransportStreamableHTTP,
		Endpoint:   endpoint.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{token: endpoint.Token, origin: "https://evil.example"}},
	}); err == nil {
		_ = client.Close()
		t.Fatal("非法 Origin 不应通过")
	}
	client, err := mcpclient.Connect(ctx, mcpclient.Config{
		Transport:  mcpclient.TransportStreamableHTTP,
		Endpoint:   endpoint.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{token: endpoint.Token}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 3 {
		t.Fatalf("对外工具数为 %d，期望 3", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
		if tool.InputSchema == nil {
			t.Fatalf("工具 %s 缺少参数 schema", tool.Name)
		}
	}
	if !names["get_current_novel"] || !names["read_novel"] || !names["failed_novel"] || names["blocked_tool"] {
		t.Fatalf("对外工具名单错误: %+v", names)
	}

	current, err := client.CallTool(ctx, "get_current_novel", nil)
	if err != nil || current.IsError || current.StructuredContent.(map[string]any)["novel_id"] != float64(1) {
		t.Fatalf("读取当前小说失败: result=%+v err=%v", current, err)
	}
	assertContentMatchesStructured(t, current)
	read, err := client.CallTool(ctx, "read_novel", nil)
	if err != nil || read.IsError || read.StructuredContent.(map[string]any)["novel_id"] != float64(1) {
		t.Fatalf("读取小说工具失败: result=%+v err=%v", read, err)
	}
	assertContentMatchesStructured(t, read)
	failure, err := client.CallTool(ctx, "failed_novel", nil)
	if err != nil || !failure.IsError {
		t.Fatalf("失败工具应返回执行错误: result=%+v err=%v", failure, err)
	}
	assertContentMatchesStructured(t, failure)
	failureData := failure.StructuredContent.(map[string]any)
	if failureData["error"] != "小说不可读" || failureData["data"].(map[string]any)["reason"] != "测试原因" {
		t.Fatalf("失败工具丢失了错误或数据: %+v", failureData)
	}

	activeID.Store(2)
	read, err = client.CallTool(ctx, "read_novel", nil)
	if err != nil || read.IsError || read.StructuredContent.(map[string]any)["novel_id"] != float64(2) {
		t.Fatalf("切书后使用了旧小说: result=%+v err=%v", read, err)
	}
	if result, err := client.CallTool(ctx, "blocked_tool", nil); err == nil && result != nil && !result.IsError {
		t.Fatalf("未授权工具仍可调用: %+v", result)
	}
	if result, err := client.CallTool(ctx, "read_novel", map[string]any{"unexpected": true}); err == nil && result != nil && !result.IsError {
		t.Fatalf("非法参数仍可调用: %+v", result)
	}

	activeID.Store(0)
	if result, err := client.CallTool(ctx, "get_current_novel", nil); err != nil || !result.IsError {
		t.Fatalf("没有当前小说时应拒绝调用: result=%+v err=%v", result, err)
	} else {
		assertContentMatchesStructured(t, result)
	}
}

func assertContentMatchesStructured(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("工具结果文本块数错误: %d", len(result.Content))
	}
	item, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("工具结果不是文本: %T", result.Content[0])
	}
	var data any
	if err := json.Unmarshal([]byte(item.Text), &data); err != nil {
		t.Fatalf("工具结果文本不是 JSON: %v", err)
	}
	if !reflect.DeepEqual(data, result.StructuredContent) {
		t.Fatalf("文本和结构化结果不一致: text=%+v structured=%+v", data, result.StructuredContent)
	}
}
