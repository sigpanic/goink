package openairesponses

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/responses"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func requestProvider(t *testing.T) Provider {
	t.Helper()
	return newTestProvider(t, "https://configured.invalid/v8/responses?route=1", "instance-key", &http.Client{})
}

func requestInput() adapter.Request {
	return adapter.Request{ProviderName: "relay", Model: "gpt-6.1-sol", Messages: []adapter.Message{{Role: "user", Content: "hello"}}}
}

func pointer[T any](value T) *T { return &value }

func encodeParams(t *testing.T, params responses.ResponseNewParams) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestBuildRequestUsesStatelessSDKEncodingWithoutMutatingInput(t *testing.T) {
	var body map[string]json.RawMessage
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://configured.invalid/v8/responses?route=1" {
			t.Error("SDK request lost its complete endpoint")
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		return jsonResponse(sdkResponse), nil
	})}
	p := newTestProvider(t, "https://configured.invalid/v8/responses?route=1", "instance-key", client)
	req := requestInput()
	req.Tools = []adapter.ToolDefinition{{Name: "read", Description: "Read a chapter", InputSchema: json.RawMessage(`{"type":"object","properties":{"chapter":{"type":"integer","default":9007199254740993}},"required":[]}`)}}
	req.Options = &adapter.CallOptions{ToolChoice: map[string]any{"type": "function", "function": map[string]any{"name": "read"}}, SessionID: "local-session"}
	before, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(req)
	if err != nil || string(after) != string(before) {
		t.Fatal("request building mutated business data")
	}
	// 构造结果不能继续引用调用方的可变 schema。
	req.Tools[0].InputSchema[2] = 'X'
	opts, err := responsesURLOptions(p.ChatURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.client.New(context.Background(), params, opts...); err != nil {
		t.Fatal(err)
	}
	if string(body["store"]) != "false" || string(body["background"]) != "false" || string(body["model"]) != `"gpt-6.1-sol"` {
		t.Fatal("stateless settings were not encoded explicitly")
	}
	for _, field := range []string{"previous_response_id", "conversation", "user", "safety_identifier", "prompt_cache_key"} {
		if _, exists := body[field]; exists {
			t.Errorf("local session was used as server state or identity: %s", field)
		}
	}
	var tools []struct {
		Type, Name string
		Strict     *bool
		Parameters json.RawMessage
	}
	if err := json.Unmarshal(body["tools"], &tools); err != nil || len(tools) != 1 || tools[0].Type != "function" || tools[0].Name != "read" || tools[0].Strict == nil || *tools[0].Strict {
		t.Fatalf("SDK tool encoding changed: %s %v", body["tools"], err)
	}
	if string(tools[0].Parameters) != `{"type":"object","properties":{"chapter":{"type":"integer","default":9007199254740993}},"required":[]}` {
		t.Fatalf("tool schema changed precision or optional fields: %s", tools[0].Parameters)
	}
	if string(body["tool_choice"]) != `{"name":"read","type":"function"}` {
		t.Fatalf("named tool choice was not flattened: %s", body["tool_choice"])
	}
}

