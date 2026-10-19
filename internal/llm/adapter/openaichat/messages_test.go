package openaichat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestBuildMessagesPreservesExistingHistoryCleanup(t *testing.T) {
	backend := newTestAdapter(t, adapter.Provider{}, http.DefaultClient)
	arguments := `{"chapter":3,"id":9007199254740993}`
	history := []adapter.Message{
		{Role: "system", Content: "identity"},
		{Role: "tool", ToolCallID: "orphan", Content: "must not send"},
		{Role: "user", Content: "read"},
		{Role: "assistant", Content: "reading", ToolCalls: []adapter.ToolCall{
			{ID: "call-1", Name: "read", Arguments: json.RawMessage(arguments)},
			{ID: "call-1", Name: "duplicate", Arguments: json.RawMessage(`{}`)},
			{ID: "call-2", Name: "read", Arguments: json.RawMessage(`{"chapter":4}`)},
		}},
		{Role: "tool", ToolCallID: "call-1", ToolName: "read", Content: `{"success":true}`},
		{Role: "tool", ToolCallID: "call-1", ToolName: "read", Content: "duplicate result"},
		{Role: "tool", ToolCallID: "call-2", ToolName: "read", Content: `{"success":false,"error":"missing"}`},
		{Role: "system", Content: "state"},
	}
	before, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	got, err := backend.buildMessages(history)
	if err != nil {
		t.Fatal(err)
	}
	want := []map[string]any{
		{"role": "system", "content": "identity\n\nstate"},
		{"role": "user", "content": "read"},
		{"role": "assistant", "content": "reading", "reasoning_content": "", "tool_calls": []map[string]any{
			{"id": "call-1", "type": "function", "function": map[string]any{"name": "read", "arguments": arguments}},
			{"id": "call-2", "type": "function", "function": map[string]any{"name": "read", "arguments": `{"chapter":4}`}},
		}},
		{"role": "tool", "tool_call_id": "call-1", "name": "read", "content": `{"success":true}`},
		{"role": "tool", "tool_call_id": "call-2", "name": "read", "content": `{"success":false,"error":"missing"}`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected prepared history:\ngot  %#v\nwant %#v", got, want)
	}
	got[2]["tool_calls"].([]map[string]any)[0]["function"].(map[string]any)["arguments"] = "changed"
	got[1]["content"] = "changed"
	after, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("request preparation or mutation changed the source history")
	}
}

