package gemini

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestProviderSnapshotsAreIndependent(t *testing.T) {
	temperature := 0.4
	configs := adapter.ProviderMap{
		"official": {Name: "official", ProviderType: "google", ChatURL: "https://generativelanguage.googleapis.com/v1/interactions", APIKey: "official-key"},
		"relay": {
			Name: "relay", ProviderType: "google", ChatURL: "https://relay.invalid/v8/interactions", APIKey: "relay-key", Temperature: &temperature,
			Models: []adapter.ModelInfo{{ID: "custom", ReasoningLevels: []string{"high", "low"}}},
		},
	}
	backend, err := New(configs, &http.Client{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := backend.Providers()
	if !reflect.DeepEqual(want, configs) {
		t.Fatal("construction changed the complete instance configuration")
	}
	temperature = 99
	configs["relay"].Models[0].ID = "changed"
	configs["relay"].Models[0].ReasoningLevels[0] = "changed"
	delete(configs, "official")
	for range 2 {
		got := backend.Providers()
		if !reflect.DeepEqual(got, want) {
			t.Fatal("provider snapshots share mutable configuration")
		}
		p := got["relay"]
		*p.Temperature = 123
		p.Models[0].ReasoningLevels[0] = "changed through snapshot"
		delete(got, "official")
	}
	for _, name := range []string{"", "missing", "google"} {
		if _, err := backend.provider(name); err == nil {
			t.Fatalf("unknown instance %q fell back to a type or builtin", name)
		}
	}
}

func TestProviderConstructionRejectsInvalidInstances(t *testing.T) {
	valid := adapter.Provider{Name: "relay", ProviderType: "google", ChatURL: "https://relay.invalid"}
	for _, tt := range []struct {
		name  string
		alter func(*adapter.Provider)
	}{
		{"missing name", func(p *adapter.Provider) { p.Name = "" }},
		{"mismatched name", func(p *adapter.Provider) { p.Name = "other" }},
		{"whitespace name", func(p *adapter.Provider) { p.Name = " relay " }},
		{"missing type", func(p *adapter.Provider) { p.ProviderType = "" }},
		{"wrong protocol type", func(p *adapter.Provider) { p.ProviderType = "openai" }},
		{"builtin name as type", func(p *adapter.Provider) { p.ProviderType = "Gemini" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := valid
			tt.alter(&p)
			if _, err := New(adapter.ProviderMap{"relay": p}, &http.Client{}, nil); err == nil {
				t.Fatal("invalid instance was accepted")
			}
		})
	}
	if _, err := New(nil, nil, nil); err == nil {
		t.Fatal("missing shared HTTP client was accepted")
	}
	empty, err := New(nil, &http.Client{}, nil)
	if err != nil || len(empty.Providers()) != 0 {
		t.Fatalf("empty configuration activated builtin providers: %v", err)
	}
}

func TestBuiltinProvidersAreNamedIndependentInstances(t *testing.T) {
	first, second := BuiltinProviders(), BuiltinProviders()
	p, ok := first["Gemini"]
	if !ok || p.Name != "Gemini" || p.APIKey != "" || p.Temperature != nil || len(p.Models) == 0 {
		t.Fatal("builtin must be a named instance with model metadata and no credentials")
	}
	if p.ChatURL != "https://generativelanguage.googleapis.com/v1/interactions" || ValidateProviderType(p.ProviderType) != nil {
		t.Fatal("builtin must use the complete Interactions endpoint and explicit behavior type")
	}
	for i := range p.Models {
		p.Models[i].ID = "changed"
		p.Models[i].ReasoningLevels[0] = "changed"
	}
	delete(first, "Gemini")
	if !reflect.DeepEqual(second, BuiltinProviders()) {
		t.Fatal("builtin snapshots share mutable model metadata")
	}
	if adapter.ProtocolGeminiInteractions == adapter.ProtocolGeminiGenerateContent {
		t.Fatal("Interactions reused the GenerateContent protocol identity")
	}
}
