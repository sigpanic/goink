package mcp_tools

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/invopop/jsonschema"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/approval"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/search"
	"github.com/sigpanic/goink/internal/skill"
	"github.com/sigpanic/goink/internal/storage"
)

// ── 接口 ──────────────────────────────────────────────

// Tool 是每个 MCP 工具实现的接口。
type Tool interface {
	Name() string
	Description() string
	Category() ToolCategory
	JSONSchema() json.RawMessage
	ExposeToLLM() bool
	NewArgs() any // 返回零值 args 指针，供 Registry 反序列化 + 校验用
	Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error)
}

// ── 上下文 ────────────────────────────────────────────

// SubAgentRequest 传递给子 Agent 运行器的参数。
type SubAgentRequest struct {
	AgentType   string // "memory" | "review"
	NovelID     int64
	ToolID      string // run_subagent 的 tool call ID，子 Agent 事件路由用
	Instruction string
}

// RunSubAgentFunc 由 agent 包注入，避免循环依赖。
type RunSubAgentFunc func(ctx context.Context, req SubAgentRequest) (report string, err error)

// ToolContext 是工具执行时注入的上下文。
type ToolContext struct {
	DB           *gorm.DB
	NovelID      int64
	ToolID       string
	RawArgs      json.RawMessage                                                  // 原始 JSON，update 工具用于 PATCH DB 实体
	Approver     approval.Approver                                                // 审批能力，nil 表示工具无需审批
	EmitApproval func(toolID string, approvalType string, payload map[string]any) // 向 agent 事件流推送审批事件（EventToolCall, phase="awaiting_approval"）

	RunSubAgent   RunSubAgentFunc                                                       // 子 Agent 运行器，由 agent 包注入
	SkillStore    *skill.Store                                                          // 技能存储，read 工具用于读取内置 skill
	SearchService *search.Service                                                       // 搜索服务，write 工具用于更新正文缓存
	WebSearch     func(ctx context.Context, query string) (*llm.WebSearchResult, error) // 网络搜索，由 agent 注入 DeepSeek 闭包；nil 表示未配置
}

// ── 结果 ──────────────────────────────────────────────

// ToolResult 是工具执行结果。
type ToolResult struct {
	Success  bool            `json:"success"`
	Data     map[string]any  `json:"data,omitempty"`
	Error    string          `json:"error,omitempty"`
	ErrKind  string          `json:"err_kind,omitempty"` // "" = 业务错误，"system" = 系统错误
	Metadata map[string]any  `json:"metadata,omitempty"`
	Inject   []InjectMessage `json:"inject,omitempty"`
}

// InjectMessage 由工具返回，agent loop 会后追加到对话流。固定 to_api=true, to_frontend=false。
type InjectMessage struct {
	Role    string `json:"role"` // "user" | "system"
	Content string `json:"content"`
}

// ── 分类 ──────────────────────────────────────────────

// ToolCategory 是工具分类，仅作组织用途。
type ToolCategory string

const (
	CategoryNovelManagement  ToolCategory = "novel_management"
	CategoryMemoryRetrieval  ToolCategory = "memory_retrieval"
	CategoryConsistencyCheck ToolCategory = "consistency_check"
	CategoryWritingAssistant ToolCategory = "writing_assistant"
)

// ── 展示 ──────────────────────────────────────────────

// DisplayPhase 表示工具展示所处的阶段。
type DisplayPhase int

const (
	PhaseSelected  DisplayPhase = iota // 工具被选中
	PhaseExecuting                     // 正在执行
	PhaseCompleted                     // 执行成功
	PhaseFailed                        // 执行失败
	PhaseCancelled                     // 用户取消
)

// DisplayInfo 是某个阶段的前端展示信息。
type DisplayInfo struct {
	DisplayText  string
	ActivityKind string
	Metadata     map[string]any
}

// ── 注册表 ────────────────────────────────────────────

// Registry 是 MCP 工具注册表，无全局单例，由 main.go 创建后注入。
type Registry struct {
	tools    map[string]Tool
	logger   *slog.Logger
	validate *validator.Validate
}

// NewRegistry 创建新的工具注册表。
func NewRegistry(logger *slog.Logger) *Registry {
	return &Registry{
		tools:    make(map[string]Tool),
		logger:   logger,
		validate: validator.New(),
	}
}

// Register 注册工具。同名工具后注册的覆盖先注册的。
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// Get 按名获取工具。
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List 返回全部已注册工具。
func (r *Registry) List() []Tool {
	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// ListByCategory 按 category 分组返回全部工具信息，供管理端展示。
func (r *Registry) ListByCategory() map[string][]ToolInfo {
	result := make(map[string][]ToolInfo)
	for _, t := range r.tools {
		cat := string(t.Category())
		result[cat] = append(result[cat], ToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			Category:    cat,
		})
	}
	return result
}

// ToolInfo 是工具的公开元信息。
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// OpenAI 生成 OpenAI Function Calling 格式的工具列表。
// allowed=nil 表示全部 expose_to_llm=true 的工具；非 nil 只取白名单内的。
func (r *Registry) OpenAI(allowed map[string]bool) []map[string]any {
	keys := make([]string, 0, len(r.tools))
	for k := range r.tools {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var list []map[string]any
	for _, k := range keys {
		t := r.tools[k]
		if !t.ExposeToLLM() || !allow(allowed, t.Name()) {
			continue
		}
		list = append(list, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  t.JSONSchema(),
			},
		})
	}
	return list
}

