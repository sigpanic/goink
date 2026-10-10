package mcpserver

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sigpanic/goink/internal/mcp_tools"
	"github.com/sigpanic/goink/internal/version"
	"gorm.io/gorm"
)

// ErrNoCurrentNovel 表示 Goink 界面当前没有打开的小说。
var ErrNoCurrentNovel = errors.New("当前没有打开的小说")

const DefaultPort = 17890

// CurrentNovel 是每次 MCP 调用看到的当前小说快照。
type CurrentNovel struct {
	ID    int64
	Title string
}

// CurrentNovelFunc 在请求开始时读取当前小说，不使用上次打开的历史设置。
type CurrentNovelFunc func(context.Context) (CurrentNovel, error)

// Endpoint 是显式启动本地服务后返回的连接信息。Token 只在启动时返回。
type Endpoint struct {
	URL   string
	Token string
}

type runningServer struct {
	url      string
	http     *http.Server
	done     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	stopping bool
	calls    sync.WaitGroup
	stopOnce sync.Once
	stopped  chan struct{}
	stopErr  error
}

func (instance *runningServer) track(handler mcp.ToolHandler) mcp.ToolHandler {
	return func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		instance.mu.Lock()
		if instance.stopping {
			instance.mu.Unlock()
			return toolError("MCP server 正在停止"), nil
		}
		// 与关闭入口共用锁，停止登记后才允许 Wait，避免零计数时 Add/Wait 竞态。
		instance.calls.Add(1)
		instance.mu.Unlock()
		defer instance.calls.Done()

		callCtx, cancel := context.WithCancel(ctx)
		stopCancel := context.AfterFunc(instance.ctx, cancel)
		defer stopCancel()
		defer cancel()
		if instance.ctx.Err() != nil {
			cancel()
		}
		return handler(callCtx, request)
	}
}

// Server 将明确选中的 Goink 工具提供为本地 MCP 协议端点。
type Server struct {
	registry *mcp_tools.Registry
	db       *gorm.DB
	current  CurrentNovelFunc
	logger   *slog.Logger
	allowed  map[string]bool
	port     int

	mu      sync.Mutex
	running *runningServer
}

// New 构建默认不监听的服务。allowed 只包含明确允许对外调用的工具名。
func New(registry *mcp_tools.Registry, db *gorm.DB, current CurrentNovelFunc, logger *slog.Logger, allowed []string) *Server {
	allowSet := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowSet[name] = true
	}
	return &Server{registry: registry, db: db, current: current, logger: logger, allowed: allowSet, port: DefaultPort}
}

// SetPort 在服务停止时设置端口；0 供测试显式申请临时端口。
func (s *Server) SetPort(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running != nil {
		return fmt.Errorf("请先停止 MCP server")
	}
	if port < 0 || port > 65535 {
		return fmt.Errorf("MCP 端口必须在 0 到 65535 之间")
	}
	s.port = port
	return nil
}

// Start 使用调用方提供的令牌启动本地服务；令牌的生成和持久化由管理层负责。
func (s *Server) Start(token string) (Endpoint, error) {
	secret, err := hex.DecodeString(token)
	if err != nil || len(secret) != 32 {
		return Endpoint{}, fmt.Errorf("MCP 访问令牌无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running != nil {
		return Endpoint{}, fmt.Errorf("MCP server 已启动")
	}
	if s.registry == nil || s.current == nil || s.logger == nil {
		return Endpoint{}, fmt.Errorf("MCP server 依赖未初始化")
	}
	for name := range s.allowed {
		if name == "get_current_novel" {
			return Endpoint{}, fmt.Errorf("get_current_novel 是 MCP server 内置工具")
		}
		if _, ok := s.registry.Get(name); !ok {
			return Endpoint{}, fmt.Errorf("MCP server 工具不存在: %s", name)
		}
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.port))
	if err != nil {
		return Endpoint{}, fmt.Errorf("监听本地 MCP 端点: %w", err)
	}
	serverCtx, cancel := context.WithCancel(context.Background())
	instance := &runningServer{
		url:  "http://" + listener.Addr().String() + "/mcp",
		done: make(chan struct{}), ctx: serverCtx, cancel: cancel,
		stopped: make(chan struct{}),
	}
	protocolServer := mcp.NewServer(&mcp.Implementation{Name: "goink", Version: version.Version}, nil)
	s.addTools(protocolServer, instance)
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return protocolServer
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	mux := http.NewServeMux()
	mux.Handle("/mcp", s.authorize(token, mcpHandler))
	httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	instance.http = httpServer
	s.running = instance
	go func() {
		defer close(instance.done)
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("本地 MCP server 停止", "err", err)
		}
	}()
	return Endpoint{URL: instance.url, Token: token}, nil
}

