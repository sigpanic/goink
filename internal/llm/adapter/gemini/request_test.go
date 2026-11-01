package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	types "google.golang.org/genai/interactions/models/interactions"
	"google.golang.org/genai/interactions/models/operations"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func requestProvider() Provider {
	config := builtin["Gemini"].Clone()
	config.Name, config.APIKey, config.ChatURL = "relay", "private-key", "https://relay.invalid/v8/interactions?tenant=a"
	return Provider{Provider: config}
}

func requestFixture() adapter.Request {
	return adapter.Request{ProviderName: "relay", Model: "gemini-3.8-flash", Messages: []adapter.Message{{Role: "user", Content: "你好"}}}
}

func requestTools() []adapter.ToolDefinition {
	return []adapter.ToolDefinition{{Name: "read", Description: "读取章节", InputSchema: json.RawMessage(`{"type":"object","properties":{"chapter":{"type":"integer","maximum":9007199254740993}},"x-extension":{"keep":true}}`)}}
}

// sdkRequestBody 经过真实 SDK Create 编码和 HTTP 委托，避免只验证 Go 结构体。
func sdkRequestBody(t *testing.T, p Provider, params types.CreateModelInteraction) json.RawMessage {
	t.Helper()
	var body json.RawMessage
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != p.ChatURL || r.Header.Get("X-Goog-Api-Key") != p.APIKey {
			t.Error("built request lost endpoint or provider credentials")
		}
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(sdkTextResponse))}, nil
	})}
	service, err := assembleProvider(p.Provider, client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.client.Create(context.Background(), operations.CreateInteractionRequest{Body: operations.NewCreateInteractionRequestBody(params)}); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestRequestSDKBodyPreservesHistorySchemasAndNumbers(t *testing.T) {
	p, req := requestProvider(), requestFixture()
	req.Tools = requestTools()
	req.Options = &adapter.CallOptions{ToolChoice: map[string]any{"type": "function", "function": map[string]any{"name": "read"}}, SessionID: "local-only"}
	req.Messages = []adapter.Message{
		{Role: "system", Content: "规则一"}, {Role: "system", Content: "规则二"},
		{Role: "user", Content: "查章节"},
		{Role: "assistant", Content: "查一下", ThinkingContent: "不要作为输入正文发送", ToolCalls: []adapter.ToolCall{
			{ID: "a", Name: "read", Arguments: json.RawMessage(`{"chapter":9007199254740993}`)},
			{ID: "b", Name: "read", Arguments: json.RawMessage(`{"chapter":2}`)},
		}},
		{Role: "tool", ToolCallID: "b", ToolName: "read", Content: "第二章"},
		{Role: "tool", ToolCallID: "a", Content: ""},
		{Role: "system", Content: "临时提示"}, {Role: "user", Content: "继续"},
	}
	before, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	body := sdkRequestBody(t, p, params)
	want := json.RawMessage(`{
		"model":"gemini-3.8-flash","store":false,"background":false,
		"system_instruction":"规则一\n\n规则二",
		"generation_config":{"max_output_tokens":65536,"thinking_level":"medium","thinking_summaries":"auto","tool_choice":{"allowed_tools":{"mode":"any","tools":["read"]}}},
		"tools":[{"type":"function","name":"read","description":"读取章节","parameters":{"type":"object","properties":{"chapter":{"type":"integer","maximum":9007199254740993}},"x-extension":{"keep":true}}}],
		"input":[
			{"type":"user_input","content":[{"type":"text","text":"查章节"}]},
			{"type":"model_output","content":[{"type":"text","text":"查一下"}]},
			{"type":"function_call","id":"a","name":"read","arguments":{"chapter":9007199254740993}},
			{"type":"function_call","id":"b","name":"read","arguments":{"chapter":2}},
			{"type":"function_result","call_id":"a","name":"read","result":[{"type":"text","text":""}]},
			{"type":"function_result","call_id":"b","name":"read","result":[{"type":"text","text":"第二章"}]},
			{"type":"user_input","content":[{"type":"text","text":"<system-reminder>\n临时提示\n</system-reminder>"}]},
			{"type":"user_input","content":[{"type":"text","text":"继续"}]}
		]
	}`)
	if !sameJSON(body, want) {
		t.Fatalf("SDK request body mismatch:\n%s", body)
	}
	after, _ := json.Marshal(req)
	if string(before) != string(after) {
		t.Fatal("request conversion mutated caller messages, tools or options")
	}
	// 构建后的工具 schema 与源数据独立，后续 SDK 编码不能污染源配置。
	params.Tools[0].Function.Parameters.(json.RawMessage)[0] = 'x'
	if !json.Valid(req.Tools[0].InputSchema) {
		t.Fatal("SDK schema aliases the caller's input")
	}
}

func TestRequestModelDefaultsAndOutputLimits(t *testing.T) {
	p := requestProvider()
	for _, model := range builtin["Gemini"].Models {
		t.Run(model.ID, func(t *testing.T) {
			req := requestFixture()
			req.Model = model.ID
			params, err := p.buildRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			config := params.GenerationConfig
			if *config.MaxOutputTokens != model.MaxOutputTokens {
				t.Fatalf("unexpected output cap: %d", *config.MaxOutputTokens)
			}
			if config.ThinkingLevel == nil || string(*config.ThinkingLevel) != model.ReasoningLevels[0] || config.ThinkingSummaries == nil || *config.ThinkingSummaries != types.ThinkingSummariesAuto {
				t.Fatalf("unexpected thinking defaults: %+v", config)
			}
		})
	}
	for _, tc := range []struct {
		name, model string
		configured  int
		requested   *int
		want        int
		wantError   bool
	}{
		{name: "unknown model", model: "relay-model", want: 4096},
		{name: "configured unknown", model: "relay-model", configured: 8192, want: 8192},
		{name: "custom lower cap", model: "gemini-4-argon", configured: 1000, want: 1000},
		{name: "cannot raise official cap", model: "gemini-4-argon", configured: 100000, want: 65536},
		{name: "explicit output", model: "gemini-3.8-flash", requested: ptr(512), want: 512},
		{name: "zero", model: "gemini-3.8-flash", requested: ptr(0), wantError: true},
		{name: "negative", model: "relay-model", requested: ptr(-1), wantError: true},
		{name: "over cap", model: "gemini-4-argon", requested: ptr(65537), wantError: true},
		{name: "over custom cap", model: "relay-model", configured: 1000, requested: ptr(1001), wantError: true},
		{name: "empty", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider, req := requestProvider(), requestFixture()
			provider.Models = []adapter.ModelInfo{{ID: tc.model, MaxOutputTokens: tc.configured}}
			req.Model, req.Options = tc.model, &adapter.CallOptions{MaxTokens: tc.requested}
			params, err := provider.buildRequest(req)
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected validation: %v", err)
			}
			if err == nil && *params.GenerationConfig.MaxOutputTokens != tc.want {
				t.Fatalf("got max_output_tokens=%d, want %d", *params.GenerationConfig.MaxOutputTokens, tc.want)
			}
		})
	}
}

