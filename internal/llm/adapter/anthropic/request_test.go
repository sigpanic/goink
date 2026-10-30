package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func testRequest() (Provider, adapter.Request) {
	p := Provider{Provider: adapter.Provider{Name: "relay", ProviderType: "anthropic", ChatURL: "https://relay.invalid/v8/messages", APIKey: "key"}}
	req := adapter.Request{
		ProviderName: p.Name, Model: "relay-model",
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
		Tools:    []adapter.ToolDefinition{{Name: "read", Description: "Read chapter", InputSchema: json.RawMessage(`{"type":"object","properties":{"chapter":{"type":"integer","minimum":9007199254740993}},"required":["chapter"],"additionalProperties":false,"$defs":{"custom":{"const":9007199254740995}}}`)}},
	}
	return p, req
}

func testPtr[T any](value T) *T { return &value }

func requestJSON(t *testing.T, params sdk.MessageNewParams) map[string]json.RawMessage {
	t.Helper()
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRequestPreservesSchemaAndInputOwnership(t *testing.T) {
	p, req := testRequest()
	p.Temperature = testPtr(0.4)
	req.Messages = append(req.Messages,
		adapter.Message{Role: "assistant", Content: "reading", ToolCalls: []adapter.ToolCall{{ID: "call1", Name: "read", Arguments: json.RawMessage(`{"chapter":9007199254740993}`)}}},
		adapter.Message{Role: "tool", ToolCallID: "call1", ToolName: "read", Content: "chapter body"},
	)
	before, _ := json.Marshal(req)
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"input_schema":`, `"$defs":`, `"additionalProperties":false`, `9007199254740993`, `9007199254740995`, `"type":"tool_use"`, `"type":"tool_result"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("wire request lost %s: %s", want, data)
		}
	}
	for _, unwanted := range []string{`"tool_calls"`, `"parameters"`, `"llm_state"`, `"session_id"`} {
		if strings.Contains(string(data), unwanted) {
			t.Fatalf("wire request leaked %s", unwanted)
		}
	}
	params.Messages[0].Content[0].OfText.Text = "changed"
	input := params.Messages[1].Content[1].OfToolUse.Input.(json.RawMessage)
	input[0] = '['
	params.Tools[0].OfTool.Name = "changed"
	params.Temperature.Value = 0.9
	after, _ := json.Marshal(req)
	if string(before) != string(after) || *p.Temperature != 0.4 {
		t.Fatal("request construction or SDK params mutated caller-owned data")
	}
	encodedSchema, err := json.Marshal(params.Tools[0].OfTool.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	req.Tools[0].InputSchema[0] = '['
	afterSchema, err := json.Marshal(params.Tools[0].OfTool.InputSchema)
	if err != nil || string(afterSchema) != string(encodedSchema) {
		t.Fatal("prepared SDK schema aliases the caller's JSON bytes")
	}
}

func TestRequestOptions(t *testing.T) {
	for _, tt := range []struct {
		name, model, wantThinking, wantEffort string
		opts                                  *adapter.CallOptions
		wantErr                               bool
	}{
		{name: "unknown-basic", model: "relay-model"},
		{name: "opus-default", model: "claude-opus-5-5", wantThinking: "adaptive", wantEffort: "medium"},
		{name: "sonnet-default", model: "claude-sonnet-5-5", wantThinking: "adaptive", wantEffort: "high"},
		{name: "haiku-off", model: "claude-haiku-5-5", opts: &adapter.CallOptions{ThinkingEnabled: testPtr(false)}, wantThinking: "disabled", wantEffort: "medium"},
		{name: "haiku-force", model: "claude-haiku-5-5", opts: &adapter.CallOptions{ToolChoice: "required"}, wantThinking: "adaptive", wantEffort: "medium"},
		{name: "default-temperature", model: "claude-opus-5-5", opts: &adapter.CallOptions{Temperature: testPtr(1.0)}, wantThinking: "adaptive", wantEffort: "medium"},
		{name: "opus-off", model: "claude-opus-5-5", opts: &adapter.CallOptions{ThinkingEnabled: testPtr(false)}, wantErr: true},
		{name: "sonnet-off-is-not-between-tools", model: "claude-sonnet-5-5", opts: &adapter.CallOptions{ThinkingEnabled: testPtr(false)}, wantErr: true},
		{name: "haiku-off-max", model: "claude-haiku-5-5", opts: &adapter.CallOptions{ThinkingEnabled: testPtr(false), ReasoningEffort: testPtr("max")}, wantErr: true},
		{name: "bad-effort", model: "claude-opus-5-5", opts: &adapter.CallOptions{ReasoningEffort: testPtr("adaptive")}, wantErr: true},
		{name: "fable-force", model: "claude-fable-5-1", opts: &adapter.CallOptions{ToolChoice: "required"}, wantErr: true},
		{name: "opus-temperature", model: "claude-opus-5-5", opts: &adapter.CallOptions{Temperature: testPtr(0.3)}, wantErr: true},
		{name: "unknown-thinking", model: "relay-model", opts: &adapter.CallOptions{ThinkingEnabled: testPtr(true)}, wantErr: true},
		{name: "unknown-effort", model: "relay-model", opts: &adapter.CallOptions{ReasoningEffort: testPtr("high")}, wantErr: true},
		{name: "zero-max-tokens", model: "relay-model", opts: &adapter.CallOptions{MaxTokens: testPtr(0)}, wantErr: true},
		{name: "negative-max-tokens", model: "relay-model", opts: &adapter.CallOptions{MaxTokens: testPtr(-1)}, wantErr: true},
		{name: "nan-temperature", model: "relay-model", opts: &adapter.CallOptions{Temperature: testPtr(math.NaN())}, wantErr: true},
		{name: "infinite-temperature", model: "relay-model", opts: &adapter.CallOptions{Temperature: testPtr(math.Inf(1))}, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, req := testRequest()
			req.Model, req.Options = tt.model, tt.opts
			params, err := p.buildRequest(req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			wire := requestJSON(t, params)
			var thinking struct{ Type, Display string }
			if raw := wire["thinking"]; raw != nil {
				if err := json.Unmarshal(raw, &thinking); err != nil {
					t.Fatal(err)
				}
			}
			if thinking.Type != tt.wantThinking || string(params.OutputConfig.Effort) != tt.wantEffort {
				t.Fatalf("wrong thinking/effort: %s %s", thinking.Type, params.OutputConfig.Effort)
			}
			if thinking.Type == "adaptive" && thinking.Display != "summarized" {
				t.Fatal("adaptive thinking must request visible summaries")
			}
			if tt.name == "default-temperature" && wire["temperature"] != nil {
				t.Fatal("default-only sampling parameter should be omitted")
			}
		})
	}
}

