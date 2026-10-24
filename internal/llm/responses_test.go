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
	"sync/atomic"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

const rootResponsesMessage = `{"type":"message","id":"msg_root","role":"assistant","status":"completed","content":[{"type":"output_text","text":"responses","annotations":[]}]}`
const rootResponsesResult = `{"id":"resp_root","object":"response","status":"completed","output":[` + rootResponsesMessage + `]}`

func TestResponsesRegistrationConfigurationAndProtocolIsolation(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPost {
			t.Error("generation used a non-POST request")
		}
		if r.URL.Path == "/chat" {
			if r.Header.Get("Authorization") != "Bearer chat-key" {
				t.Error("same-name Chat instance used Responses credentials")
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"content":"chat"}}]}`)
			return
		}
		if r.URL.RequestURI() != "/proxy/v8/responses?route=responses" || r.Header.Get("Authorization") != "Bearer responses-key" {
			t.Error("Responses generation lost its complete instance route or credentials")
		}
		var params struct {
			Stream            bool
			Store, Background *bool
		}
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil || params.Store == nil || *params.Store || params.Background == nil || *params.Background {
			t.Error("registered Responses call lost stateless settings")
		}
		if !params.Stream {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, rootResponsesResult)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, frame := range []string{
			`{"type":"response.created","response":{"id":"resp_root","object":"response","status":"in_progress","output":[]}}`,
			`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_root","role":"assistant","status":"in_progress","content":[]}}`,
			`{"type":"response.content_part.added","output_index":0,"item_id":"msg_root","content_index":0,"part":{"type":"output_text","text":"responses","annotations":[]}}`,
			`{"type":"response.output_item.done","output_index":0,"item":` + rootResponsesMessage + `}`,
			`{"type":"response.completed","response":` + rootResponsesResult + `}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", frame)
		}
	}))
	defer server.Close()
	config := emptyUserConfig()
	config.Adapters[adapter.ProtocolOpenAIResponses] = AdapterConfig{Providers: []adapter.Provider{
		{Name: "OpenAI", ChatURL: server.URL + "/proxy/v8/responses?route=responses", APIKey: "responses-key"},
		{Name: "chat-only", ProviderType: "openai", ChatURL: server.URL + "/responses"},
	}}
	config.Adapters[adapter.ProtocolOpenAIChat] = AdapterConfig{Providers: []adapter.Provider{
		{Name: "OpenAI", ProviderType: "openai", ChatURL: server.URL + "/chat", APIKey: "chat-key", Models: []adapter.ModelInfo{{ID: "gpt-6.1-sol", Name: "Chat model"}}},
		{Name: "chat-only", ProviderType: "openai", ChatURL: server.URL + "/chat", APIKey: "chat-key"},
	}}
	client, err := NewClient(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.http.CloseIdleConnections()
	providers := client.Providers()
	if len(providers) != 2 || len(providers[adapter.ProtocolOpenAIResponses]) != 1 || providers[adapter.ProtocolOpenAIResponses]["OpenAI"].ProviderType != "openai" || len(providers[adapter.ProtocolOpenAIChat]) != 2 {
		t.Fatalf("protocol registration or key filtering failed: %+v", providers)
	}
	model, ok := client.ProviderModel(adapter.ProtocolOpenAIResponses, "OpenAI", "gpt-6.1-sol")
	if !ok || !model.SupportsThinking || !model.SupportsVision || model.ContextWindow == 0 {
		t.Fatal("registered provider lost preset capabilities")
	}
	selectionKeys := make(map[string]bool)
	for _, option := range Models(providers) {
		if option.ProviderName == "OpenAI" && option.Model.ID == "gpt-6.1-sol" {
			selectionKeys[option.Key] = true
		}
	}
	if len(selectionKeys) != 2 {
		t.Fatal("same-name provider/model selection collided across protocols")
	}
	messages := []Message{{Role: "user", Content: "hi"}}
	for protocol, want := range map[adapter.Protocol]string{adapter.ProtocolOpenAIResponses: "responses", adapter.ProtocolOpenAIChat: "chat"} {
		text, err := client.GenerateText(context.Background(), protocol, "OpenAI", "gpt-6.1-sol", messages)
		if err != nil || text != want {
			t.Fatalf("registered generation for %s: %q %v", protocol, text, err)
		}
	}
	var text string
	var state *ResponseState
	for event := range client.ChatStream(context.Background(), adapter.ProtocolOpenAIResponses, "OpenAI", "gpt-6.1-sol", messages, nil, nil) {
		switch event.Type {
		case EventError:
			t.Fatal(event.Error)
		case EventContent:
			text += event.Data
		case EventResponseState:
			state = event.State
		}
	}
	if text != "responses" || state == nil || !state.Complete || !state.TurnComplete || state.Origin.AdapterName != adapter.ProtocolOpenAIResponses || state.Origin.ProviderName != "OpenAI" || requests.Load() != 3 {
		t.Fatalf("registered stream did not finish correctly: %q %+v", text, state)
	}
	if _, err := client.GenerateText(context.Background(), adapter.ProtocolOpenAIResponses, "chat-only", "gpt-6.1-sol", messages); err == nil || requests.Load() != 3 {
		t.Fatal("missing Responses provider fell back to same-name Chat provider")
	}
	path := filepath.Join(t.TempDir(), "llm.enc")
	if err := client.SaveConfig(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadUserConfig(path)
	if err != nil || !reflect.DeepEqual(loaded, config) || loaded.Adapters[adapter.ProtocolOpenAIResponses].Providers[0].ProviderType != "" {
		t.Fatalf("Responses config did not preserve user overrides: %+v %v", loaded, err)
	}
	oldBackend := client.adapters[adapter.ProtocolOpenAIResponses]
	invalid := emptyUserConfig()
	invalid.Adapters[adapter.ProtocolOpenAIResponses] = AdapterConfig{Providers: []adapter.Provider{
		{Name: "relay", ProviderType: "anthropic", ChatURL: server.URL, APIKey: "key"},
	}}
	if err := client.Reload(invalid); err == nil || client.adapters[adapter.ProtocolOpenAIResponses] != oldBackend {
		t.Fatal("invalid provider type replaced active Responses adapter")
	}
	loaded.Adapters[adapter.ProtocolOpenAIResponses].Providers[0].APIKey = ""
	if err := client.Reload(loaded); err != nil {
		t.Fatal(err)
	}
	if _, active := client.adapters[adapter.ProtocolOpenAIResponses]; active || oldBackend.Providers()["OpenAI"].APIKey != "responses-key" {
		t.Fatal("reload retained a keyless adapter or changed an old snapshot")
	}
	view, err := BuildConfigView(loaded)
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]bool)
	for _, provider := range view.Providers {
		if provider.AdapterName == adapter.ProtocolOpenAIResponses {
			found[provider.Name] = true
			if provider.Name == "OpenAI" && (provider.APIKey != "" || len(provider.BuiltinModels) == 0 || provider.Source != "builtin") {
				t.Fatal("settings view lost the unconfigured Responses builtin")
			}
		}
	}
	if !found["OpenAI"] || !found["chat-only"] {
		t.Fatal("settings view hid keyless builtin or custom Responses providers")
	}
}

func TestResponsesTemporaryProviderDiscoveryAndProbes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer probe-key" || r.URL.RawQuery != "route=responses" {
			t.Error("temporary Responses provider lost credentials or query")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v8/models":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"object":"list","data":[{"id":"relay-model"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v8/responses":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"probe\",\"object\":\"response\",\"status\":\"in_progress\",\"output\":[]}}\n\n")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	input := ProviderProbeInput{
		AdapterName:  adapter.ProtocolOpenAIResponses,
		ProviderName: "temporary",
		ProviderType: "openai",
		ChatURL:      "  " + server.URL + "/v8/responses?route=responses  ",
		APIKey:       "probe-key",
	}
	models, err := DiscoverModels(context.Background(), input)
	if err != nil || len(models) != 1 || models[0].ID != "relay-model" {
		t.Fatalf("temporary Models discovery failed: %+v %v", models, err)
	}
	result, err := TestConnection(context.Background(), TestConnectionInput{ProviderProbeInput: input, ModelID: "relay-model"})
	if err != nil || result.URL != strings.TrimSpace(input.ChatURL) || result.Warning != "" {
		t.Fatalf("temporary connection probe failed: %+v %v", result, err)
	}
	input.ProviderType = "anthropic"
	if _, err := DiscoverModels(context.Background(), input); err == nil || !strings.Contains(err.Error(), "provider type") {
		t.Fatalf("Responses accepted another protocol's provider type: %v", err)
	}
}