func TestRequestThinkingAndTemperatureOptions(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		opts        adapter.CallOptions
		levels      []string
		want        string
		wantError   bool
	}{
		{name: "explicit level", model: "gemini-3.8-flash", opts: adapter.CallOptions{ReasoningEffort: ptr("high")}, want: "high"},
		{name: "minimal supported", model: "gemini-3.6-flash", opts: adapter.CallOptions{ReasoningEffort: ptr("minimal")}, want: "minimal"},
		{name: "minimal unsupported", model: "gemini-3.8-flash", opts: adapter.CallOptions{ReasoningEffort: ptr("minimal")}, wantError: true},
		{name: "unknown level", model: "gemini-3.8-flash", opts: adapter.CallOptions{ReasoningEffort: ptr("max")}, wantError: true},
		{name: "configured restriction", model: "gemini-3.8-flash", levels: []string{"low"}, opts: adapter.CallOptions{ReasoningEffort: ptr("high")}, wantError: true},
		{name: "cannot expand official levels", model: "gemini-3.8-flash", levels: []string{"minimal"}, wantError: true},
		{name: "configured default", model: "gemini-3.8-flash", levels: []string{"low"}, want: "low"},
		{name: "cannot disable", model: "gemini-3.8-flash", opts: adapter.CallOptions{ThinkingEnabled: ptr(false)}, wantError: true},
		{name: "disable conflicts with level", model: "gemini-3.8-flash", opts: adapter.CallOptions{ThinkingEnabled: ptr(false), ReasoningEffort: ptr("low")}, wantError: true},
		{name: "unknown capability", model: "relay-gemini-3.8-flash", opts: adapter.CallOptions{ThinkingEnabled: ptr(true)}, wantError: true},
		{name: "unknown effort", model: "relay-model", opts: adapter.CallOptions{ReasoningEffort: ptr("high")}, wantError: true},
		{name: "removed temperature", model: "gemini-3.8-flash", opts: adapter.CallOptions{Temperature: ptr(0.7)}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, req := requestProvider(), requestFixture()
			if tc.levels != nil {
				p.Models = []adapter.ModelInfo{{ID: tc.model, ReasoningLevels: tc.levels}}
			}
			req.Model, req.Options = tc.model, &tc.opts
			params, err := p.buildRequest(req)
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected validation: %v", err)
			}
			if err != nil {
				return
			}
			got := ""
			if level := params.GenerationConfig.ThinkingLevel; level != nil {
				got = string(*level)
			}
			if got != tc.want {
				t.Fatalf("got thinking level %q, want %q", got, tc.want)
			}
		})
	}
	p := requestProvider()
	p.Temperature = ptr(1.0)
	if _, err := p.buildRequest(requestFixture()); err == nil {
		t.Fatal("provider temperature silently accepted a removed v1 field")
	}
}

