package gemini

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestResponseRejectsInvalidStatesAndStepsWithoutPartialTools(t *testing.T) {
	for _, tc := range []struct {
		name, old, replacement, kind string
	}{
		{name: "missing id", old: `"id":"interaction_tools"`, replacement: `"id":""`},
		{name: "non-string id", old: `"id":"interaction_tools"`, replacement: `"id":123`},
		{name: "wrong object", old: `"object":"interaction"`, replacement: `"object":"message"`},
		{name: "managed agent", old: `"object":"interaction"`, replacement: `"object":"interaction","agent":"research"`},
		{name: "completed with tools", old: `"status":"requires_action"`, replacement: `"status":"completed"`},
		{name: "in progress", old: `"status":"requires_action"`, replacement: `"status":"in_progress"`, kind: "invalid_response"},
		{name: "queued", old: `"status":"requires_action"`, replacement: `"status":"queued"`, kind: "invalid_response"},
		{name: "incomplete", old: `"status":"requires_action"`, replacement: `"status":"incomplete"`, kind: "interaction_incomplete"},
		{name: "cancelled", old: `"status":"requires_action"`, replacement: `"status":"cancelled"`, kind: "interaction_cancelled"},
		{name: "failed", old: `"status":"requires_action"`, replacement: `"status":"failed"`, kind: "interaction_failed"},
		{name: "diagnostic errors", old: `"status":"requires_action"`, replacement: `"status":"requires_action","errors":[{"code":"model_fault","message":"failed inside the model"}]`, kind: "model_fault"},
		{name: "second tool has array arguments", old: `"id":"b","name":"read","arguments":{}`, replacement: `"id":"b","name":"read","arguments":[]`, kind: "tool_args_invalid"},
		{name: "second tool has null arguments", old: `"id":"b","name":"read","arguments":{}`, replacement: `"id":"b","name":"read","arguments":null`, kind: "tool_args_invalid"},
		{name: "duplicate call id", old: `"id":"b","name":"read","arguments":{}`, replacement: `"id":"a","name":"read","arguments":{}`, kind: "tool_args_invalid"},
		{name: "empty call name", old: `"id":"b","name":"read","arguments":{}`, replacement: `"id":"b","name":"","arguments":{}`, kind: "tool_args_invalid"},
		{name: "missing signature", old: `"signature":"opaque-signature"`, replacement: `"signature":""`},
		{name: "invalid summary", old: `"type":"text","text":"摘要"`, replacement: `"type":"text","text":123`},
		{name: "unknown summary", old: `"type":"text","text":"摘要"`, replacement: `"type":"document","text":"摘要"`},
		{name: "non-text output", old: `"type":"text","text":"后"`, replacement: `"type":"audio","data":"AA=="`},
		{name: "missing output text", old: `"type":"text","text":"后"`, replacement: `"type":"text"`, kind: "invalid_response"},
		{name: "server tool", old: `"type":"function_call","id":"b"`, replacement: `"type":"google_search_call","id":"b"`},
		{name: "step error", old: `"type":"function_call","id":"b"`, replacement: `"type":"function_call","error":{"code":13,"message":"failed"},"id":"b"`, kind: "invalid_response"},
		{name: "invalid usage", old: `"total_cached_tokens":3`, replacement: `"total_cached_tokens":8`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := requestProvider()
			params, err := p.buildRequest(requestFixture())
			if err != nil {
				t.Fatal(err)
			}
			raw := strings.Replace(generatedToolInteraction, tc.old, tc.replacement, 1)
			if raw == generatedToolInteraction {
				t.Fatal("test replacement did not match fixture")
			}
			response, err := p.convertResponse(params, json.RawMessage(raw))
			if tc.kind == "" {
				wantCalls := 2
				if tc.name == "server tool" {
					wantCalls = 1
				}
				if err != nil || response.State == nil || response.State.TurnComplete || len(response.ToolCalls) != wantCalls || response.Content == "" {
					t.Fatalf("auxiliary data blocked usable output: %+v, %v", response, err)
				}
				return
			}
			var apiErr *adapter.APIError
			if !errors.As(err, &apiErr) || apiErr.Kind != tc.kind || response.State != nil || len(response.ToolCalls) != 0 {
				t.Fatalf("invalid response delivered a partial tool group or wrong error: response=%+v err=%+v", response, err)
			}
			if tc.kind == "tool_args_invalid" && !apiErr.Retryable {
				t.Fatal("tool argument errors lost the existing retry contract")
			}
		})
	}
}

