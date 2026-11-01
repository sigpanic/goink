package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

const generatedToolInteraction = `{
	"id":"interaction_tools","object":"interaction","model":"upstream-snapshot","status":"requires_action",
	"created":"2026-10-23T00:00:00Z","updated":"2026-10-23T00:00:01Z",
	"steps":[
		{"type":"thought","signature":"opaque-signature","summary":[{"type":"text","text":"摘要"},{"type":"image","uri":"data:image/png;base64,AA=="}],"extension":{"n":9007199254740995}},
		{"type":"model_output","content":[{"type":"text","text":"先","extension":{"keep":true}}]},
		{"type":"function_call","id":"a","name":"read","arguments":{"chapter":9007199254740993},"extension":{"keep":true}},
		{"type":"thought","signature":"empty-summary-signature","summary":[]},
		{"type":"model_output","content":[{"type":"text","text":"后"}]},
		{"type":"function_call","id":"b","name":"read","arguments":{}}
	],
	"usage":{"total_input_tokens":7,"total_output_tokens":20,"total_thought_tokens":22,"total_cached_tokens":3,"total_tokens":49,"total_tool_use_tokens":50}
}`

func generateAdapter(t *testing.T, handler http.HandlerFunc) *Adapter {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	p := requestProvider()
	p.ChatURL = server.URL + "/relay/v8/interactions?route=gemini"
	backend, err := New(adapter.ProviderMap{p.Name: p.Provider}, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return backend
}

func TestGenerateNativeStateSurvivesStorageAndToolContinuation(t *testing.T) {
	var requests atomic.Int32
	var replayed []json.RawMessage
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		count := requests.Add(1)
		if r.Method != http.MethodPost || r.RequestURI != "/relay/v8/interactions?route=gemini" || r.Header.Get("X-Goog-Api-Key") != "private-key" || r.Header.Get("Accept") != "application/json" {
			t.Error("generate lost the complete endpoint, credentials or JSON request mode")
		}
		var request struct {
			Input                     []json.RawMessage `json:"input"`
			Stream, Store, Background *bool
			PreviousInteractionID     *string `json:"previous_interaction_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Stream == nil || *request.Stream || request.Store == nil || *request.Store || request.Background == nil || *request.Background || request.PreviousInteractionID != nil {
			t.Errorf("invalid stateless Generate request: %+v, %v", request, err)
		}
		w.Header().Set("Content-Type", "application/json")
		if count == 1 {
			fmt.Fprint(w, generatedToolInteraction)
			return
		}
		replayed = request.Input
		fmt.Fprint(w, sdkTextResponse)
	})
	req := requestFixture()
	req.Tools = requestTools()
	before, _ := json.Marshal(req)
	response, err := backend.Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 || response.Content != "先后" || response.ThinkingContent != "摘要" || len(response.ToolCalls) != 2 {
		t.Fatalf("unexpected result or automatic tool loop: %+v", response)
	}
	if response.ToolCalls[0].ToolID != "a" || string(response.ToolCalls[0].ArgumentsJSON) != `{"chapter":9007199254740993}` || response.ToolCalls[1].ToolID != "b" {
		t.Fatal("tool order, ID or argument precision changed")
	}
	if response.State == nil || !response.State.Complete || response.State.TurnComplete || response.State.Origin.Model != req.Model || response.State.Origin.ProviderName != req.ProviderName || response.StopReason != "requires_action" {
		t.Fatalf("incorrect tool-waiting state: %+v", response.State)
	}
	wantUsage := map[string]any{
		"prompt_tokens": float64(7), "completion_tokens": float64(42), "total_tokens": float64(49),
		"prompt_cache_hit_tokens": float64(3), "prompt_cache_miss_tokens": float64(4),
		"completion_tokens_details": map[string]any{"reasoning_tokens": float64(22)}, "total_tool_use_tokens": float64(50),
	}
	if !reflect.DeepEqual(response.Usage, wantUsage) {
		t.Fatalf("usage counters were omitted or double counted: %v", response.Usage)
	}
	after, _ := json.Marshal(req)
	if string(before) != string(after) || strings.Contains(string(response.State.Payload), "private-key") {
		t.Fatal("Generate mutated caller inputs or stored provider credentials")
	}
	message := adapter.Message{Role: "assistant", Content: response.Content, ThinkingContent: response.ThinkingContent, State: response.State}
	for _, call := range response.ToolCalls {
		message.ToolCalls = append(message.ToolCalls, adapter.ToolCall{ID: call.ToolID, Name: call.ToolName, Arguments: call.ArgumentsJSON})
	}
	stored, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var loaded adapter.Message
	if err := json.Unmarshal(stored, &loaded); err != nil {
		t.Fatal(err)
	}
	req.Messages = append(req.Messages, loaded, adapter.Message{Role: "tool", ToolCallID: "b", Content: "第二章"}, adapter.Message{Role: "tool", ToolCallID: "a", Content: "第一章"})
	req.Model = "gemini-3.6-flash"
	final, err := backend.Generate(context.Background(), req)
	if err != nil || final.Content != "ok" || final.State == nil || !final.State.TurnComplete || requests.Load() != 2 {
		t.Fatalf("stateless continuation failed: %+v, %v", final, err)
	}
	var original struct{ Steps []json.RawMessage }
	if err := json.Unmarshal([]byte(generatedToolInteraction), &original); err != nil {
		t.Fatal(err)
	}
	if len(replayed) != len(original.Steps)+3 {
		t.Fatalf("tool loop has incorrect history length: %d", len(replayed))
	}
	for i, step := range original.Steps {
		if !sameJSON(step, replayed[i+1]) {
			t.Fatalf("native step %d changed through SDK, storage or model switch: %s", i, replayed[i+1])
		}
	}
}

func TestGenerateMapsHTTPFailuresWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		name, body, contentType, kind, message string
		status                                 int
	}{
		{name: "signature", status: 400, body: `{"error":{"code":"signature_invalid","message":"bad signature"}}`, contentType: "application/json", kind: "signature_invalid", message: "bad signature"},
		{name: "auth", status: 401, body: `{"error":{"code":"unauthenticated","message":"bad key"}}`, contentType: "application/json", kind: "unauthenticated", message: "bad key"},
		{name: "rate limit", status: 429, body: `{"error":{"code":"rate_limit","message":"slow down"}}`, contentType: "application/json", kind: "rate_limit", message: "slow down"},
		{name: "server", status: 503, body: `{"error":{"code":"unavailable","message":"try later"}}`, contentType: "application/json", kind: "unavailable", message: "try later"},
		{name: "numeric Google code", status: 429, body: `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"quota"}}`, contentType: "application/json", kind: "RESOURCE_EXHAUSTED", message: "quota"},
		{name: "proxy HTML", status: 502, body: "<html>proxy failure</html>", contentType: "text/html", message: "Bad Gateway"},
		{name: "broken error JSON", status: 500, body: `{`, contentType: "application/json", message: "Internal Server Error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			backend := generateAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", tc.contentType)
				w.Header().Set("Retry-After", "12")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			response, err := backend.Generate(context.Background(), requestFixture())
			var apiErr *adapter.APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tc.status || apiErr.Kind != tc.kind || apiErr.Message != tc.message || apiErr.RetryAfter != 12*time.Second || apiErr.Retryable != adapter.StatusRetryable(tc.status) {
				t.Fatalf("HTTP failure lost metadata: %+v", err)
			}
			if requests.Load() != 1 || response.State != nil || len(response.ToolCalls) > 0 {
				t.Fatal("HTTP failure was retried or delivered tool calls/state")
			}
		})
	}
}

func TestGenerateValidatesBeforeSending(t *testing.T) {
	var requests atomic.Int32
	backend := generateAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, sdkTextResponse)
	})
	for _, edit := range []func(*adapter.Request){
		func(req *adapter.Request) { req.ProviderName = "unknown" },
		func(req *adapter.Request) { req.Model = "" },
		func(req *adapter.Request) { req.Options = &adapter.CallOptions{Temperature: ptr(0.7)} },
		func(req *adapter.Request) {
			req.Tools = []adapter.ToolDefinition{{Name: "read", InputSchema: json.RawMessage(`[]`)}}
		},
	} {
		req := requestFixture()
		edit(&req)
		if _, err := backend.Generate(context.Background(), req); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	if requests.Load() != 0 {
		t.Fatal("invalid input caused an HTTP request")
	}
}

func TestGenerateRejectsMalformedSuccessResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body, contentType string
	}{
		{name: "HTML success", body: "<html>proxy page</html>", contentType: "text/html"},
		{name: "malformed JSON", body: `{`, contentType: "application/json"},
		{name: "null interaction", body: `null`, contentType: "application/json"},
		{name: "missing native steps", body: `{"id":"test","status":"completed","output_text":"text only"}`, contentType: "application/json"},
		{name: "bad second function", body: strings.Replace(generatedToolInteraction, `"id":"b","name":"read","arguments":{}`, `"id":"b","name":"read","arguments":[]`, 1), contentType: "application/json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			backend := generateAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", tc.contentType)
				fmt.Fprint(w, tc.body)
			})
			response, err := backend.Generate(context.Background(), requestFixture())
			if err == nil || response.State != nil || len(response.ToolCalls) > 0 || requests.Load() != 1 {
				t.Fatalf("malformed success response delivered tools/state or retried: response=%+v err=%v", response, err)
			}
			if tc.contentType == "text/html" && err.Error() == "[200] OK" {
				t.Fatal("protocol failure was reported as OK")
			}
		})
	}
}

type countedResponseBody struct {
	io.Reader
	closed *atomic.Int32
}

func (b countedResponseBody) Close() error { b.closed.Add(1); return nil }

func TestGenerateClosesUnexpectedSSEAndBrokenBodies(t *testing.T) {
	readErr := errors.New("response reader failed")
	for _, tc := range []struct {
		name, contentType string
		reader            io.Reader
		cause             error
	}{
		{name: "unexpected SSE", contentType: "text/event-stream", reader: strings.NewReader(sdkStreamFixture())},
		{name: "failed body", contentType: "application/json", reader: iotest.ErrReader(readErr), cause: readErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var closed atomic.Int32
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {tc.contentType}}, Body: countedResponseBody{Reader: tc.reader, closed: &closed}}, nil
			})}
			p := requestProvider()
			backend, err := New(adapter.ProviderMap{p.Name: p.Provider}, client, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := backend.Generate(context.Background(), requestFixture())
			if err == nil || (tc.cause != nil && !errors.Is(err, tc.cause)) || response.State != nil || len(response.ToolCalls) > 0 || closed.Load() != 1 {
				t.Fatalf("body error lost its chain or resource cleanup: err=%v closed=%d response=%+v", err, closed.Load(), response)
			}
		})
	}
}

func TestGeneratePreservesCancellationAndTransportErrors(t *testing.T) {
	for _, bodyStarted := range []bool{false, true} {
		t.Run(fmt.Sprint(bodyStarted), func(t *testing.T) {
			started, released := make(chan struct{}), make(chan struct{})
			backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if bodyStarted {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"id":`)
					w.(http.Flusher).Flush()
				}
				close(started)
				select {
				case <-r.Context().Done():
				case <-time.After(5 * time.Second):
				}
				close(released)
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := backend.Generate(ctx, requestFixture()); done <- err }()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("Generate did not reach the shared HTTP transport")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("context cancellation lost: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Generate ignored cancellation")
			}
			select {
			case <-released:
			case <-time.After(3 * time.Second):
				t.Fatal("Generate did not release the connection")
			}
		})
	}
	transportErr := errors.New("transport failed")
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, transportErr })}
	p := requestProvider()
	backend, err := New(adapter.ProviderMap{p.Name: p.Provider}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Generate(context.Background(), requestFixture()); !errors.Is(err, transportErr) {
		t.Fatalf("transport error chain lost: %v", err)
	}
}
