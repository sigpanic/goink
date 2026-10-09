// TODO: 因计划调整，本文件暂时保留，但尚未经详细 review。
// 未来开展 MCP Client 功能时，需要重新 review。

package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sigpanic/goink/internal/mcpconfig"
)

type ConnectionStatus struct {
	Connected bool   `json:"connected"`
	ToolCount int    `json:"tool_count"`
	LastError string `json:"last_error"`
}

type managedConnection struct {
	config mcpconfig.Connection
	secret mcpconfig.Secrets
	ctx    context.Context
	cancel context.CancelFunc
	op     chan struct{}
	client *Client
	tools  []*mcp.Tool
	status ConnectionStatus
}

// Manager 按 server 隔离连接与请求。配置替换立即取消旧连接的请求，旧刷新结果不会回填新目录。
type Manager struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	entries map[string]*managedConnection
	closed  bool
	closing sync.WaitGroup
}

func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{ctx: ctx, cancel: cancel, entries: map[string]*managedConnection{}}
}

// Configure 接收后端私有快照，不接受前端直接传入的运行时连接对象。
func (m *Manager) Configure(config mcpconfig.Connection, secret mcpconfig.Secrets) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.retireLocked(config.ID)
	ctx, cancel := context.WithCancel(m.ctx)
	m.entries[config.ID] = &managedConnection{config: config, secret: secret, ctx: ctx, cancel: cancel, op: make(chan struct{}, 1)}
}

func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retireLocked(id)
}

// SetAccess 保留管理员已发现的工具列表，但撤销旧连接及其在途请求。
func (m *Manager) SetAccess(config mcpconfig.Connection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.entries[config.ID]
	if old == nil || m.closed {
		return
	}
	m.retireLocked(config.ID)
	ctx, cancel := context.WithCancel(m.ctx)
	m.entries[config.ID] = &managedConnection{
		config: config, secret: old.secret, ctx: ctx, cancel: cancel,
		op:    make(chan struct{}, 1),
		tools: old.tools, status: ConnectionStatus{ToolCount: len(old.tools)},
	}
}

func (m *Manager) retireLocked(id string) {
	e := m.entries[id]
	if e == nil {
		return
	}
	delete(m.entries, id)
	e.cancel()
	m.closing.Go(func() {
		e.op <- struct{}{}
		defer func() { <-e.op }()
		if e.client != nil {
			_ = e.client.Close()
		}
	})
}

func (m *Manager) Status(id string) ConnectionStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.entries[id]; e != nil {
		return e.status
	}
	return ConnectionStatus{}
}

// Refresh 也用于手动连接测试：允许用户测试已保存但尚未启用的配置。
// 测试不会修改启用状态或逐工具许可。
func (m *Manager) Refresh(ctx context.Context, id string) ([]*mcp.Tool, error) {
	e, err := m.entry(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := requestContext(ctx, e.ctx)
	defer cancel()
	if err := e.acquire(ctx); err != nil {
		return nil, err
	}
	defer func() { <-e.op }()
	if err := m.connect(ctx, e); err != nil {
		return nil, err
	}
	list, err := e.client.ListTools(ctx)
	if err != nil {
		return nil, m.failed(e, ctx, "读取工具列表失败，请检查连接并重试")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries[id] != e || ctx.Err() != nil {
		return nil, errors.New("MCP 配置已变更或请求已取消")
	}
	e.tools = list
	e.status = ConnectionStatus{Connected: true, ToolCount: len(list)}
	return cloneTools(list), nil
}

func (m *Manager) Tools(id string) []*mcp.Tool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.entries[id]; e != nil {
		return cloneTools(e.tools)
	}
	return nil
}

// CallTool 在发送前重新核对当前配置与许可；新发现的工具默认不可调用。
func (m *Manager) CallTool(ctx context.Context, id, name string, args map[string]any) (*mcp.CallToolResult, error) {
	e, err := m.entry(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := requestContext(ctx, e.ctx)
	defer cancel()
	if err := e.acquire(ctx); err != nil {
		return nil, err
	}
	defer func() { <-e.op }()
	m.mu.Lock()
	allowed := m.entries[id] == e && e.config.Enabled && e.config.AllowedTools[name]
	m.mu.Unlock()
	if !allowed {
		return nil, errors.New("MCP server 或工具未启用")
	}
	if err := m.connect(ctx, e); err != nil {
		return nil, err
	}
	result, err := e.client.CallTool(ctx, name, args)
	if err != nil {
		return nil, m.failed(e, ctx, "MCP 工具调用失败，请检查连接并刷新工具列表")
	}
	if ctx.Err() != nil {
		return nil, errors.New("MCP 请求已取消或超时")
	}
	return result, nil
}

func (m *Manager) entry(id string) (*managedConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.entries[id] == nil {
		return nil, errors.New("MCP 连接不存在或服务已关闭")
	}
	return m.entries[id], nil
}

func (m *Manager) connect(ctx context.Context, e *managedConnection) error {
	if ctx.Err() != nil {
		return errors.New("MCP 请求已取消或超时")
	}
	if e.client != nil {
		return nil
	}
	config := Config{
		Transport: Transport(e.config.Transport), Command: e.config.Command,
		Args: e.config.Args, Env: e.secret.Env, Endpoint: e.config.Endpoint,
		HTTPClient: &http.Client{
			Transport:     credentialTransport{token: e.secret.BearerToken},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	client, err := Connect(ctx, config)
	if err != nil {
		return m.failed(e, ctx, "连接 MCP server 失败，请检查命令、地址和认证配置")
	}
	e.client = client
	m.mu.Lock()
	if m.entries[e.config.ID] == e {
		e.status.Connected = true
		e.status.LastError = ""
	}
	m.mu.Unlock()
	return nil
}

func (e *managedConnection) acquire(ctx context.Context) error {
	select {
	case e.op <- struct{}{}:
		return nil
	case <-ctx.Done():
		return errors.New("MCP 请求已取消或超时")
	}
}

func (m *Manager) failed(e *managedConnection, ctx context.Context, message string) error {
	if ctx.Err() != nil {
		message = "MCP 请求已取消或超时"
	}
	if e.client != nil {
		_ = e.client.Close()
		e.client = nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries[e.config.ID] == e {
		e.tools = nil
		e.status = ConnectionStatus{LastError: message}
	}
	// 不回传外部 server 的原始错误，它可能回显认证头、参数或环境变量。
	return errors.New(message)
}

func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	m.cancel()
	for id := range m.entries {
		m.retireLocked(id)
	}
	m.mu.Unlock()
	done := make(chan struct{})
	go func() { m.closing.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func requestContext(parent, lifetime context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	stop := context.AfterFunc(lifetime, cancel)
	if lifetime.Err() != nil {
		cancel()
	}
	return ctx, func() { stop(); cancel() }
}

type credentialTransport struct{ token string }

func (t credentialTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	if t.token != "" {
		copy.Header.Set("Authorization", "Bearer "+t.token)
	}
	return http.DefaultTransport.RoundTrip(copy)
}

func cloneTools(list []*mcp.Tool) []*mcp.Tool {
	// SDK 的工具 schema 也含 map/slice，通过 JSON 拷贝避免调用方改写缓存。
	data, _ := json.Marshal(list)
	result := []*mcp.Tool{}
	_ = json.Unmarshal(data, &result)
	return result
}
