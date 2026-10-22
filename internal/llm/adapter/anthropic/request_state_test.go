package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func replayFixture(t *testing.T) (Provider, adapter.Request, json.RawMessage) {
	t.Helper()
	p, req := testRequest()
	prefix, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := p.requestHash(prefix)
	if err != nil {
		t.Fatal(err)
	}
	content := json.RawMessage(`[
		{"type":"thinking","thinking":"","signature":"test-signature"},
		{"type":"redacted_thinking","data":"opaque-data"},
		{"type":"text","text":"reading","vendor_extension":{"n":9007199254740995}},
		{"type":"tool_use","id":"call1","name":"read","input":{"chapter":9007199254740993}}
	]`)
	payload, err := json.Marshal(replayPayload{Content: content, RequestHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	message := adapter.Message{
		Role: "assistant", Content: "reading", ThinkingContent: "display text is not a native thinking block",
		ToolCalls: []adapter.ToolCall{{ID: "call1", Name: "read", Arguments: json.RawMessage(`{"chapter":9007199254740993}`)}},
		State: &adapter.ResponseState{
			Origin:   adapter.MessageOrigin{AdapterName: adapter.ProtocolAnthropicMessages, ProviderName: p.Name, ProviderType: p.ProviderType, Model: req.Model},
			Complete: true, StopReason: "tool_use", Payload: payload,
		},
	}
	// 模拟消息落库和重载；测试签名只验证透传，不证明真实服务端会接受它。
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var restored adapter.Message
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	req.Messages = append(req.Messages, restored, adapter.Message{Role: "tool", ToolCallID: "call1", Content: "chapter body"})
	return p, req, content
}

func TestRequestReplaysNativeBlocksAfterMessageRoundTrip(t *testing.T) {
	p, req, content := replayFixture(t)
	before, _ := json.Marshal(req)
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	wire := requestJSON(t, params)
	var messages []struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(wire["messages"], &messages); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 || !sameJSONArray(messages[1].Content, content) {
		t.Fatalf("native content changed: %s", wire["messages"])
	}
	data, _ := json.Marshal(params)
	if strings.Contains(string(data), "display text") || strings.Contains(string(data), "request_hash") || strings.Contains(string(data), `"origin"`) {
		t.Fatal("replay leaked display-only or internal state fields")
	}
	after, _ := json.Marshal(req)
	if string(before) != string(after) {
		t.Fatal("native replay changed stored payload")
	}
	// 修改源 payload 后，请求仍持有自己的原生块副本。
	for i := range req.Messages[1].State.Payload {
		req.Messages[1].State.Payload[i] = ' '
	}
	again, err := json.Marshal(params)
	if err != nil || string(again) != string(data) {
		t.Fatal("prepared request aliases native payload bytes")
	}
}

func TestRequestDropsIncompatibleNativeState(t *testing.T) {
	for _, kind := range []string{"protocol", "provider", "provider-type", "model", "incomplete", "prefix", "tools", "options", "credential", "endpoint", "public-content", "public-arguments", "missing-result", "missing-hash"} {
		t.Run(kind, func(t *testing.T) {
			p, req, _ := replayFixture(t)
			message := &req.Messages[1]
			switch kind {
			case "protocol":
				message.State.Origin.AdapterName = adapter.ProtocolOpenAIChat
				message.State.Payload = json.RawMessage(`not Anthropic JSON`)
			case "provider":
				message.State.Origin.ProviderName = "other relay"
			case "provider-type":
				message.State.Origin.ProviderType = "other"
			case "model":
				req.Model = "other-model"
			case "incomplete":
				message.State.Complete = false
			case "prefix":
				req.Messages[0].Content = "edited prompt"
			case "tools":
				req.Tools[0].Description = "changed tool"
			case "options":
				req.Options = &adapter.CallOptions{MaxTokens: testPtr(1000)}
			case "credential":
				p.APIKey = "different-account-key"
			case "endpoint":
				p.ChatURL = "https://another.invalid/messages"
			case "public-content":
				message.Content = "edited answer"
			case "public-arguments":
				message.ToolCalls[0].Arguments = json.RawMessage(`{"chapter":2}`)
			case "missing-result":
				req.Messages[2] = adapter.Message{Role: "user", Content: "continue"}
			case "missing-hash":
				message.State.Payload = json.RawMessage(`{"content":[{"type":"thinking","thinking":"","signature":"test-signature"}]}`)
			}
			params, err := p.buildRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(params)
			for _, private := range []string{"test-signature", "opaque-data", "vendor_extension", "display text"} {
				if strings.Contains(string(data), private) {
					t.Fatalf("incompatible state leaked %s: %s", private, data)
				}
			}
			if kind != "missing-result" && !strings.Contains(string(data), `"tool_use_id":"call1"`) {
				t.Fatal("dropping reasoning lost the common tool result")
			}
		})
	}
}

func TestRequestRejectsInvalidCompatibleNativeBlocks(t *testing.T) {
	for _, content := range []string{
		`null`, `[]`, `{}`,
		`[{"type":"thinking","thinking":"","signature":""}]`,
		`[{"type":"thinking","signature":"test-signature"}]`,
		`[{"type":"redacted_thinking","data":""}]`,
		`[{"type":"server_tool_use","id":"server1","name":"web_search","input":{}}]`,
	} {
		t.Run(content, func(t *testing.T) {
			p, req, _ := replayFixture(t)
			var payload replayPayload
			if err := json.Unmarshal(req.Messages[1].State.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			payload.Content = json.RawMessage(content)
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			req.Messages[1].State.Payload = data
			if _, err := p.buildRequest(req); err == nil {
				t.Fatal("invalid native blocks were silently replayed")
			}
		})
	}
}

func TestRequestKeepsThinkingOnlyAssistantWhenReplayIsValid(t *testing.T) {
	p, req, _ := replayFixture(t)
	message := &req.Messages[1]
	message.Content, message.ToolCalls = "", nil
	var payload replayPayload
	if err := json.Unmarshal(message.State.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	payload.Content = json.RawMessage(`[{"type":"thinking","thinking":"","signature":"test-signature"}]`)
	message.State.Payload, _ = json.Marshal(payload)
	req.Messages[2] = adapter.Message{Role: "user", Content: "continue"}
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(params)
	if len(params.Messages) != 3 || !strings.Contains(string(data), "test-signature") {
		t.Fatal("valid empty thinking block was discarded")
	}
}
