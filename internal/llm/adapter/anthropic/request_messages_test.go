package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestRequestProjectsSystemAndRepairsToolHistory(t *testing.T) {
	p, req := testRequest()
	call := adapter.ToolCall{ID: "call1", Name: "read", Arguments: json.RawMessage(`{"chapter":1}`)}
	second := adapter.ToolCall{ID: "call2", Name: "read", Arguments: json.RawMessage(`{"chapter":2}`)}
	req.Messages = []adapter.Message{
		{Role: "system", Content: "first"}, {Role: "system", Content: "second"},
		{Role: "user", Content: "request"},
		{Role: "tool", ToolCallID: "orphan", Content: "ignore"},
		{Role: "assistant", Content: "reading", ThinkingContent: "display-only", ToolCalls: []adapter.ToolCall{call, call, second, {ID: "missing", Name: "read", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", ToolCallID: "call2", Content: "second body"},
		{Role: "tool", ToolCallID: "call1", Content: "first body"},
		{Role: "tool", ToolCallID: "call1", Content: "duplicate"},
		{Role: "tool", ToolCallID: "unknown", Content: "ignore"},
		{Role: "system", Content: "late reminder"},
		{Role: "user", Content: "continue"},
	}
	before, _ := json.Marshal(req)
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	wire := requestJSON(t, params)
	if !sameJSONArray(wire["system"], json.RawMessage(`[{"text":"first","type":"text"},{"text":"second","type":"text"}]`)) {
		t.Fatalf("leading system order changed: %s", wire["system"])
	}
	want := json.RawMessage(`[
		{"role":"user","content":[{"type":"text","text":"request"}]},
		{"role":"assistant","content":[{"type":"text","text":"reading"},{"type":"tool_use","id":"call1","name":"read","input":{"chapter":1}},{"type":"tool_use","id":"call2","name":"read","input":{"chapter":2}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"call1","is_error":false,"content":[{"type":"text","text":"first body"}]},{"type":"tool_result","tool_use_id":"call2","is_error":false,"content":[{"type":"text","text":"second body"}]},{"type":"text","text":"<system-reminder>\nlate reminder\n</system-reminder>"},{"type":"text","text":"continue"}]}
	]`)
	if !sameJSONArray(wire["messages"], want) {
		t.Fatalf("wrong history projection:\ngot %s\nwant %s", wire["messages"], want)
	}
	after, _ := json.Marshal(req)
	if string(before) != string(after) {
		t.Fatal("history cleanup mutated the input")
	}
}

func TestRequestDoesNotPairToolsAcrossConversationBoundary(t *testing.T) {
	p, req := testRequest()
	req.Messages = append(req.Messages,
		adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call1", Name: "read", Arguments: json.RawMessage(`{}`)}}},
		adapter.Message{Role: "user", Content: "new question"},
		adapter.Message{Role: "tool", ToolCallID: "call1", Content: "late result"},
	)
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(params)
	if len(params.Messages) != 1 || strings.Contains(string(data), "tool_use") || strings.Contains(string(data), "tool_result") {
		t.Fatalf("orphan tool state leaked into request: %s", data)
	}
}

func TestRequestAllowsEmptyToolResultWithoutEmptyTextBlock(t *testing.T) {
	p, req := testRequest()
	req.Messages = append(req.Messages,
		adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call1", Name: "read", Arguments: json.RawMessage(`{}`)}}},
		adapter.Message{Role: "tool", ToolCallID: "call1"},
	)
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	block := params.Messages[2].Content[0].OfToolResult
	if block == nil || block.ToolUseID != "call1" || len(block.Content) != 0 {
		t.Fatal("empty result must not contain an empty nested text block")
	}
}

func TestRequestRejectsInvalidHistories(t *testing.T) {
	for _, tt := range []struct {
		name     string
		messages []adapter.Message
	}{
		{"empty", nil},
		{"system-only", []adapter.Message{{Role: "system", Content: "system"}}},
		{"assistant-first", []adapter.Message{{Role: "assistant", Content: "answer"}, {Role: "user", Content: "question"}}},
		{"prefill", []adapter.Message{{Role: "user", Content: "question"}, {Role: "assistant", Content: "partial"}}},
		{"unknown-role", []adapter.Message{{Role: "developer", Content: "instructions"}}},
		{"empty-id", []adapter.Message{{Role: "user", Content: "question"}, {Role: "assistant", ToolCalls: []adapter.ToolCall{{Name: "read", Arguments: json.RawMessage(`{}`)}}}, {Role: "user", Content: "continue"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, req := testRequest()
			req.Messages = tt.messages
			if _, err := p.buildRequest(req); err == nil {
				t.Fatal("invalid history was accepted")
			}
		})
	}
	for _, input := range []string{"", "null", "[]", `{"chapter":`} {
		p, req := testRequest()
		req.Messages = append(req.Messages,
			adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call1", Name: "read", Arguments: json.RawMessage(input)}}},
			adapter.Message{Role: "tool", ToolCallID: "call1", Content: "body"},
		)
		if _, err := p.buildRequest(req); err == nil {
			t.Fatalf("invalid retained tool input accepted: %s", input)
		}
	}
}

func TestRequestRejectsAmbiguousToolIDs(t *testing.T) {
	p, base := testRequest()
	call := adapter.ToolCall{ID: "call1", Name: "read", Arguments: json.RawMessage(`{}`)}
	for _, kind := range []string{"conflicting-declarations", "reused-id", "wrong-result-name"} {
		t.Run(kind, func(t *testing.T) {
			req := base
			req.Messages = append([]adapter.Message(nil), base.Messages...)
			assistant := adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{call}}
			result := adapter.Message{Role: "tool", ToolCallID: "call1", Content: "body"}
			switch kind {
			case "conflicting-declarations":
				conflict := call
				conflict.Name = "other"
				assistant.ToolCalls = append(assistant.ToolCalls, conflict)
			case "wrong-result-name":
				result.ToolName = "other"
			}
			req.Messages = append(req.Messages, assistant, result)
			if kind == "reused-id" {
				req.Messages = append(req.Messages, assistant, result)
			}
			if _, err := p.buildRequest(req); err == nil {
				t.Fatal("ambiguous tool identity was accepted")
			}
		})
	}
}

func sameJSONArray(a, b json.RawMessage) bool {
	// 测试按 JSON 值比较，同时保留大整数的词法精度。
	var left, right any
	decode := func(raw json.RawMessage, target *any) error {
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		return decoder.Decode(target)
	}
	if decode(a, &left) != nil || decode(b, &right) != nil {
		return false
	}
	x, _ := json.Marshal(left)
	y, _ := json.Marshal(right)
	return string(x) == string(y)
}
