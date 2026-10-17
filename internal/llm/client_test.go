package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/llm/adapter/openaichat"
)

type stubAdapter struct {
	stream   func(context.Context, adapter.Request) <-chan StreamEvent
	generate func(context.Context, adapter.Request) (adapter.Response, error)
}

func TestWailsTypesPreservePackageIdentity(t *testing.T) {
	for name, value := range map[string]any{"ModelInfo": ModelInfo{}, "TestConnectionResult": TestConnectionResult{}} {
		typ := reflect.TypeOf(value)
		if typ.Name() != name || typ.PkgPath() != "github.com/sigpanic/goink/internal/llm" {
			t.Fatalf("Wails type namespace changed: %s.%s", typ.PkgPath(), typ.Name())
		}
	}
}

func (s stubAdapter) Stream(ctx context.Context, req adapter.Request) <-chan StreamEvent {
	return s.stream(ctx, req)
}

func (s stubAdapter) Generate(ctx context.Context, req adapter.Request) (adapter.Response, error) {
	return s.generate(ctx, req)
}

func TestClientDelegatesUnifiedRequests(t *testing.T) {
	messages := []map[string]any{{"role": "user", "content": "hello"}}
	tools := []map[string]any{{"type": "function", "function": map[string]any{"name": "lookup"}}}
	opts := &CallOptions{SessionID: "session-test"}
	backend := stubAdapter{
		stream: func(_ context.Context, req adapter.Request) <-chan StreamEvent {
			if req.Model != "model" || !reflect.DeepEqual(req.Messages, messages) || !reflect.DeepEqual(req.Tools, tools) || req.Options != opts {
				t.Errorf("unexpected stream request: %+v", req)
			}
			ch := make(chan StreamEvent, 1)
			ch <- StreamEvent{Type: EventContent, Data: "stream result"}
			close(ch)
			return ch
		},
		generate: func(_ context.Context, req adapter.Request) (adapter.Response, error) {
			if req.Model != "model" || !reflect.DeepEqual(req.Messages, messages) {
				t.Errorf("unexpected generate request: %+v", req)
			}
			return adapter.Response{Content: "title", ThinkingContent: "private reasoning"}, nil
		},
	}
	client := NewClient(nil, nil)
	client.adapters["test"] = backend
	var text string
	for event := range client.ChatStream(context.Background(), "test", messages, tools, "model", opts) {
		text += event.Data
	}
	if text != "stream result" {
		t.Fatalf("stream text = %q", text)
	}
	text, err := client.GenerateText(context.Background(), "test", messages, "model")
	if err != nil || text != "title" {
		t.Fatalf("GenerateText = %q, %v", text, err)
	}
	if _, err := client.GenerateText(context.Background(), "missing", nil, "model"); err == nil {
		t.Fatal("missing provider should fail")
	}
	var events []StreamEvent
	for event := range client.ChatStream(context.Background(), "missing", nil, nil, "model", nil) {
		events = append(events, event)
	}
	if len(events) != 1 || events[0].Type != EventError || events[0].Error == nil {
		t.Fatalf("missing provider events = %+v", events)
	}
	backend.generate = func(context.Context, adapter.Request) (adapter.Response, error) {
		return adapter.Response{}, &APIError{StatusCode: 429, Retryable: true}
	}
	client.adapters["test"] = backend
	_, err = client.GenerateText(context.Background(), "test", messages, "model")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 {
		t.Fatalf("GenerateText error = %v", err)
	}
}

func TestClientSnapshotAndReloadDuringStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	release := make(chan struct{})
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		if r.Header.Get("Authorization") != "Bearer old-key" || payload["temperature"] != 0.4 || payload["max_tokens"] != float64(123) || payload["reasoning_effort"] != "high" {
			t.Errorf("old snapshot changed: headers=%v, payload=%v", r.Header, payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"old\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
			fmt.Fprint(w, "data: [DONE]\n\n")
		case <-r.Context().Done():
		}
	}))
	defer oldServer.Close()
	defer cancel()
	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer new-key" {
			t.Errorf("new key missing: %v", r.Header)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"new"}}]}`)
	}))
	defer newServer.Close()
	temperature := 0.4
	providers := map[string]Provider{"test": {
		ChatURL: oldServer.URL, APIKey: "old-key", Temperature: &temperature,
		Models: []ModelInfo{{ID: "model", MaxOutputTokens: 123, SupportsThinking: true, ReasoningLevels: []string{"high"}}},
	}}
	client := NewClient(providers, nil)
	temperature = 1.9
	providers["test"].Models[0].MaxOutputTokens = 999
	providers["test"].Models[0].ReasoningLevels[0] = "mutated"
	providers["test"] = Provider{ChatURL: "invalid", APIKey: "mutated"}
	stream := client.ChatStream(ctx, "test", nil, nil, "model", nil)
	select {
	case event := <-stream:
		if event.Type != EventContent || event.Data != "old" {
			t.Fatalf("first event = %+v", event)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	client.Reload(map[string]Provider{"test": {ChatURL: newServer.URL, APIKey: "new-key"}})
	text, err := client.GenerateText(ctx, "test", nil, "model")
	if err != nil || text != "new" {
		t.Fatalf("reloaded generation = %q, %v", text, err)
	}
	close(release)
	for event := range stream {
		if event.Type == EventError {
			t.Fatal(event.Error)
		}
	}
}

func TestCallToolCancelsProducerAfterResult(t *testing.T) {
	done := make(chan struct{})
	backend := stubAdapter{stream: func(ctx context.Context, _ adapter.Request) <-chan StreamEvent {
		ch := make(chan StreamEvent)
		go func() {
			defer close(done)
			defer close(ch)
			select {
			case ch <- StreamEvent{Type: EventToolCallEnd, Delta: &ToolCallDelta{ToolName: "lookup", ArgumentsJSON: json.RawMessage(`{"ok":true}`)}}:
			case <-ctx.Done():
				return
			}
			<-ctx.Done()
		}()
		return ch
	}}
	client := NewClient(nil, nil)
	client.adapters["test"] = backend
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := client.CallTool(ctx, "test", "model", nil, nil, "lookup", nil)
	if err != nil || string(result) != `{"ok":true}` {
		t.Fatalf("CallTool = %s, %v", result, err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("CallTool left its producer running")
	}
}

func TestDiscoverModelsUsesProviderAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/v1/models" || r.Header.Get("api-key") != "secret" || r.Header.Get("Authorization") != "" {
			t.Errorf("discovery request: path=%s, headers=%v", r.URL.Path, r.Header)
		}
		fmt.Fprint(w, `{"data":[{"id":"mimo-model","context_length":1000,"supports_reasoning":true}]}`)
	}))
	defer server.Close()
	builtin := map[string]Provider{"test": {BuildHeaders: openaichat.PresetHooks("mimo").BuildHeaders}}
	models, err := DiscoverModels(context.Background(), builtin, "test", server.URL+"/proxy/v1/chat/completions", "secret")
	if err != nil || len(models) != 1 || models[0].ContextWindow != 1000 || !models[0].SupportsThinking {
		t.Fatalf("discovered models = %+v, %v", models, err)
	}
}