func TestRequestOptionsRespectExactModelCapabilities(t *testing.T) {
	for _, tt := range []struct {
		name, model, effort string
		options             *adapter.CallOptions
		configured          adapter.ModelInfo
		wantError           bool
	}{
		{name: "Astra default", model: "gpt-6-astra", effort: "medium"},
		{name: "Sol 6.1 default", model: "gpt-6.1-sol", effort: "medium"},
		{name: "Luna max", model: "gpt-6-luna", effort: "max", options: &adapter.CallOptions{ReasoningEffort: pointer("max")}},
		{name: "Sol 6 disabled", model: "gpt-6-sol", effort: "none", options: &adapter.CallOptions{ThinkingEnabled: pointer(false)}},
		{name: "5.6 alias", model: "gpt-5.6", effort: "medium"},
		{name: "5.6 Sol", model: "gpt-5.6-sol", effort: "medium"},
		{name: "5.6 Terra", model: "gpt-5.6-terra", effort: "medium"},
		{name: "5.6 Luna", model: "gpt-5.6-luna", effort: "medium"},
		{name: "Cyber default", model: "gpt-5.6-cyber"},
		{name: "unknown default", model: "relay-alias"},
		{name: "unknown configured capability is not proof", model: "relay-alias", configured: adapter.ModelInfo{ReasoningLevels: []string{"high"}}, options: &adapter.CallOptions{ReasoningEffort: pointer("high")}, wantError: true},
		{name: "Cyber explicit effort", model: "gpt-5.6-cyber", options: &adapter.CallOptions{ReasoningEffort: pointer("high")}, wantError: true},
		{name: "Astra cannot disable", model: "gpt-6-astra", options: &adapter.CallOptions{ThinkingEnabled: pointer(false)}, wantError: true},
		{name: "Sol 6.1 cannot disable", model: "gpt-6.1-sol", options: &adapter.CallOptions{ReasoningEffort: pointer("none")}, wantError: true},
		{name: "minimal unsupported", model: "gpt-5.6-sol", options: &adapter.CallOptions{ReasoningEffort: pointer("minimal")}, wantError: true},
		{name: "thinking and none conflict", model: "gpt-6-luna", options: &adapter.CallOptions{ThinkingEnabled: pointer(true), ReasoningEffort: pointer("none")}, wantError: true},
		{name: "disabled and high conflict", model: "gpt-6-sol", options: &adapter.CallOptions{ThinkingEnabled: pointer(false), ReasoningEffort: pointer("high")}, wantError: true},
		{name: "configured effort", model: "gpt-6-sol", configured: adapter.ModelInfo{ReasoningLevels: []string{"high", "low"}}, effort: "high"},
		{name: "configured cap", model: "gpt-6-sol", configured: adapter.ModelInfo{ReasoningLevels: []string{"high", "low"}}, options: &adapter.CallOptions{ReasoningEffort: pointer("max")}, wantError: true},
		{name: "configured cap cannot expand", model: "gpt-6.1-sol", configured: adapter.ModelInfo{ReasoningLevels: []string{"none", "high"}}, wantError: true},
		{name: "enable configured thinking", model: "gpt-6-sol", configured: adapter.ModelInfo{ReasoningLevels: []string{"none", "high"}}, options: &adapter.CallOptions{ThinkingEnabled: pointer(true)}, effort: "high"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := requestProvider(t)
			tt.configured.ID = tt.model
			p.Models = []adapter.ModelInfo{tt.configured}
			req := requestInput()
			req.Model, req.Options = tt.model, tt.options
			params, err := p.buildRequest(req)
			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected options result: %v", err)
			}
			if err != nil {
				return
			}
			if string(params.Reasoning.Effort) != tt.effort {
				t.Fatalf("effort=%q, want %q", params.Reasoning.Effort, tt.effort)
			}
			if tt.effort == "" || tt.effort == "none" {
				if params.Reasoning.Summary != "" {
					t.Fatal("unknown or disabled thinking requested a summary")
				}
			} else if params.Reasoning.Summary != "auto" {
				t.Fatal("thinking did not request a visible summary")
			}
		})
	}
}

func TestRequestOutputLimitsAndTemperature(t *testing.T) {
	for _, tt := range []struct {
		name, model string
		limit       int
		temperature *float64
		options     *adapter.CallOptions
		wantMax     int64
		wantError   bool
	}{
		{name: "known default", model: "gpt-6-sol", wantMax: 128_000},
		{name: "unknown default", model: "relay-alias", wantMax: 4096},
		{name: "configured smaller limit", model: "gpt-6-sol", limit: 1024, wantMax: 1024},
		{name: "configured limit cannot expand", model: "gpt-6-sol", limit: 999_999, options: &adapter.CallOptions{MaxTokens: pointer(128_001)}, wantError: true},
		{name: "zero output", model: "gpt-6-sol", options: &adapter.CallOptions{MaxTokens: pointer(0)}, wantError: true},
		{name: "negative output", model: "gpt-6-sol", options: &adapter.CallOptions{MaxTokens: pointer(-1)}, wantError: true},
		{name: "exceeds configured limit", model: "gpt-6-sol", limit: 1024, options: &adapter.CallOptions{MaxTokens: pointer(1025)}, wantError: true},
		{name: "sampling with thinking", model: "gpt-6-sol", temperature: pointer(1.0), wantError: true},
		{name: "sampling with none", model: "gpt-6-sol", temperature: pointer(0.5), options: &adapter.CallOptions{ThinkingEnabled: pointer(false)}, wantMax: 128_000},
		{name: "unknown sampling", model: "relay-alias", temperature: pointer(0.0), wantMax: 4096},
		{name: "temperature call override", model: "relay-alias", temperature: pointer(5.0), options: &adapter.CallOptions{Temperature: pointer(0.2)}, wantMax: 4096},
		{name: "nan", model: "relay-alias", temperature: pointer(math.NaN()), wantError: true},
		{name: "infinity", model: "relay-alias", temperature: pointer(math.Inf(1)), wantError: true},
		{name: "negative temperature", model: "relay-alias", temperature: pointer(-0.1), wantError: true},
		{name: "high temperature", model: "relay-alias", temperature: pointer(2.1), wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := requestProvider(t)
			p.Models, p.Temperature = []adapter.ModelInfo{{ID: tt.model, MaxOutputTokens: tt.limit}}, tt.temperature
			req := requestInput()
			req.Model, req.Options = tt.model, tt.options
			params, err := p.buildRequest(req)
			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected limits result: %v", err)
			}
			if err == nil && params.MaxOutputTokens.Value != tt.wantMax {
				t.Fatalf("max_output_tokens=%d, want %d", params.MaxOutputTokens.Value, tt.wantMax)
			}
		})
	}
}

