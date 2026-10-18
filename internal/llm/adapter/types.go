package adapter

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/sigpanic/goink/internal/apperr"
)

// ModelInfo 描述一个具体模型的元信息。
type ModelInfo struct {
	ID               string   `json:"id"`                         // 模型标识，如 "deepseek-v4-pro"
	Name             string   `json:"name"`                       // 显示名称，如 "DeepSeek V4 Pro"
	ContextWindow    int      `json:"context_window"`             // 上下文窗口大小（token 数）
	MaxOutputTokens  int      `json:"max_output_tokens"`          // 最大输出 token 数
	SupportsThinking bool     `json:"supports_thinking"`          // 是否支持思考模式
	ReasoningLevels  []string `json:"reasoning_levels,omitempty"` // 多级推理等级，如 ["low","high","max"]；nil=二值开关
	SupportsVision   bool     `json:"supports_vision"`            // 是否支持图片输入
}

// Clone 返回独立的模型元信息副本，复制可变的推理等级列表。
func (m ModelInfo) Clone() ModelInfo {
	m.ReasoningLevels = slices.Clone(m.ReasoningLevels)
	return m
}

// APIError 表示 LLM API 的调用错误，包含可重试标记。
type APIError struct {
	StatusCode int
	Message    string
	Retryable  bool
	RetryAfter time.Duration // 服务商指定的重试等待时间（来自 Retry-After header），0 表示无指定
	Kind       string        // 错误类型标记，用于 agent 层区分处理路径（如 "tool_args_invalid"）；空字符串=普通错误
}

func (e *APIError) Error() string {
	return fmt.Sprintf("[%d] %s", e.StatusCode, e.Message)
}

// Code 实现 apperr.Coder 接口，将 StatusCode 映射为 apperr.Code 供 apperr.CodeFromError 提取。
// 映射逻辑从 apperr.codeFromLLMAPIError 搬来（v1.3.0 集中映射改接口模式），
// apperr 不再 import llm，依赖方向反转为 llm → apperr。
func (e *APIError) Code() apperr.Code {
	switch {
	case e.StatusCode == 429:
		return apperr.CodeLLMRateLimited
	case e.StatusCode == 404:
		return apperr.CodeLLMNotFound
	case e.StatusCode == 403:
		return apperr.CodeLLMForbidden
	case e.StatusCode >= 500:
		return apperr.CodeLLMServerError
	default:
		return apperr.CodeLLMClientError
	}
}

// CallOptions LLM 调用的可选参数。nil 时全部使用 Provider/ModelInfo 默认值。
type CallOptions struct {
	Temperature     *float64 // 不传走 Provider 默认
	MaxTokens       *int     // 不传走 ModelInfo 默认
	ReasoningEffort *string  // 不传从 ModelInfo 取
	ThinkingEnabled *bool    // 不传从 ModelInfo 判断
	ToolChoice      any      // OpenAI tool_choice payload; nil = provider default
	SessionID       string   // 会话标识（如 Goink session_id），仅需要会话感知的服务商消费；空=无会话上下文（hook 内部兜底）
}

// StreamEventType 流式事件的类型。
type StreamEventType int

const (
	EventThinking          StreamEventType = iota // 模型推理/思考内容（DeepSeek reasoning_content）
	EventContent                                  // 普通文本内容
	EventToolCallStart                            // 工具调用开始（携带名称和 ID）
	EventToolCallArguments                        // 工具参数增量（arguments 累计字符串）
	EventToolCallEnd                              // 工具调用完成（arguments 已 parse 为 JSON）
	EventUsage                                    // 本次调用 token 用量
	EventError                                    // 错误（HTTP 错误或网络异常）
	EventResponseState                            // 响应收尾及原生回放状态，不代表业务 turn 一定结束
)

// StreamEvent 流式对话过程中产出的单个事件。
// Data 承载 thinking/content 文本，Delta 承载工具调用信息，Usage 承载 token 统计。
// 错误事件通过 Error 字段传递。
type StreamEvent struct {
	Type  StreamEventType
	Data  string         // EventThinking / EventContent 的文本数据
	Delta *ToolCallDelta // EventToolCallStart / EventToolCallArguments / EventToolCallEnd
	Usage map[string]any // EventUsage 时，原样透传 API 返回的 usage 对象
	Error error          // EventError 时的错误信息
	State *ResponseState // EventResponseState 时的公共收尾信息和原生状态
}

// ToolCallDelta 描述一次工具调用的增量信息。
// 工具调用通过 SSE 流分多次下发：先来 ID 和名称，再逐片来 arguments 字符串。
// 客户端内部维护按 index 的累积缓冲区，流结束后校验并发送原始 JSON。
type ToolCallDelta struct {
	ToolName      string          // EventToolCallStart 时填充
	ToolID        string          // EventToolCallStart 时填充
	ArgumentsText string          // EventToolCallArguments 时，累积后的完整 JSON 字符串
	ArgumentsJSON json.RawMessage // EventToolCallEnd 时，原始 JSON 字节，由调用方按需反序列化
}

// Request 是一次 Goink 模型调用的输入，适配器负责构造实际 HTTP / SDK 请求。
// Messages 和 Tools 暂时沿用现有 map 结构，调用期间由调用方保持只读。
type Request struct {
	ProviderName string // 已选适配器内部的 provider 名称，由调用方显式指定
	Model        string
	Messages     []map[string]any
	Tools        []map[string]any
	Options      *CallOptions
}

// Response 是协议无关的完整生成结果；工具声明仅表示模型输出，不执行工具。
// 来源及原生回放状态在后续持久化阶段补齐，显示用 ThinkingContent 不充当回放状态。
type Response struct {
	Content         string
	ThinkingContent string
	ToolCalls       []ToolCallDelta
	Usage           map[string]any
	StopReason      string
	State           *ResponseState
}

// TestConnectionResult 是最小调用探测的结果。
type TestConnectionResult struct {
	URL     string
	Warning string
}

// StatusRetryable 判断 HTTP 状态码是否适合进行业务重试。
// statusRetryable 判断 HTTP 状态码对应的错误是否可重试。
// 429（限流）、408（超时）、5xx（服务端错误）可重试。
func StatusRetryable(code int) bool {
	return code == 429 || code == 408 || (code >= 500 && code < 600)
}
