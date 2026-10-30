package anthropic

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
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

const generatedTextResponse = `{"id":"msg_text","type":"message","role":"assistant","model":"actual-model-snapshot","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`

const generatedToolResponse = `{
	"id":"msg_tools","type":"message","role":"assistant","model":"actual-model-snapshot",
	"content":[
		{"type":"thinking","thinking":"summary","signature":"test-signature"},
		{"type":"thinking","thinking":"","signature":"empty-thinking-signature"},
		{"type":"redacted_thinking","data":"opaque-data"},
		{"type":"text","text":"reading","extension":{"n":9007199254740995}},
		{"type":"tool_use","id":"call1","name":"read","input":{"chapter":9007199254740993}},
		{"type":"text","text":"and"},
		{"type":"tool_use","id":"call2","name":"read","input":{}}
	],
	"stop_reason":"tool_use",
	"usage":{"input_tokens":5,"cache_creation_input_tokens":7,"cache_read_input_tokens":11,"output_tokens":13,"output_tokens_details":{"thinking_tokens":3},"cache_creation":{"ephemeral_5m_input_tokens":7,"ephemeral_1h_input_tokens":0}}
}`

func generateAdapter(t *testing.T, handler http.HandlerFunc) *Adapter {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	p, _ := testRequest()
	p.ChatURL = server.URL + "/relay/v8/messages?route=claude"
	p.Models = BuiltinProviders()["Claude"].Models
	backend, err := New(adapter.ProviderMap{p.Name: p.Provider}, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return backend
}

func TestGenerateProducesReusableNativeState(t *testing.T) {
	var requests atomic.Int32
	var replayedContent json.RawMessage
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		count := requests.Add(1)
		if r.Method != http.MethodPost || r.RequestURI != "/relay/v8/messages?route=claude" || r.Header.Get("X-Api-Key") != "key" {
			t.Error("generate did not use the complete configured endpoint and credentials")
		}
		var request struct {
			Stream   bool
			Messages []struct {
				Content json.RawMessage
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Stream {
			t.Error("generate must send a non-streaming Messages request")
		}
		w.Header().Set("Content-Type", "application/json")
		if count == 1 {
			fmt.Fprint(w, generatedToolResponse)
			return
		}
		if len(request.Messages) != 3 {
			t.Errorf("tool loop has %d messages, want 3", len(request.Messages))
		} else {
			replayedContent = request.Messages[1].Content
		}
		fmt.Fprint(w, generatedTextResponse)
	})
	_, req := testRequest()
	before, _ := json.Marshal(req)
	response, err := backend.Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 || response.Content != "readingand" || response.ThinkingContent != "summary" || len(response.ToolCalls) != 2 {
		t.Fatalf("unexpected generation result: %+v", response)
	}
	if response.ToolCalls[0].ToolID != "call1" || string(response.ToolCalls[0].ArgumentsJSON) != `{"chapter":9007199254740993}` || response.ToolCalls[1].ToolID != "call2" {
		t.Fatal("tool IDs, order or argument precision changed")
	}
	if response.State == nil || !response.State.Complete || response.State.TurnComplete || response.State.Origin.Model != req.Model || response.StopReason != "tool_use" {
		t.Fatalf("unexpected response state: %+v", response.State)
	}
	wantUsage := map[string]any{
		"prompt_tokens": float64(23), "completion_tokens": float64(13), "total_tokens": float64(36),
		"prompt_cache_hit_tokens": float64(11), "prompt_cache_miss_tokens": float64(12), "cache_creation_input_tokens": float64(7),
		"completion_tokens_details": map[string]any{"reasoning_tokens": float64(3)},
	}
	if !reflect.DeepEqual(response.Usage, wantUsage) {
		t.Fatalf("usage double-counted or omitted cache/thinking: %v", response.Usage)
	}
	after, _ := json.Marshal(req)
	if string(before) != string(after) {
		t.Fatal("Generate mutated caller inputs")
	}
	message := adapter.Message{Role: "assistant", Content: response.Content, ThinkingContent: response.ThinkingContent, State: response.State}
	for _, call := range response.ToolCalls {
		message.ToolCalls = append(message.ToolCalls, adapter.ToolCall{ID: call.ToolID, Name: call.ToolName, Arguments: call.ArgumentsJSON})
	}
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var restored adapter.Message
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	req.Messages = append(req.Messages, restored,
		adapter.Message{Role: "tool", ToolCallID: "call2", Content: "second"},
		adapter.Message{Role: "tool", ToolCallID: "call1", Content: "first"},
	)
	final, err := backend.Generate(context.Background(), req)
	if err != nil || final.Content != "done" || final.State == nil || !final.State.TurnComplete || requests.Load() != 2 {
		t.Fatalf("tool loop continuation failed: %+v, %v", final, err)
	}
	var original struct{ Content json.RawMessage }
	if err := json.Unmarshal([]byte(generatedToolResponse), &original); err != nil {
		t.Fatal(err)
	}
	if !sameJSONArray(replayedContent, original.Content) {
		t.Fatalf("native blocks changed after Generate and message storage: %s", replayedContent)
	}
}

func TestGenerateDoesNotLowerLargeOutputLimit(t *testing.T) {
	var requests atomic.Int32
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var request struct {
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.MaxTokens != 128000 {
			t.Error("generate silently changed the configured output limit")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, generatedTextResponse)
	})
	_, req := testRequest()
	req.Model = "claude-opus-5-5"
	response, err := backend.Generate(context.Background(), req)
	if err != nil || response.Content != "done" || requests.Load() != 1 {
		t.Fatalf("SDK rejected large non-streaming request before sending: %v", err)
	}
}

