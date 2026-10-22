package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func streamFrame(kind, data string) string { return "event: " + kind + "\ndata: " + data + "\n\n" }

func streamToolsFixture() string {
	return strings.Join([]string{
		streamFrame("message_start", `{"type":"message_start","message":{"id":"msg_stream","type":"message","role":"assistant","model":"relay-model","content":[],"stop_reason":null,"usage":{"input_tokens":5,"cache_creation_input_tokens":7,"cache_read_input_tokens":11,"output_tokens":0}}}`),
		streamFrame("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`),
		streamFrame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"summary"}}`),
		streamFrame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-"}}`),
		streamFrame("ping", `{"type":"ping"}`),
		streamFrame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"part"}}`),
		streamFrame("content_block_stop", `{"type":"content_block_stop","index":0}`),
		streamFrame("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"redacted_thinking","data":"opaque-data"}}`),
		streamFrame("content_block_stop", `{"type":"content_block_stop","index":1}`),
		streamFrame("content_block_start", `{"type":"content_block_start","index":2,"content_block":{"type":"text","text":"pre","extension":{"n":9007199254740995}}}`),
		streamFrame("content_block_delta", `{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"view"}}`),
		streamFrame("future_event", `{"type":"future_event","notice":"ignored by SDK"}`),
		streamFrame("content_block_delta", `{"type":"content_block_delta","index":2,"delta":{"type":"citations_delta","citation":{"type":"char_location","cited_text":"quote","document_index":0,"document_title":"chapter","start_char_index":0,"end_char_index":5}}}`),
		streamFrame("content_block_stop", `{"type":"content_block_stop","index":2}`),
		streamFrame("content_block_start", `{"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"call1","name":"read","input":{}}}`),
		streamFrame("content_block_delta", `{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{\"chapter\":"}}`),
		streamFrame("content_block_start", `{"type":"content_block_start","index":4,"content_block":{"type":"tool_use","id":"call2","name":"read","input":{"ready":true}}}`),
		streamFrame("content_block_stop", `{"type":"content_block_stop","index":4}`),
		streamFrame("content_block_delta", `{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"9007199254740993}"}}`),
		streamFrame("content_block_stop", `{"type":"content_block_stop","index":3}`),
		streamFrame("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}`),
		streamFrame("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":13,"output_tokens_details":{"thinking_tokens":3}}}`),
		streamFrame("message_stop", `{"type":"message_stop"}`),
	}, "")
}

func collectStream(t *testing.T, backend *Adapter, req adapter.Request) []adapter.StreamEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var events []adapter.StreamEvent
	for event := range backend.Stream(ctx, req) {
		events = append(events, event)
	}
	if ctx.Err() != nil {
		t.Fatal("stream did not complete before test deadline")
	}
	return events
}

func TestStreamReplaysNativeStateAndPublishesCompleteToolGroup(t *testing.T) {
	var requests atomic.Int32
	var replayed json.RawMessage
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Stream   bool
			Messages []struct{ Content json.RawMessage }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if r.RequestURI != "/relay/v8/messages?route=claude" {
			t.Error("stream changed the complete endpoint")
		}
		if requests.Add(1) == 1 {
			if !request.Stream {
				t.Error("missing stream flag")
			}
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			fmt.Fprint(w, streamToolsFixture())
		} else {
			if len(request.Messages) != 3 {
				t.Error("incorrect continuation history")
			} else {
				replayed = request.Messages[1].Content
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, generatedTextResponse)
		}
	})
	_, req := testRequest()
	events := collectStream(t, backend, req)
	var text, thinking strings.Builder
	var calls []adapter.ToolCall
	var state *adapter.ResponseState
	var usage map[string]any
	var argumentTexts []string
	for _, event := range events {
		switch event.Type {
		case adapter.EventError:
			t.Fatal(event.Error)
		case adapter.EventContent:
			text.WriteString(event.Data)
		case adapter.EventThinking:
			thinking.WriteString(event.Data)
		case adapter.EventToolCallArguments:
			argumentTexts = append(argumentTexts, event.Delta.ArgumentsText)
		case adapter.EventResponseState:
			state = event.State
		case adapter.EventUsage:
			if usage != nil {
				t.Fatal("cumulative usage was published more than once")
			}
			usage = event.Usage
		case adapter.EventToolCallEnd:
			calls = append(calls, adapter.ToolCall{ID: event.Delta.ToolID, Name: event.Delta.ToolName, Arguments: event.Delta.ArgumentsJSON})
		}
	}
	if text.String() != "preview" || thinking.String() != "summary" || len(calls) != 2 || state == nil || !state.Complete {
		t.Fatalf("missing stream output: text=%q, thinking=%q, calls=%v, state=%+v", text.String(), thinking.String(), calls, state)
	}
	if calls[0].ID != "call1" || string(calls[0].Arguments) != `{"chapter":9007199254740993}` || calls[1].ID != "call2" || string(calls[1].Arguments) != `{"ready":true}` {
		t.Fatal("interleaved tools lost their order or raw input")
	}
	if len(argumentTexts) != 2 || argumentTexts[0] != `{"chapter":` || argumentTexts[1] != `{"chapter":9007199254740993}` {
		t.Fatalf("tool argument events must carry accumulated text: %v", argumentTexts)
	}
	if usage["completion_tokens"] != float64(13) || usage["prompt_tokens"] != float64(23) || usage["total_tokens"] != float64(36) {
		t.Fatalf("usage was added instead of replaced: %v", usage)
	}
	for i, want := range []adapter.StreamEventType{adapter.EventUsage, adapter.EventResponseState, adapter.EventToolCallEnd, adapter.EventToolCallEnd} {
		if events[len(events)-4+i].Type != want {
			t.Fatal("tool group was published before complete usage and native state")
		}
	}
	var payload replayPayload
	if err := json.Unmarshal(state.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"signature":"sig-part"`, `opaque-data`, `9007199254740995`, `"citations"`} {
		if !strings.Contains(string(payload.Content), field) {
			t.Fatalf("stream accumulation lost %s: %s", field, payload.Content)
		}
	}
	message := adapter.Message{Role: "assistant", Content: text.String(), ThinkingContent: thinking.String(), ToolCalls: calls, State: state}
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var restored adapter.Message
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	req.Messages = append(req.Messages, restored,
		adapter.Message{Role: "tool", ToolCallID: "call1", Content: "first"},
		adapter.Message{Role: "tool", ToolCallID: "call2", Content: "second"})
	if _, err := backend.Generate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if !sameJSONArray(replayed, payload.Content) {
		t.Fatalf("streamed state changed on the next SDK request: %s", replayed)
	}
}

