package openairesponses

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/responses"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func convertNativeResponse(t *testing.T, raw string) (adapter.Response, error) {
	t.Helper()
	p := requestProvider(t)
	params, err := p.buildRequest(requestInput())
	if err != nil {
		t.Fatal(err)
	}
	var result responses.Response
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return adapter.Response{}, err
	}
	return p.convertResponse(params, &result)
}

func responseWithOutput(output string) string {
	return `{"id":"resp_1","object":"response","status":"completed","output":` + output + `}`
}

func TestResponseRetainsRefusalAndDoesNotExposeEncryptedReasoning(t *testing.T) {
	output := `[` + sdkReasoning + `,
		{"type":"message","id":"msg_refusal","role":"assistant","status":"completed","content":[
			{"type":"output_text","text":"first"},
			{"type":"refusal","refusal":"cannot comply"},
			{"type":"output_text","text":"last"}
		]},
		{"type":"reasoning","id":"rs_2","encrypted_content":"private-state","summary":[{"type":"summary_text","text":"next"},{"type":"summary_text","text":"step"}]}
	]`
	response, err := convertNativeResponse(t, responseWithOutput(output))
	if err != nil || response.Content != "firstcannot complylast" || response.ThinkingContent != "plannextstep" ||
		response.State == nil || !response.State.Complete || !response.State.TurnComplete || len(response.ToolCalls) != 0 || response.Usage != nil {
		t.Fatalf("invalid text/refusal/reasoning projection: %+v, %v", response, err)
	}
	var payload replayPayload
	compact, err := json.Marshal(json.RawMessage(output))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(response.State.Payload, &payload); err != nil || string(payload.Output) != string(compact) || payload.RequestHash == "" {
		t.Fatal("response state lost the original native output")
	}
	if strings.Contains(response.Content+response.ThinkingContent, "private-state") {
		t.Fatal("encrypted reasoning was exposed as display text")
	}
}

func TestResponseRejectsPartialOrFailedEnvelopes(t *testing.T) {
	for _, tt := range []struct {
		name, raw, kind string
		retryable       bool
	}{
		{"missing ID", strings.Replace(generatedTextResponse, `"id":"resp_final"`, `"id":null`, 1), "invalid_response", false},
		{"numeric ID", strings.Replace(generatedTextResponse, `"id":"resp_final"`, `"id":123`, 1), "invalid_response", false},
		{"wrong object", strings.Replace(generatedTextResponse, `"object":"response"`, `"object":"message"`, 1), "invalid_response", false},
		{"in progress", strings.Replace(generatedTextResponse, `"status":"completed"`, `"status":"in_progress"`, 1), "invalid_response", false},
		{"queued", strings.Replace(generatedTextResponse, `"status":"completed"`, `"status":"queued"`, 1), "invalid_response", false},
		{"unknown status", strings.Replace(generatedTextResponse, `"status":"completed"`, `"status":"requires_action"`, 1), "invalid_response", false},
		{"cancelled", strings.Replace(generatedToolResponse, `"status":"completed"`, `"status":"cancelled"`, 1), "response_cancelled", false},
		{"failed", strings.Replace(generatedToolResponse, `"status":"completed"`, `"status":"failed"`, 1), "response_failed", false},
		{"max output", `{"id":"resp_1","object":"response","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[` + sdkFunction + `]}`, "response_incomplete", false},
		{"content filter", `{"id":"resp_1","object":"response","status":"incomplete","incomplete_details":{"reason":"content_filter"}}`, "response_incomplete", false},
		{"rate limit", `{"id":"resp_1","object":"response","status":"failed","error":{"code":"rate_limit_exceeded","message":"busy"}}`, "rate_limit_exceeded", true},
		{"server error", `{"error":{"code":"server_error","message":"busy"}}`, "server_error", true},
		{"invalid prompt", `{"error":{"code":"invalid_prompt","message":"invalid request"}}`, "invalid_prompt", false},
		{"unknown error", `{"error":{"message":"failed"}}`, "response_failed", false},
		{"malformed error", `{"error":{}}`, "invalid_response", false},
		{"empty output", responseWithOutput(`[]`), "invalid_response", false},
		{"null output", responseWithOutput(`null`), "invalid_response", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response, err := convertNativeResponse(t, tt.raw)
			var fault *adapter.APIError
			if !errors.As(err, &fault) || fault.Kind != tt.kind || fault.Retryable != tt.retryable {
				t.Fatalf("unexpected response failure: %v", err)
			}
			if !reflect.DeepEqual(response, adapter.Response{}) {
				t.Fatal("failed response exposed partial tools or native state")
			}
		})
	}
	p := requestProvider(t)
	if _, err := p.convertResponse(sdkRequest(), nil); err == nil {
		t.Fatal("nil SDK response accepted")
	}
}