func TestRequestUsesConfiguredModelDefaultsAndExplicitOverrides(t *testing.T) {
	p, req := testRequest()
	p.Models = []adapter.ModelInfo{{ID: "claude-haiku-5-5", MaxOutputTokens: 8000, ReasoningLevels: []string{"low", "medium"}}}
	req.Model = p.Models[0].ID
	params, err := p.buildRequest(req)
	if err != nil || params.MaxTokens != 8000 || params.OutputConfig.Effort != "low" {
		t.Fatalf("model defaults: %+v, %v", params, err)
	}
	req.Options = &adapter.CallOptions{MaxTokens: testPtr(2000), ReasoningEffort: testPtr("medium")}
	params, err = p.buildRequest(req)
	if err != nil || params.MaxTokens != 2000 || params.OutputConfig.Effort != "medium" {
		t.Fatalf("explicit options: %+v, %v", params, err)
	}
	for _, opts := range []*adapter.CallOptions{
		{MaxTokens: testPtr(8001)}, {ReasoningEffort: testPtr("max")},
	} {
		req.Options = opts
		if _, err := p.buildRequest(req); err == nil {
			t.Fatal("configured model limits were ignored")
		}
	}
}

func TestRequestToolChoiceMapping(t *testing.T) {
	for _, tt := range []struct {
		value any
		want  string
	}{
		{"auto", `{"type":"auto"}`},
		{"none", `{"type":"none"}`},
		{"required", `{"type":"any"}`},
		{map[string]any{"type": "function", "function": map[string]any{"name": "read"}}, `{"name":"read","type":"tool"}`},
	} {
		p, req := testRequest()
		req.Options = &adapter.CallOptions{ToolChoice: tt.value}
		params, err := p.buildRequest(req)
		if err != nil {
			t.Fatal(err)
		}
		if !sameJSON(requestJSON(t, params)["tool_choice"], json.RawMessage(tt.want)) {
			t.Fatalf("wrong tool_choice: %s", requestJSON(t, params)["tool_choice"])
		}
	}
	for _, value := range []any{true, "any", "invalid", map[string]any{"type": "tool", "name": "read"}, map[string]any{"type": "function", "function": map[string]any{"name": "missing"}}, map[string]any{"type": "function", "function": map[string]any{"name": "read"}, "extra": true}} {
		p, req := testRequest()
		req.Options = &adapter.CallOptions{ToolChoice: value}
		if _, err := p.buildRequest(req); err == nil {
			t.Fatalf("unsupported tool choice accepted: %v", value)
		}
	}
	p, req := testRequest()
	req.Tools = nil
	req.Options = &adapter.CallOptions{ToolChoice: "required"}
	if _, err := p.buildRequest(req); err == nil {
		t.Fatal("required tool choice accepted without tools")
	}
}

func TestRequestRejectsInvalidTools(t *testing.T) {
	for _, schema := range []string{"", "null", "[]", `{"type":"array"}`, `{"properties":{}}`, `{"type":"object"`} {
		p, req := testRequest()
		req.Tools[0].InputSchema = json.RawMessage(schema)
		if _, err := p.buildRequest(req); err == nil {
			t.Fatalf("invalid schema accepted: %s", schema)
		}
	}
	for _, name := range []string{"", "  "} {
		p, req := testRequest()
		req.Tools[0].Name = name
		if _, err := p.buildRequest(req); err == nil {
			t.Fatal("empty tool name accepted")
		}
	}
	p, req := testRequest()
	req.Tools = append(req.Tools, req.Tools[0])
	if _, err := p.buildRequest(req); err == nil {
		t.Fatal("duplicate tool definitions accepted")
	}
}

func TestBuiltRequestCanBeSentThroughSDK(t *testing.T) {
	p, req := testRequest()
	var received map[string]json.RawMessage
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != p.ChatURL || r.Header.Get("X-Api-Key") != p.APIKey {
			t.Error("wrong endpoint or credentials")
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}]}`))}, nil
	})}
	backend, err := New(adapter.ProviderMap{p.Name: p.Provider}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err = backend.provider(p.Name)
	if err != nil {
		t.Fatal(err)
	}
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	urlOption, err := messagesURLOption(p.ChatURL)
	if err != nil {
		t.Fatal(err)
	}
	response, err := p.client.Messages.New(context.Background(), params, urlOption)
	if err != nil || len(response.Content) != 1 || response.Content[0].Text != "ok" {
		t.Fatalf("SDK rejected constructed params: %v", err)
	}
	want := requestJSON(t, params)
	delete(received, "stream")
	if !reflect.DeepEqual(received, want) {
		t.Fatalf("SDK wire body differs from constructed request: got %v, want %v", received, want)
	}
}
