package openairesponses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

const generatedTextResponse = `{"id":"resp_final","object":"response","status":"completed","model":"actual-model-snapshot","output":[{"type":"message","id":"msg_final","role":"assistant","status":"completed","content":[{"type":"output_text","text":"done","annotations":[]}]}]}`

const generatedToolResponse = `{
	"id":"resp_tools","object":"response","status":"completed","model":"actual-model-snapshot",
	"output":[` + sdkReasoning + `,
		{"type":"reasoning","id":"rs_2","summary":[],"encrypted_content":"signature-only"},
		` + sdkMessage + `,` + sdkFunction + `,
		{"type":"message","id":"msg_2","role":"assistant","status":"completed","content":[{"type":"output_text","text":"and","annotations":[]}]},
		{"type":"function_call","id":"fc_2","call_id":"call_2","name":"read","arguments":"{}","status":"completed","caller":{"type":"direct"}}
	],
	"usage":{"input_tokens":20,"output_tokens":13,"total_tokens":33,"input_tokens_details":{"cached_tokens":7,"cache_write_tokens":3},"output_tokens_details":{"reasoning_tokens":5}}
}`

func generateAdapter(t *testing.T, transport http.RoundTripper) *Adapter {
	t.Helper()
	backend, err := New(adapter.ProviderMap{"relay": {
		Name:         "relay",
		ProviderType: "openai",
		ChatURL:      "https://configured.invalid/proxy/v8/responses?route=1",
		APIKey:       "instance-key",
	}}, &http.Client{Transport: transport}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return backend
}

func TestGenerateProducesReusableNativeState(t *testing.T) {
	var requests []map[string]json.RawMessage
	backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != "https://configured.invalid/proxy/v8/responses?route=1" ||
			r.Header.Get("Authorization") != "Bearer instance-key" || r.Header.Get("Accept") != "application/json" {
			t.Error("Generate lost its complete endpoint, credentials or JSON request contract")
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if string(body["stream"]) != "false" || string(body["store"]) != "false" || string(body["background"]) != "false" ||
			body["previous_response_id"] != nil || body["conversation"] != nil {
			t.Error("Generate must send a single stateless non-streaming request")
		}
		requests = append(requests, body)
		if len(requests) == 1 {
			return jsonResponse(generatedToolResponse), nil
		}
		return jsonResponse(generatedTextResponse), nil
	}))
	req := requestInput()
	req.Model = "gpt-5.6-sol"
	req.Tools = []adapter.ToolDefinition{{Name: "read", InputSchema: json.RawMessage(`{"type":"object","properties":{"chapter":{"type":"integer"}}}`)}}
	before, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	response, err := backend.Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || response.Content != "okand" || response.ThinkingContent != "plan" || len(response.ToolCalls) != 2 || response.StopReason != "completed" {
		t.Fatalf("unexpected generation: %+v", response)
	}
	if response.ToolCalls[0].ToolID != "call_1" || response.ToolCalls[1].ToolID != "call_2" ||
		string(response.ToolCalls[0].ArgumentsJSON) != `{"chapter":9007199254740993}` || response.ToolCalls[0].ArgumentsText != string(response.ToolCalls[0].ArgumentsJSON) {
		t.Fatal("function call IDs, order or exact JSON arguments changed")
	}
	if response.State == nil || !response.State.Complete || response.State.TurnComplete || response.State.Origin.Model != req.Model ||
		response.State.Origin.ProviderName != "relay" || response.State.Origin.AdapterName != adapter.ProtocolOpenAIResponses {
		t.Fatalf("invalid tool round state: %+v", response.State)
	}
	wantUsage := map[string]any{
		"prompt_tokens":               float64(20),
		"completion_tokens":           float64(13),
		"total_tokens":                float64(33),
		"prompt_cache_hit_tokens":     float64(7),
		"prompt_cache_miss_tokens":    float64(13),
		"cache_creation_input_tokens": float64(3),
		"completion_tokens_details":   map[string]any{"reasoning_tokens": float64(5)},
	}
	if !reflect.DeepEqual(response.Usage, wantUsage) {
		t.Fatalf("usage double-counted cache or reasoning: %v", response.Usage)
	}
	after, err := json.Marshal(req)
	if err != nil || string(before) != string(after) {
		t.Fatal("Generate mutated the caller's request")
	}
	message := adapter.Message{Role: "assistant", Content: response.Content, ThinkingContent: response.ThinkingContent, State: response.State}
	for _, call := range response.ToolCalls {
		message.ToolCalls = append(message.ToolCalls, adapter.ToolCall{ID: call.ToolID, Name: call.ToolName, Arguments: call.ArgumentsJSON})
	}
	stored, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var restored adapter.Message
	if err := json.Unmarshal(stored, &restored); err != nil {
		t.Fatal(err)
	}
	req.Model = "gpt-5.6-terra"
	req.Messages = append(req.Messages, restored,
		adapter.Message{Role: "tool", ToolCallID: "call_2", ToolName: "read", Content: ""},
		adapter.Message{Role: "tool", ToolCallID: "call_1", ToolName: "read", Content: `{"result":42}`},
	)
	final, err := backend.Generate(context.Background(), req)
	if err != nil || final.Content != "done" || final.State == nil || !final.State.TurnComplete || len(requests) != 2 {
		t.Fatalf("stateless tool continuation failed: %+v, %v", final, err)
	}
	var original struct{ Output []json.RawMessage }
	var replayed []json.RawMessage
	if err := json.Unmarshal([]byte(generatedToolResponse), &original); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(requests[1]["input"], &replayed); err != nil || len(replayed) != len(original.Output)+3 {
		t.Fatalf("invalid replayed input: %s", requests[1]["input"])
	}
	for i, item := range original.Output {
		if !sameJSON(item, replayed[i+1]) {
			t.Fatalf("native output item %d lost ciphertext, order, precise numbers or extensions: %s", i, replayed[i+1])
		}
	}
	for i, want := range []struct{ callID, output string }{{"call_1", `{"result":42}`}, {"call_2", ""}} {
		var result struct {
			Type   string `json:"type"`
			CallID string `json:"call_id"`
			Output string `json:"output"`
		}
		if err := json.Unmarshal(replayed[len(original.Output)+1+i], &result); err != nil ||
			result.Type != "function_call_output" || result.CallID != want.callID || result.Output != want.output {
			t.Fatalf("function result correlation changed: %+v", result)
		}
	}
}

