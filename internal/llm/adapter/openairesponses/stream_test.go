package openairesponses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func streamRequest() adapter.Request {
	req := requestInput()
	req.Model = "gpt-5.6-sol"
	req.Tools = []adapter.ToolDefinition{
		{Name: "read", InputSchema: json.RawMessage(`{"type":"object","properties":{"chapter":{"type":"integer"}}}`)},
		{Name: "lookup", InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`)},
	}
	return req
}

func recordedStream(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/responses-tools.sse")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func collectStream(t *testing.T, backend *Adapter, req adapter.Request) []adapter.StreamEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := backend.Stream(ctx, req)
	var events []adapter.StreamEvent
	for {
		select {
		case event, open := <-ch:
			if !open {
				return events
			}
			events = append(events, event)
		case <-ctx.Done():
			t.Fatal("stream failed to finish")
		}
	}
}

func replayStream(t *testing.T, wire string, readErr error) []adapter.StreamEvent {
	t.Helper()
	var attempts atomic.Int32
	var closed atomic.Bool
	backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts.Add(1)
		result := jsonResponse("")
		result.Header.Set("Content-Type", "text/event-stream; charset=utf-8")
		var reader io.Reader = strings.NewReader(wire)
		if readErr != nil {
			reader = io.MultiReader(reader, errorReader{readErr})
		}
		result.Body = &trackedBody{Reader: reader, closed: &closed}
		return result, nil
	}))
	events := collectStream(t, backend, streamRequest())
	if attempts.Load() != 1 || !closed.Load() {
		t.Fatal("Stream retried or did not close its response body")
	}
	return events
}

func streamFailure(t *testing.T, events []adapter.StreamEvent) error {
	t.Helper()
	var fault error
	for _, event := range events {
		switch event.Type {
		case adapter.EventResponseState, adapter.EventToolCallEnd, adapter.EventUsage:
			t.Fatal("failed stream exposed completed tools, usage or replay state")
		case adapter.EventError:
			if fault != nil || event.Error == nil {
				t.Fatal("failed stream must emit exactly one nonempty error")
			}
			fault = event.Error
		}
	}
	if fault == nil {
		t.Fatal("stream failure was treated as success")
	}
	return fault
}

func TestStreamReplayPreservesNativeToolLoopAndCumulativeArguments(t *testing.T) {
	fixture := recordedStream(t)
	for _, sentinel := range []string{"", "data: [DONE]\n\n"} {
		t.Run(fmt.Sprint(sentinel != ""), func(t *testing.T) {
			var requests []map[string]json.RawMessage
			backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost || r.URL.String() != "https://configured.invalid/proxy/v8/responses?route=1" || r.Header.Get("Authorization") != "Bearer instance-key" {
					t.Error("Stream lost its complete endpoint or instance credentials")
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				requests = append(requests, body)
				if len(requests) > 1 {
					return jsonResponse(generatedTextResponse), nil
				}
				if r.Header.Get("Accept") != "text/event-stream" || string(body["stream"]) != "true" ||
					string(body["store"]) != "false" || body["previous_response_id"] != nil || body["conversation"] != nil {
					t.Error("Stream did not send a single stateless SSE request")
				}
				result := jsonResponse(fixture + sentinel)
				result.Header.Set("Content-Type", "text/event-stream")
				return result, nil
			}))
			req := streamRequest()
			before, _ := json.Marshal(req)
			events := collectStream(t, backend, req)
			var text, thinking string
			var state *adapter.ResponseState
			var usage map[string]any
			var calls []adapter.ToolCall
			arguments := make(map[string][]string)
			var tail []adapter.StreamEventType
			for _, event := range events {
				switch event.Type {
				case adapter.EventError:
					t.Fatal(event.Error)
				case adapter.EventContent:
					text += event.Data
				case adapter.EventThinking:
					thinking += event.Data
				case adapter.EventToolCallStart:
					if event.Delta.ToolID == "fc_1" || event.Delta.ToolID == "fc_2" {
						t.Fatal("tool start used an item ID instead of call_id")
					}
				case adapter.EventToolCallArguments:
					arguments[event.Delta.ToolID] = append(arguments[event.Delta.ToolID], event.Delta.ArgumentsText)
				case adapter.EventUsage:
					usage = event.Usage
					tail = append(tail, event.Type)
				case adapter.EventResponseState:
					state = event.State
					tail = append(tail, event.Type)
				case adapter.EventToolCallEnd:
					calls = append(calls, adapter.ToolCall{ID: event.Delta.ToolID, Name: event.Delta.ToolName, Arguments: event.Delta.ArgumentsJSON})
					tail = append(tail, event.Type)
				}
			}
			if text != "previewno" || thinking != "plan" || len(requests) != 1 || len(calls) != 2 || calls[0].ID != "call_1" || calls[1].ID != "call_2" {
				t.Fatalf("replay lost text, refusal, summary or output-index tool order: %q %q %+v", text, thinking, calls)
			}
			if !reflect.DeepEqual(arguments["call_1"], []string{`{"chapter":`, `{"chapter":9007199254740993}`, `{"chapter":9007199254740993}`}) ||
				!reflect.DeepEqual(arguments["call_2"], []string{`{"query":"x"}`, `{"query":"x"}`}) {
				t.Fatalf("tool arguments are not cumulative and independent: %v", arguments)
			}
			if !reflect.DeepEqual(tail, []adapter.StreamEventType{adapter.EventUsage, adapter.EventResponseState, adapter.EventToolCallEnd, adapter.EventToolCallEnd}) ||
				state == nil || !state.Complete || state.TurnComplete || state.Origin.Model != req.Model || state.Origin.ProviderName != "relay" {
				t.Fatalf("invalid completion ordering or state: %v %+v", tail, state)
			}
			if usage["prompt_tokens"] != float64(20) || usage["completion_tokens"] != float64(13) || usage["total_tokens"] != float64(33) ||
				!reflect.DeepEqual(usage["completion_tokens_details"], map[string]any{"reasoning_tokens": float64(5)}) {
				t.Fatalf("stream usage double-counted reasoning: %v", usage)
			}
			var payload replayPayload
			if err := json.Unmarshal(state.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			var output []json.RawMessage
			if err := json.Unmarshal(payload.Output, &output); err != nil || len(output) != 5 {
				t.Fatalf("native output missing: %s", payload.Output)
			}
			var reasoning struct {
				EncryptedContent string `json:"encrypted_content"`
			}
			if err := json.Unmarshal(output[0], &reasoning); err != nil || reasoning.EncryptedContent != "complete-ciphertext" ||
				!strings.Contains(string(output[4]), "signature-only") || !strings.Contains(string(output[2]), "9007199254740995") {
				t.Fatal("native output did not preserve complete item.done ciphertext or precise extensions")
			}
			after, _ := json.Marshal(req)
			if string(before) != string(after) {
				t.Fatal("Stream mutated caller inputs")
			}
			message := adapter.Message{Role: "assistant", Content: text, ThinkingContent: thinking, ToolCalls: calls, State: state}
			stored, err := json.Marshal(message)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(stored, &message); err != nil {
				t.Fatal(err)
			}
			req.Model = "gpt-5.6-terra"
			req.Messages = append(req.Messages, message,
				adapter.Message{Role: "tool", ToolCallID: "call_2", ToolName: "lookup", Content: ""},
				adapter.Message{Role: "tool", ToolCallID: "call_1", ToolName: "read", Content: `{"result":42}`},
			)
			result, err := backend.Generate(context.Background(), req)
			if err != nil || result.Content != "done" || !result.State.TurnComplete || len(requests) != 2 {
				t.Fatalf("stream state could not continue after message storage: %+v %v", result, err)
			}
			var input []json.RawMessage
			if err := json.Unmarshal(requests[1]["input"], &input); err != nil || len(input) != 8 {
				t.Fatalf("invalid continuation input: %s", requests[1]["input"])
			}
			for i := range output {
				if !sameJSON(output[i], input[i+1]) {
					t.Fatalf("native replay changed output item %d: %s", i, input[i+1])
				}
			}
		})
	}
}

func TestStreamRejectsTruncationContradictionsAndUnsupportedItems(t *testing.T) {
	base := sdkStreamEvents()
	mutate := func(index int, old, replacement string) string {
		frames := append([]string(nil), base...)
		frames[index] = strings.Replace(frames[index], old, replacement, 1)
		return sseEvents(frames)
	}
	withTerminal := func(terminal string) string {
		return sseEvents(base[:15]) + sseEvents([]string{terminal})
	}
	for _, tt := range []struct{ name, wire, kind string }{
		{"empty", "", "incomplete_response"},
		{"early DONE", "data: [DONE]\n\n", "incomplete_response"},
		{"missing terminal", sseEvents(base[:15]), "incomplete_response"},
		{"unfinished final frame", strings.TrimRight(sseEvents(base), "\n"), "incomplete_response"},
		{"missing created", sseEvents(base[1:]), "invalid_response"},
		{"duplicate created", sseEvents(base[:1]) + sseEvents(base), "invalid_response"},
		{"negative output index", mutate(6, `"output_index":1`, `"output_index":-1`), "invalid_response"},
		{"fractional output index", mutate(6, `"output_index":1`, `"output_index":1.5`), "invalid_response"},
		{"string output index", mutate(6, `"output_index":1`, `"output_index":"1"`), "invalid_response"},
		{"orphan delta", mutate(6, `"output_index":1`, `"output_index":99`), "invalid_response"},
		{"wrong item ID", mutate(6, `"item_id":"msg_1"`, `"item_id":"other"`), "invalid_response"},
		{"duplicate item index", mutate(4, `"output_index":1`, `"output_index":0`), "invalid_response"},
		{"changed function call ID", mutate(14, `"call_id":"call_1"`, `"call_id":"call_2"`), "invalid_response"},
		{"changed done arguments", mutate(13, `9007199254740993`, `9007199254740995`), "tool_args_invalid"},
		{"invalid object arguments", mutate(13, `"{\"chapter\":9007199254740993}"`, `"[]"`), "tool_args_invalid"},
		{"malformed summary delta", mutate(2, `"delta":"plan"`, `"delta":123`), "invalid_response"},
		{"negative summary index", mutate(2, `"summary_index":0`, `"summary_index":-1`), "invalid_response"},
		{"unsupported item", mutate(10, `"type":"function_call"`, `"type":"web_search_call"`), "invalid_response"},
		{"terminal identity", mutate(15, `"id":"resp_1"`, `"id":"resp_other"`), "invalid_response"},
		{"terminal text", mutate(15, `"text":"ok"`, `"text":"changed"`), "invalid_response"},
		{"terminal ciphertext type", mutate(15, `"opaque-test-state"`, `123`), "invalid_response"},
		{"terminal summary", mutate(15, `"text":"plan"`, `"text":"changed"`), "invalid_response"},
		{"missing item done", sseEvents(base[:14]) + sseEvents(base[15:]), "invalid_response"},
		{"empty terminal output", withTerminal(`{"type":"response.completed","response":{"id":"resp_1","object":"response","status":"completed","output":[]}}`), "invalid_response"},
		{"late delta", sseEvents(base) + sseEvents(base[6:7]), "invalid_response"},
		{"failed", withTerminal(`{"type":"response.failed","response":{"id":"resp_1","object":"response","status":"failed","error":{"code":"server_error","message":"busy"}}}`), "server_error"},
		{"incomplete", withTerminal(`{"type":"response.incomplete","response":{"id":"resp_1","object":"response","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`), "response_incomplete"},
		{"flat error", sseEvents(base[:1]) + sseEvents([]string{`{"type":"error","code":"rate_limit_exceeded","message":"busy"}`}), "rate_limit_exceeded"},
		{"nested error", sseEvents(base[:1]) + sseEvents([]string{`{"error":{"code":"server_error","message":"busy"}}`}), "server_error"},
		{"malformed JSON", sseEvents(base[:1]) + "data: {broken}\n\n", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := streamFailure(t, replayStream(t, tt.wire, nil))
			if tt.kind != "" {
				var fault *adapter.APIError
				if !errors.As(err, &fault) || fault.Kind != tt.kind {
					t.Fatalf("wrong stream failure: %v", err)
				}
			}
		})
	}
	for _, replacement := range []string{`"namespace":"server"`, `"async":true`, `"caller":{"type":"program","program_id":"p_1"}`} {
		wire := strings.ReplaceAll(sdkStreamFixture(), `"extension":null`, replacement)
		streamFailure(t, replayStream(t, wire, nil))
	}
	streamFailure(t, replayStream(t, strings.ReplaceAll(recordedStream(t), `"call_2"`, `"call_1"`), nil))
}

func TestStreamPropagatesReadErrorsAfterTerminalWithoutCompletion(t *testing.T) {
	for _, wire := range []string{sseEvents(sdkStreamEvents()[:8]), sdkStreamFixture()} {
		err := streamFailure(t, replayStream(t, wire, io.ErrUnexpectedEOF))
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("read error was replaced by success or missing-terminal error: %v", err)
		}
	}
}

func TestStreamKeepsPrivateReasoningTextOutOfDisplay(t *testing.T) {
	frames := sdkStreamEvents()
	wire := sseEvents(frames[:2]) + sseEvents([]string{
		`{"type":"response.content_part.added","output_index":0,"item_id":"rs_1","content_index":0,"part":{"type":"reasoning_text","text":""}}`,
		`{"type":"response.reasoning_text.delta","output_index":0,"item_id":"rs_1","content_index":0,"delta":"private internal text"}`,
		`{"type":"response.reasoning_text.done","output_index":0,"item_id":"rs_1","content_index":0,"text":"private internal text"}`,
	}) + sseEvents(frames[2:])
	wire = strings.ReplaceAll(wire, sdkReasoning, strings.TrimSuffix(sdkReasoning, "}")+`,"content":[{"type":"reasoning_text","text":"private internal text"}]}`)
	var state *adapter.ResponseState
	for _, event := range replayStream(t, wire, nil) {
		if event.Type == adapter.EventError || strings.Contains(event.Data, "private internal") {
			t.Fatalf("private reasoning text affected public events: %+v", event)
		}
		if event.Type == adapter.EventResponseState {
			state = event.State
		}
	}
	if state == nil || !strings.Contains(string(state.Payload), "private internal text") {
		t.Fatal("private native reasoning was removed from replay state")
	}
}

func TestStreamTextOnlyCompletesWithoutToolCalls(t *testing.T) {
	base := sdkStreamEvents()
	frames := []string{base[0]}
	for _, frame := range base[4:10] {
		frames = append(frames, strings.ReplaceAll(frame, `"output_index":1`, `"output_index":0`))
	}
	frames = append(frames, `{"type":"response.completed","response":{"id":"resp_1","object":"response","status":"completed","output":[`+sdkMessage+`]}}`)
	var text string
	var state *adapter.ResponseState
	for _, event := range replayStream(t, sseEvents(frames), nil) {
		switch event.Type {
		case adapter.EventError:
			t.Fatal(event.Error)
		case adapter.EventContent:
			text += event.Data
		case adapter.EventResponseState:
			state = event.State
		case adapter.EventToolCallStart, adapter.EventToolCallArguments, adapter.EventToolCallEnd:
			t.Fatal("text-only stream invented a tool call")
		}
	}
	if text != "ok" || state == nil || !state.Complete || !state.TurnComplete || state.StopReason != "completed" {
		t.Fatalf("text-only stream did not complete the turn: %q %+v", text, state)
	}
}

func TestStreamRejectsHTTPFailuresAndWrongContentType(t *testing.T) {
	for _, status := range []int{200, 401, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var attempts atomic.Int32
			var closed atomic.Bool
			backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts.Add(1)
				result := jsonResponse("")
				result.StatusCode = status
				result.Header.Set("Retry-After", "7")
				result.Body = &trackedBody{Reader: strings.NewReader(`{"error":{"code":"test_error","message":"test failure"}}`), closed: &closed}
				return result, nil
			}))
			err := streamFailure(t, collectStream(t, backend, streamRequest()))
			var fault *adapter.APIError
			if !errors.As(err, &fault) || attempts.Load() != 1 || !closed.Load() {
				t.Fatalf("HTTP failure retried, leaked or lost error metadata: %v", err)
			}
			if status == 200 {
				if fault.Kind != "invalid_response" {
					t.Fatalf("JSON response was accepted as SSE: %v", err)
				}
			} else if fault.StatusCode != status || fault.Kind != "test_error" || fault.Retryable != adapter.StatusRetryable(status) || fault.RetryAfter != 7*time.Second {
				t.Fatalf("HTTP failure details lost: %+v", fault)
			}
		})
	}
}

