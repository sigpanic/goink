package openaichat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func writeChunk(w io.Writer, value any) {
	fmt.Fprint(w, "data: ")
	json.NewEncoder(w).Encode(value)
	fmt.Fprint(w, "\n")
}

func toolChunk(index int, id, name, arguments string) map[string]any {
	return map[string]any{"choices": []any{map[string]any{"delta": map[string]any{
		"tool_calls": []any{map[string]any{"index": index, "id": id, "function": map[string]any{"name": name, "arguments": arguments}}},
	}}}}
}

func TestStreamToolGroupValidationAndUsageOrder(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(fmt.Sprintf("invalid=%v", invalid), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				writeChunk(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"reasoning_content": "think", "content": "text"}}}, "usage": map[string]any{"total_tokens": 1}})
				writeChunk(w, toolChunk(0, "call-a", "first", `{"x":`))
				writeChunk(w, toolChunk(0, "", "", `1}`))
				args := `{"y":2}`
				if invalid {
					args = `{"y":`
				}
				writeChunk(w, toolChunk(1, "call-b", "second", args))
				writeChunk(w, toolChunk(2, "call-a", "first", `{"x":1}`))
				writeChunk(w, map[string]any{"choices": []any{}, "usage": map[string]any{"total_tokens": 99}})
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			backend := newTestAdapter(t, adapter.Provider{ChatURL: server.URL}, server.Client())
			var ends []ToolCallDelta
			usageCount, errorCount, thinkingCount := 0, 0, 0
			for event := range backend.Stream(context.Background(), adapter.Request{ProviderName: "test", Model: "model"}) {
				switch event.Type {
				case EventThinking:
					thinkingCount++
				case EventUsage:
					usageCount++
					if len(ends) > 0 || event.Usage["total_tokens"] != float64(99) {
						t.Errorf("unexpected usage or order: %+v", event)
					}
				case EventToolCallEnd:
					if usageCount != 1 {
						t.Error("tool became executable before final usage")
					}
					ends = append(ends, *event.Delta)
				case EventError:
					errorCount++
					var apiErr *adapter.APIError
					if !errors.As(event.Error, &apiErr) || apiErr.Kind != "tool_args_invalid" {
						t.Errorf("unexpected error: %v", event.Error)
					}
				}
			}
			if usageCount != 1 || thinkingCount != 1 {
				t.Fatalf("usage=%d, thinking=%d", usageCount, thinkingCount)
			}
			if invalid {
				if len(ends) != 0 || errorCount != 1 {
					t.Fatalf("invalid group: ends=%v, errors=%d", ends, errorCount)
				}
			} else if len(ends) != 2 || errorCount != 0 || ends[0].ToolID != "call-a" || ends[1].ToolID != "call-b" || string(ends[0].ArgumentsJSON) != `{"x":1}` {
				t.Fatalf("tool group: ends=%+v, errors=%d", ends, errorCount)
			}
		})
	}
}

func TestGenerateCompleteResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		if payload["stream"] != false || payload["stream_options"] != nil || payload["max_tokens"] != float64(256) || payload["tool_choice"] != "auto" || payload["tools"] == nil || r.Header.Get("x-session") != "session" {
			t.Errorf("non-stream request: %v, headers=%v", payload, r.Header)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"answer","reasoning_content":"thinking","tool_calls":[{"id":"call-1","function":{"name":"lookup","arguments":"{\"x\":1}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`)
	}))
	defer server.Close()
	backend := newTestAdapter(t, adapter.Provider{ChatURL: server.URL}, server.Client())
	p := backend.providers["test"]
	p.BuildHeaders = func(opts *CallOptions, headers map[string]string) map[string]string {
		headers["x-session"] = opts.SessionID
		return headers
	}
	backend.providers["test"] = p
	maxTokens := 256
	result, err := backend.Generate(context.Background(), adapter.Request{ProviderName: "test",
		Model: "model", Tools: []adapter.ToolDefinition{{Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		Options: &CallOptions{MaxTokens: &maxTokens, SessionID: "session", ToolChoice: "auto"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "answer" || result.ThinkingContent != "thinking" || result.StopReason != "tool_calls" || result.Usage["total_tokens"] != float64(12) || len(result.ToolCalls) != 1 || string(result.ToolCalls[0].ArgumentsJSON) != `{"x":1}` {
		t.Fatalf("incomplete result: %+v", result)
	}
}

func TestGenerateRejectsInvalidToolGroup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"partial","tool_calls":[{"id":"a","function":{"name":"first","arguments":"{}"}},{"id":"b","function":{"name":"second","arguments":"{"}}]}}]}`)
	}))
	defer server.Close()
	backend := newTestAdapter(t, adapter.Provider{ChatURL: server.URL}, server.Client())
	result, err := backend.Generate(context.Background(), adapter.Request{ProviderName: "test", Model: "model"})
	var apiErr *adapter.APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != "tool_args_invalid" || len(result.ToolCalls) != 0 || result.Content != "" {
		t.Fatalf("invalid group returned partial result: %+v, %v", result, err)
	}
}

func TestBothModesPreserveHTTPErrorClassification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"limited"}}`)
	}))
	defer server.Close()
	backend := newTestAdapter(t, adapter.Provider{ChatURL: server.URL}, server.Client())
	p := backend.providers["test"]
	p.ParseError = func([]byte) error { return errors.New("custom error") }
	backend.providers["test"] = p
	_, err := backend.Generate(context.Background(), adapter.Request{ProviderName: "test", Model: "model"})
	check := func(err error) {
		t.Helper()
		var apiErr *adapter.APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 || !apiErr.Retryable || apiErr.RetryAfter != 120*time.Second || apiErr.Message != "custom error" {
			t.Fatalf("error classification changed: %v", err)
		}
	}
	check(err)
	errorsSeen := 0
	for event := range backend.Stream(context.Background(), adapter.Request{ProviderName: "test", Model: "model"}) {
		if event.Type == EventError {
			errorsSeen++
			check(event.Error)
		}
	}
	if errorsSeen != 1 {
		t.Fatalf("stream errors = %d", errorsSeen)
	}
}

