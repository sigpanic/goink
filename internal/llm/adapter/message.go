package adapter

import (
	"encoding/json"
	"fmt"
	"slices"
)

// Message 是模型调用所需的业务消息，不包含数据库身份、可见性或 SDK 类型。
// 工具结果使用 Content，ToolCallID 关联声明；State 只由目标适配器解释。
type Message struct {
	Role            string         `json:"role"`
	Content         string         `json:"content"`
	ThinkingContent string         `json:"thinking_content,omitempty"`
	ToolCalls       []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID      string         `json:"tool_call_id,omitempty"`
	ToolName        string         `json:"tool_name,omitempty"`
	State           *ResponseState `json:"llm_state,omitempty"`
}

// Clone 返回独立副本，复制工具参数和原生响应 payload。
func (m Message) Clone() Message {
	m.ToolCalls = slices.Clone(m.ToolCalls)
	for i := range m.ToolCalls {
		m.ToolCalls[i] = m.ToolCalls[i].Clone()
	}
	if m.State != nil {
		state := m.State.Clone()
		m.State = &state
	}
	return m
}

// ToolCall 描述完整的客户端函数调用；不包含流式参数累计过程。
// JSON 保持现有 metadata.tool_calls 形状，arguments 在存储中仍为字符串。
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// Clone 复制原始参数，避免请求处理修改历史中的参数字节。
func (t ToolCall) Clone() ToolCall {
	t.Arguments = slices.Clone(t.Arguments)
	return t
}

// storedToolCall 保留现有 OpenAI 形状的公共存储约定，不表示来源协议。
type storedToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// MarshalJSON 将业务调用编码为既有存储格式，不重编码参数内容。
func (t ToolCall) MarshalJSON() ([]byte, error) {
	if !json.Valid(t.Arguments) {
		return nil, fmt.Errorf("invalid tool arguments JSON for %q", t.Name)
	}
	stored := storedToolCall{ID: t.ID, Type: "function"}
	stored.Function.Name = t.Name
	stored.Function.Arguments = string(t.Arguments)
	return json.Marshal(stored)
}

// UnmarshalJSON 读取公共工具信息，不从厂商原生 payload 推导调用。
func (t *ToolCall) UnmarshalJSON(data []byte) error {
	var stored storedToolCall
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	if stored.Type != "function" {
		return fmt.Errorf("unsupported tool call type %q", stored.Type)
	}
	arguments := json.RawMessage(stored.Function.Arguments)
	if !json.Valid(arguments) {
		return fmt.Errorf("invalid tool arguments JSON for %q", stored.Function.Name)
	}
	*t = ToolCall{ID: stored.ID, Name: stored.Function.Name, Arguments: arguments}
	return nil
}

// MessageOrigin 记录模型响应的生成来源；不包含密钥，也不直接证明状态兼容。
// 用户输入和来源未知的历史可以没有来源状态。
type MessageOrigin struct {
	AdapterName  Protocol `json:"adapter_name"`
	ProviderName string   `json:"provider_name"`
	ProviderType string   `json:"provider_type"`
	Model        string   `json:"model"`
}

// ResponseState 是响应的公共收尾信息和不透明原生状态。
// Complete 只表示单次协议响应完整；等待工具或服务端续接时 TurnComplete 为 false。
// StopReason 保留协议停止原因，Payload 的结构、签名和兼容性由具体适配器负责。
// 该结构可随消息保存，但不得直接作为厂商请求字段发送。
type ResponseState struct {
	Origin       MessageOrigin   `json:"origin"`
	Complete     bool            `json:"complete"`
	TurnComplete bool            `json:"turn_complete"`
	StopReason   string          `json:"stop_reason,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

// Clone 原样复制 payload，不解析或重建其中的协议块。
func (s ResponseState) Clone() ResponseState {
	s.Payload = slices.Clone(s.Payload)
	return s
}