func TestStreamRejectsInputsBeforeSending(t *testing.T) {
	var attempts atomic.Int32
	backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts.Add(1)
		return nil, errors.New("invalid request reached the transport")
	}))
	for _, kind := range []string{"provider", "model", "schema"} {
		req := streamRequest()
		switch kind {
		case "provider":
			req.ProviderName = "missing"
		case "model":
			req.Model = ""
		case "schema":
			req.Tools[0].InputSchema = json.RawMessage(`[]`)
		}
		streamFailure(t, collectStream(t, backend, req))
	}
	if attempts.Load() != 0 {
		t.Fatal("invalid request caused HTTP I/O")
	}
}

func TestStreamPublishesBeforeTerminalAndCancellationClosesConnection(t *testing.T) {
	fixture := recordedStream(t)
	cut := strings.LastIndex(fixture, "event: response.completed")
	if cut < 0 {
		t.Fatal("fixture has no terminal event")
	}
	for _, cancelEarly := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelEarly), func(t *testing.T) {
			release, stopped := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(stopped)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, fixture[:cut])
				w.(http.Flusher).Flush()
				select {
				case <-r.Context().Done():
					return
				case <-release:
					fmt.Fprint(w, fixture[cut:])
				}
			}))
			defer server.Close()
			backend, err := New(adapter.ProviderMap{"relay": {Name: "relay", ProviderType: "openai", ChatURL: server.URL + "/proxy/v8/responses?route=1", APIKey: "instance-key"}}, server.Client(), nil)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ch := backend.Stream(ctx, streamRequest())
			seenText, seenArguments := false, false
			for !seenText || !seenArguments {
				select {
				case event, open := <-ch:
					if !open || event.Type == adapter.EventError || event.Type == adapter.EventToolCallEnd || event.Type == adapter.EventResponseState {
						t.Fatalf("stream finished or failed before terminal release: %+v", event)
					}
					seenText = seenText || event.Type == adapter.EventContent
					seenArguments = seenArguments || event.Type == adapter.EventToolCallArguments
				case <-ctx.Done():
					t.Fatal("live events were withheld until the terminal response")
				}
			}
			if cancelEarly {
				cancel()
			} else {
				close(release)
			}
			ends, states := 0, 0
			timeout := time.NewTimer(3 * time.Second)
			defer timeout.Stop()
			for open := true; open; {
				select {
				case event, ok := <-ch:
					open = ok
					if ok {
						switch event.Type {
						case adapter.EventError:
							t.Fatal(event.Error)
						case adapter.EventToolCallEnd:
							ends++
						case adapter.EventResponseState:
							states++
						}
					}
				case <-timeout.C:
					t.Fatal("Stream did not close after completion/cancellation")
				}
			}
			if (cancelEarly && (ends != 0 || states != 0)) || (!cancelEarly && (ends != 2 || states != 1)) {
				t.Fatalf("unexpected tool/state completion: %d/%d", ends, states)
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("SDK stream did not release the server connection")
			}
		})
	}
}

func TestStreamCancellationUnblocksFullEventBuffer(t *testing.T) {
	frames := sdkStreamEvents()
	wire := sseEvents(frames[:6]) + strings.Repeat(sseEvents(frames[6:7]), 500)
	var closed atomic.Bool
	backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		result := jsonResponse("")
		result.Header.Set("Content-Type", "text/event-stream")
		result.Body = &trackedBody{Reader: strings.NewReader(wire), closed: &closed}
		return result, nil
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := backend.Stream(ctx, streamRequest())
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for len(ch) != cap(ch) {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("test did not fill the event buffer")
		}
	}
	cancel()
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	for {
		select {
		case event, open := <-ch:
			if !open {
				if !closed.Load() {
					t.Fatal("blocked producer did not close the network body")
				}
				return
			}
			if event.Type == adapter.EventToolCallEnd || event.Type == adapter.EventResponseState {
				t.Fatal("cancelled stream published completed state")
			}
		case <-timeout.C:
			t.Fatal("producer remained blocked after cancellation")
		}
	}
}