func TestBuildMessagesKeepsReasoningSeparateFromForeignState(t *testing.T) {
	backend := newTestAdapter(t, adapter.Provider{}, http.DefaultClient)
	for _, source := range []adapter.Protocol{adapter.ProtocolOpenAIChat, adapter.ProtocolAnthropicMessages, adapter.ProtocolGeminiGenerateContent, ""} {
		t.Run(string(source), func(t *testing.T) {
			history := []adapter.Message{{
				Role: "assistant", Content: "answer", ThinkingContent: "displayed thinking",
				ToolCalls: []adapter.ToolCall{{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{}`)}},
				State: &adapter.ResponseState{
					Origin: adapter.MessageOrigin{AdapterName: source}, Complete: true,
					Payload: json.RawMessage(`{"signature":"private","content":[{"type":"thinking"}]}`),
				},
			}}
			got, err := backend.buildMessages(history)
			if err != nil {
				t.Fatal(err)
			}
			thinking, hasThinking := got[0]["reasoning_content"]
			if source == adapter.ProtocolOpenAIChat {
				if !hasThinking || thinking != "displayed thinking" {
					t.Fatal("existing OpenAI reasoning was lost")
				}
			} else if hasThinking {
				t.Fatal("foreign or unknown reasoning was sent as OpenAI reasoning_content")
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "llm_state") || strings.Contains(string(encoded), "private") {
				t.Fatalf("native state leaked into the outgoing request: %s", encoded)
			}
			if got[0]["content"] != "answer" || len(got[0]["tool_calls"].([]map[string]any)) != 1 {
				t.Fatal("discarding private state also lost business content")
			}
		})
	}
	got, err := backend.buildMessages([]adapter.Message{{Role: "assistant", ThinkingContent: "legacy thinking"}})
	if err != nil || got[0]["reasoning_content"] != "legacy thinking" {
		t.Fatalf("history without source state changed: %v, %v", got, err)
	}
}

func TestBuildMessagesDoesNotInventMissingToolResults(t *testing.T) {
	backend := newTestAdapter(t, adapter.Provider{}, http.DefaultClient)
	history := []adapter.Message{{Role: "assistant", ToolCalls: []adapter.ToolCall{
		{ID: "pending", Name: "read", Arguments: json.RawMessage(`{}`)},
	}}}
	got, err := backend.buildMessages(history)
	if err != nil || len(got) != 1 || len(got[0]["tool_calls"].([]map[string]any)) != 1 {
		t.Fatalf("preparation silently repaired or removed an unfinished call: %v, %v", got, err)
	}
	nilHistory, err := backend.buildMessages(nil)
	if err != nil || nilHistory != nil {
		t.Fatalf("nil history changed: %v, %v", nilHistory, err)
	}
}

func TestBothModesPrepareHistoryBeforeProviderHooks(t *testing.T) {
	tests := []struct {
		providerType string
		model        string
	}{
		{"openai", "model"}, {"mimo", "model"}, {"qwen", "model"},
		{"minimax", "model"}, {"moonshot", "kimi-k3-test"},
	}
	for _, tt := range tests {
		t.Run(tt.providerType, func(t *testing.T) {
			requests := make(chan map[string]any, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if tt.providerType == "mimo" && (r.Header.Get("api-key") != "test-key" || r.Header.Get("Authorization") != "") {
					t.Errorf("Mimo headers changed: %v", r.Header)
				}
				requests <- payload
				if payload["stream"] == true {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
				} else {
					fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
				}
			}))
			defer server.Close()
			backend := newTestAdapter(t, adapter.Provider{
				ProviderType: tt.providerType, ChatURL: server.URL, APIKey: "test-key",
				Models: []adapter.ModelInfo{{ID: tt.model, SupportsThinking: true, ReasoningLevels: []string{"max"}}},
			}, server.Client())
			input := adapter.Request{ProviderName: "test", Model: tt.model, Options: &CallOptions{},
				Tools: []adapter.ToolDefinition{{Name: "read", InputSchema: json.RawMessage(`{"type":"object","properties":{"chapter":{"type":"integer"}}}`)}},
				Messages: []adapter.Message{
					{Role: "system", Content: "one"}, {Role: "system", Content: "two"},
					{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"chapter":3}`)}}},
					{Role: "tool", ToolCallID: "call-1", ToolName: "read", Content: "result"},
					{Role: "tool", ToolCallID: "orphan", Content: "must not send"},
				}}
			response, err := backend.Generate(context.Background(), input)
			if err != nil || response.Content != "ok" {
				t.Fatalf("generate: %+v, %v", response, err)
			}
			for event := range backend.Stream(context.Background(), input) {
				if event.Type == EventError {
					t.Fatal(event.Error)
				}
			}
			nonStream, stream := <-requests, <-requests
			if !reflect.DeepEqual(nonStream["messages"], stream["messages"]) {
				t.Fatal("Stream and Generate prepared different histories")
			}
			if !reflect.DeepEqual(nonStream["tools"], stream["tools"]) || len(nonStream["tools"].([]any)) != 1 {
				t.Fatal("provider hooks changed the prepared tool definitions")
			}
			messages := nonStream["messages"].([]any)
			if len(messages) != 3 || messages[0].(map[string]any)["content"] != "one\n\ntwo" {
				t.Fatalf("outbound history was not prepared: %v", messages)
			}
			switch tt.providerType {
			case "qwen":
				if nonStream["enable_thinking"] != false || stream["enable_thinking"] != true {
					t.Fatal("Qwen stream-specific thinking hook changed")
				}
			case "minimax":
				for _, payload := range []map[string]any{nonStream, stream} {
					if payload["reasoning_split"] != true || payload["thinking"].(map[string]any)["type"] != "adaptive" {
						t.Fatal("MiniMax thinking hook changed")
					}
				}
			case "moonshot":
				for _, payload := range []map[string]any{nonStream, stream} {
					if payload["temperature"] != nil || payload["thinking"] != nil || payload["max_completion_tokens"] == nil || payload["max_tokens"] != nil || payload["reasoning_effort"] != "max" {
						t.Fatalf("Moonshot parameter hook changed: %v", payload)
					}
				}
			}
		})
	}
}

func TestBothModesRejectInvalidHistoryBeforeHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	backend := newTestAdapter(t, adapter.Provider{ChatURL: server.URL}, server.Client())
	input := adapter.Request{ProviderName: "test", Model: "model", Messages: []adapter.Message{{Role: "assistant", ToolCalls: []adapter.ToolCall{
		{ID: "bad", Name: "read", Arguments: json.RawMessage(`{"chapter":`)},
	}}}}
	if _, err := backend.Generate(context.Background(), input); err == nil || !strings.Contains(err.Error(), "invalid arguments JSON") {
		t.Fatalf("invalid Generate history: %v", err)
	}
	errorCount := 0
	for event := range backend.Stream(context.Background(), input) {
		if event.Type != EventError || !strings.Contains(event.Error.Error(), "invalid arguments JSON") {
			t.Fatalf("unexpected Stream event: %+v", event)
		}
		errorCount++
	}
	if errorCount != 1 || requests.Load() != 0 {
		t.Fatalf("invalid history: errors=%d, HTTP requests=%d", errorCount, requests.Load())
	}
}
