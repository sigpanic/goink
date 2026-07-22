package session

import (
	"io"
	"log/slog"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestToAPIFormat_UserMessage(t *testing.T) {
	m := &Message{Role: "user", Content: "你好"}
	result := m.ToAPIFormat(discardLogger())
	if result["role"] != "user" {
		t.Errorf("role: got %v", result["role"])
	}
	if result["content"] != "你好" {
		t.Errorf("content: got %v", result["content"])
	}
	if _, exists := result["reasoning_content"]; exists {
		t.Error("user messages should not have reasoning_content")
	}
}

func TestToAPIFormat_AssistantWithThinking(t *testing.T) {
	m := &Message{
		Role:            "assistant",
		Content:         "回复内容",
		ThinkingContent: "思考过程",
	}
	result := m.ToAPIFormat(discardLogger())
	if result["role"] != "assistant" {
		t.Errorf("role: got %v", result["role"])
	}
	if result["reasoning_content"] != "思考过程" {
		t.Errorf("reasoning_content: got %v", result["reasoning_content"])
	}
}

func TestToAPIFormat_AssistantWithToolCalls(t *testing.T) {
	m := &Message{
		Role:          "assistant",
		ExtraMetadata: `{"tool_calls":[{"id":"1","function":{"name":"read","arguments":"{}"}}]}`,
	}
	result := m.ToAPIFormat(discardLogger())
	if tc, ok := result["tool_calls"]; !ok || tc == nil {
		t.Error("tool_calls should be present")
	}
	// 没有 thinking 但有 tool_calls → reasoning_content 为空字符串
	if rc, ok := result["reasoning_content"]; !ok || rc != "" {
		t.Errorf("reasoning_content should be empty string, got %v", rc)
	}
}

func TestToAPIFormat_ToolMessage(t *testing.T) {
	m := &Message{
		Role:          "tool",
		ExtraMetadata: `{"tool_call_id":"call_123","tool_name":"read"}`,
	}
	result := m.ToAPIFormat(discardLogger())
	if result["role"] != "tool" {
		t.Errorf("role: got %v", result["role"])
	}
	if result["tool_call_id"] != "call_123" {
		t.Errorf("tool_call_id: got %v", result["tool_call_id"])
	}
	if result["name"] != "read" {
		t.Errorf("name: got %v", result["name"])
	}
}

func TestToAPIFormat_SystemMessage(t *testing.T) {
	m := &Message{Role: "system", Content: "系统提示"}
	result := m.ToAPIFormat(discardLogger())
	if result["role"] != "system" {
		t.Errorf("role: got %v", result["role"])
	}
	// system 消息不应有任何额外字段
	if _, exists := result["reasoning_content"]; exists {
		t.Error("system messages should not have reasoning_content")
	}
}
