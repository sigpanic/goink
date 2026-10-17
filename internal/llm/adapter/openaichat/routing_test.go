package openaichat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestMultipleProvidersRouteAllOperations(t *testing.T) {
	type target struct {
		name, providerType string
		maxTokens          int
	}
	targets := []target{{"relay-a", "mimo", 123}, {"relay-b", "openai", 456}}
	configs := make(map[string]adapter.Provider)
	counts := make(map[string]*atomic.Int32)
	for _, target := range targets {
		count := &atomic.Int32{}
		counts[target.name] = count
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count.Add(1)
			if target.providerType == "mimo" {
				if r.Header.Get("api-key") != target.name+"-key" || r.Header.Get("Authorization") != "" {
					t.Errorf("%s: wrong Mimo authentication: %v", target.name, r.Header)
				}
			} else if r.Header.Get("Authorization") != "Bearer "+target.name+"-key" || r.Header.Get("api-key") != "" {
				t.Errorf("%s: wrong bearer authentication: %v", target.name, r.Header)
			}
			if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
				fmt.Fprintf(w, `{"data":[{"id":%q}]}`, target.name+"-model")
				return
			}
			if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
				t.Errorf("unexpected target: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
				return
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
				return
			}
			if payload["model"] != target.name+"-model" || (payload["max_tokens"] != float64(1) && payload["max_tokens"] != float64(target.maxTokens)) {
				t.Errorf("%s: wrong model configuration: %v", target.name, payload)
			}
			if payload["stream"] == true {
				w.Header().Set("Content-Type", "text/event-stream")
				writeChunk(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": target.name}}}})
				fmt.Fprint(w, "data: [DONE]\n\n")
			} else {
				fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, target.name)
			}
		}))
		t.Cleanup(server.Close)
		configs[target.name] = adapter.Provider{
			ProviderType: target.providerType, ChatURL: server.URL + "/v1/chat/completions", APIKey: target.name + "-key",
			Models: []ModelInfo{{ID: target.name + "-model", MaxOutputTokens: target.maxTokens}},
		}
	}
	backend, err := New(configs, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, target := range targets {
		request := adapter.Request{ProviderName: target.name, Model: target.name + "-model", Options: &CallOptions{}}
		response, err := backend.Generate(ctx, request)
		if err != nil || response.Content != target.name {
			t.Fatalf("%s generate: %+v, %v", target.name, response, err)
		}
		var content string
		for event := range backend.Stream(ctx, request) {
			if event.Type == EventError {
				t.Fatal(event.Error)
			}
			if event.Type == EventContent {
				content += event.Data
			}
		}
		if content != target.name {
			t.Fatalf("%s stream: %q", target.name, content)
		}
		models, err := backend.DiscoverModels(ctx, target.name)
		if err != nil || len(models) != 1 || models[0].ID != request.Model {
			t.Fatalf("%s discovery: %+v, %v", target.name, models, err)
		}
		result, err := backend.TestConnection(ctx, target.name, request.Model)
		if err != nil || result.URL != configs[target.name].ChatURL {
			t.Fatalf("%s connection: %+v, %v", target.name, result, err)
		}
		if count := counts[target.name].Load(); count != 4 {
			t.Fatalf("%s received %d requests, want 4", target.name, count)
		}
	}
}

func TestConnectionFallbackDoesNotModifyProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "draft-key" || r.Header.Get("Authorization") != "" {
			t.Errorf("connection probe lost provider hooks: %v", r.Header)
		}
		if r.URL.Path != "/v1/chat/completions" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<!DOCTYPE html><html>settings</html>")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
	}))
	defer server.Close()
	backend, err := New(map[string]adapter.Provider{"draft": {ProviderType: "mimo", ChatURL: server.URL, APIKey: "draft-key"}}, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := backend.TestConnection(context.Background(), "draft", "model")
	if err != nil || result.URL != server.URL+"/v1/chat/completions" {
		t.Fatalf("fallback result: %+v, %v", result, err)
	}
	if backend.Providers()["draft"].ChatURL != server.URL {
		t.Fatal("connection test changed the active provider URL")
	}
}
