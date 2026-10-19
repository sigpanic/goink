package session

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestLLMMessageMapsExistingToolMetadata(t *testing.T) {
	assistant := Message{
		ID: 42, Role: "assistant", Content: "读取章节", ThinkingContent: "先获取原文",
		ExtraMetadata: `{"tool_calls":[{"id":"call_123","type":"function","function":{"name":"read_chapter","arguments":"{\"chapter\":3,\"id\":9007199254740993}"}}],"tool_displays":[{"tool_id":"call_123","phase":"completed"}]}`,
	}
	got, err := assistant.ToLLMMessage()
	if err != nil {
		t.Fatal(err)
	}
	want := adapter.Message{
		Role: "assistant", Content: "读取章节", ThinkingContent: "先获取原文",
		ToolCalls: []adapter.ToolCall{{ID: "call_123", Name: "read_chapter", Arguments: json.RawMessage(`{"chapter":3,"id":9007199254740993}`)}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("existing tool call changed: got %+v, want %+v", got, want)
	}
	tool := Message{
		Role: "tool", Content: `{"success":false,"error":"章节不存在"}`,
		ExtraMetadata: `{"tool_call_id":"call_123","tool_name":"read_chapter"}`,
	}
	result, err := tool.ToLLMMessage()
	if err != nil || result.ToolCallID != "call_123" || result.ToolName != "read_chapter" || result.Content != tool.Content {
		t.Fatalf("tool result changed: %+v, %v", result, err)
	}
	// 每次读取都解码到独立对象，不共享参数字节或修改数据库记录。
	got.ToolCalls[0].Arguments[0] = '['
	again, err := assistant.ToLLMMessage()
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("reading or mutating a mapped message changed stored history: %+v, %v", again, err)
	}
}

func TestLLMMessageRoundTripPreservesNativeAndOtherMetadata(t *testing.T) {
	tests := []struct {
		protocol adapter.Protocol
		payload  string
	}{
		{adapter.ProtocolAnthropicMessages, `{"content":[{"type":"thinking","thinking":"","signature":"sig<opaque>&"},{"type":"redacted_thinking","data":"opaque"},{"type":"tool_use","id":"native-only","input":{"id":9007199254740993}}]}`},
		{adapter.ProtocolGeminiGenerateContent, `{"parts":[{"text":"","thoughtSignature":"test-signature"},{"functionCall":{"name":"read","args":{"id":9007199254740993}}}]}`},
		{adapter.Protocol("openai_responses"), `{"output":[{"type":"reasoning","encrypted_content":"opaque","summary":[]},{"type":"function_call","call_id":"native-only","arguments":"{}"}]}`},
	}
	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			input := adapter.Message{
				Role: "assistant", Content: "正文", ThinkingContent: "显示用思考",
				State: &adapter.ResponseState{
					Origin:   adapter.MessageOrigin{AdapterName: tt.protocol, ProviderName: "中转实例", ProviderType: "test", Model: "test-model"},
					Complete: true, TurnComplete: false, StopReason: "native-reason",
					Payload: json.RawMessage(tt.payload),
				},
			}
			draft := Message{
				SessionID: "session-1", TurnID: 7, Version: 2, ToAPI: true, ToFrontend: true,
				AgentType: "main", SubTaskID: "subtask", EventType: EventSystemInterrupted,
				CreatedAt: time.Date(2020, 1, 2, 3, 4, 0, 0, time.Local), TokenCount: 123,
				ExtraMetadata: `{"tool_displays":[{"phase":"failed"}],"audit":{"large_id":9007199254740993},"unknown":[null,{"keep":true}]}`,
			}
			before := draft
			stored, err := draft.WithLLMMessage(input)
			if err != nil {
				t.Fatal(err)
			}
			if draft != before {
				t.Fatal("mapping mutated the draft")
			}
			attributes := stored
			attributes.Role, attributes.Content, attributes.ThinkingContent = "", "", ""
			attributes.ExtraMetadata = draft.ExtraMetadata
			if attributes != draft {
				t.Fatal("mapping changed caller-owned storage attributes")
			}
			var oldMetadata, newMetadata map[string]json.RawMessage
			if err := json.Unmarshal([]byte(draft.ExtraMetadata), &oldMetadata); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(stored.ExtraMetadata), &newMetadata); err != nil {
				t.Fatal(err)
			}
			for key, value := range oldMetadata {
				if !bytes.Equal(newMetadata[key], value) {
					t.Fatalf("metadata.%s was lost or reinterpreted: %s", key, newMetadata[key])
				}
			}
			restored, err := stored.ToLLMMessage()
			if err != nil {
				t.Fatal(err)
			}
			// 比较 JSON 值而非空白/转义拼写；UseNumber 防止大整数被 float64 舍入。
			assertSameLLMJSON(t, restored.State.Payload, input.State.Payload)
			payload := restored.State.Payload
			restored.State.Payload = input.State.Payload
			if !reflect.DeepEqual(restored, input) {
				t.Fatalf("response state or business fields changed: %+v", restored)
			}
			if restored.ToolCalls != nil {
				t.Fatal("session inferred common tool calls from native payload")
			}
			input.State.Payload[0] = '['
			input.State.Origin.Model = "changed"
			again, err := stored.ToLLMMessage()
			if err != nil || again.State.Origin.Model != "test-model" || !bytes.Equal(again.State.Payload, payload) {
				t.Fatal("stored metadata shares mutable state with the input")
			}
		})
	}
}

