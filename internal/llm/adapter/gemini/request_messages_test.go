package gemini

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestRequestToolPairingAndHistoryValidation(t *testing.T) {
	call := adapter.ToolCall{ID: "a", Name: "read", Arguments: json.RawMessage(`{"nested":{"b":2,"a":9007199254740993}}`)}
	assistant := adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{call}}
	result := adapter.Message{Role: "tool", ToolCallID: "a", ToolName: "read", Content: "章节"}
	user := adapter.Message{Role: "user", Content: "继续"}
	equivalent := call.Clone()
	equivalent.Arguments = json.RawMessage(`{"nested":{"a":9007199254740993,"b":2}}`)
	conflicting := call.Clone()
	conflicting.Arguments = json.RawMessage(`{"nested":{"a":9007199254740992,"b":2}}`)
	for _, tc := range []struct {
		name      string
		history   []adapter.Message
		calls     int
		results   int
		wantError string
	}{
		{name: "paired", history: []adapter.Message{user, assistant, result}, calls: 1, results: 1},
		{name: "duplicate declaration", history: []adapter.Message{user, {Role: "assistant", ToolCalls: []adapter.ToolCall{call, equivalent}}, result}, calls: 1, results: 1},
		{name: "duplicate result", history: []adapter.Message{user, assistant, result, result}, calls: 1, results: 1},
		{name: "conflicting declaration", history: []adapter.Message{user, {Role: "assistant", ToolCalls: []adapter.ToolCall{call, conflicting}}, result}, wantError: "conflicting tool call"},
		{name: "conflicting result", history: []adapter.Message{user, assistant, result, {Role: "tool", ToolCallID: "a", Content: "其他章节"}}, wantError: "conflicting tool result"},
		{name: "missing result", history: []adapter.Message{user, assistant, user}},
		{name: "orphan result", history: []adapter.Message{result, user}},
		{name: "cannot cross user boundary", history: []adapter.Message{user, assistant, user, result}},
		{name: "cannot cross system boundary", history: []adapter.Message{user, assistant, {Role: "system", Content: "提示"}, result}},
		{name: "cannot cross assistant boundary", history: []adapter.Message{user, assistant, {Role: "assistant", Content: "正文"}, result}},
		{name: "reused call id", history: []adapter.Message{user, assistant, result, user, assistant, result}, wantError: "reused tool call"},
		{name: "wrong result name", history: []adapter.Message{user, assistant, {Role: "tool", ToolCallID: "a", ToolName: "write"}}, wantError: "name does not match"},
		{name: "empty call id", history: []adapter.Message{user, {Role: "assistant", ToolCalls: []adapter.ToolCall{{Name: "read", Arguments: json.RawMessage(`{}`)}}}}, wantError: "id must not be empty"},
		{name: "missing call name", history: []adapter.Message{user, {Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "a", Arguments: json.RawMessage(`{}`)}}}, {Role: "tool", ToolCallID: "a"}}, wantError: "requires a name"},
		{name: "array arguments", history: []adapter.Message{user, {Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "a", Name: "read", Arguments: json.RawMessage(`[]`)}}}, result}, wantError: "object arguments"},
		{name: "invalid JSON arguments", history: []adapter.Message{user, {Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "a", Name: "read", Arguments: json.RawMessage(`{`)}}}, result}, wantError: "object arguments"},
		{name: "unknown role", history: []adapter.Message{{Role: "developer", Content: "规则"}}, wantError: "unsupported role"},
		{name: "user cannot declare tools", history: []adapter.Message{{Role: "user", Content: "内容", ToolCalls: []adapter.ToolCall{call}}}, wantError: "require assistant role"},
		{name: "leading system cannot declare tools", history: []adapter.Message{{Role: "system", ToolCalls: []adapter.ToolCall{call}}, user}, wantError: "require assistant role"},
		{name: "tool cannot declare tools", history: []adapter.Message{user, assistant, {Role: "tool", ToolCallID: "a", ToolCalls: []adapter.ToolCall{call}}}, wantError: "require assistant role"},
		{name: "empty history", wantError: "no input steps"},
		{name: "system only", history: []adapter.Message{{Role: "system", Content: "规则"}}, wantError: "no input steps"},
		{name: "display thinking is not input", history: []adapter.Message{{Role: "assistant", ThinkingContent: "思考"}}, wantError: "no input steps"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := requestFixture()
			req.Messages = tc.history
			params, err := requestProvider().buildRequest(req)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("got %v, want error containing %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(params.Input)
			if err != nil {
				t.Fatal(err)
			}
			var steps []struct{ Type string }
			if err := json.Unmarshal(raw, &steps); err != nil {
				t.Fatal(err)
			}
			calls, results := 0, 0
			for _, step := range steps {
				if step.Type == "function_call" {
					calls++
				}
				if step.Type == "function_result" {
					results++
				}
			}
			if calls != tc.calls || results != tc.results {
				t.Fatalf("got calls/results=%d/%d, want %d/%d: %s", calls, results, tc.calls, tc.results, raw)
			}
		})
	}
}
