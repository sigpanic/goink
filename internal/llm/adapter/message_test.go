package adapter_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestToolCallExistingStorageRoundTrip(t *testing.T) {
	arguments := `{"chapter":3,"large_id":9007199254740993,"nested":{"text":"第三章"}}`
	stored := `{"id":"call_123","type":"function","function":{"name":"read_chapter","arguments":` + quoted(t, arguments) + `}}`
	var call adapter.ToolCall
	if err := json.Unmarshal([]byte(stored), &call); err != nil {
		t.Fatal(err)
	}
	if call.ID != "call_123" || call.Name != "read_chapter" || string(call.Arguments) != arguments {
		t.Fatalf("stored tool call lost business fields: %+v", call)
	}
	encoded, err := json.Marshal(call)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != stored {
		t.Fatalf("storage format changed:\ngot  %s\nwant %s", encoded, stored)
	}
}

func TestToolCallRejectsMalformedStorage(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"invalid outer JSON", `{"id":`},
		{"server tool is not a function", `{"type":"server_tool_use","function":{"name":"search","arguments":"{}"}}`},
		{"arguments must remain a string", `{"type":"function","function":{"name":"read","arguments":{}}}`},
		{"truncated arguments", `{"type":"function","function":{"name":"read","arguments":"{\"chapter\":"}}`},
		{"missing arguments", `{"type":"function","function":{"name":"read"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := adapter.ToolCall{ID: "existing", Name: "existing", Arguments: json.RawMessage(`{}`)}
			call := original.Clone()
			if err := json.Unmarshal([]byte(tt.data), &call); err == nil {
				t.Fatal("malformed storage was accepted")
			}
			if !reflect.DeepEqual(call, original) {
				t.Fatal("failed decode changed the existing call")
			}
		})
	}
	if _, err := json.Marshal(adapter.ToolCall{Name: "read", Arguments: json.RawMessage(`{"chapter":`)}); err == nil {
		t.Fatal("invalid argument JSON was stored as an apparently valid string")
	}
}

func TestMessageRoundTripPreservesNativeBlocks(t *testing.T) {
	payload := json.RawMessage(`{"content":[{"type":"thinking","thinking":"","signature":"test-signature"},{"type":"redacted_thinking","data":"test-redacted-data"},{"type":"tool_use","id":"toolu_1","name":"read_chapter","input":{"chapter":3}}]}`)
	message := adapter.Message{
		Role: "assistant", Content: "读取章节。", ThinkingContent: "显示用思考",
		ToolCalls: []adapter.ToolCall{{ID: "toolu_1", Name: "read_chapter", Arguments: json.RawMessage(`{"chapter":3}`)}},
		State: &adapter.ResponseState{
			Origin: adapter.MessageOrigin{
				AdapterName: adapter.ProtocolAnthropicMessages, ProviderName: "官方实例", ProviderType: "anthropic", Model: "test-model",
			},
			Complete: true, TurnComplete: false, StopReason: "tool_use", Payload: payload,
		},
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var restored adapter.Message
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, message) {
		t.Fatalf("message changed after serialization:\ngot  %+v\nwant %+v", restored, message)
	}
	if !bytes.Equal(restored.State.Payload, payload) {
		t.Fatal("native block order, empty thinking or signatures changed")
	}
}

func TestMessageCloneIsolatesMutableHistory(t *testing.T) {
	original := adapter.Message{
		Role:      "assistant",
		ToolCalls: []adapter.ToolCall{{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{"chapter":3}`)}},
		State: &adapter.ResponseState{
			Origin:  adapter.MessageOrigin{ProviderName: "original"},
			Payload: json.RawMessage(`{"signature":"original"}`),
		},
	}
	before, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	cloned := original.Clone()
	cloned.ToolCalls[0].ID = "changed"
	cloned.ToolCalls[0].Arguments[0] = '['
	cloned.State.Origin.ProviderName = "changed"
	cloned.State.Payload[0] = '['
	after, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("changing the outbound copy modified the source history")
	}
	plain := (adapter.Message{Role: "user", Content: "你好"}).Clone()
	if plain.ToolCalls != nil || plain.State != nil {
		t.Fatal("clone added tool calls or native state to a plain message")
	}
}

func TestExistingStreamEventValuesRemainStable(t *testing.T) {
	events := []adapter.StreamEventType{
		adapter.EventThinking, adapter.EventContent, adapter.EventToolCallStart,
		adapter.EventToolCallArguments, adapter.EventToolCallEnd, adapter.EventUsage, adapter.EventError,
	}
	for i, event := range events {
		if int(event) != i {
			t.Fatalf("existing event value changed: got %d, want %d", event, i)
		}
	}
	if int(adapter.EventResponseState) != len(events) {
		t.Fatal("response state event must be appended after existing events")
	}
}

func quoted(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