func TestResponseRequiresNativeStepsAndValidToolStatus(t *testing.T) {
	for _, raw := range []string{
		`null`, `[]`, `{`,
		`{"id":"test","status":"completed","output_text":"flattened text only"}`,
		`{"id":"test","status":"completed","steps":[]}`,
		`{"id":"test","status":"completed","steps":[{"type":"user_input","content":[{"type":"text","text":"echo"}]}]}`,
	} {
		p := requestProvider()
		params, err := p.buildRequest(requestFixture())
		if err != nil {
			t.Fatal(err)
		}
		if response, err := p.convertResponse(params, json.RawMessage(raw)); err == nil || response.State != nil || len(response.ToolCalls) > 0 {
			t.Fatalf("invalid response was accepted: %s, %+v", raw, response)
		}
	}
}

func TestResponseOwnsNativePayloadAndToolArguments(t *testing.T) {
	p := requestProvider()
	params, err := p.buildRequest(requestFixture())
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(generatedToolInteraction)
	response, err := p.convertResponse(params, raw)
	if err != nil {
		t.Fatal(err)
	}
	payload := string(response.State.Payload)
	for i := range raw {
		raw[i] = 'x'
	}
	if string(response.State.Payload) != payload || string(response.ToolCalls[0].ArgumentsJSON) != `{"chapter":9007199254740993}` {
		t.Fatal("response aliases the source body")
	}
	response.ToolCalls[0].ArgumentsJSON[0] = '['
	if string(response.State.Payload) != payload {
		t.Fatal("tool arguments alias the native replay payload")
	}
}

func TestNormalizeUsagePreservesOptionalFieldsAndProviderTotals(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      map[string]any
	}{
		{name: "missing"},
		{name: "null", raw: "null"},
		{name: "null with whitespace", raw: " null "},
		{name: "empty", raw: `{}`},
		{name: "input only", raw: `{"total_input_tokens":7}`, want: map[string]any{"prompt_tokens": float64(7)}},
		{name: "zero output", raw: `{"total_output_tokens":0}`, want: map[string]any{"completion_tokens": float64(0)}},
		{name: "thoughts exceed visible output", raw: `{"total_output_tokens":3,"total_thought_tokens":22}`, want: map[string]any{"completion_tokens": float64(25), "completion_tokens_details": map[string]any{"reasoning_tokens": float64(22)}}},
		{name: "provider total", raw: `{"total_input_tokens":7,"total_output_tokens":20,"total_thought_tokens":22,"total_tokens":52}`, want: map[string]any{"prompt_tokens": float64(7), "completion_tokens": float64(42), "total_tokens": float64(52), "completion_tokens_details": map[string]any{"reasoning_tokens": float64(22)}}},
		{name: "cached subset", raw: `{"total_input_tokens":7,"total_cached_tokens":3}`, want: map[string]any{"prompt_tokens": float64(7), "prompt_cache_hit_tokens": float64(3), "prompt_cache_miss_tokens": float64(4)}},
		{name: "tool use is detail", raw: `{"total_input_tokens":100,"total_output_tokens":25,"total_tokens":125,"total_tool_use_tokens":50}`, want: map[string]any{"prompt_tokens": float64(100), "completion_tokens": float64(25), "total_tokens": float64(125), "total_tool_use_tokens": float64(50)}},
		{name: "negative", raw: `{"total_thought_tokens":-1}`},
		{name: "fraction", raw: `{"total_output_tokens":1.5}`},
		{name: "string", raw: `{"total_tokens":"7"}`},
		{name: "explicit null counter", raw: `{"total_input_tokens":null}`},
		{name: "overflow", raw: `{"total_tokens":9223372036854775808}`},
		{name: "cache exceeds input", raw: `{"total_input_tokens":7,"total_cached_tokens":8}`},
		{name: "array", raw: `[]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeUsage(json.RawMessage(tc.raw))
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("usage mapping mismatch: got=%v err=%v want=%v", got, err, tc.want)
			}
		})
	}
}