func TestResponseRejectsInvalidToolGroupWithoutPartialResults(t *testing.T) {
	for _, tt := range []struct{ name, call string }{
		{"malformed JSON arguments", strings.Replace(sdkFunction, `\"chapter\":9007199254740993}`, `\"chapter\":`, 1)},
		{"array arguments", strings.Replace(sdkFunction, `"{\"chapter\":9007199254740993}"`, `"[]"`, 1)},
		{"null arguments", strings.Replace(sdkFunction, `"{\"chapter\":9007199254740993}"`, `"null"`, 1)},
		{"missing call ID", strings.Replace(sdkFunction, `"call_id":"call_1"`, `"call_id":""`, 1)},
		{"blank name", strings.Replace(sdkFunction, `"name":"read"`, `"name":" "`, 1)},
		{"duplicate call ID", strings.Replace(sdkFunction, `"id":"fc_1"`, `"id":"fc_2"`, 1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			first := strings.Replace(sdkFunction, `"call_id":"call_1"`, `"call_id":"call_good"`, 1)
			first = strings.Replace(first, `"id":"fc_1"`, `"id":"fc_good"`, 1)
			if tt.name == "duplicate call ID" {
				first = sdkFunction
			}
			response, err := convertNativeResponse(t, responseWithOutput(`[`+sdkMessage+`,`+first+`,`+tt.call+`]`))
			var fault *adapter.APIError
			if !errors.As(err, &fault) || fault.Kind != "tool_args_invalid" || !fault.Retryable || !reflect.DeepEqual(response, adapter.Response{}) {
				t.Fatalf("invalid function group was not rejected atomically: %+v, %v", response, err)
			}
		})
	}
}

func TestResponseRejectsUnsupportedOrMalformedNativeItems(t *testing.T) {
	for _, tt := range []struct{ name, item string }{
		{"server tool", `{"type":"web_search_call","id":"ws_1","status":"completed"}`},
		{"provider program", strings.Replace(sdkFunction, `"extension":null`, `"caller":{"type":"program","program_id":"prog_1"}`, 1)},
		{"async function", strings.Replace(sdkFunction, `"extension":null`, `"async":true`, 1)},
		{"namespaced function", strings.Replace(sdkFunction, `"extension":null`, `"namespace":"server"`, 1)},
		{"incomplete function", strings.Replace(sdkFunction, `"status":"completed"`, `"status":"incomplete"`, 1)},
		{"non-string text", strings.Replace(sdkMessage, `"text":"ok"`, `"text":123`, 1)},
		{"null text", strings.Replace(sdkMessage, `"text":"ok"`, `"text":null`, 1)},
		{"missing ciphertext", strings.Replace(sdkReasoning, `"opaque-test-state"`, `""`, 1)},
		{"non-string summary", strings.Replace(sdkReasoning, `"text":"plan"`, `"text":false`, 1)},
		{"unsupported summary", strings.Replace(sdkReasoning, `"summary_text"`, `"reasoning_text"`, 1)},
		{"duplicate item ID", sdkMessage + `,` + sdkMessage},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response, err := convertNativeResponse(t, responseWithOutput(`[`+tt.item+`]`))
			if err == nil || !reflect.DeepEqual(response, adapter.Response{}) {
				t.Fatalf("unsupported native state accepted: %+v, %v", response, err)
			}
		})
	}
}

