package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCountStructuredMessagesPreservesLegacyEstimate(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "你是写作助手。"},
		{Role: "user", Content: "读取第三章。"},
		{Role: "assistant", Content: "正在读取。", ThinkingContent: "需要获取章节。", ToolCalls: []ToolCall{
			{ID: "call_123", Name: "read_chapter", Arguments: json.RawMessage(`{"chapter":3,"id":9007199254740993}`)},
		}},
		{Role: "tool", ToolCallID: "call_123", ToolName: "read_chapter", Content: `{"success":true,"data":"章节正文"}`},
		{Role: "user", Content: "继续。"},
	}
	// 固定旧格式的计数输入，验证参数字符串及工具关联开销仍使用既有口径。
	legacyParts := map[string][]string{
		"system": {"你是写作助手。"},
		"user":   {"读取第三章。", "继续。"},
		"assistant": {"正在读取。", "需要获取章节。",
			`[{"id":"call_123","type":"function","function":{"name":"read_chapter","arguments":"{\"chapter\":3,\"id\":9007199254740993}"}}]`},
		"tool": {`{"success":true,"data":"章节正文"}`, "call_123"},
	}
	wantByRole := make(map[string]int)
	wantTotal := 0
	for role, parts := range legacyParts {
		for _, part := range parts {
			n, err := CountTokens(part)
			if err != nil {
				t.Fatal(err)
			}
			wantByRole[role] += n
			wantTotal += n
		}
	}
	total, byRole, err := CountMessagesTokens(messages)
	if err != nil {
		t.Fatal(err)
	}
	if total != wantTotal || !reflect.DeepEqual(byRole, wantByRole) {
		t.Fatalf("estimate changed: got total=%d roles=%v, want total=%d roles=%v", total, byRole, wantTotal, wantByRole)
	}
}

func TestCountMessageTokensExcludesOpaqueState(t *testing.T) {
	message := Message{Role: "assistant", Content: "回答", ThinkingContent: "可显示的思考"}
	want, err := CountMessageTokens(message)
	if err != nil {
		t.Fatal(err)
	}
	message.State = &ResponseState{
		Origin:  MessageOrigin{ProviderName: strings.Repeat("provider", 1000), Model: "model"},
		Payload: json.RawMessage(`{"signature":"` + strings.Repeat("encrypted", 1000) + `"}`),
	}
	got, err := CountMessageTokens(message)
	if err != nil || got != want {
		t.Fatalf("opaque state changed the text estimate: got %d, want %d, error=%v", got, want, err)
	}
	empty, err := CountMessageTokens(Message{})
	if err != nil || empty != 0 {
		t.Fatalf("empty message = %d, %v", empty, err)
	}
}

func TestCountMessageTokensRejectsMalformedToolArguments(t *testing.T) {
	message := Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "bad", Name: "read", Arguments: json.RawMessage(`{"chapter":`)}}}
	if _, err := CountMessageTokens(message); err == nil {
		t.Fatal("malformed tool arguments silently produced an underestimated count")
	}
	total, byRole, err := CountMessagesTokens([]Message{{Role: "user", Content: "valid"}, message})
	if err == nil || total != 0 || byRole != nil {
		t.Fatalf("invalid history returned a partial estimate: total=%d, roles=%v, error=%v", total, byRole, err)
	}
}
