package llm

import "github.com/sigpanic/goink/internal/llm/adapter"

// ModelInfo 与适配层共用同一类型，供根包调用方直接使用。
type ModelInfo = adapter.ModelInfo
type ModelSelection = adapter.ModelSelection
type APIError = adapter.APIError
type CallOptions = adapter.CallOptions
type StreamEventType = adapter.StreamEventType
type StreamEvent = adapter.StreamEvent
type ToolCallDelta = adapter.ToolCallDelta
type ToolDefinition = adapter.ToolDefinition
type Message = adapter.Message
type ToolCall = adapter.ToolCall
type MessageOrigin = adapter.MessageOrigin
type ResponseState = adapter.ResponseState

const (
	EventThinking          = adapter.EventThinking
	EventThinkingDone      = adapter.EventThinkingDone
	EventContent           = adapter.EventContent
	EventToolCallStart     = adapter.EventToolCallStart
	EventToolCallArguments = adapter.EventToolCallArguments
	EventToolCallEnd       = adapter.EventToolCallEnd
	EventUsage             = adapter.EventUsage
	EventError             = adapter.EventError
	EventResponseState     = adapter.EventResponseState
)