func TestNormalizeUsagePreservesProviderTotalsAndOptionalDetails(t *testing.T) {
	usage, err := normalizeUsage(json.RawMessage(`{"input_tokens":10,"output_tokens":7,"total_tokens":19,"input_tokens_details":{"cached_tokens":2,"cache_write_tokens":3},"output_tokens_details":{"reasoning_tokens":4}}`))
	if err != nil || usage["prompt_tokens"] != float64(10) || usage["completion_tokens"] != float64(7) || usage["total_tokens"] != float64(19) ||
		usage["prompt_cache_hit_tokens"] != float64(2) || usage["prompt_cache_miss_tokens"] != float64(8) || usage["cache_creation_input_tokens"] != float64(3) ||
		!reflect.DeepEqual(usage["completion_tokens_details"], map[string]any{"reasoning_tokens": float64(4)}) {
		t.Fatalf("usage replaced provider total or counted cache/thinking twice: %v, %v", usage, err)
	}
	for _, raw := range []string{"", "null"} {
		usage, err := normalizeUsage(json.RawMessage(raw))
		if err != nil || usage != nil {
			t.Fatalf("missing usage became zero: %v, %v", usage, err)
		}
	}
	usage, err = normalizeUsage(json.RawMessage(`{"input_tokens":0,"output_tokens":0,"total_tokens":0}`))
	if err != nil || len(usage) != 3 || usage["prompt_tokens"] != float64(0) {
		t.Fatalf("valid zero usage was dropped or invented optional details: %v, %v", usage, err)
	}
}

func TestNormalizeUsageRejectsInvalidKnownCounters(t *testing.T) {
	for _, raw := range []string{
		`[]`, `{}`, `true`,
		`{"input_tokens":1,"output_tokens":2}`,
		`{"input_tokens":null,"output_tokens":2,"total_tokens":3}`,
		`{"input_tokens":"1","output_tokens":2,"total_tokens":3}`,
		`{"input_tokens":1,"output_tokens":2.5,"total_tokens":3}`,
		`{"input_tokens":-1,"output_tokens":2,"total_tokens":3}`,
		`{"input_tokens":1,"output_tokens":-2,"total_tokens":3}`,
		`{"input_tokens":1,"output_tokens":2,"total_tokens":-3}`,
		`{"input_tokens":9223372036854775808,"output_tokens":2,"total_tokens":3}`,
		`{"input_tokens":1,"output_tokens":2,"total_tokens":3,"input_tokens_details":[]}`,
		`{"input_tokens":1,"output_tokens":2,"total_tokens":3,"input_tokens_details":{"cached_tokens":2}}`,
		`{"input_tokens":1,"output_tokens":2,"total_tokens":3,"input_tokens_details":{"cached_tokens":-1}}`,
		`{"input_tokens":1,"output_tokens":2,"total_tokens":3,"input_tokens_details":{"cache_write_tokens":"1"}}`,
		`{"input_tokens":1,"output_tokens":2,"total_tokens":3,"input_tokens_details":{"cache_write_tokens":2}}`,
		`{"input_tokens":1,"output_tokens":2,"total_tokens":3,"output_tokens_details":false}`,
		`{"input_tokens":1,"output_tokens":2,"total_tokens":3,"output_tokens_details":{"reasoning_tokens":3}}`,
		`{"input_tokens":1,"output_tokens":2,"total_tokens":3,"output_tokens_details":{"reasoning_tokens":-1}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			if usage, err := normalizeUsage(json.RawMessage(raw)); err == nil || usage != nil {
				t.Fatalf("invalid usage accepted: %v", usage)
			}
		})
	}
	raw := strings.Replace(generatedToolResponse, `"input_tokens":20`, `"input_tokens":"20"`, 1)
	if response, err := convertNativeResponse(t, raw); err == nil || !reflect.DeepEqual(response, adapter.Response{}) {
		t.Fatal("invalid usage exposed otherwise valid tools or state")
	}
}
