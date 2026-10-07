package mcpserver

import (
	"context"
	"crypto/rand"
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
	http *http.Server
	done chan struct{}
}

// Server 将明确选中的 Goink 工具提供为本地 MCP 协议端点。
type Server struct {
	registry *mcp_tools.Registry
	db       *gorm.DB
	current  CurrentNovelFunc
	logger   *slog.Logger
	allowed  map[string]bool

	mu      sync.Mutex
	running *runningServer
}

// New 构建默认不监听的服务。allowed 只包含明确允许对外调用的工具名。
func New(registry *mcp_tools.Registry, db *gorm.DB, current CurrentNovelFunc, logger *slog.Logger, allowed []string) *Server {
	allowSet := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowSet[name] = true
	}
	return &Server{registry: registry, db: db, current: current, logger: logger, allowed: allowSet}
}

// Start 显式启动回环地址上的 Streamable HTTP 服务，并生成本次运行的令牌。
func (s *Server) Start() (Endpoint, error) {
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

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Endpoint{}, fmt.Errorf("监听本地 MCP 端点: %w", err)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		_ = listener.Close()
		return Endpoint{}, fmt.Errorf("生成 MCP 访问令牌: %w", err)
	}
	token := hex.EncodeToString(secret)

	protocolServer := mcp.NewServer(&mcp.Implementation{Name: "goink", Version: version.Version}, nil)
	s.addTools(protocolServer)
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return protocolServer
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	mux := http.NewServeMux()
	mux.Handle("/mcp", s.authorize(token, mcpHandler))
	httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	instance := &runningServer{http: httpServer, done: make(chan struct{})}
	s.running = instance
	go func() {
		defer close(instance.done)
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("本地 MCP server 停止", "err", err)
		}
	}()
	return Endpoint{URL: "http://" + listener.Addr().String() + "/mcp", Token: token}, nil
}

// Stop 停止接收新请求，等待在途调用结束；超时后强制关闭连接。
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	instance := s.running
	s.mu.Unlock()
	if instance == nil {
		return nil
	}
	err := instance.http.Shutdown(ctx)
	if err != nil {
		_ = instance.http.Close()
	}
	<-instance.done
	s.mu.Lock()
	if s.running == instance {
		s.running = nil
	}
	s.mu.Unlock()
	return err
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

func (s *Server) addTools(protocolServer *mcp.Server) {
	protocolServer.AddTool(&mcp.Tool{
		Name:        "get_current_novel",
		Description: "获取 Goink 界面当前打开的小说 ID 和书名。",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
	}, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		novel, err := s.current(ctx)
		if err != nil {
			return s.currentNovelError(err), nil
		}
		return encodeResult(map[string]any{"novel_id": novel.ID, "title": novel.Title}, false)
	})

	for name := range s.allowed {
		tool, _ := s.registry.Get(name)
		protocolServer.AddTool(&mcp.Tool{Name: name, Description: tool.Description(), InputSchema: tool.JSONSchema()}, s.toolHandler(name))
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