func TestGenerateMapsHTTPFailuresWithoutRetry(t *testing.T) {
	for _, status := range []int{400, 401, 404, 408, 429, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var requests atomic.Int32
			backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "12")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"type":"error","error":{"type":"test_protocol_error","message":"test failure"}}`)
			})
			_, req := testRequest()
			response, err := backend.Generate(context.Background(), req)
			var apiErr *adapter.APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != status || apiErr.Kind != "test_protocol_error" || apiErr.Message != "test failure" || apiErr.RetryAfter != 12*time.Second {
				t.Fatalf("HTTP failure details lost: %+v", err)
			}
			if apiErr.Retryable != adapter.StatusRetryable(status) || requests.Load() != 1 || response.State != nil || len(response.ToolCalls) != 0 {
				t.Fatal("HTTP error retried or exposed a response")
			}
		})
	}
}

func TestGenerateRejectsInputsBeforeSending(t *testing.T) {
	var requests atomic.Int32
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, generatedTextResponse)
	})
	for _, kind := range []string{"unknown-provider", "empty-model", "invalid-schema"} {
		_, req := testRequest()
		switch kind {
		case "unknown-provider":
			req.ProviderName = "missing"
		case "empty-model":
			req.Model = ""
		case "invalid-schema":
			req.Tools[0].InputSchema = json.RawMessage(`[]`)
		}
		if _, err := backend.Generate(context.Background(), req); err == nil {
			t.Fatalf("invalid request %s was accepted", kind)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("invalid request reached the server")
	}
}

func TestGeneratePreservesCancellationAndTransportErrors(t *testing.T) {
	for _, kind := range []string{"cancel", "deadline", "transport"} {
		t.Run(kind, func(t *testing.T) {
			started := make(chan struct{})
			sentinel := errors.New("transport failure")
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				close(started)
				if kind != "transport" {
					<-r.Context().Done()
					return nil, r.Context().Err()
				}
				return nil, sentinel
			})}
			p, req := testRequest()
			backend, err := New(adapter.ProviderMap{p.Name: p.Provider}, client, nil)
			if err != nil {
				t.Fatal(err)
			}
			timeout := 5 * time.Second
			if kind == "deadline" {
				timeout = 200 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := backend.Generate(ctx, req)
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
				if !errors.Is(err, want) {
					t.Fatalf("lost error chain: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Generate remained blocked after cancellation or transport error")
			}
		})
	}
}

func TestGenerateRejectsNonJSONAndMalformedResponses(t *testing.T) {
	for _, body := range []string{"<html>relay homepage</html>", `{"content":`, `null`, `{}`, `[]`, `{"type":"error","error":{"message":"failed"}}`} {
		backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(body, "<") {
				w.Header().Set("Content-Type", "text/html")
			} else {
				w.Header().Set("Content-Type", "application/json")
			}
			io.WriteString(w, body)
		})
		_, req := testRequest()
		if response, err := backend.Generate(context.Background(), req); err == nil || response.State != nil || len(response.ToolCalls) != 0 {
			t.Fatalf("invalid success response accepted: %s", body)
		}
	}
}