func TestProviderHooksAcrossBothModes(t *testing.T) {
	for _, provider := range []string{"qwen", "minimax", "moonshot", "mimo", "opencode"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", provider, stream), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var payload map[string]any
					json.NewDecoder(r.Body).Decode(&payload)
					switch provider {
					case "qwen":
						if payload["enable_thinking"] != stream || payload["thinking"] != nil || payload["reasoning_effort"] != nil {
							t.Errorf("qwen payload: %v", payload)
						}
					case "minimax":
						thinking, _ := payload["thinking"].(map[string]any)
						if payload["reasoning_split"] != true || thinking["type"] != "adaptive" {
							t.Errorf("minimax payload: %v", payload)
						}
					case "moonshot":
						if payload["temperature"] != nil || payload["thinking"] != nil || payload["max_tokens"] != nil || payload["max_completion_tokens"] != float64(128) || payload["reasoning_effort"] != "max" {
							t.Errorf("moonshot payload: %v", payload)
						}
					case "mimo":
						if r.Header.Get("api-key") != "secret" || r.Header.Get("Authorization") != "" {
							t.Errorf("mimo headers: %v", r.Header)
						}
					case "opencode":
						if r.Header.Get("x-opencode-session") != "session" || !strings.HasPrefix(r.Header.Get("User-Agent"), "Goink/") {
							t.Errorf("opencode headers: %v", r.Header)
						}
					}
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
					} else {
						fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
					}
				}))
				defer server.Close()
				backend := newTestAdapter(t, adapter.Provider{ChatURL: server.URL, APIKey: "secret", ProviderType: provider, Models: []ModelInfo{{ID: "kimi-k3", SupportsThinking: true, ReasoningLevels: []string{"max"}}}}, server.Client())
				maxTokens := 128
				req := adapter.Request{ProviderName: "test", Model: "kimi-k3", Options: &CallOptions{MaxTokens: &maxTokens, SessionID: "session"}}
				if stream {
					for event := range backend.Stream(context.Background(), req) {
						if event.Type == EventError {
							t.Fatal(event.Error)
						}
					}
				} else if _, err := backend.Generate(context.Background(), req); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// testHTTPClient 返回测试用 HTTP client：生产路径要求上层注入，测试统一显式提供。
func testHTTPClient() *http.Client { return &http.Client{} }

type trackedBody struct {
	io.Reader
	closed chan struct{}
}

func (b *trackedBody) Close() error {
	close(b.closed)
	return nil
}

func TestStreamCancellationReleasesBlockedProducer(t *testing.T) {
	body := &trackedBody{
		Reader: strings.NewReader(strings.Repeat("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n", 1000)),
		closed: make(chan struct{}),
	}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: body}, nil
	})}
	backend := newTestAdapter(t, adapter.Provider{ChatURL: "https://example.invalid"}, client)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := backend.Stream(ctx, adapter.Request{ProviderName: "test", Model: "model"})
	select {
	case event := <-stream:
		if event.Type != EventContent {
			t.Fatalf("first event = %+v", event)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not start")
	}
	cancel()
	select {
	case <-body.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("producer blocked after cancellation without a consumer")
	}
	for range stream {
	}
}

func TestGenerateWithoutOptionsPreservesMinimalPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if len(payload) != 3 || payload["model"] != "model" || payload["stream"] != false {
			t.Errorf("legacy minimal payload changed: %v", payload)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"title"}}]}`)
	}))
	defer server.Close()
	backend := newTestAdapter(t, adapter.Provider{ChatURL: server.URL, Models: []ModelInfo{{ID: "model", SupportsThinking: true, MaxOutputTokens: 1000}}}, server.Client())
	response, err := backend.Generate(context.Background(), adapter.Request{ProviderName: "test", Model: "model"})
	if err != nil || response.Content != "title" {
		t.Fatalf("minimal generation = %+v, %v", response, err)
	}
}

func TestBothModesCancelInFlightHTTP(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			started := make(chan struct{})
			stopped := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
				close(stopped)
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			backend := newTestAdapter(t, adapter.Provider{ChatURL: server.URL}, server.Client())
			done := make(chan error, 1)
			go func() {
				if stream {
					for range backend.Stream(ctx, adapter.Request{ProviderName: "test", Model: "model"}) {
					}
					done <- ctx.Err()
				} else {
					_, err := backend.Generate(ctx, adapter.Request{ProviderName: "test", Model: "model"})
					done <- err
				}
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("HTTP request did not start")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel returned %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("canceled call did not exit")
			}
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("underlying HTTP request was not canceled")
			}
		})
	}
}

func newTestAdapter(t *testing.T, config adapter.Provider, client *http.Client) *Adapter {
	t.Helper()
	if config.Name == "" {
		config.Name = "test"
	}
	if config.ProviderType == "" {
		config.ProviderType = "openai"
	}
	backend, err := New(map[string]adapter.Provider{"test": config}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	return backend
}
