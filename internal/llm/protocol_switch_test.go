package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

type switchFixture struct {
	Protocol        adapter.Protocol  `json:"protocol"`
	ProviderType    string            `json:"provider_type"`
	Path            string            `json:"path"`
	ToolsResponse   json.RawMessage   `json:"tools_response"`
	TextResponse    json.RawMessage   `json:"text_response"`
	StreamEvents    []json.RawMessage `json:"stream_events"`
	ExpectedRequest json.RawMessage   `json:"expected_request"`
}

// 用真实适配器生成历史，再经公共 JSON 契约往返；不依赖 Agent、Session 或服务端会话。
func TestProtocolSwitchPreservesStoredHistory(t *testing.T) {
	fixtures := loadSwitchFixtures(t)
	for _, source := range fixtures {
		for _, target := range fixtures {
			if source.Protocol == target.Protocol {
				continue
			}
			for _, mode := range []string{"generate", "stream"} {
				t.Run(string(source.Protocol)+"_to_"+string(target.Protocol)+"/"+mode, func(t *testing.T) {
					var attempts atomic.Int32
					requests := make(chan map[string]json.RawMessage, 4)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						fixture := source
						if attempts.Add(1) != 1 {
							fixture = target
						}
						if r.Method != http.MethodPost || r.URL.RequestURI() != fixture.Path+"?route="+string(fixture.Protocol) {
							t.Errorf("wrong protocol endpoint: %s %s", r.Method, r.URL)
						}
						var body map[string]json.RawMessage
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
							http.Error(w, "invalid JSON request", http.StatusBadRequest)
							return
						}
						select {
						case requests <- body:
						default:
							t.Error("unexpected automatic requests")
						}
						if attempts.Load() == 1 {
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write(fixture.ToolsResponse)
							return
						}
						if string(body["stream"]) != "true" {
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write(fixture.TextResponse)
							return
						}
						w.Header().Set("Content-Type", "text/event-stream")
						for _, raw := range fixture.StreamEvents {
							var compact bytes.Buffer
							if err := json.Compact(&compact, raw); err != nil {
								t.Error(err)
								return
							}
							var event struct{ Type string }
							if err := json.Unmarshal(raw, &event); err != nil {
								t.Error(err)
								return
							}
							if event.Type != "" {
								_, _ = fmt.Fprintf(w, "event: %s\n", event.Type)
							}
							_, _ = fmt.Fprintf(w, "data: %s\n\n", compact.Bytes())
						}
					}))
					t.Cleanup(server.Close)
					config := emptyUserConfig()
					for _, fixture := range fixtures {
						config.Adapters[fixture.Protocol] = AdapterConfig{Providers: []adapter.Provider{{
							Name: "relay", ProviderType: fixture.ProviderType, APIKey: "test-instance-key",
							ChatURL: server.URL + fixture.Path + "?route=" + string(fixture.Protocol),
						}}}
					}
					client, err := NewClient(config, nil)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(client.http.CloseIdleConnections)
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					req := adapter.Request{
						ProviderName: "relay", Model: "switch-model",
						Messages: []Message{{Role: "system", Content: "你是网文助手。"}, {Role: "user", Content: "请比较这两章。"}},
						Tools:    []ToolDefinition{{Name: "read", Description: "读取章节", InputSchema: json.RawMessage(`{"type":"object","properties":{"chapter":{"type":"integer"}}}`)}},
					}
					generated := callSwitchAdapter(t, ctx, client, source.Protocol, req, false)
					message := Message{Role: "assistant", Content: generated.Content, ThinkingContent: generated.ThinkingContent, State: generated.State}
					for _, call := range generated.ToolCalls {
						message.ToolCalls = append(message.ToolCalls, ToolCall{ID: call.ToolID, Name: call.ToolName, Arguments: call.ArgumentsJSON})
					}
					wantCalls := []ToolCall{
						{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{"chapter":9007199254740993}`)},
						{ID: "call_2", Name: "read", Arguments: json.RawMessage(`{}`)},
					}
					if message.Content != "先读取两章。" || message.ThinkingContent != "private-thought" || !reflect.DeepEqual(message.ToolCalls, wantCalls) {
						t.Fatalf("source response lost common fields or argument precision: %+v", message)
					}
					if message.State == nil || !message.State.Complete || message.State.TurnComplete ||
						message.State.Origin != (MessageOrigin{AdapterName: source.Protocol, ProviderName: "relay", ProviderType: source.ProviderType, Model: req.Model}) {
						t.Fatalf("source response did not supply its tool-waiting response state: %+v", message.State)
					}
					if source.Protocol == adapter.ProtocolOpenAIChat {
						if len(message.State.Payload) != 0 || message.State.StopReason != "tool_calls" {
							t.Fatalf("Chat response state has native payload or lost its stop reason: %+v", message.State)
						}
					} else if !bytes.Contains(message.State.Payload, []byte("private-")) {
						t.Fatalf("source response lost its native payload: %+v", message.State)
					}
					stored := switchJSON(t, message)
					var loaded Message
					if err := json.Unmarshal(stored, &loaded); err != nil || !bytes.Equal(stored, switchJSON(t, loaded)) {
						t.Fatalf("public message storage round trip failed: %v", err)
					}
					req.Messages = append(req.Messages, loaded,
						Message{Role: "tool", ToolCallID: "call_2", ToolName: "read", Content: "第二章原文"},
						Message{Role: "tool", ToolCallID: "call_1", ToolName: "read", Content: ""},
						Message{Role: "user", Content: "继续比较。"},
					)
					before := switchJSON(t, req)
					result := callSwitchAdapter(t, ctx, client, target.Protocol, req, mode == "stream")
					if result.Content != "续聊成功" || len(result.ToolCalls) != 0 {
						t.Fatalf("target failed to continue or repeated a historical tool call: %+v", result)
					}
					if result.State == nil || !result.State.Complete || !result.State.TurnComplete ||
						result.State.Origin != (MessageOrigin{AdapterName: target.Protocol, ProviderName: "relay", ProviderType: target.ProviderType, Model: req.Model}) {
						t.Fatalf("target did not supply its own complete response state: %+v", result.State)
					}
					if !bytes.Equal(before, switchJSON(t, req)) {
						t.Fatal("switching rewrote the original history, native state or tool schema")
					}
					if attempts.Load() != 2 || len(requests) != 2 {
						t.Fatalf("expected exactly one source and one target request, got %d", attempts.Load())
					}
					<-requests
					outbound := switchJSON(t, <-requests)
					for _, internal := range []string{"private-", `"llm_state"`, `"request_hash"`, `"origin"`, `"reasoning_content"`, `"previous_response_id"`, `"previous_interaction_id"`, `"conversation"`} {
						if bytes.Contains(outbound, []byte(internal)) {
							t.Fatalf("foreign reasoning or internal state leaked into target request (%s): %s", internal, outbound)
						}
					}
					assertSwitchJSONSubset(t, decodeSwitchJSON(t, outbound), decodeSwitchJSON(t, target.ExpectedRequest), "request")
				})
			}
		}
	}
}

func loadSwitchFixtures(t *testing.T) []switchFixture {
	t.Helper()
	files, err := filepath.Glob("testdata/protocol-switch/*.json")
	if err != nil || len(files) != 4 {
		t.Fatalf("expected four protocol fixtures: %v %v", files, err)
	}
	var fixtures []switchFixture
	seen := make(map[adapter.Protocol]bool)
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var fixture switchFixture
		if err := json.Unmarshal(data, &fixture); err != nil || fixture.Protocol == "" || seen[fixture.Protocol] {
			t.Fatalf("invalid or duplicate protocol fixture %s: %v", file, err)
		}
		seen[fixture.Protocol] = true
		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

func callSwitchAdapter(t *testing.T, ctx context.Context, client *Client, protocol adapter.Protocol, req adapter.Request, stream bool) adapter.Response {
	t.Helper()
	if !stream {
		backend, err := client.protocolAdapter(protocol)
		if err != nil {
			t.Fatal(err)
		}
		response, err := backend.Generate(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	var response adapter.Response
	events := client.ChatStream(ctx, protocol, req.ProviderName, req.Model, req.Messages, req.Tools, req.Options)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return response
			}
			switch event.Type {
			case EventError:
				t.Fatal(event.Error)
			case EventContent:
				response.Content += event.Data
			case EventThinking:
				response.ThinkingContent += event.Data
			case EventToolCallEnd:
				if event.Delta == nil {
					t.Fatal("tool completion event has no call")
				}
				response.ToolCalls = append(response.ToolCalls, *event.Delta)
			case EventResponseState:
				if event.State == nil || response.State != nil {
					t.Fatal("missing or duplicate response state")
				}
				response.State = event.State
			}
		case <-ctx.Done():
			t.Fatal("stream did not close: ", ctx.Err())
		}
	}
}

func switchJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decodeSwitchJSON(t *testing.T, data []byte) any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// 只容许额外对象字段；数组长度、顺序及数值精度必须匹配，防止隐藏原生块混入请求。
func assertSwitchJSONSubset(t *testing.T, got, want any, path string) {
	t.Helper()
	switch expected := want.(type) {
	case map[string]any:
		actual, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("%s: expected an object, got %v", path, got)
		}
		for key, value := range expected {
			field, exists := actual[key]
			if !exists {
				t.Fatalf("%s.%s: missing field", path, key)
			}
			assertSwitchJSONSubset(t, field, value, path+"."+key)
		}
	case []any:
		actual, ok := got.([]any)
		if !ok || len(actual) != len(expected) {
			t.Fatalf("%s: expected %d items, got %v", path, len(expected), got)
		}
		for i, value := range expected {
			assertSwitchJSONSubset(t, actual[i], value, fmt.Sprintf("%s[%d]", path, i))
		}
	default:
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %v, want %v", path, got, want)
		}
	}
}
