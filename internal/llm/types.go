package llm

import (
	"encoding/json"
	"fmt"
	"time"
)

// Provider 定义一个大模型供应商的完整配置。
// 默认行为走 OpenAI 兼容格式，钩子函数用于处理供应商差异。
// 同时用作用户配置的 JSON 格式（Hooks 不参与序列化，反序列化后为 nil）。
type Provider struct {
	Name         string                                         `json:"name"`                  // 供应商名称，如 "DeepSeek"
	ChatURL      string                                         `json:"chat_url"`              // 聊天补全端点
	APIKey       string                                         `json:"api_key,omitempty"`     // API 密钥
	PlatformURL  string                                         `json:"-"`                     // 注册平台 URL，仅内置模板使用
	HelpText     string                                         `json:"-"`                     // 注册引导文字，仅内置模板使用
	Models       []ModelInfo                                    `json:"models"`                // 可用模型列表
	Temperature  *float64                                       `json:"temperature,omitempty"` // 默认创意度 0~2，nil 表示未设置，运行时取内置默认
	BuildRequest func(payload map[string]any) map[string]any    `json:"-"`                     // 发送前改造请求体，nil 则原样发送
	BuildHeaders func(base map[string]string) map[string]string `json:"-"`                     // 发送前改造请求头，nil 则使用默认 Bearer 鉴权
	ParseError   func(body []byte) error                        `json:"-"`                     // 解析非标准错误响应体，nil 则使用默认 OpenAI 格式解析
}

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

// APIError 表示 LLM API 的调用错误，包含可重试标记。
type APIError struct {
	StatusCode int
	Message    string
	Retryable  bool
	RetryAfter time.Duration // 服务商指定的重试等待时间（来自 Retry-After header），0 表示无指定
}

func (e *APIError) Error() string {
	return fmt.Sprintf("[%d] %s", e.StatusCode, e.Message)
}

// CallOptions LLM 调用的可选参数。nil 时全部使用 Provider/ModelInfo 默认值。
type CallOptions struct {
	Temperature     *float64 // 不传走 Provider 默认
	MaxTokens       *int     // 不传走 ModelInfo 默认
	ReasoningEffort *string  // 不传从 ModelInfo 取
	ThinkingEnabled *bool    // 不传从 ModelInfo 判断
	ToolChoice      any      // OpenAI tool_choice payload; nil = provider default
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
