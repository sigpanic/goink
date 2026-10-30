package openaichat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestBothModesRecordResponseState(t *testing.T) {
	for _, tc := range []struct {
		name         string
		stopReason   string
		hasTools     bool
		turnComplete bool
	}{
		{"normal stop", "stop", false, true},
		{"tool calls", "tool_calls", true, false},
		{"output limit", "length", false, false},
		{"filtered", "content_filter", false, true},
		{"missing reason", "", false, true},
		{"missing reason with tools", "", true, false},
		{"stop with tools", "stop", true, false},
		{"unknown reason", "vendor_reason", false, false},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", tc.name, stream), func(t *testing.T) {
				message := map[string]any{"content": "answer", "reasoning_content": "thinking"}
				if tc.hasTools {
					message["tool_calls"] = []any{map[string]any{
						"index": 0, "id": "call-1", "type": "function",
						"function": map[string]any{"name": "lookup", "arguments": "{}"},
					}}
				}
				usage := map[string]any{"total_tokens": 12}
				var wire strings.Builder
				if stream {
					writeChunk(&wire, map[string]any{"choices": []any{map[string]any{"delta": message}}})
					writeChunk(&wire, map[string]any{"choices": []any{map[string]any{"finish_reason": tc.stopReason}}})
					writeChunk(&wire, map[string]any{"choices": []any{}, "usage": usage})
					wire.WriteString("data: [DONE]\n\n")
				} else {
					require.NoError(t, json.NewEncoder(&wire).Encode(map[string]any{
						"choices": []any{map[string]any{"message": message, "finish_reason": tc.stopReason}}, "usage": usage,
					}))
				}
				client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(wire.String()))}, nil
				})}
				backend := newTestAdapter(t, adapter.Provider{ChatURL: "https://offline.invalid", ProviderType: "deepseek"}, client)
				request := adapter.Request{ProviderName: "test", Model: "requested-model"}
				var state *adapter.ResponseState
				if stream {
					usageCount, stateCount, toolCount := 0, 0, 0
					for event := range backend.Stream(context.Background(), request) {
						switch event.Type {
						case EventError:
							require.NoError(t, event.Error)
						case EventUsage:
							usageCount++
							assert.Nil(t, state)
						case adapter.EventResponseState:
							stateCount++
							assert.Equal(t, 1, usageCount)
							assert.Zero(t, toolCount)
							state = event.State
						case EventToolCallEnd:
							toolCount++
							require.NotNil(t, state)
							assert.Equal(t, "call-1", event.Delta.ToolID)
						}
					}
					assert.Equal(t, 1, usageCount)
					assert.Equal(t, 1, stateCount)
					assert.Equal(t, tc.hasTools, toolCount > 0)
				} else {
					result, err := backend.Generate(context.Background(), request)
					require.NoError(t, err)
					assert.Equal(t, tc.stopReason, result.StopReason)
					state = result.State
				}
				require.Equal(t, &adapter.ResponseState{
					Origin: adapter.MessageOrigin{
						AdapterName: adapter.ProtocolOpenAIChat, ProviderName: "test", ProviderType: "deepseek", Model: request.Model,
					},
					Complete: true, TurnComplete: tc.turnComplete, StopReason: tc.stopReason,
				}, state)

				history := adapter.Message{Role: "assistant", Content: "answer", ThinkingContent: "thinking", State: state}
				stored, err := json.Marshal(history)
				require.NoError(t, err)
				var loaded adapter.Message
				require.NoError(t, json.Unmarshal(stored, &loaded))
				require.Equal(t, history, loaded)
				outgoing, err := backend.buildMessages([]adapter.Message{loaded})
				require.NoError(t, err)
				assert.Equal(t, []map[string]any{{"role": "assistant", "content": "answer", "reasoning_content": "thinking"}}, outgoing)
			})
		}
	}
}

func TestStreamResponseStateRequiresTerminalSignal(t *testing.T) {
	for _, tc := range []struct {
		name      string
		terminal  string
		wantState bool
	}{
		{"finish reason without DONE", "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n", true},
		{"DONE without finish reason", "data: [DONE]\n\n", true},
		{"bare EOF", "", false},
		{"null reason then EOF", "data: {\"choices\":[{\"finish_reason\":null}]}\n\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"}}]}\n\n" + tc.terminal
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(wire))}, nil
			})}
			backend := newTestAdapter(t, adapter.Provider{ChatURL: "https://offline.invalid"}, client)
			var state *adapter.ResponseState
			for event := range backend.Stream(context.Background(), adapter.Request{ProviderName: "test", Model: "model"}) {
				if event.Type == adapter.EventResponseState {
					require.Nil(t, state)
					state = event.State
				}
				if event.Type == EventError {
					require.NoError(t, event.Error)
				}
			}
			assert.Equal(t, tc.wantState, state != nil)
		})
	}
}

func TestStreamFailuresDoNotPublishResponseState(t *testing.T) {
	for _, tc := range []struct {
		name      string
		wire      string
		readError error
	}{
		{"empty response", "data: [DONE]\n\n", nil},
		{"invalid tool group", "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", nil},
		{"read error after terminal chunk", "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\n", io.ErrUnexpectedEOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				var body io.Reader = strings.NewReader(tc.wire)
				if tc.readError != nil {
					body = io.MultiReader(body, iotest.ErrReader(tc.readError))
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(body)}, nil
			})}
			backend := newTestAdapter(t, adapter.Provider{ChatURL: "https://offline.invalid"}, client)
			errorsSeen := 0
			for event := range backend.Stream(context.Background(), adapter.Request{ProviderName: "test", Model: "model"}) {
				assert.NotEqual(t, adapter.EventResponseState, event.Type)
				assert.NotEqual(t, EventToolCallEnd, event.Type)
				if event.Type == EventError {
					errorsSeen++
				}
			}
			assert.Equal(t, 1, errorsSeen)
		})
	}
}
