package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

type stubAdapter struct {
	stream   func(context.Context, adapter.Request) <-chan StreamEvent
	generate func(context.Context, adapter.Request) (adapter.Response, error)
}

func TestWailsTypesUseAdapterPackageIdentity(t *testing.T) {
	for name, value := range map[string]any{"ModelInfo": ModelInfo{}, "TestConnectionResult": TestConnectionResult{}} {
		typ := reflect.TypeOf(value)
		if typ.Name() != name || typ.PkgPath() != "github.com/sigpanic/goink/internal/llm/adapter" {
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

func (s stubAdapter) Providers() adapter.ProviderMap {
	return nil
}

func (s stubAdapter) DiscoverModels(context.Context, string) ([]adapter.ModelInfo, error) {
	return nil, errors.New("unexpected model discovery")
}

func (s stubAdapter) TestConnection(context.Context, string, string) (adapter.TestConnectionResult, error) {
	return adapter.TestConnectionResult{}, errors.New("unexpected connection test")
}

func TestClientDelegatesUnifiedRequests(t *testing.T) {
	state := &ResponseState{
		Origin:   MessageOrigin{AdapterName: adapter.ProtocolAnthropicMessages, ProviderName: "source", Model: "source-model"},
		Complete: true, Payload: json.RawMessage(`{"content":[{"type":"thinking","signature":"private"}]}`),
	}
	messages := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call-1", Name: "lookup", Arguments: json.RawMessage(`{"id":1}`)}}, State: state},
		{Role: "tool", ToolCallID: "call-1", ToolName: "lookup", Content: `{"ok":true}`},
	}
	tools := []ToolDefinition{{Name: "lookup", Description: "Look up an item", InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer"}}}`)}}
	effort := "high"
	opts := &CallOptions{SessionID: "session-test", ReasoningEffort: &effort}
	backend := stubAdapter{
		stream: func(_ context.Context, req adapter.Request) <-chan StreamEvent {
			if req.ProviderName != "my relay" || req.Model != "model" || !reflect.DeepEqual(req.Messages, messages) || !reflect.DeepEqual(req.Tools, tools) || req.Options != opts {
				t.Errorf("unexpected stream request: %+v", req)
			}
			ch := make(chan StreamEvent, 2)
			ch <- StreamEvent{Type: EventContent, Data: "stream result"}
			ch <- StreamEvent{Type: EventResponseState, State: state}
			close(ch)
			return ch
		},
		generate: func(_ context.Context, req adapter.Request) (adapter.Response, error) {
			if req.ProviderName != "my relay" || req.Model != "model" || !reflect.DeepEqual(req.Messages, messages) || req.Options != opts {
				t.Errorf("unexpected generate request: %+v", req)
			}
			return adapter.Response{Content: "title", ThinkingContent: "private reasoning"}, nil
		},
	}
	client, err := NewClient(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	client.adapters[adapter.ProtocolOpenAIChat] = backend
	var text string
	var receivedState *ResponseState
	for event := range client.ChatStream(context.Background(), adapter.ProtocolOpenAIChat, "my relay", "model", messages, tools, opts) {
		text += event.Data
		if event.Type == EventResponseState {
			receivedState = event.State
		}
	}
	if text != "stream result" {
		t.Fatalf("stream text = %q", text)
	}
	if receivedState != state {
		t.Fatal("root streaming did not transparently forward response state")
	}
	text, err = client.GenerateText(context.Background(), adapter.ProtocolOpenAIChat, "my relay", "model", messages, opts)
	if err != nil || text != "title" {
		t.Fatalf("GenerateText = %q, %v", text, err)
	}
	if _, err := client.GenerateText(context.Background(), "missing", "my relay", "model", nil, nil); err == nil {
		t.Fatal("unknown protocol should fail without falling back to OpenAI Chat")
	}
	var events []StreamEvent
	for event := range client.ChatStream(context.Background(), "missing", "my relay", "model", nil, nil, nil) {
		events = append(events, event)
	}
	if len(events) != 1 || events[0].Type != EventError || events[0].Error == nil {
		t.Fatalf("unknown protocol events = %+v", events)
	}
	backend.generate = func(context.Context, adapter.Request) (adapter.Response, error) {
		return adapter.Response{}, &APIError{StatusCode: 429, Retryable: true}
	}
	client.adapters[adapter.ProtocolOpenAIChat] = backend
	_, err = client.GenerateText(context.Background(), adapter.ProtocolOpenAIChat, "my relay", "model", messages, nil)
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
	config := testUserConfig(adapter.Provider{
		Name: "test", ProviderType: "openai", ChatURL: oldServer.URL, APIKey: "old-key", Temperature: &temperature,
		Models: []adapter.ModelInfo{{ID: "model", MaxOutputTokens: 123, SupportsThinking: true, ReasoningLevels: []string{"high"}}},
	})
	client, err := NewClient(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	temperature = 1.9
	config.Adapters[adapter.ProtocolOpenAIChat].Providers[0].Models[0].MaxOutputTokens = 999
	config.Adapters[adapter.ProtocolOpenAIChat].Providers[0].Models[0].ReasoningLevels[0] = "mutated"
	config.Adapters[adapter.ProtocolOpenAIChat].Providers[0] = adapter.Provider{ChatURL: "invalid", APIKey: "mutated"}
	snapshot := client.Providers()[adapter.ProtocolOpenAIChat]["test"]
	*snapshot.Temperature = 1.8
	snapshot.Models[0].ReasoningLevels[0] = "snapshot mutation"
	stream := client.ChatStream(ctx, adapter.ProtocolOpenAIChat, "test", "model", nil, nil, nil)
	select {
	case event := <-stream:
		if event.Type != EventContent || event.Data != "old" {
			t.Fatalf("first event = %+v", event)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := client.Reload(testUserConfig(adapter.Provider{Name: "test", ProviderType: "openai", ChatURL: newServer.URL, APIKey: "new-key"})); err != nil {
		t.Fatal(err)
	}
	text, err := client.GenerateText(ctx, adapter.ProtocolOpenAIChat, "test", "model", nil, nil)
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
	messages := []Message{{Role: "user", Content: "lookup"}}
	tools := []ToolDefinition{{Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)}}
	backend := stubAdapter{stream: func(ctx context.Context, req adapter.Request) <-chan StreamEvent {
		if req.ProviderName != "test" || req.Model != "model" || !reflect.DeepEqual(req.Messages, messages) || !reflect.DeepEqual(req.Tools, tools) {
			t.Errorf("unexpected CallTool request: %+v", req)
		}
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
	client, err := NewClient(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	client.adapters[adapter.ProtocolOpenAIChat] = backend
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := client.CallTool(ctx, adapter.ProtocolOpenAIChat, "test", "model", messages, tools, "lookup", nil)
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
	models, err := DiscoverModels(context.Background(), ProviderProbeInput{
		AdapterName: adapter.ProtocolOpenAIChat, ProviderName: "my relay", ProviderType: "xiaomi",
		ChatURL: server.URL + "/proxy/v1/chat/completions", APIKey: "secret",
	})
	if err != nil || len(models) != 1 || models[0].ContextWindow != 1000 || !models[0].SupportsThinking {
		t.Fatalf("discovered models = %+v, %v", models, err)
	}
}

func TestClientRejectedConfigPreservesFileAndRuntime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer old-key" {
			t.Errorf("runtime credentials changed after failure: %v", r.Header)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"old"}}]}`)
	}))
	defer server.Close()
	original := testUserConfig(adapter.Provider{Name: "test", ProviderType: "openai", ChatURL: server.URL, APIKey: "old-key"})
	client, err := NewClient(original, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "llm.json.enc")
	if err := client.SaveConfig(path, original); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	invalid := testUserConfig(adapter.Provider{Name: "test", ProviderType: "unknown", ChatURL: server.URL, APIKey: "new-key"})
	validReplacement := testUserConfig(adapter.Provider{Name: "test", ProviderType: "openai", ChatURL: server.URL, APIKey: "new-key"})
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"invalid reload", func() error { return client.Reload(invalid) }},
		{"invalid save", func() error { return client.SaveConfig(path, invalid) }},
		{"failed write", func() error { return client.SaveConfig(filepath.Join(path, "missing"), validReplacement) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil {
				t.Fatal("expected configuration update to fail")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(before) {
				t.Fatalf("original encrypted config changed: %v", err)
			}
			text, err := client.GenerateText(context.Background(), adapter.ProtocolOpenAIChat, "test", "model", nil, nil)
			if err != nil || text != "old" {
				t.Fatalf("previous runtime not preserved: %q, %v", text, err)
			}
		})
	}
}
