package anthropic

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestResponsePreservesTextStopReasons(t *testing.T) {
	for _, stop := range []string{"end_turn", "stop_sequence", "max_tokens", "refusal", "model_context_window_exceeded"} {
		t.Run(stop, func(t *testing.T) {
			p, req := testRequest()
			params, err := p.buildRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			var message sdk.Message
			if err := json.Unmarshal([]byte(strings.Replace(generatedTextResponse, `"end_turn"`, `"`+stop+`"`, 1)), &message); err != nil {
				t.Fatal(err)
			}
			response, err := p.convertResponse(params, &message)
			if err != nil || response.Content != "done" || response.StopReason != stop || response.State == nil || !response.State.Complete {
				t.Fatalf("valid terminal response rejected: %+v, %v", response, err)
			}
			wantTurnComplete := stop == "end_turn" || stop == "stop_sequence" || stop == "refusal"
			if response.State.TurnComplete != wantTurnComplete || response.State.StopReason != stop {
				t.Fatal("protocol completion was confused with turn completion")
			}
			if response.Usage["prompt_tokens"] != float64(2) || response.Usage["completion_tokens"] != float64(3) || response.Usage["total_tokens"] != float64(5) {
				t.Fatal("usage without cache counters was not normalized")
			}
		})
	}
}

func TestResponseRejectsInvalidEnvelopeAndContent(t *testing.T) {
	for _, tt := range []struct {
		name, field, replacement, kind string
	}{
		{"missing-id", "id", `""`, "invalid_response"},
		{"wrong-type", "type", `"error"`, "invalid_response"},
		{"wrong-role", "role", `"user"`, "invalid_response"},
		{"null-content", "content", `null`, "invalid_response"},
		{"empty-content", "content", `[]`, "invalid_response"},
		{"unknown-block", "content", `[{"type":"future_block","data":"opaque"}]`, "invalid_response"},
		{"server-tool", "content", `[{"type":"server_tool_use","id":"server1","name":"web_search","input":{}}]`, "invalid_response"},
		{"missing-text", "content", `[{"type":"text"}]`, "invalid_response"},
		{"invalid-text", "content", `[{"type":"text","text":123}]`, "invalid_response"},
		{"missing-signature", "content", `[{"type":"thinking","thinking":"summary"}]`, "invalid_response"},
		{"missing-thinking", "content", `[{"type":"thinking","signature":"sig"}]`, "invalid_response"},
		{"missing-redacted-data", "content", `[{"type":"redacted_thinking","data":""}]`, "invalid_response"},
		{"null-stop", "stop_reason", `null`, "invalid_response"},
		{"unsupported-stop", "stop_reason", `"future_stop"`, "invalid_response"},
		{"server-continuation", "stop_reason", `"pause_turn"`, "invalid_response"},
		{"tool-stop-without-call", "stop_reason", `"tool_use"`, "invalid_response"},
		{"missing-usage", "usage", `null`, "invalid_response"},
		{"negative-usage", "usage", `{"input_tokens":-1,"output_tokens":3}`, "invalid_response"},
		{"invalid-cache", "usage", `{"input_tokens":1,"output_tokens":3,"cache_read_input_tokens":"10"}`, "invalid_response"},
		{"invalid-output", "usage", `{"input_tokens":1,"output_tokens":"3"}`, "invalid_response"},
		{"invalid-reasoning", "usage", `{"input_tokens":1,"output_tokens":3,"output_tokens_details":{"thinking_tokens":4}}`, "invalid_response"},
		{"invalid-reasoning-type", "usage", `{"input_tokens":1,"output_tokens":3,"output_tokens_details":{"thinking_tokens":"2"}}`, "invalid_response"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]json.RawMessage
			if err := json.Unmarshal([]byte(generatedTextResponse), &body); err != nil {
				t.Fatal(err)
			}
			body[tt.field] = json.RawMessage(tt.replacement)
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			var message sdk.Message
			if err := json.Unmarshal(data, &message); err != nil {
				// SDK 本身也可能直接拒绝错误字段类型。
				return
			}
			p, req := testRequest()
			params, err := p.buildRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			response, err := p.convertResponse(params, &message)
			var apiErr *adapter.APIError
			if !errors.As(err, &apiErr) || apiErr.Kind != tt.kind || response.State != nil || len(response.ToolCalls) != 0 {
				t.Fatalf("invalid response produced usable state: %+v, %v", response, err)
			}
		})
	}
}

func TestResponseValidatesEntireToolGroup(t *testing.T) {
	for _, input := range []string{`[]`, `null`, `"truncated JSON"`, `123`} {
		t.Run(input, func(t *testing.T) {
			data := strings.Replace(generatedToolResponse, `"id":"call2","name":"read","input":{}`, `"id":"call2","name":"read","input":`+input, 1)
			var message sdk.Message
			if err := json.Unmarshal([]byte(data), &message); err != nil {
				t.Fatal(err)
			}
			p, req := testRequest()
			params, err := p.buildRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			response, err := p.convertResponse(params, &message)
			var apiErr *adapter.APIError
			if !errors.As(err, &apiErr) || apiErr.Kind != "tool_args_invalid" || !apiErr.Retryable || len(response.ToolCalls) != 0 || response.State != nil {
				t.Fatalf("part of an invalid tool group was exposed: %+v, %v", response, err)
			}
		})
	}
	for _, data := range []string{
		strings.Replace(generatedToolResponse, `"id":"call2"`, `"id":"call1"`, 1),
		strings.Replace(generatedToolResponse, `"id":"call2"`, `"id":""`, 1),
		strings.Replace(generatedToolResponse, `"stop_reason":"tool_use"`, `"stop_reason":"max_tokens"`, 1),
		strings.Replace(generatedToolResponse, `"id":"call2"`, `"id":"call2","caller":{"type":"code_execution","tool_id":"server1"}`, 1),
	} {
		var message sdk.Message
		if err := json.Unmarshal([]byte(data), &message); err != nil {
			t.Fatal(err)
		}
		p, req := testRequest()
		params, err := p.buildRequest(req)
		if err != nil {
			t.Fatal(err)
		}
		if response, err := p.convertResponse(params, &message); err == nil || len(response.ToolCalls) != 0 || response.State != nil {
			t.Fatal("ambiguous or unfinished tool group was accepted")
		}
	}
}

func TestResponseOwnsToolInputsAndOpaquePayload(t *testing.T) {
	var message sdk.Message
	if err := json.Unmarshal([]byte(generatedToolResponse), &message); err != nil {
		t.Fatal(err)
	}
	p, req := testRequest()
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	response, err := p.convertResponse(params, &message)
	if err != nil {
		t.Fatal(err)
	}
	payload := string(response.State.Payload)
	for i := range message.Content[4].Input {
		message.Content[4].Input[i] = ' '
	}
	if string(response.ToolCalls[0].ArgumentsJSON) != `{"chapter":9007199254740993}` || string(response.State.Payload) != payload {
		t.Fatal("response aliases SDK input bytes")
	}
	response.ToolCalls[0].ArgumentsJSON[0] = '['
	if string(response.State.Payload) != payload {
		t.Fatal("public tool args share native payload bytes")
	}
}
