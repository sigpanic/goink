package anthropic

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestProviderSnapshotsPreserveCompleteInstances(t *testing.T) {
	temperature := 0.4
	configs := adapter.ProviderMap{
		"official": {Name: "official", ProviderType: "anthropic", ChatURL: "https://api.anthropic.com/v1/messages", APIKey: "official-key"},
		"relay": {
			Name: "relay", ProviderType: "anthropic", ChatURL: "https://relay.invalid/prefix/v1/messages", APIKey: "relay-key",
			Temperature: &temperature,
			Models:      []adapter.ModelInfo{{ID: "custom-model", ReasoningLevels: []string{"high", "low"}}},
		},
	}
	backend, err := New(configs, &http.Client{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := backend.Providers()
	if !reflect.DeepEqual(want, configs) {
		t.Fatal("construction changed complete instance configuration")
	}
	temperature = 99
	configs["relay"].Models[0].ID = "changed"
	configs["relay"].Models[0].ReasoningLevels[0] = "changed"
	delete(configs, "official")
	for i := 0; i < 2; i++ {
		got := backend.Providers()
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("configuration snapshot was mutated: %+v", got)
		}
		p := got["relay"]
		*p.Temperature = 123
		p.Models[0].ID = "changed through snapshot"
		p.Models[0].ReasoningLevels[0] = "changed through snapshot"
		delete(got, "official")
	}
	for _, name := range []string{"", "missing", "anthropic"} {
		if _, err := backend.provider(name); err == nil {
			t.Fatalf("unknown instance %q fell back to a type or builtin", name)
		}
	}
}

func TestProviderConstructionRejectsInvalidConfiguration(t *testing.T) {
	valid := adapter.Provider{Name: "relay", ProviderType: "anthropic", ChatURL: "https://relay.invalid"}
	for _, tt := range []struct {
		name  string
		alter func(*adapter.Provider)
	}{
		{"missing name", func(p *adapter.Provider) { p.Name = "" }},
		{"mismatched name", func(p *adapter.Provider) { p.Name = "other" }},
		{"missing type", func(p *adapter.Provider) { p.ProviderType = "" }},
		{"wrong protocol type", func(p *adapter.Provider) { p.ProviderType = "openai" }},
		{"instance name as type", func(p *adapter.Provider) { p.ProviderType = "Claude" }},
		{"missing URL", func(p *adapter.Provider) { p.ChatURL = "" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := valid
			tt.alter(&p)
			if _, err := New(adapter.ProviderMap{"relay": p}, &http.Client{}, nil); err == nil {
				t.Fatal("invalid configuration was accepted")
			}
		})
	}
	for _, name := range []string{"", " ", " relay "} {
		p := valid
		p.Name = name
		if _, err := New(adapter.ProviderMap{name: p}, &http.Client{}, nil); err == nil {
			t.Fatalf("invalid instance name %q was accepted", name)
		}
	}
	if _, err := New(nil, nil, nil); err == nil {
		t.Fatal("missing injected HTTP client was accepted")
	}
	empty, err := New(nil, &http.Client{}, nil)
	if err != nil || len(empty.Providers()) != 0 {
		t.Fatalf("empty configuration activated builtin instances: %v", err)
	}
}

func TestBuiltinProvidersAreIndependentNamedInstances(t *testing.T) {
	first := BuiltinProviders()
	second := BuiltinProviders()
	p, ok := first["Claude"]
	if !ok || p.Name != "Claude" || p.APIKey != "" || p.Temperature != nil || len(p.Models) == 0 {
		t.Fatal("builtin should be a named instance with explicit models and no credentials or temperature override")
	}
	if err := ValidateProviderType(p.ProviderType); err != nil {
		t.Fatal(err)
	}
	if p.ChatURL != "https://api.anthropic.com/v1/messages" {
		t.Fatal("builtin must provide the complete Messages endpoint")
	}
	p.Models[0].ID = "changed"
	p.Models[0].ReasoningLevels[0] = "changed"
	delete(first, "Claude")
	if !reflect.DeepEqual(BuiltinProviders(), second) {
		t.Fatal("builtin snapshots share mutable model data")
	}
	// 内置名称不负责选择行为；同一类型可创建任意命名的自定义实例。
	custom := adapter.Provider{Name: "my relay", ProviderType: "anthropic", ChatURL: "https://relay.invalid"}
	if _, err := New(adapter.ProviderMap{custom.Name: custom}, &http.Client{}, nil); err != nil {
		t.Fatal(err)
	}
}
