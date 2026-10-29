package openairesponses

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func replayRequest(t *testing.T, p Provider, model string) (adapter.Request, replayPayload) {
	t.Helper()
	req := requestInput()
	req.Model = model
	req.Tools = []adapter.ToolDefinition{{Name: "read", InputSchema: json.RawMessage(`{"type":"object","properties":{"chapter":{"type":"integer"}}}`)}}
	prefix, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := p.requestHash(prefix)
	if err != nil {
		t.Fatal(err)
	}
	payload := replayPayload{Output: json.RawMessage("[" + sdkReasoning + "," + sdkMessage + "," + sdkFunction + "]"), RequestHash: hash}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	message := adapter.Message{
		Role: "assistant", Content: "ok", ThinkingContent: "plan",
		ToolCalls: []adapter.ToolCall{{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{"chapter":9007199254740993}`)}},
		State: &adapter.ResponseState{
			Complete: true, StopReason: "completed",
			Origin:  adapter.MessageOrigin{AdapterName: adapter.ProtocolOpenAIResponses, ProviderName: p.Name, ProviderType: p.ProviderType, Model: model},
			Payload: data,
		},
	}
	// 模拟公共 Message 存储往返，原生数据由 Session 透传而非解释。
	stored, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(stored, &message); err != nil {
		t.Fatal(err)
	}
	req.Messages = append(req.Messages, message, adapter.Message{Role: "tool", ToolCallID: "call_1", ToolName: "read", Content: `{"result":42}`})
	return req, payload
}

func TestNativeOutputReplaySurvivesStorageAndSDKEncoding(t *testing.T) {
	var input []json.RawMessage
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body struct{ Input []json.RawMessage }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		input = body.Input
		return jsonResponse(sdkResponse), nil
	})}
	p := newTestProvider(t, "https://configured.invalid/custom/responses", "instance-key", client)
	req, _ := replayRequest(t, p, "gpt-5.6-sol")
	// 同族切模型、变更输出上限和 effort 不需要伪造或重建 reasoning。
	req.Model = "gpt-5.6-terra"
	req.Options = &adapter.CallOptions{ReasoningEffort: pointer("high"), MaxTokens: pointer(4096)}
	req.Messages[1].ThinkingContent = "changed display summary"
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	// SDK 参数已独占原文，之后修改业务消息不会改变本次出站。
	req.Messages[1].State.Payload[2] = 'X'
	opts, err := responsesURLOptions(p.ChatURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.client.New(context.Background(), params, opts...); err != nil {
		t.Fatal(err)
	}
	if len(input) != 5 || string(input[1]) != sdkReasoning || string(input[2]) != sdkMessage || string(input[3]) != sdkFunction {
		t.Fatalf("native output order, ciphertext or exact fields changed: %s", encodeParams(t, params)["input"])
	}
	var result struct {
		Type, ID, Output string
		CallID           string `json:"call_id"`
	}
	if err := json.Unmarshal(input[4], &result); err != nil || result.Type != "function_call_output" || result.CallID != "call_1" || result.ID != "" || result.Output != `{"result":42}` {
		t.Fatalf("tool output did not link to call_id: %s %v", input[4], err)
	}
}

func TestReplayCompatibilityDropsOnlyOpaqueState(t *testing.T) {
	for _, tt := range []struct {
		name  string
		alter func(*Provider, *adapter.Request)
	}{
		{"different protocol", func(p *Provider, req *adapter.Request) {
			req.Messages[1].State.Origin.AdapterName = adapter.ProtocolOpenAIChat
		}},
		{"different instance", func(p *Provider, req *adapter.Request) { p.Name = "other" }},
		{"different behavior", func(p *Provider, req *adapter.Request) { p.ProviderType = "other" }},
		{"different endpoint", func(p *Provider, req *adapter.Request) { p.ChatURL = "https://other.invalid/responses" }},
		{"different credential", func(p *Provider, req *adapter.Request) { p.APIKey = "different-key" }},
		{"different family", func(p *Provider, req *adapter.Request) { req.Model = "gpt-6-luna" }},
		{"changed history", func(p *Provider, req *adapter.Request) { req.Messages[0].Content = "changed" }},
		{"changed tools", func(p *Provider, req *adapter.Request) { req.Tools[0].Description = "changed" }},
		{"edited public text", func(p *Provider, req *adapter.Request) { req.Messages[1].Content = "edited" }},
		{"edited public arguments", func(p *Provider, req *adapter.Request) {
			req.Messages[1].ToolCalls[0].Arguments = json.RawMessage(`{"chapter":2}`)
		}},
		{"incomplete state", func(p *Provider, req *adapter.Request) { req.Messages[1].State.Complete = false }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := requestProvider(t)
			req, _ := replayRequest(t, p, "gpt-5.6-sol")
			tt.alter(&p, &req)
			params, err := p.buildRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			raw := string(encodeParams(t, params)["input"])
			if strings.Contains(raw, "encrypted_content") || strings.Contains(raw, "opaque-test-state") || strings.Contains(raw, "plan") || !strings.Contains(raw, "function_call_output") || !strings.Contains(raw, "call_1") {
				t.Fatalf("fallback lost public tools or leaked opaque reasoning: %s", raw)
			}
		})
	}
	for _, tt := range []struct{ source, target string }{
		{"gpt-6-astra", "gpt-6-luna"}, {"gpt-6-sol", "gpt-6-astra"}, {"gpt-6.1-sol", "gpt-6.1-sol"},
	} {
		p := requestProvider(t)
		req, _ := replayRequest(t, p, tt.source)
		req.Model = tt.target
		params, err := p.buildRequest(req)
		if err != nil || !strings.Contains(string(encodeParams(t, params)["input"]), "opaque-test-state") {
			t.Fatalf("compatible replay %s -> %s failed: %v", tt.source, tt.target, err)
		}
	}
}

func TestReplayProjectionFallsBackFromDamagedMatchingState(t *testing.T) {
	for _, output := range []string{
		`null`, `[]`, `{}`, `[null]`,
		`[{"type":"reasoning","id":"rs_1"}]`,
		`[{"type":"message","role":"user","content":[{"type":"output_text","text":"ok"}]}]`,
		`[{"type":"message","role":"assistant","status":"incomplete","content":[{"type":"output_text","text":"ok"}]}]`,
		`[{"type":"message","role":"assistant","content":[{"type":"input_image"}]}]`,
		`[{"type":"function_call","call_id":"call_1","name":"read","arguments":"[]"}]`,
		`[{"type":"function_call","id":"fc_1","name":"read","arguments":"{}"}]`,
		`[` + sdkFunction + `,` + sdkFunction + `]`,
		`[{"type":"web_search_call","id":"ws_1","status":"completed"}]`,
	} {
		t.Run(output, func(t *testing.T) {
			p := requestProvider(t)
			req, payload := replayRequest(t, p, "gpt-6-sol")
			payload.Output = json.RawMessage(output)
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			req.Messages[1].State.Payload = data
			params, err := p.buildRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			req.Messages[1].State = nil
			fallback, err := p.buildRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			if string(encodeParams(t, params)["input"]) != string(encodeParams(t, fallback)["input"]) {
				t.Fatal("damaged native state changed the public history")
			}
		})
	}
}

func TestReplayDropsStatesWhoseToolGroupWasFiltered(t *testing.T) {
	p := requestProvider(t)
	req, _ := replayRequest(t, p, "gpt-6-sol")
	req.Messages = req.Messages[:2]
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(encodeParams(t, params)["input"])
	if strings.Contains(raw, "encrypted_content") || strings.Contains(raw, "function_call") || !strings.Contains(raw, "ok") {
		t.Fatalf("incomplete native tool group was replayed: %s", raw)
	}
}

func TestReplayPreservesAssistantPhasesAndEmptyMessages(t *testing.T) {
	p := requestProvider(t)
	req, payload := replayRequest(t, p, "gpt-6-sol")
	output := []json.RawMessage{
		json.RawMessage(sdkReasoning),
		json.RawMessage(`{"type":"message","id":"empty","role":"assistant","status":"completed","phase":"commentary","content":[]}`),
		json.RawMessage(`{"type":"message","id":"progress","role":"assistant","status":"completed","phase":"commentary","content":[{"type":"output_text","text":"checking","annotations":[]}]}`),
		json.RawMessage(sdkFunction),
		json.RawMessage(`{"type":"message","id":"final","role":"assistant","status":"completed","phase":"final_answer","content":[{"type":"output_text","text":"ok","annotations":[]}]}`),
	}
	raw, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	payload.Output = raw
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req.Messages[1].Content, req.Messages[1].State.Payload = "checkingok", data
	params, err := p.buildRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var input []json.RawMessage
	if err := json.Unmarshal(encodeParams(t, params)["input"], &input); err != nil {
		t.Fatal(err)
	}
	if len(input) != len(output)+2 {
		t.Fatalf("native items were flattened: %s", encodeParams(t, params)["input"])
	}
	for i, item := range output {
		if string(input[i+1]) != string(item) {
			t.Fatalf("item %d lost its position, phase or empty content: %s", i, input[i+1])
		}
	}
}

func TestReplayFingerprintDoesNotStoreCredentials(t *testing.T) {
	p := requestProvider(t)
	req, _ := replayRequest(t, p, "gpt-6-sol")
	if strings.Contains(string(req.Messages[1].State.Payload), p.APIKey) || strings.Contains(string(req.Messages[1].State.Payload), p.ChatURL) {
		t.Fatal("replay state stores credentials or private endpoint")
	}
	req.Messages[1].State.Payload = json.RawMessage(`{`)
	if _, err := p.buildRequest(req); err != nil {
		t.Fatalf("damaged state blocked the public history: %v", err)
	}
	// 跨协议时不解释其他适配器的私有 payload。
	req.Messages[1].State.Origin.AdapterName = adapter.ProtocolAnthropicMessages
	if _, err := p.buildRequest(req); err != nil {
		t.Fatalf("foreign payload was interpreted: %v", err)
	}
}
