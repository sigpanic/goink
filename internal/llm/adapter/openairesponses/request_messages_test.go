package openairesponses

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestRequestToolHistoryPairsByCallIDAndRetainsEmptyResults(t *testing.T) {
	first := adapter.ToolCall{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{"chapter":9007199254740993,"nested":{"a":1,"b":2}}`)}
	duplicate := first.Clone()
	duplicate.Arguments = json.RawMessage(`{"nested":{"b":2,"a":1},"chapter":9007199254740993}`)
	second := adapter.ToolCall{ID: "call_2", Name: "read", Arguments: json.RawMessage(`{"chapter":2}`)}
	missing := adapter.ToolCall{ID: "missing", Name: "read", Arguments: json.RawMessage(`{}`)}
	req := requestInput()
	req.Messages = append(req.Messages,
		adapter.Message{Role: "assistant", Content: "checking", ToolCalls: []adapter.ToolCall{first, duplicate, second, missing}},
		adapter.Message{Role: "tool", ToolCallID: "call_2", Content: ""},
		adapter.Message{Role: "tool", ToolCallID: "call_1", ToolName: "read", Content: `{"content":"first"}`},
		adapter.Message{Role: "tool", ToolCallID: "call_1", ToolName: "read", Content: `{"content":"first"}`},
		adapter.Message{Role: "tool", ToolCallID: "orphan", Content: "orphan"},
	)
	before, err := json.Marshal(req.Messages)
	if err != nil {
		t.Fatal(err)
	}
	params, err := requestProvider(t).buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var items []struct {
		Type, Role, Content, ID, Name, Arguments, Output string
		CallID                                           string `json:"call_id"`
	}
	if err := json.Unmarshal(encodeParams(t, params)["input"], &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 6 || items[0].Role != "user" || items[1].Content != "checking" {
		t.Fatalf("unexpected projected history: %+v", items)
	}
	if items[2].Type != "function_call" || items[2].CallID != "call_1" || items[2].ID != "" || items[2].Arguments != string(first.Arguments) || items[3].CallID != "call_2" {
		t.Fatalf("tool calls were not preserved and deduplicated: %+v", items[2:4])
	}
	if items[4].Type != "function_call_output" || items[4].CallID != "call_1" || items[4].Output != `{"content":"first"}` || items[5].CallID != "call_2" || items[5].Output != "" {
		t.Fatalf("tool results were not emitted in call order: %+v", items[4:])
	}
	after, err := json.Marshal(req.Messages)
	if err != nil || string(after) != string(before) {
		t.Fatal("pairing mutated the stored history")
	}
}

func TestRequestToolHistoryRejectsConflicts(t *testing.T) {
	call := adapter.ToolCall{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{"chapter":9007199254740993}`)}
	assistant := adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{call}}
	result := adapter.Message{Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: "ok"}
	for _, tt := range []struct {
		name     string
		messages []adapter.Message
	}{
		{"conflicting declaration", []adapter.Message{{Role: "assistant", ToolCalls: []adapter.ToolCall{call, {ID: call.ID, Name: call.Name, Arguments: json.RawMessage(`{"chapter":9007199254740992}`)}}}, result}},
		{"conflicting name", []adapter.Message{{Role: "assistant", ToolCalls: []adapter.ToolCall{call, {ID: call.ID, Name: "other", Arguments: call.Arguments}}}, result}},
		{"conflicting result", []adapter.Message{assistant, result, {Role: "tool", ToolCallID: call.ID, Content: "different"}}},
		{"wrong result name", []adapter.Message{assistant, {Role: "tool", ToolCallID: call.ID, ToolName: "other", Content: "ok"}}},
		{"reused call ID", []adapter.Message{assistant, result, assistant, result}},
		{"missing call ID", []adapter.Message{{Role: "assistant", ToolCalls: []adapter.ToolCall{{Name: "read", Arguments: json.RawMessage(`{}`)}}}}},
		{"invalid arguments", []adapter.Message{{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: call.ID, Name: "read", Arguments: json.RawMessage(`{`)}}}, result}},
		{"nonobject arguments", []adapter.Message{{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: call.ID, Name: "read", Arguments: json.RawMessage(`[]`)}}}, result}},
		{"blank function name", []adapter.Message{{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: call.ID, Name: " ", Arguments: json.RawMessage(`{}`)}}}, {Role: "tool", ToolCallID: call.ID, Content: "ok"}}},
		{"calls on user", []adapter.Message{{Role: "user", Content: "bad", ToolCalls: []adapter.ToolCall{call}}}},
		{"calls on result", []adapter.Message{assistant, {Role: "tool", ToolCallID: call.ID, Content: "bad", ToolCalls: []adapter.ToolCall{call}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := requestInput()
			req.Messages = append(req.Messages, tt.messages...)
			if _, err := requestProvider(t).buildRequest(req); err == nil {
				t.Fatal("invalid history was silently accepted")
			}
		})
	}
}

func TestRequestDoesNotPairToolsAcrossMessageBoundaries(t *testing.T) {
	req := requestInput()
	req.Messages = append(req.Messages,
		adapter.Message{Role: "assistant", Content: "interrupted", ToolCalls: []adapter.ToolCall{{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{}`)}}},
		adapter.Message{Role: "user", Content: "continue"},
		adapter.Message{Role: "tool", ToolCallID: "call_1", Content: "late result"},
	)
	params, err := requestProvider(t).buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	raw := encodeParams(t, params)["input"]
	if strings.Contains(string(raw), "function_call") || strings.Contains(string(raw), "late result") || !strings.Contains(string(raw), "interrupted") {
		t.Fatalf("orphan or incomplete tool group escaped filtering: %s", raw)
	}
}