func TestStreamReplaysRecordedFixtures(t *testing.T) {
	for _, name := range []string{"anthropic-tool_runner_streaming_all-01.sse", "anthropic-tool_runner_streaming_all-02.sse", "anthropic-refusal-01.sse"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/sse/anthropic/" + name)
			if err != nil {
				t.Fatal(err)
			}
			backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Write(data)
			})
			_, req := testRequest()
			var state *adapter.ResponseState
			for _, event := range collectStream(t, backend, req) {
				if event.Type == adapter.EventError {
					t.Fatal(event.Error)
				}
				if event.Type == adapter.EventResponseState {
					state = event.State
				}
			}
			if state == nil || !state.Complete {
				t.Fatal("recorded Messages stream did not finish")
			}
		})
	}
}

func TestStreamFailureDoesNotPublishExecutableTools(t *testing.T) {
	fixture := streamToolsFixture()
	stop := streamFrame("message_stop", `{"type":"message_stop"}`)
	errorFrame := streamFrame("error", `{"type":"error","error":{"type":"overloaded_error","message":"try later"}}`)
	for _, tt := range []struct{ name, body, kind string }{
		{"missing-stop", strings.TrimSuffix(fixture, stop), "incomplete_response"},
		{"truncated-input", strings.Replace(fixture, `9007199254740993}`, `9007199254740993`, 1), "tool_args_invalid"},
		{"array-input", strings.Replace(fixture, `"input":{"ready":true}`, `"input":[]`, 1), "tool_args_invalid"},
		{"duplicate-tool", strings.Replace(fixture, `"id":"call2"`, `"id":"call1"`, 1), "invalid_response"},
		{"unknown-block", strings.Replace(fixture, `"type":"redacted_thinking"`, `"type":"future_block"`, 1), "invalid_response"},
		{"unknown-delta", strings.Replace(fixture, `"type":"thinking_delta"`, `"type":"future_delta"`, 1), "invalid_response"},
		{"numeric-signature", strings.Replace(fixture, `"signature":""`, `"signature":123`, 1), "invalid_response"},
		{"negative-index", strings.Replace(fixture, `"index":0`, `"index":-1`, 1), "invalid_response"},
		{"missing-index", strings.Replace(fixture, `"index":0,`, ``, 1), "invalid_response"},
		{"open-block", strings.Replace(fixture, streamFrame("content_block_stop", `{"type":"content_block_stop","index":4}`), "", 1), "invalid_response"},
		{"stream-error", strings.Replace(fixture, stop, errorFrame, 1), "overloaded_error"},
		{"error-after-stop", fixture + errorFrame, "overloaded_error"},
		{"second-message", fixture + fixture, "invalid_response"},
		{"malformed-json", strings.Replace(fixture, stop, streamFrame("message_stop", `{"type":`), 1), ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, tt.body)
			})
			_, req := testRequest()
			var finalError error
			for _, event := range collectStream(t, backend, req) {
				if event.Type == adapter.EventToolCallEnd || event.Type == adapter.EventResponseState || event.Type == adapter.EventUsage {
					t.Fatal("failed stream exposed complete usage, replay state or executable tools")
				}
				if event.Type == adapter.EventError {
					if finalError != nil {
						t.Fatal("stream emitted multiple terminal errors")
					}
					finalError = event.Error
				}
			}
			if finalError == nil {
				t.Fatal("failed stream completed without an error")
			}
			if tt.kind != "" {
				var apiErr *adapter.APIError
				if !errors.As(finalError, &apiErr) || apiErr.Kind != tt.kind {
					t.Fatalf("error=%v, want kind=%s", finalError, tt.kind)
				}
				if tt.kind == "overloaded_error" && (apiErr.StatusCode != 200 || !apiErr.Retryable || apiErr.Message != "try later") {
					t.Fatal("HTTP 200 SSE error lost its protocol details")
				}
			}
		})
	}
}
