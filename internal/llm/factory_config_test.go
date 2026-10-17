package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/llm/adapter/openaichat"
)

type configRoundTripper func(*http.Request) (*http.Response, error)

func (f configRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestBuildAdaptersUsesMergedInstancesAndTypeHooks(t *testing.T) {
	zero, relayTemperature := 0.0, 0.5
	cfg := &UserLLMConfig{
		Version: ConfigVersion,
		Adapters: map[adapter.Protocol]AdapterConfig{
			adapter.ProtocolOpenAIChat: {Providers: []adapter.Provider{
				{Name: "MiMo", APIKey: "official-key", Temperature: &zero,
					Models: []adapter.ModelInfo{{ID: "extra-model"}}},
				{Name: "中转站1", ProviderType: "mimo", ChatURL: "https://relay.example/v1/chat/completions",
					APIKey: "relay-key", Temperature: &relayTemperature,
					Models: []adapter.ModelInfo{{ID: "relay-model"}}},
				{Name: "未启用", ProviderType: "mimo", ChatURL: "https://disabled.example",
					Models: []adapter.ModelInfo{{ID: "disabled-model"}}},
			}},
		},
	}
	official := openaichat.BuiltinProviders()["MiMo"]
	requests := 0
	client := &http.Client{Transport: configRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests++
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			return nil, err
		}
		wantKey, wantTemperature, wantModel := "official-key", 0.0, official.Models[0].ID
		if req.URL.Host == "relay.example" {
			wantKey, wantTemperature, wantModel = "relay-key", 0.5, "relay-model"
		} else if req.URL.String() != official.ChatURL {
			t.Errorf("official URL was not resolved: %s", req.URL)
		}
		if req.Header.Get("api-key") != wantKey || req.Header.Get("Authorization") != "" {
			t.Errorf("MiMo hooks did not use instance credentials: %v", req.Header)
		}
		if payload["temperature"] != wantTemperature || payload["model"] != wantModel {
			t.Errorf("instance payload changed: %v", payload)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`)),
			Request:    req,
		}, nil
	})}
	backends, err := buildAdapters(cfg, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	backend := backends[adapter.ProtocolOpenAIChat]
	providers := backend.Providers()
	if len(providers) != 2 || providers["MiMo"].Name != "MiMo" || providers["中转站1"].Name != "中转站1" {
		t.Fatalf("instance names were not retained: %+v", providers)
	}
	if len(providers["MiMo"].Models) != len(official.Models)+1 {
		t.Fatalf("builtin models and user models were not merged: %+v", providers["MiMo"].Models)
	}
	if len(providers["中转站1"].Models) != 1 || providers["中转站1"].Models[0].ID != "relay-model" {
		t.Fatalf("same-type relay inherited official models: %+v", providers["中转站1"])
	}
	for _, target := range []struct{ name, model string }{
		{"MiMo", official.Models[0].ID},
		{"中转站1", "relay-model"},
	} {
		response, err := backend.Generate(context.Background(), adapter.Request{
			ProviderName: target.name, Model: target.model, Options: &adapter.CallOptions{},
		})
		if err != nil || response.Content != "ok" {
			t.Fatalf("generate %q: %+v, %v", target.name, response, err)
		}
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	stored := cfg.Adapters[adapter.ProtocolOpenAIChat].Providers[0]
	if stored.ProviderType != "" || stored.ChatURL != "" || len(stored.Models) != 1 || stored.Models[0].ID != "extra-model" {
		t.Fatalf("runtime assembly mutated persisted overrides: %+v", stored)
	}
}