// Execute 查表 → 白名单校验 → 反序列化 + validate → 调 Tool.Execute → 兜底 panic + 错误。
// allowed=nil 表示不限制；非 nil 只放行白名单内的工具。
func (r *Registry) Execute(ctx context.Context, name string, rawArgs json.RawMessage, tc ToolContext, allowed map[string]bool) *ToolResult {
	t, ok := r.Get(name)
	if !ok {
		return &ToolResult{Success: false, Error: "工具不存在: " + name}
	}
	if !allow(allowed, name) {
		return &ToolResult{Success: false, Error: "工具禁止使用: " + name}
	}

	// 反序列化 + 校验
	args := t.NewArgs()
	// DisallowUnknownFields：拒绝 schema 外字段，防止 LLM 传 id/novel_id 等额外字段
	// 经第二次 Unmarshal（工具内部）覆盖 entity 不可变字段
	dec := json.NewDecoder(bytes.NewReader(rawArgs))
	dec.DisallowUnknownFields()
	if err := dec.Decode(args); err != nil {
		return &ToolResult{Success: false, Error: "参数格式不正确: " + err.Error()}
	}
	if err := r.validate.Struct(args); err != nil {
		return &ToolResult{Success: false, Error: "参数校验失败: " + err.Error()}
	}

	tc.RawArgs = rawArgs

	t0 := time.Now()
	var result *ToolResult
	var execErr error
	func() {
		defer func() {
			if p := recover(); p != nil {
				r.logger.Error("tool panicked", "tool", name, "panic", p)
				result = &ToolResult{Success: false, Error: "服务器内部错误，请稍后重试", ErrKind: "system"}
				execErr = nil
			}
		}()
		result, execErr = t.Execute(ctx, args, tc)
	}()

	if execErr != nil {
		r.logger.Error("tool execution failed", "tool", name, "error", execErr, "elapsed_ms", time.Since(t0).Milliseconds())
		return &ToolResult{Success: false, Error: "服务器内部错误，请稍后重试", ErrKind: "system"}
	}

	if result != nil {
		r.logger.Info("mcp tool executed", "tool", name, "elapsed_ms", time.Since(t0).Milliseconds(), "success", result.Success)
	}
	return result
}

// AllowSet 将 []string 白名单转为 map[string]bool，供 Registry 方法使用。nil → nil。
func AllowSet(allowed []string) map[string]bool {
	if allowed == nil {
		return nil
	}
	set := make(map[string]bool, len(allowed))
	for _, n := range allowed {
		set[n] = true
	}
	return set
}

// allow 校验工具名是否在白名单中。nil set = 不限制。
func allow(set map[string]bool, name string) bool {
	return set == nil || set[name]
}

// ── 分页 ──────────────────────────────────────────────

// PageArgs 是分页查询的公共参数，嵌入 Args 结构体即可。
type PageArgs struct {
	Page int `json:"page" jsonschema:"description=页码,default=1,minimum=1"                validate:"omitempty,min=1"`
	Size int `json:"size" jsonschema:"description=每页数量,default=50,minimum=1,maximum=100"  validate:"omitempty,min=1,max=100"`
}

// NormalizePage 给 Page/Size 设默认值。
func (p *PageArgs) NormalizePage() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Size < 1 || p.Size > 100 {
		p.Size = 50
	}
}

// PageMeta 从 PageResult 提取分页元信息，供 ToolResult.Data 使用。
func PageMeta[T any](r *storage.PageResult[T]) map[string]any {
	return map[string]any{
		"page":        r.Page,
		"size":        r.Size,
		"total":       r.Total,
		"total_pages": r.TotalPages,
		"truncated":   r.Total > int64(r.Page*r.Size),
	}
}

// ── JSON Schema 生成 ──────────────────────────────────

// schemaReflector 是全局配置，RequiredFromJSONSchemaTags=true 配合 jsonschema tag 的 required 选项。
// ExpandedStruct 让输出内联不含 $ref/$defs。
var schemaReflector = &jsonschema.Reflector{
	RequiredFromJSONSchemaTags: true,
	ExpandedStruct:             true,
}

// SchemaOf 从带 jsonschema tag 的 struct 生成 OpenAI function calling 兼容的 JSON Schema。
// 内部用 github.com/invopop/jsonschema 生成，取 type/properties/required 并将 $defs 内联到 $ref 位置。
func SchemaOf(v any) json.RawMessage {
	s := schemaReflector.Reflect(v)

	b, _ := json.Marshal(s)
	var full map[string]any
	_ = json.Unmarshal(b, &full)

	clean := map[string]any{"type": full["type"]}
	if props, ok := full["properties"]; ok {
		clean["properties"] = props
	}
	if req, ok := full["required"]; ok {
		clean["required"] = req
	}
	if defs, ok := full["$defs"]; ok {
		clean["$defs"] = defs
	}

	resolved := inlineDefs(clean)
	raw, _ := json.Marshal(resolved)
	return raw
}

// inlineDefs 将 $defs 中的定义递归替换到 $ref 引用位置，消除跨节点跳转。
func inlineDefs(schema map[string]any) map[string]any {
	defs, _ := schema["$defs"].(map[string]any)
	delete(schema, "$defs")
	return resolveRefs(schema, defs).(map[string]any)
}

func resolveRefs(node any, defs map[string]any) any {
	switch v := node.(type) {
	case map[string]any:
		if ref, ok := v["$ref"].(string); ok && defs != nil {
			name := strings.TrimPrefix(ref, "#/$defs/")
			if def, ok := defs[name].(map[string]any); ok {
				return resolveRefs(def, defs)
			}
		}
		for k, val := range v {
			v[k] = resolveRefs(val, defs)
		}
		return v
	case []any:
		for i, val := range v {
			v[i] = resolveRefs(val, defs)
		}
		return v
	}
	return node
}