func TestRequestToolChoicesAndSchemas(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{name: "auto", value: "auto", want: `"auto"`},
		{name: "none", value: "none", want: `"none"`},
		{name: "required", value: "required", want: `"any"`},
		{name: "unknown mode", value: "any"},
		{name: "undefined function", value: map[string]any{"type": "function", "function": map[string]any{"name": "write"}}},
		{name: "unknown field", value: map[string]any{"type": "function", "function": map[string]any{"name": "read"}, "parallel": true}},
		{name: "invalid type", value: 5},
		{name: "unencodable", value: make(chan int)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := requestFixture()
			req.Tools, req.Options = requestTools(), &adapter.CallOptions{ToolChoice: tc.value}
			params, err := requestProvider().buildRequest(req)
			if (err == nil) != (tc.want != "") {
				t.Fatalf("unexpected validation: %v", err)
			}
			if err == nil {
				raw, _ := json.Marshal(params.GenerationConfig.ToolChoice)
				if string(raw) != tc.want {
					t.Fatalf("got %s, want %s", raw, tc.want)
				}
			}
		})
	}
	for _, tools := range [][]adapter.ToolDefinition{
		{{Name: " ", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		{{Name: "read", InputSchema: json.RawMessage(`{`)}},
		{{Name: "read", InputSchema: json.RawMessage(`{"type":"array"}`)}},
		append(requestTools(), requestTools()...),
	} {
		req := requestFixture()
		req.Tools = tools
		if _, err := requestProvider().buildRequest(req); err == nil {
			t.Fatalf("invalid tools accepted: %+v", tools)
		}
	}
	req := requestFixture()
	req.Options = &adapter.CallOptions{ToolChoice: "required"}
	if _, err := requestProvider().buildRequest(req); err == nil {
		t.Fatal("forced tool use without tools was accepted")
	}
}
