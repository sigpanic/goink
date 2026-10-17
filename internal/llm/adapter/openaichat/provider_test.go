package openaichat

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestProviderAssemblyUsesCompleteInstanceConfiguration(t *testing.T) {
	firstTemperature, secondTemperature := 0.0, 1.2
	configs := map[string]adapter.Provider{
		"relay-a": {
			Name: "relay-a", ProviderType: "mimo", ChatURL: "https://first.invalid/chat", APIKey: "first-key",
			Temperature: &firstTemperature,
			Models:      []ModelInfo{{ID: "first-model", Name: "First"}},
		},
		"relay-b": {
			Name: "relay-b", ProviderType: "mimo", ChatURL: "https://second.invalid/chat", APIKey: "second-key",
			Temperature: &secondTemperature,
			Models:      []ModelInfo{{ID: "second-model", Name: "Second"}},
		},
		"MiMo": {Name: "MiMo", ProviderType: "mimo"},
	}
	backend, err := New(configs, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	providers := backend.Providers()
	if !reflect.DeepEqual(providers, configs) {
		t.Fatalf("adapter changed complete instance configuration: got %+v, want %+v", providers, configs)
	}
	for name := range configs {
		if backend.providers[name].BuildHeaders == nil {
			t.Fatalf("instance %q did not receive MiMo hooks", name)
		}
	}
}

func TestProviderHooksUseOnlyExplicitType(t *testing.T) {
	config := adapter.Provider{
		Name: "MiMo", ProviderType: "openai",
		ChatURL: "https://api.xiaomimimo.com/v1/chat/completions",
		Models:  []ModelInfo{{ID: "mimo-v2-flash"}},
	}
	backend, err := New(map[string]adapter.Provider{config.Name: config}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := backend.providers[config.Name]
	if p.BuildRequest != nil || p.BuildHeaders != nil || p.ParseError != nil {
		t.Fatal("adapter inferred provider hooks from instance name, URL or model")
	}
}

func TestProviderTypesAreValidatedIndependentlyFromBuiltins(t *testing.T) {
	for _, providerType := range []string{"openai", "deepseek", "opencode", "opencode-go", "doubao", "qwen", "zhipu", "minimax", "mimo", "moonshot"} {
		t.Run(providerType, func(t *testing.T) {
			if err := ValidateProviderType(providerType); err != nil {
				t.Fatal(err)
			}
			config := adapter.Provider{Name: "independent-instance", ProviderType: providerType}
			if _, err := New(map[string]adapter.Provider{config.Name: config}, nil, nil); err != nil {
				t.Fatalf("supported type rejected for custom instance: %v", err)
			}
		})
	}
	for _, providerType := range []string{"", "unknown", "MiMo", " mimo "} {
		if err := ValidateProviderType(providerType); err == nil {
			t.Fatalf("invalid provider type %q accepted", providerType)
		}
	}
}

func TestBuiltinProvidersAreNamedInstances(t *testing.T) {
	providers := BuiltinProviders()
	for _, name := range []string{"DeepSeek", "OpenCode Zen", "OpenCode Go", "Doubao", "Qwen", "GLM", "MiniMax", "MiMo", "Kimi"} {
		p, ok := providers[name]
		if !ok || p.Name != name {
			t.Fatalf("builtin name %q does not identify its instance: %+v", name, p)
		}
		if err := ValidateProviderType(p.ProviderType); err != nil {
			t.Fatalf("builtin %q has an unsupported type: %v", name, err)
		}
	}
	for key, p := range providers {
		if key != p.Name {
			t.Fatalf("builtin key %q differs from instance name %q", key, p.Name)
		}
	}
}

func TestProviderSnapshotsIsolateMutableConfiguration(t *testing.T) {
	temperature := 0.4
	input := map[string]adapter.Provider{"relay": {
		Name: "relay", ProviderType: "openai", ChatURL: "https://original.invalid", Temperature: &temperature,
		Models: []ModelInfo{{ID: "model", ReasoningLevels: []string{"high"}}},
	}}
	backend, err := New(input, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	temperature = 1.9
	input["relay"].Models[0].ID = "mutated"
	input["relay"].Models[0].ReasoningLevels[0] = "mutated"
	delete(input, "relay")

	for i := 0; i < 2; i++ {
		providers := backend.Providers()
		p := providers["relay"]
		if len(providers) != 1 || p.ChatURL != "https://original.invalid" || *p.Temperature != 0.4 || p.Models[0].ID != "model" || p.Models[0].ReasoningLevels[0] != "high" {
			t.Fatalf("configuration snapshot changed: %+v", p)
		}
		*p.Temperature = 2
		p.Models[0].ID = "changed through query"
		p.Models[0].ReasoningLevels[0] = "changed through query"
		delete(providers, "relay")
	}

	instances := BuiltinProviders()
	original := instances["DeepSeek"]
	wantTemperature, wantID := *original.Temperature, original.Models[0].ID
	*original.Temperature = 99
	original.Models[0].ID = "changed builtin"
	original.Models[0].ReasoningLevels[0] = "changed builtin"
	delete(instances, "DeepSeek")
	fresh := BuiltinProviders()["DeepSeek"]
	if *fresh.Temperature != wantTemperature || fresh.Models[0].ID != wantID || fresh.Models[0].ReasoningLevels[0] == "changed builtin" {
		t.Fatal("builtin query exposed mutable state")
	}
}

func TestProviderAssemblyRejectsInvalidIdentity(t *testing.T) {
	for _, tt := range []struct {
		name    string
		configs map[string]adapter.Provider
	}{
		{"empty key", map[string]adapter.Provider{"": {ProviderType: "openai"}}},
		{"empty name", map[string]adapter.Provider{"relay": {ProviderType: "openai"}}},
		{"different name", map[string]adapter.Provider{"relay": {Name: "other", ProviderType: "openai"}}},
		{"blank name", map[string]adapter.Provider{" ": {Name: " ", ProviderType: "openai"}}},
		{"name whitespace", map[string]adapter.Provider{" relay ": {Name: " relay ", ProviderType: "openai"}}},
		{"missing type", map[string]adapter.Provider{"relay": {Name: "relay"}}},
		{"builtin name without type", map[string]adapter.Provider{"MiMo": {Name: "MiMo"}}},
		{"unknown type", map[string]adapter.Provider{"relay": {Name: "relay", ProviderType: "unknown"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.configs, nil, nil); err == nil {
				t.Fatalf("invalid provider accepted: %+v", tt.configs)
			}
		})
	}
	backend, err := New(nil, nil, nil)
	if err != nil || len(backend.Providers()) != 0 {
		t.Fatalf("empty user configuration activated providers: %v", err)
	}
}

func TestUnknownProviderNeverFallsBack(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("unknown provider caused an HTTP request")
		return nil, fmt.Errorf("unexpected request")
	})}
	backend, err := New(map[string]adapter.Provider{"known": {Name: "known", ProviderType: "openai", ChatURL: "https://known.invalid"}}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, providerName := range []string{"missing", "", "openai"} {
		check := func(err error) {
			t.Helper()
			if err == nil || !strings.Contains(err.Error(), "unknown OpenAI Chat provider:") {
				t.Fatalf("missing provider %q: %v", providerName, err)
			}
		}
		request := adapter.Request{ProviderName: providerName, Model: "model"}
		_, err := backend.Generate(ctx, request)
		check(err)
		_, err = backend.DiscoverModels(ctx, providerName)
		check(err)
		_, err = backend.TestConnection(ctx, providerName, "model")
		check(err)
		events := 0
		for event := range backend.Stream(ctx, request) {
			events++
			if event.Type != EventError {
				t.Fatalf("unexpected event: %+v", event)
			}
			check(event.Error)
		}
		if events != 1 {
			t.Fatalf("unknown provider emitted %d events", events)
		}
	}
}