func TestGenerateMapsHTTPFailuresWithoutRetry(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 408, 409, 422, 429, 500, 503} {
		for _, html := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/html=%v", status, html), func(t *testing.T) {
				attempts := 0
				backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
					attempts++
					result := jsonResponse(`{"error":{"code":"test_error","type":"api_error","message":"test failure"}}`)
					result.StatusCode = status
					result.Header.Set("Retry-After", "12")
					if html {
						result.Header.Set("Content-Type", "text/html")
						result.Body = io.NopCloser(strings.NewReader("<html>relay error</html>"))
					}
					return result, nil
				}))
				response, err := backend.Generate(context.Background(), requestInput())
				var fault *adapter.APIError
				if !errors.As(err, &fault) || fault.StatusCode != status || fault.RetryAfter != 12*time.Second ||
					fault.Retryable != adapter.StatusRetryable(status) {
					t.Fatalf("HTTP status or retry metadata lost: %v", err)
				}
				if !html && (fault.Kind != "test_error" || fault.Message != "test failure") {
					t.Fatalf("provider error details lost: %+v", fault)
				}
				if html && fault.Message != http.StatusText(status) {
					t.Fatalf("invalid relay error body was exposed: %q", fault.Message)
				}
				if attempts != 1 || !reflect.DeepEqual(response, adapter.Response{}) {
					t.Fatal("HTTP failure retried or exposed partial results")
				}
			})
		}
	}
}

func TestGeneratePreservesCancellationAndTransportErrors(t *testing.T) {
	for _, kind := range []string{"cancel", "deadline", "transport", "read"} {
		t.Run(kind, func(t *testing.T) {
			started := make(chan struct{})
			sentinel := errors.New("test I/O failure")
			var attempts atomic.Int32
			var closed atomic.Bool
			backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts.Add(1)
				close(started)
				if kind == "transport" {
					return nil, sentinel
				}
				if kind == "read" {
					result := jsonResponse("")
					result.Body = &trackedBody{Reader: io.MultiReader(strings.NewReader(generatedTextResponse), errorReader{sentinel}), closed: &closed}
					return result, nil
				}
				<-r.Context().Done()
				return nil, r.Context().Err()
			}))
			timeout := 5 * time.Second
			if kind == "deadline" {
				timeout = 200 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := backend.Generate(ctx, requestInput())
				done <- err
			}()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("Generate did not start")
			}
			want := sentinel
			switch kind {
			case "cancel":
				cancel()
				want = context.Canceled
			case "deadline":
				want = context.DeadlineExceeded
			}
			select {
			case err := <-done:
				if !errors.Is(err, want) || attempts.Load() != 1 {
					t.Fatalf("lost error chain or retried: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Generate remained blocked")
			}
			if kind == "read" && !closed.Load() {
				t.Fatal("SDK did not close the failed response body")
			}
		})
	}
}

func TestGenerateRejectsSSEBeforeReadingBody(t *testing.T) {
	var closed atomic.Bool
	backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		result := jsonResponse("")
		result.Header.Set("Content-Type", "text/event-stream; charset=utf-8")
		result.Body = &trackedBody{Reader: errorReader{errors.New("must not read an unbounded SSE body")}, closed: &closed}
		return result, nil
	}))
	response, err := backend.Generate(context.Background(), requestInput())
	var fault *adapter.APIError
	if !errors.As(err, &fault) || fault.Kind != "invalid_response" || !closed.Load() || !reflect.DeepEqual(response, adapter.Response{}) {
		t.Fatalf("unexpected SSE was not rejected and closed: %v", err)
	}
}

func TestGenerateRejectsMalformedResponses(t *testing.T) {
	for _, body := range []string{"<html>relay homepage</html>", `{"output":`, `null`, `{}`, `[]`} {
		t.Run(body, func(t *testing.T) {
			backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return jsonResponse(body), nil
			}))
			response, err := backend.Generate(context.Background(), requestInput())
			if err == nil || !reflect.DeepEqual(response, adapter.Response{}) {
				t.Fatalf("malformed response accepted: %s", body)
			}
		})
	}
}

func TestGenerateRejectsInputsBeforeSending(t *testing.T) {
	attempts := 0
	backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		return jsonResponse(generatedTextResponse), nil
	}))
	for _, kind := range []string{"provider", "model", "schema"} {
		req := requestInput()
		switch kind {
		case "provider":
			req.ProviderName = "missing"
		case "model":
			req.Model = ""
		case "schema":
			req.Tools = []adapter.ToolDefinition{{Name: "read", InputSchema: json.RawMessage(`[]`)}}
		}
		if _, err := backend.Generate(context.Background(), req); err == nil {
			t.Fatalf("invalid %s accepted", kind)
		}
	}
	if attempts != 0 {
		t.Fatal("invalid request reached the server")
	}
}

type trackedBody struct {
	io.Reader
	closed *atomic.Bool
}

func (b *trackedBody) Close() error {
	b.closed.Store(true)
	return nil
}
