package gemini

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

const replayFixtureSteps = `[
	{"type":"thought","signature":"opaque-signature-1","summary":[],"extension":{"keep":9007199254740993}},
	{"type":"model_output","content":[{"type":"text","text":"先","annotation":{"keep":true}}]},
	{"type":"function_call","id":"a","name":"read","arguments":{"chapter":9007199254740993},"extension":{"keep":true}},
	{"type":"thought","signature":"opaque-signature-2","summary":[{"type":"text","text":"思考摘要"}]},
	{"type":"model_output","content":[{"type":"text","text":"后"}]}
]`

func replayFixture(t *testing.T, p Provider) adapter.Request {
	t.Helper()
	req := requestFixture()
	req.Tools = requestTools()
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := p.requestHash(params)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(replayPayload{Steps: json.RawMessage(replayFixtureSteps), RequestHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	message := adapter.Message{
		Role: "assistant", Content: "先后", ThinkingContent: "显示文本不参与签名构造",
		ToolCalls: []adapter.ToolCall{{ID: "a", Name: "read", Arguments: json.RawMessage(`{"chapter":9007199254740993}`)}},
		State: &adapter.ResponseState{
			Origin:   adapter.MessageOrigin{AdapterName: adapter.ProtocolGeminiInteractions, ProviderName: p.Name, ProviderType: p.ProviderType, Model: req.Model},
			Complete: true, StopReason: "requires_action", Payload: payload,
		},
	}
	// 公共消息序列化等价于现有持久化边界，不需要引入 session 依赖。
	stored, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), p.APIKey) {
		t.Fatal("native state contains provider credentials")
	}
	var loaded adapter.Message
	if err := json.Unmarshal(stored, &loaded); err != nil {
		t.Fatal(err)
	}
	req.Messages = append(req.Messages, loaded, adapter.Message{Role: "tool", ToolCallID: "a", Content: "章节"}, adapter.Message{Role: "user", Content: "继续"})
	return req
}

func TestRequestNativeStepsSurviveStorageAndSDKEncoding(t *testing.T) {
	p := requestProvider()
	req := replayFixture(t, p)
	before, _ := json.Marshal(req)
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	body := sdkRequestBody(t, p, params)
	var sent struct {
		Input []json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatal(err)
	}
	var expected []json.RawMessage
	if err := json.Unmarshal([]byte(replayFixtureSteps), &expected); err != nil {
		t.Fatal(err)
	}
	if len(sent.Input) != len(expected)+3 {
		t.Fatalf("native step order changed: %s", body)
	}
	for i, step := range expected {
		if !sameJSON(sent.Input[i+1], step) {
			t.Fatalf("native step %d lost raw fields: %s", i, sent.Input[i+1])
		}
	}
	after, _ := json.Marshal(req)
	if string(before) != string(after) {
		t.Fatal("native replay mutated stored common messages")
	}
	params.Input.ArrayOfStep[1].UnknownRaw[0] = 'x'
	if !json.Valid(req.Messages[1].State.Payload) {
		t.Fatal("native SDK input aliases stored payload")
	}
}

func TestRequestReplayCompatibilityAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name   string
		edit   func(*Provider, *adapter.Request)
		replay bool
	}{
		{name: "same request", replay: true},
		{name: "Gemini model switch keeps thoughts", replay: true, edit: func(_ *Provider, req *adapter.Request) { req.Model = "gemini-3.6-flash" }},
		{name: "generation options may change", replay: true, edit: func(_ *Provider, req *adapter.Request) {
			req.Options = &adapter.CallOptions{MaxTokens: ptr(1000), ReasoningEffort: ptr("low")}
		}},
		{name: "display thinking may change", replay: true, edit: func(_ *Provider, req *adapter.Request) { req.Messages[1].ThinkingContent = "新显示文本" }},
		{name: "different endpoint", edit: func(p *Provider, _ *adapter.Request) { p.ChatURL += "&other=true" }},
		{name: "different key", edit: func(p *Provider, _ *adapter.Request) { p.APIKey = "other" }},
		{name: "different instance", edit: func(p *Provider, _ *adapter.Request) { p.Name = "another-relay" }},
		{name: "different history", edit: func(_ *Provider, req *adapter.Request) { req.Messages[0].Content = "不同的输入" }},
		{name: "different system", edit: func(_ *Provider, req *adapter.Request) {
			req.Messages = append([]adapter.Message{{Role: "system", Content: "新规则"}}, req.Messages...)
		}},
		{name: "different tools", edit: func(_ *Provider, req *adapter.Request) { req.Tools[0].Description = "新工具说明" }},
		{name: "changed common content", edit: func(_ *Provider, req *adapter.Request) { req.Messages[1].Content = "新正文" }},
		{name: "changed common arguments", edit: func(_ *Provider, req *adapter.Request) {
			req.Messages[1].ToolCalls[0].Arguments = json.RawMessage(`{"chapter":9007199254740992}`)
		}},
		{name: "filtered missing tool", edit: func(_ *Provider, req *adapter.Request) { req.Messages = append(req.Messages[:2], req.Messages[3:]...) }},
		{name: "incomplete state", edit: func(_ *Provider, req *adapter.Request) {
			req.Messages[1].State.Complete = false
			req.Messages[1].State.Payload = json.RawMessage(`{`)
		}},
		{name: "foreign protocol", edit: func(_ *Provider, req *adapter.Request) {
			req.Messages[1].State.Origin.AdapterName = adapter.ProtocolAnthropicMessages
			req.Messages[1].State.Payload = json.RawMessage(`{`)
		}},
		{name: "different provider type", edit: func(_ *Provider, req *adapter.Request) { req.Messages[1].State.Origin.ProviderType = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := requestProvider()
			req := replayFixture(t, p)
			if tc.edit != nil {
				tc.edit(&p, &req)
			}
			params, err := p.buildRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(string(raw), "opaque-signature"); got != tc.replay {
				t.Fatalf("got replay=%v, want %v: %s", got, tc.replay, raw)
			}
			if strings.Contains(string(raw), "显示文本") {
				t.Fatal("display-only thinking was sent to the model")
			}
		})
	}
}

func TestRequestRejectsCorruptCompatibleNativeState(t *testing.T) {
	for _, raw := range []string{
		`null`, `[]`,
		`[{"type":"thought","summary":[]}]`,
		`[{"type":"thought","signature":"sig","summary":{}}]`,
		`[{"type":"model_output","content":[{"type":"image","uri":"https://example.invalid/image"}]}]`,
		`[{"type":"model_output","content":[{"type":"text"}]}]`,
		`[{"type":"model_output","error":{"message":"failed"}}]`,
		`[{"type":"google_search_call","id":"server-tool"}]`,
		`[{"type":"function_call","id":"a","name":"read","arguments":[]}]`,
		`[{"type":"function_call","id":"a","name":"read","arguments":{}},{"type":"function_call","id":"a","name":"read","arguments":{}}]`,
	} {
		t.Run(raw, func(t *testing.T) {
			p := requestProvider()
			req := replayFixture(t, p)
			var payload replayPayload
			if err := json.Unmarshal(req.Messages[1].State.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			payload.Steps = json.RawMessage(raw)
			req.Messages[1].State.Payload, _ = json.Marshal(payload)
			if _, err := p.buildRequest(req); err == nil {
				t.Fatal("corrupt compatible native state was accepted")
			}
		})
	}
	p := requestProvider()
	req := replayFixture(t, p)
	req.Messages[1].State.Payload = json.RawMessage(`{`)
	if _, err := p.buildRequest(req); err == nil {
		t.Fatal("invalid replay envelope was silently discarded")
	}
}

func TestRequestPreservesSignatureOnlyThought(t *testing.T) {
	p := requestProvider()
	req := replayFixture(t, p)
	var payload replayPayload
	if err := json.Unmarshal(req.Messages[1].State.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	payload.Steps = json.RawMessage(`[{"type":"thought","signature":"signature-only"}]`)
	message := req.Messages[1].Clone()
	message.Content, message.ToolCalls = "", nil
	message.State.Payload, _ = json.Marshal(payload)
	req.Messages = []adapter.Message{req.Messages[0], message, {Role: "user", Content: "继续"}}
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	body := sdkRequestBody(t, p, params)
	if !strings.Contains(string(body), `"signature":"signature-only"`) {
		t.Fatalf("empty public content discarded the original thought: %s", body)
	}
}