func TestRequestRejectsInvalidToolsAndChoices(t *testing.T) {
	valid := adapter.ToolDefinition{Name: "read", InputSchema: json.RawMessage(`{"type":"object"}`)}
	for _, tt := range []struct {
		name   string
		tools  []adapter.ToolDefinition
		choice any
	}{
		{name: "empty name", tools: []adapter.ToolDefinition{{Name: " ", InputSchema: valid.InputSchema}}},
		{name: "duplicate name", tools: []adapter.ToolDefinition{valid, valid}},
		{name: "invalid JSON", tools: []adapter.ToolDefinition{{Name: "read", InputSchema: json.RawMessage(`{`)}}},
		{name: "array schema", tools: []adapter.ToolDefinition{{Name: "read", InputSchema: json.RawMessage(`{"type":"array"}`)}}},
		{name: "null schema", tools: []adapter.ToolDefinition{{Name: "read", InputSchema: json.RawMessage(`null`)}}},
		{name: "required without definitions", choice: "required"},
		{name: "unknown choice", tools: []adapter.ToolDefinition{valid}, choice: "any"},
		{name: "undefined named choice", tools: []adapter.ToolDefinition{valid}, choice: map[string]any{"type": "function", "function": map[string]any{"name": "missing"}}},
		{name: "unknown choice field", tools: []adapter.ToolDefinition{valid}, choice: map[string]any{"type": "function", "function": map[string]any{"name": "read", "extra": true}}},
		{name: "wrong choice type", tools: []adapter.ToolDefinition{valid}, choice: 123},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := requestInput()
			req.Tools, req.Options = tt.tools, &adapter.CallOptions{ToolChoice: tt.choice}
			if _, err := requestProvider(t).buildRequest(req); err == nil {
				t.Fatal("invalid tools or choice was accepted")
			}
		})
	}
	for _, mode := range []string{"auto", "none", "required"} {
		req := requestInput()
		req.Tools, req.Options = []adapter.ToolDefinition{valid}, &adapter.CallOptions{ToolChoice: mode}
		params, err := requestProvider(t).buildRequest(req)
		if err != nil || string(params.ToolChoice.OfToolChoiceMode.Value) != mode {
			t.Fatalf("valid tool choice %q failed: %v", mode, err)
		}
	}
}

func TestRequestPreservesMessageRolesAndIgnoresForeignState(t *testing.T) {
	req := requestInput()
	req.Messages = []adapter.Message{
		{Role: "system", Content: "first"}, {Role: "system", Content: "second"},
		{Role: "user", Content: "[2026-10-23] hello"}, {Role: "system", Content: "middle"},
		{Role: "developer", Content: "developer"},
		{Role: "assistant", Content: "answer", ThinkingContent: "private-display", State: &adapter.ResponseState{
			Complete: true, Origin: adapter.MessageOrigin{AdapterName: adapter.ProtocolGeminiInteractions}, Payload: json.RawMessage(`{`),
		}},
	}
	params, err := requestProvider(t).buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	raw := encodeParams(t, params)["input"]
	var messages []struct{ Role, Content string }
	if err := json.Unmarshal(raw, &messages); err != nil {
		t.Fatal(err)
	}
	if len(messages) != len(req.Messages) {
		t.Fatalf("message count=%d, want %d", len(messages), len(req.Messages))
	}
	for i, message := range req.Messages {
		if messages[i].Role != message.Role || messages[i].Content != message.Content {
			t.Fatal("message order, role, or already rendered timestamp changed")
		}
	}
	if strings.Contains(string(raw), "private-display") || strings.Contains(string(raw), "llm_state") {
		t.Fatal("display thinking or opaque state leaked into public messages")
	}
	for _, invalid := range []adapter.Request{{Model: " "}, {Model: "gpt-6-sol"}, {Model: "gpt-6-sol", Messages: []adapter.Message{{Role: "other", Content: "bad"}}}} {
		if _, err := requestProvider(t).buildRequest(invalid); err == nil {
			t.Fatal("empty or unsupported input was accepted")
		}
	}
	if !reflect.DeepEqual(req.Messages[5].State.Payload, json.RawMessage(`{`)) {
		t.Fatal("foreign state was modified")
	}
}
