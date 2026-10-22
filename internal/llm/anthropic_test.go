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

func TestAnthropicRegistrationConfigurationAndReload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/proxy/v8/messages?route=claude" || r.Header.Get("X-Api-Key") != "claude-key" || r.Header.Get("Authorization") != "" {
			t.Errorf("incorrect Anthropic instance routing: %s %v", r.URL, r.Header)
		}
		var params struct{ Stream bool }
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			t.Error(err)
		}
		if params.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"relay-model\",\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"+
				"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"claude\"}}\n\n"+
				"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"+
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n"+
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		} else {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"msg_test","type":"message","role":"assistant","model":"relay-model","content":[{"type":"text","text":"claude"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
		}
	}))
	defer server.Close()
	config := emptyUserConfig()
	config.Adapters[adapter.ProtocolAnthropicMessages] = AdapterConfig{Providers: []adapter.Provider{
		{Name: "Anthropic", APIKey: "claude-key", ChatURL: server.URL + "/proxy/v8/messages?route=claude"},
		{Name: "shared", ProviderType: "anthropic", ChatURL: server.URL + "/custom/messages"},
	}}
	config.Adapters[adapter.ProtocolOpenAIChat] = AdapterConfig{Providers: []adapter.Provider{{Name: "shared", ProviderType: "openai", ChatURL: "https://unused.invalid", APIKey: "openai-key"}}}
	client, err := NewClient(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.http.CloseIdleConnections()
	providers := client.Providers()
	if len(providers) != 2 || len(providers[adapter.ProtocolAnthropicMessages]) != 1 || providers[adapter.ProtocolAnthropicMessages]["Anthropic"].ProviderType != "anthropic" || providers[adapter.ProtocolOpenAIChat]["shared"].ProviderType != "openai" {
		t.Fatalf("configuration filtering or namespace isolation failed: %+v", providers)
	}
	messages := []Message{{Role: "user", Content: "hi"}}
	text, err := client.GenerateText(context.Background(), adapter.ProtocolAnthropicMessages, "Anthropic", "relay-model", messages)
	if err != nil || text != "claude" {
		t.Fatalf("registered Generate: %s %v", text, err)
	}
	var streamed string
	var state *ResponseState
	for event := range client.ChatStream(context.Background(), adapter.ProtocolAnthropicMessages, "Anthropic", "relay-model", messages, nil, nil) {
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
	if streamed != "claude" || state == nil || state.Origin.AdapterName != adapter.ProtocolAnthropicMessages {
		t.Fatalf("registered Stream: %s %+v", streamed, state)
	}
	if _, err := client.GenerateText(context.Background(), adapter.ProtocolAnthropicMessages, "shared", "relay-model", messages); err == nil {
		t.Fatal("missing Anthropic instance fell back to OpenAI instance with same name")
	}
	oldBackend := client.adapters[adapter.ProtocolAnthropicMessages]
	path := filepath.Join(t.TempDir(), "llm.enc")
	if err := client.SaveConfig(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadUserConfig(path)
	if err != nil || !reflect.DeepEqual(loaded, config) {
		t.Fatalf("new-format configuration round trip: %+v %v", loaded, err)
	}
	if loaded.Adapters[adapter.ProtocolAnthropicMessages].Providers[0].ProviderType != "" {
		t.Fatal("builtin type was materialized in user overrides")
	}
	loaded.Adapters[adapter.ProtocolAnthropicMessages].Providers[0].APIKey = ""
	if err := client.Reload(loaded); err != nil {
		t.Fatal(err)
	}
	if _, active := client.adapters[adapter.ProtocolAnthropicMessages]; active || oldBackend.Providers()["Anthropic"].APIKey != "claude-key" {
		t.Fatal("reload did not deactivate keyless protocol or changed old snapshot")
	}
	view, err := BuildConfigView(loaded)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range view.Providers {
		if p.AdapterName == adapter.ProtocolAnthropicMessages && p.Name == "Anthropic" && p.APIKey == "" && len(p.BuiltinModels) > 0 {
			return
		}
	}
	t.Fatal("settings view hid unconfigured Anthropic builtin")
}

func TestAnthropicTemporaryProviderProbes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "probe-key" || r.URL.Query().Get("route") != "claude" {
			t.Errorf("temporary probe lost instance credentials or query: %s %v", r.URL, r.Header)
		}
		if r.URL.Path == "/v8/models" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"relay-model","display_name":"Relay"}],"has_more":false}`)
			return
		}
		if r.URL.Path == "/v8/messages" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_probe\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[]}}\n\n")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	input := ProviderProbeInput{AdapterName: adapter.ProtocolAnthropicMessages, ProviderName: "temporary", ProviderType: "anthropic", ChatURL: server.URL + "/v8/messages?route=claude", APIKey: "probe-key"}
	models, err := DiscoverModels(context.Background(), input)
	if err != nil || len(models) != 1 || models[0].ID != "relay-model" {
		t.Fatalf("temporary discovery: %+v %v", models, err)
	}
	result, err := TestConnection(context.Background(), TestConnectionInput{ProviderProbeInput: input, ModelID: "relay-model"})
	if err != nil || result.URL != input.ChatURL {
		t.Fatalf("temporary connection test: %+v %v", result, err)
	}
	input.ProviderType = "openai"
	if _, err := DiscoverModels(context.Background(), input); err == nil || !strings.Contains(err.Error(), "provider type") {
		t.Fatalf("cross-protocol type accepted: %v", err)
	}
	for raw, want := range map[string]string{
		" relay.invalid/v8/messages ": "https://relay.invalid/v8/messages", "localhost:123/v1": "https://localhost:123/v1",
		"http://relay.invalid": "http://relay.invalid", "ftp://relay.invalid": "ftp://relay.invalid", "": "",
	} {
		if got := normalizeProbeURL(raw); got != want {
			t.Fatalf("temporary URL %q normalized to %q; want %q", raw, got, want)
		}
	}
}
