package session

import (
	"encoding/json"
	"fmt"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// llmMetadata 只解析模型消息使用的扩展字段，不包含展示和审计信息。
// ToolCall 沿用既有存储形状；State 内的原生 payload 不在 session 中解释。
type llmMetadata struct {
	ToolCalls  []adapter.ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	State      *adapter.ResponseState `json:"llm_state,omitempty"`
}

// ToLLMMessage 提取原始业务消息，不添加时间前缀，也不执行历史清洗。
// 用于存储往返；实际模型请求应使用 ToLLMRequestMessage。
func (m Message) ToLLMMessage() (adapter.Message, error) {
	var metadata llmMetadata
	if m.ExtraMetadata != "" {
		if err := json.Unmarshal([]byte(m.ExtraMetadata), &metadata); err != nil {
			return adapter.Message{}, fmt.Errorf("message %d: decode LLM metadata: %w", m.ID, err)
		}
	}
	return adapter.Message{
		Role:            m.Role,
		Content:         m.Content,
		ThinkingContent: m.ThinkingContent,
		HasThinking:     m.HasThinking,
		ToolCalls:       metadata.ToolCalls,
		ToolCallID:      metadata.ToolCallID,
		ToolName:        metadata.ToolName,
		State:           metadata.State,
	}, nil
}

// ToLLMRequestMessage 生成出站副本，为前端可见的真实用户消息添加发送时间前缀。
// 时间只取原始 CreatedAt；注入消息和已渲染的压缩副本 ToFrontend=false，不重复加前缀。
// 该结果用于请求，不应作为原始用户正文写回；压缩副本须显式使用 ToFrontend=false。
func (m Message) ToLLMRequestMessage() (adapter.Message, error) {
	message, err := m.ToLLMMessage()
	if err != nil {
		return adapter.Message{}, err
	}
	if m.Role == "user" && m.ToFrontend && !m.CreatedAt.IsZero() {
		message.Content = "[" + m.CreatedAt.Local().Format("2006-01-02 15:04") + "] " + m.Content
	}
	return message, nil
}

// WithLLMMessage 将原始业务消息合入未入库草稿，返回独立的新记录，不修改接收方。
// 接收方提供 session、turn、可见性等存储属性及其他 metadata；不得传入已有 ID 的记录。
// 仅替换公共模型字段，保留展示、审计和未知 metadata；不从原生 payload 推导工具调用。
func (m Message) WithLLMMessage(message adapter.Message) (Message, error) {
	if m.ID != 0 {
		return Message{}, fmt.Errorf("message %d: LLM mapping requires an unsaved draft", m.ID)
	}
	metadata := make(map[string]json.RawMessage)
	if m.ExtraMetadata != "" {
		if err := json.Unmarshal([]byte(m.ExtraMetadata), &metadata); err != nil {
			return Message{}, fmt.Errorf("decode draft metadata: %w", err)
		}
		if metadata == nil {
			metadata = make(map[string]json.RawMessage)
		}
	}
	fields := []struct {
		name    string
		value   any
		present bool
	}{
		{"tool_calls", message.ToolCalls, len(message.ToolCalls) > 0},
		{"tool_call_id", message.ToolCallID, message.ToolCallID != ""},
		{"tool_name", message.ToolName, message.ToolName != ""},
		{"llm_state", message.State, message.State != nil},
	}
	for _, field := range fields {
		delete(metadata, field.name)
		if !field.present {
			continue
		}
		data, err := json.Marshal(field.value)
		if err != nil {
			return Message{}, fmt.Errorf("encode metadata.%s: %w", field.name, err)
		}
		metadata[field.name] = data
	}
	m.ExtraMetadata = ""
	if len(metadata) > 0 {
		data, err := json.Marshal(metadata)
		if err != nil {
			return Message{}, fmt.Errorf("encode draft metadata: %w", err)
		}
		m.ExtraMetadata = string(data)
	}
	m.Role = message.Role
	m.Content = message.Content
	m.ThinkingContent = message.ThinkingContent
	m.HasThinking = message.HasThinking
	return m, nil
}