func TestLLMMessageReplacesOnlyOwnedMetadata(t *testing.T) {
	draft := Message{ExtraMetadata: `{"tool_calls":[{"old":true}],"tool_call_id":"old","tool_name":"old","llm_state":{"old":true},"display_text":"keep"}`}
	input := adapter.Message{
		Role:      "assistant",
		ToolCalls: []adapter.ToolCall{{ID: "new", Name: "read", Arguments: json.RawMessage(`{"chapter":3}`)}},
	}
	stored, err := draft.WithLLMMessage(input)
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stored.ExtraMetadata), &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata) != 2 || string(metadata["display_text"]) != `"keep"` {
		t.Fatalf("stale model fields remained or display metadata was lost: %s", stored.ExtraMetadata)
	}
	got, err := stored.ToLLMMessage()
	if err != nil || !reflect.DeepEqual(got, input) {
		t.Fatalf("replacement did not round trip: %+v, %v", got, err)
	}
	for _, extra := range []string{"", "{}", "null"} {
		plain, err := (Message{ExtraMetadata: extra}).WithLLMMessage(adapter.Message{Role: "user", Content: "正文"})
		if err != nil || plain.ExtraMetadata != "" {
			t.Fatalf("plain message gained metadata: %+v, %v", plain, err)
		}
	}
}