type Status struct {
	Running  bool   `json:"running"`
	Stopping bool   `json:"stopping"`
	URL      string `json:"url"`
}

func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running == nil {
		return Status{}
	}
	s.running.mu.Lock()
	defer s.running.mu.Unlock()
	return Status{Running: !s.running.stopping, Stopping: s.running.stopping, URL: s.running.url}
}

// SetAllowedTools 只在旧请求排空后替换策略，运行中的请求始终使用原来的不可变白名单。
func (s *Server) SetAllowedTools(names []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running != nil {
		return fmt.Errorf("请先停止 MCP server")
	}
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		if _, ok := s.registry.Get(name); !ok || name == "get_current_novel" {
			return fmt.Errorf("MCP server 工具不可用")
		}
		allowed[name] = true
	}
	s.allowed = allowed
	return nil
}

// Stop 停止接收新请求，等待在途调用结束；超时后强制关闭连接。
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	instance := s.running
	s.mu.Unlock()
	if instance == nil {
		return nil
	}
	instance.stopOnce.Do(func() {
		instance.mu.Lock()
		instance.stopping = true
		instance.cancel()
		instance.mu.Unlock()
		// 调用方超时只结束本次等待；后台继续排空，完成前不能重启同一个服务。
		go s.finishStop(ctx, instance)
	})
	select {
	case <-instance.stopped:
		return instance.stopErr
	case <-ctx.Done():
		_ = instance.http.Close()
		return ctx.Err()
	}
}

func (s *Server) finishStop(ctx context.Context, instance *runningServer) {
	if err := instance.http.Shutdown(ctx); err != nil {
		instance.stopErr = instance.http.Close()
	}
	instance.calls.Wait()
	<-instance.done
	s.mu.Lock()
	if s.running == instance {
		s.running = nil
	}
	s.mu.Unlock()
	close(instance.stopped)
}

func (s *Server) authorize(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedOrigin(r.Header.Get("Origin")) {
			http.Error(w, "禁止的来源", http.StatusForbidden)
			return
		}
		candidate, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) != 1 {
			http.Error(w, "未授权", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowedOrigin(origin string) bool {
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) addTools(protocolServer *mcp.Server, instance *runningServer) {
	protocolServer.AddTool(&mcp.Tool{
		Name:        "get_current_novel",
		Description: "获取 Goink 界面当前打开的小说 ID 和书名。",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
	}, instance.track(func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		novel, err := s.current(ctx)
		if err != nil {
			return s.currentNovelError(err), nil
		}
		return encodeResult(map[string]any{"novel_id": novel.ID, "title": novel.Title}, false)
	}))

	for name := range s.allowed {
		tool, _ := s.registry.Get(name)
		protocolServer.AddTool(&mcp.Tool{Name: name, Description: tool.Description(), InputSchema: tool.JSONSchema()}, instance.track(s.toolHandler(name)))
	}
}

func (s *Server) toolHandler(name string) mcp.ToolHandler {
	return func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		novel, err := s.current(ctx)
		if err != nil {
			return s.currentNovelError(err), nil
		}
		result := s.registry.Execute(ctx, name, request.Params.Arguments, mcp_tools.ToolContext{
			DB: s.db, NovelID: novel.ID, Logger: s.logger,
		}, s.allowed)
		if result == nil {
			return toolError("工具没有返回结果"), nil
		}
		var payload map[string]any
		if result.Success {
			payload = result.Data
		} else {
			payload = map[string]any{"error": result.Error, "data": result.Data}
		}
		if payload == nil {
			payload = map[string]any{}
		}
		mapped, err := encodeResult(payload, !result.Success)
		if err != nil {
			s.logger.Error("编码 MCP 工具结果失败", "tool", name, "err", err)
			return toolError("工具结果无法编码"), nil
		}
		return mapped, nil
	}
}

func encodeResult(data map[string]any, isError bool) (*mcp.CallToolResult, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(encoded)}},
		StructuredContent: data,
		IsError:           isError,
	}, nil
}

func (s *Server) currentNovelError(err error) *mcp.CallToolResult {
	if errors.Is(err, ErrNoCurrentNovel) {
		return toolError(ErrNoCurrentNovel.Error())
	}
	s.logger.Error("获取当前小说失败", "err", err)
	return toolError("获取当前小说失败")
}

func toolError(message string) *mcp.CallToolResult {
	result, _ := encodeResult(map[string]any{"error": message}, true)
	return result
}
