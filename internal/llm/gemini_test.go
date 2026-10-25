package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestGeminiRegistrationConfigurationAndReload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.RequestURI() != "/proxy/v8/interactions?route=gemini" || r.Header.Get("X-Goog-Api-Key") != "gemini-key" || r.Header.Get("Authorization") != "" {
			t.Errorf("incorrect Gemini instance routing: %s %v", r.URL, r.Header)
		}
		var params struct {
			Stream            bool
			Store, Background *bool
		}
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil || params.Store == nil || *params.Store || params.Background == nil || *params.Background {
			t.Errorf("root call lost stateless settings: %+v %v", params, err)
		}
		if params.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			for _, event := range []string{
				`{"event_type":"interaction.created","interaction":{"id":"root","status":"in_progress"}}`,
				`{"event_type":"step.start","index":0,"step":{"type":"model_output","content":[{"type":"text","text":"gemini"}]}}`,
				`{"event_type":"step.stop","index":0}`,
				`{"event_type":"interaction.status_update","interaction_id":"root","status":"completed"}`,
			} {
				fmt.Fprintf(w, "data: %s\n\n", event)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"root","object":"interaction","status":"completed","model":"gemini-3.8-flash","created":"2026-10-23T00:00:00Z","updated":"2026-10-23T00:00:01Z","steps":[{"type":"model_output","content":[{"type":"text","text":"gemini"}]}]}`)
	}))
	defer server.Close()
	config := emptyUserConfig()
	config.Adapters[adapter.ProtocolGeminiInteractions] = AdapterConfig{Providers: []adapter.Provider{
		{Name: "Google", APIKey: "gemini-key", ChatURL: server.URL + "/proxy/v8/interactions?route=gemini"},
		{Name: "shared", ProviderType: "google", ChatURL: server.URL + "/custom/interactions"},
	}}
	config.Adapters[adapter.ProtocolOpenAIChat] = AdapterConfig{Providers: []adapter.Provider{
		{Name: "shared", ProviderType: "openai", ChatURL: "https://unused.invalid/v1/chat/completions", APIKey: "openai-key"},
	}}
	client, err := NewClient(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.http.CloseIdleConnections()
	providers := client.Providers()
	if len(providers) != 2 || len(providers[adapter.ProtocolGeminiInteractions]) != 1 || providers[adapter.ProtocolGeminiInteractions]["Google"].ProviderType != "google" || providers[adapter.ProtocolOpenAIChat]["shared"].ProviderType != "openai" {
		t.Fatalf("key filtering or protocol namespace isolation failed: %+v", providers)
	}
	if model, ok := client.ProviderModel(adapter.ProtocolGeminiInteractions, "Google", "gemini-3.8-flash"); !ok || !model.SupportsThinking || len(Models(providers)) == 0 {
		t.Fatal("registered provider lost preset model metadata")
	}
	messages := []Message{{Role: "user", Content: "hi"}}
	text, err := client.GenerateText(context.Background(), adapter.ProtocolGeminiInteractions, "Google", "gemini-3.8-flash", messages, nil)
	if err != nil || text != "gemini" {
		t.Fatalf("registered Generate: %q %v", text, err)
	}
	var streamed string
	var state *ResponseState
	for event := range client.ChatStream(context.Background(), adapter.ProtocolGeminiInteractions, "Google", "gemini-3.8-flash", messages, nil, nil) {
		if event.Type == EventError {
			t.Fatal(event.Error)
		}
		if event.Type == EventContent {
			streamed += event.Data
		}
		if event.Type == EventResponseState {
			state = event.State
		}
	}
	if streamed != "gemini" || state == nil || !state.Complete || !state.TurnComplete || state.Origin.AdapterName != adapter.ProtocolGeminiInteractions || state.Origin.ProviderName != "Google" {
		t.Fatalf("registered Stream: %q %+v", streamed, state)
	}
	if _, err := client.GenerateText(context.Background(), adapter.ProtocolGeminiInteractions, "shared", "relay-model", messages, nil); err == nil {
		t.Fatal("missing Gemini instance fell back to same-name OpenAI instance")
	}
	if _, err := newProtocolAdapter(adapter.ProtocolGeminiGenerateContent, nil, client.http, nil); err == nil {
		t.Fatal("legacy GenerateContent protocol was silently mapped to Interactions")
	}
	oldBackend := client.adapters[adapter.ProtocolGeminiInteractions]
	path := filepath.Join(t.TempDir(), "llm.enc")
	if err := client.SaveConfig(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadUserConfig(path)
	if err != nil || !reflect.DeepEqual(loaded, config) {
		t.Fatalf("configuration round trip: %+v %v", loaded, err)
	}
	if loaded.Adapters[adapter.ProtocolGeminiInteractions].Providers[0].ProviderType != "" {
		t.Fatal("builtin provider type was written into user overrides")
	}
	invalid := emptyUserConfig()
	invalid.Adapters[adapter.ProtocolGeminiInteractions] = AdapterConfig{Providers: []adapter.Provider{
		{Name: "invalid", ProviderType: "anthropic", ChatURL: server.URL, APIKey: "key"},
	}}
	if err := client.Reload(invalid); err == nil || client.adapters[adapter.ProtocolGeminiInteractions].Providers()["Google"].APIKey != "gemini-key" {
		t.Fatal("invalid reload replaced the active snapshot")
	}
	loaded.Adapters[adapter.ProtocolGeminiInteractions].Providers[0].APIKey = ""
	if err := client.Reload(loaded); err != nil {
		t.Fatal(err)
	}
	if _, active := client.adapters[adapter.ProtocolGeminiInteractions]; active || oldBackend.Providers()["Google"].APIKey != "gemini-key" {
		t.Fatal("reload did not deactivate keyless protocol or modified an old snapshot")
	}
	view, err := BuildConfigView(loaded)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, provider := range view.Providers {
		if provider.AdapterName == adapter.ProtocolGeminiInteractions {
			found[provider.Name] = true
			if provider.Name == "Google" && (provider.APIKey != "" || len(provider.BuiltinModels) == 0) {
				t.Fatal("settings view lost the unconfigured Google preset")
			}
		}
	}
	if !found["Google"] || !found["shared"] {
		t.Fatal("settings view hid keyless builtin or custom instances")
	}
}

func TestGeminiTemporaryProviderProbes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Goog-Api-Key") != "probe-key" || r.URL.Query().Get("route") != "gemini" {
			t.Errorf("temporary probe lost credentials or query: %s %v", r.URL, r.Header)
		}
		if r.Method == http.MethodGet && r.URL.Path == "/v8/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"models":[{"name":"models/relay-model","displayName":"Relay"}]}`)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v8/interactions" {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"event_type\":\"interaction.created\",\"interaction\":{\"id\":\"probe\",\"status\":\"in_progress\"}}\n\n")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	input := ProviderProbeInput{AdapterName: adapter.ProtocolGeminiInteractions, ProviderName: "temporary", ProviderType: "google", ChatURL: server.URL + "/v8/interactions?route=gemini", APIKey: "probe-key"}
	models, err := DiscoverModels(context.Background(), input)
	if err != nil || len(models) != 1 || models[0].ID != "relay-model" || models[0].Name != "Relay" {
		t.Fatalf("temporary discovery: %+v %v", models, err)
	}
	result, err := TestConnection(context.Background(), TestConnectionInput{ProviderProbeInput: input, ModelID: "relay-model"})
	if err != nil || result.URL != input.ChatURL || result.Warning != "" {
		t.Fatalf("temporary connection probe: %+v %v", result, err)
	}
	input.ProviderType = "openai"
	if _, err := DiscoverModels(context.Background(), input); err == nil || !strings.Contains(err.Error(), "provider type") {
		t.Fatalf("cross-protocol provider type accepted: %v", err)
	}
}