func TestLLMMessageTimePrefixAndRawRoundTrip(t *testing.T) {
	sentAt := time.Date(2020, 1, 2, 3, 4, 0, 0, time.Local)
	tests := []struct {
		name string
		msg  Message
	}{
		{"real user", Message{Role: "user", Content: "你好", ToFrontend: true, CreatedAt: sentAt}},
		{"unsaved user", Message{Role: "user", Content: "你好", ToFrontend: true}},
		{"injected user", Message{Role: "user", Content: "提醒", CreatedAt: sentAt}},
		{"compressed copy", Message{Role: "user", Content: "[2019-01-02 03:04] 原文", CreatedAt: sentAt}},
		{"system", Message{Role: "system", Content: "系统提示", ToFrontend: true, CreatedAt: sentAt}},
		{"assistant", Message{Role: "assistant", Content: "回复", ToFrontend: true, CreatedAt: sentAt}},
		{"tool", Message{Role: "tool", Content: "结果", ToFrontend: true, CreatedAt: sentAt}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.msg
			raw, err := tt.msg.ToLLMMessage()
			if err != nil || raw.Content != tt.msg.Content {
				t.Fatalf("raw mapping rendered the content: %+v, %v", raw, err)
			}
			request, err := tt.msg.ToLLMRequestMessage()
			if err != nil {
				t.Fatal(err)
			}
			if want := tt.msg.ToAPIFormat(discardLogger())["content"]; request.Content != want {
				t.Fatalf("time prefix changed: got %q, want %q", request.Content, want)
			}
			stored, err := tt.msg.WithLLMMessage(raw)
			if err != nil {
				t.Fatal(err)
			}
			again, err := stored.ToLLMRequestMessage()
			if err != nil || !reflect.DeepEqual(again, request) || tt.msg != before {
				t.Fatalf("round trip duplicated a prefix or modified stored content: %+v, %v", again, err)
			}
		})
	}
	user := Message{Role: "user", Content: "你好", ToFrontend: true, CreatedAt: sentAt}
	request, err := user.ToLLMRequestMessage()
	if err != nil {
		t.Fatal(err)
	}
	// 压缩保留已渲染内容时，显式关闭前端标志；新建时间不能冒充原发言时间。
	copy, err := (Message{CreatedAt: sentAt.AddDate(1, 0, 0), ToFrontend: false}).WithLLMMessage(request)
	if err != nil {
		t.Fatal(err)
	}
	again, err := copy.ToLLMRequestMessage()
	if err != nil || again.Content != "[2020-01-02 03:04] 你好" {
		t.Fatalf("compressed copy gained its creation time: %+v, %v", again, err)
	}
}

func TestLLMMessageRejectsCorruptMetadataAndStoredDrafts(t *testing.T) {
	for _, extra := range []string{
		`{"tool_calls":`, `[]`, `"text"`,
		`{"tool_calls":{}}`, `{"tool_calls":[{"type":"function","function":{"name":"read","arguments":"{"}}]}`,
		`{"tool_call_id":123}`, `{"tool_name":[]}`, `{"llm_state":"bad"}`,
	} {
		t.Run(extra, func(t *testing.T) {
			msg := Message{ID: 42, Role: "assistant", ExtraMetadata: extra}
			if _, err := msg.ToLLMMessage(); err == nil || !strings.Contains(err.Error(), "message 42") {
				t.Fatalf("corrupt metadata did not identify the stored message: %v", err)
			}
			if _, err := msg.ToLLMRequestMessage(); err == nil {
				t.Fatal("request rendering hid corrupt metadata")
			}
		})
	}
	inputs := []adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{Name: "read", Arguments: json.RawMessage(`{`)}}},
		{Role: "assistant", State: &adapter.ResponseState{Payload: json.RawMessage(`{`)}},
	}
	for _, input := range inputs {
		draft := Message{ExtraMetadata: `{"audit":"keep"}`}
		before := draft
		if _, err := draft.WithLLMMessage(input); err == nil || draft != before {
			t.Fatalf("invalid model fields were stored or modified the draft: %v", err)
		}
	}
	for _, extra := range []string{`{"audit":`, `[]`} {
		if _, err := (Message{ExtraMetadata: extra}).WithLLMMessage(adapter.Message{Role: "user"}); err == nil {
			t.Fatal("corrupt draft metadata was silently discarded")
		}
	}
	if _, err := (Message{ID: 42}).WithLLMMessage(adapter.Message{Role: "user"}); err == nil {
		t.Fatal("mapping accepted an existing record for replacement")
	}
}

func assertSameLLMJSON(t *testing.T, got, want []byte) {
	t.Helper()
	decode := func(data []byte) any {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if !reflect.DeepEqual(decode(got), decode(want)) {
		t.Fatalf("opaque JSON values changed:\ngot  %s\nwant %s", got, want)
	}
}
