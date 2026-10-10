package mcpclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sigpanic/goink/internal/version"
)

// Transport 表示外部 MCP server 的连接方式。
type Transport string

const (
	TransportStdio          Transport = "stdio"
	TransportStreamableHTTP Transport = "streamable_http"
)

// Config 提供单个连接的运行时参数。调用方可通过 HTTPClient 注入认证能力；
// 持久化配置和凭据管理不属于本包。
type Config struct {
	Transport  Transport
	Command    string
	Args       []string
	Env        map[string]string
	Endpoint   string
	HTTPClient *http.Client
}

// Client 持有 SDK 返回的连接对象。Close 会释放传输连接，
// 使用 stdio 时也会关闭子进程。
type Client struct {
	session *mcp.ClientSession
}

// Connect 与外部 server 建立连接并协商协议版本。ctx 只限制此过程；
// 后续请求使用各自传入的 context。
func Connect(ctx context.Context, config Config) (*Client, error) {
	transport, err := newTransport(config)
	if err != nil {
		return nil, err
	}

	sdkClient := mcp.NewClient(&mcp.Implementation{Name: "goink", Version: version.Version}, nil)
	session, err := sdkClient.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect MCP server: %w", err)
	}
	return &Client{session: session}, nil
}

func newTransport(config Config) (mcp.Transport, error) {
	switch config.Transport {
	case TransportStdio:
		if strings.TrimSpace(config.Command) == "" {
			return nil, fmt.Errorf("stdio MCP server requires a command")
		}
		cmd := exec.Command(config.Command, config.Args...)
		if len(config.Env) > 0 {
			cmd.Env = os.Environ()
			for key, value := range config.Env {
				if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
					return nil, fmt.Errorf("invalid stdio MCP environment variable")
				}
				cmd.Env = append(cmd.Env, key+"="+value)
			}
		}
		return &mcp.CommandTransport{Command: cmd}, nil
	case TransportStreamableHTTP:
		endpoint, err := url.Parse(config.Endpoint)
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
			return nil, fmt.Errorf("streamable HTTP MCP server requires an http(s) endpoint")
		}
		return &mcp.StreamableClientTransport{
			Endpoint:   config.Endpoint,
			HTTPClient: config.HTTPClient,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported MCP transport %q", config.Transport)
	}
}

// ListTools 读取所有 tools/list 分页，并保留 server 返回的 schema。
func (c *Client) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	var tools []*mcp.Tool
	for tool, err := range c.session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("list MCP tools: %w", err)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

// CallTool 保留完整 MCP 结果，包括结构化内容和 IsError。
// 工具自身失败仍是调用结果，不当作传输错误。
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	if name == "" {
		return nil, fmt.Errorf("MCP tool name is required")
	}
	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("call MCP tool %q: %w", name, err)
	}
	return result, nil
}

// Close 结束 MCP 会话，并在使用 stdio 时停止子进程。
func (c *Client) Close() error {
	return c.session.Close()
}
