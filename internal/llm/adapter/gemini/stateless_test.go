package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// 以下样本依据 2026-10-27 官方 /v1/interactions、store=false 的实测结构构造。
// 保留缺失 ID / 首帧状态的形态，正文和签名用固定测试值；不是原始 HTTP 录制。
const statelessCreatedEvent = `{"event_type":"interaction.created","interaction":{"object":"interaction","model":"gemini-3.1-flash-lite"}}`
const statelessStatusEvent = `{"event_type":"interaction.status_update","status":"in_progress"}`
const statelessCompletedEvent = `{"event_type":"interaction.completed","interaction":{"status":"completed","object":"interaction","model":"gemini-3.1-flash-lite","usage":{"total_input_tokens":4,"total_output_tokens":2,"total_thought_tokens":0,"total_tokens":6}}}`
const statelessTextStream = "event: interaction.created\ndata: " + statelessCreatedEvent + "\n\n" +
	"event: interaction.status_update\ndata: " + statelessStatusEvent + "\n\n" +
	"event: step.start\ndata: {\"event_type\":\"step.start\",\"index\":0,\"step\":{\"type\":\"thought\"}}\n\n" +
	"event: step.delta\ndata: {\"event_type\":\"step.delta\",\"index\":0,\"delta\":{\"type\":\"thought_signature\",\"signature\":\"stateless-signature\"}}\n\n" +
	"event: step.stop\ndata: {\"event_type\":\"step.stop\",\"index\":0}\n\n" +
	"event: step.start\ndata: {\"event_type\":\"step.start\",\"index\":1,\"step\":{\"type\":\"model_output\"}}\n\n" +
	"event: step.delta\ndata: {\"event_type\":\"step.delta\",\"index\":1,\"delta\":{\"type\":\"text\",\"text\":\"OK.\"}}\n\n" +
	"event: step.stop\ndata: {\"event_type\":\"step.stop\",\"index\":1}\n\n" +
	"event: interaction.completed\ndata: " + statelessCompletedEvent + "\n\nevent: done\ndata: [DONE]\n\n"
const statelessTextResponse = `{"object":"interaction","model":"gemini-3.1-flash-lite","status":"completed","steps":[{"type":"thought","signature":"stateless-signature"},{"type":"model_output","content":[{"type":"text","text":"OK."}]}],"usage":{"total_input_tokens":4,"total_output_tokens":2,"total_thought_tokens":0,"total_tokens":6}}`

func TestStatelessGenerateAndStreamWithoutInteractionMetadata(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
			var requests atomic.Int32
			backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				var params recordedParameters
				if err := json.NewDecoder(r.Body).Decode(&params); err != nil || params.Store == nil || *params.Store || params.Stream == nil || *params.Stream != streaming || params.PreviousInteractionID != nil {
					t.Errorf("invalid stateless request: %+v %v", params, err)
				}
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, statelessTextStream)
				} else {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, statelessTextResponse)
				}
			})
			req := requestFixture()
			req.Model = "gemini-3.1-flash-lite"
			var response adapter.Response
			if streaming {
				for _, event := range collectStream(t, backend.Stream(context.Background(), req)) {
					switch event.Type {
					case adapter.EventError:
						t.Fatal(event.Error)
					case adapter.EventContent:
						response.Content += event.Data
					case adapter.EventResponseState:
						response.State = event.State
					case adapter.EventUsage:
						response.Usage = event.Usage
					}
				}
			} else {
				var err error
				response, err = backend.Generate(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
			}
			if requests.Load() != 1 || response.Content != "OK." || response.State == nil || !response.State.Complete || !response.State.TurnComplete || response.Usage["total_tokens"] != float64(6) {
				t.Fatalf("stateless response failed to complete: %+v", response)
			}
			if !strings.Contains(string(response.State.Payload), "stateless-signature") {
				t.Fatal("stateless response lost its native signature")
			}
		})
	}
}

func TestResponseRequiresIdentityUnlessStoreIsExplicitlyFalse(t *testing.T) {
	for _, store := range []*bool{nil, ptr(true), ptr(false)} {
		for _, id := range []string{"", `"id":"stored",`, `"id":null,`, `"id":123,`, `"id":"",`} {
			name := "default"
			if store != nil {
				name = fmt.Sprint(*store)
			}
			t.Run(name+"/"+id, func(t *testing.T) {
				p := requestProvider()
				params, err := p.buildRequest(requestFixture())
				if err != nil {
					t.Fatal(err)
				}
				params.Store = store
				raw := strings.Replace(statelessTextResponse, `{`, `{`+id, 1)
				response, err := p.convertResponse(params, json.RawMessage(raw))
				wantSuccess := id == `"id":"stored",` || (store != nil && !*store)
				if (err == nil) != wantSuccess || (response.State != nil) != wantSuccess {
					t.Fatalf("incorrect response identity validation: response=%+v err=%v", response, err)
				}
			})
		}
	}
}

func TestStatelessStreamValidatesPresentMetadataAndCompletion(t *testing.T) {
	withID := func(event, id string) string {
		return strings.Replace(event, `"interaction":{`, `"interaction":{"id":`+id+`,`, 1)
	}
	for _, tc := range []struct {
		name, created, update, completed string
		wantError                        bool
	}{
		{name: "all identity fields omitted"},
		{name: "identity only in created", created: withID(statelessCreatedEvent, `"a"`)},
		{name: "identity only in terminal", completed: withID(statelessCompletedEvent, `"a"`)},
		{name: "late identity", update: `{"event_type":"interaction.status_update","interaction_id":"a","status":"in_progress"}`, completed: withID(statelessCompletedEvent, `"a"`)},
		{name: "identity conflict across missing update", created: withID(statelessCreatedEvent, `"a"`), completed: withID(statelessCompletedEvent, `"b"`)},
		{name: "late identity conflict", update: `{"event_type":"interaction.status_update","interaction_id":"a","status":"in_progress"}`, completed: withID(statelessCompletedEvent, `"b"`)},
		{name: "numeric created ID", created: withID(statelessCreatedEvent, `123`)},
		{name: "null created ID", created: withID(statelessCreatedEvent, `null`)},
		{name: "empty created ID", created: withID(statelessCreatedEvent, `""`)},
		{name: "numeric update ID", update: `{"event_type":"interaction.status_update","interaction_id":123,"status":"in_progress"}`},
		{name: "null update ID", update: `{"event_type":"interaction.status_update","interaction_id":null,"status":"in_progress"}`},
		{name: "numeric terminal ID", completed: withID(statelessCompletedEvent, `123`)},
		{name: "null created status", created: strings.Replace(statelessCreatedEvent, `"object":`, `"status":null,"object":`, 1)},
		{name: "terminal status in created", created: strings.Replace(statelessCreatedEvent, `"object":`, `"status":"completed","object":`, 1)},
		{name: "empty created interaction", created: `{"event_type":"interaction.created","interaction":{}}`, wantError: true},
		{name: "missing update status", update: `{"event_type":"interaction.status_update"}`, wantError: true},
		{name: "missing terminal status", completed: strings.Replace(statelessCompletedEvent, `"status":"completed",`, "", 1)},
		{name: "nonterminal completed event", completed: strings.Replace(statelessCompletedEvent, `"completed"`, `"in_progress"`, 1), wantError: true},
		{name: "failed interaction", completed: strings.Replace(statelessCompletedEvent, `"completed"`, `"failed"`, 1), wantError: true},
		{name: "incomplete interaction", completed: strings.Replace(statelessCompletedEvent, `"completed"`, `"incomplete"`, 1), wantError: true},
		{name: "tool status without functions", completed: strings.Replace(statelessCompletedEvent, `"completed"`, `"requires_action"`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := statelessTextStream
			for _, replacement := range [][2]string{{statelessCreatedEvent, tc.created}, {statelessStatusEvent, tc.update}, {statelessCompletedEvent, tc.completed}} {
				if replacement[1] != "" {
					body = strings.Replace(body, replacement[0], replacement[1], 1)
				}
			}
			var closed atomic.Int32
			backend := streamWithReader(t, strings.NewReader(body), "text/event-stream", &closed)
			events := collectStream(t, backend.Stream(context.Background(), requestFixture()))
			if closed.Load() != 1 {
				t.Fatalf("body closed %d times", closed.Load())
			}
			if tc.wantError {
				requireStreamFailure(t, events)
				return
			}
			var complete bool
			for _, event := range events {
				if event.Type == adapter.EventError {
					t.Fatal(event.Error)
				}
				if event.Type == adapter.EventResponseState {
					complete = event.State.Complete && event.State.TurnComplete
				}
			}
			if !complete {
				t.Fatal("stream did not complete")
			}
		})
	}
}

func TestStoredStreamStillRequiresIdentityAndInitialStatus(t *testing.T) {
	for _, store := range []*bool{nil, ptr(true), ptr(false)} {
		for _, tc := range []struct {
			name, body    string
			statelessOnly bool
		}{
			{name: "stored response", body: sdkStreamFixture()},
			{name: "stateless response", body: statelessTextStream, statelessOnly: true},
			{name: "missing initial status", body: strings.Replace(sdkStreamFixture(), `,"status":"in_progress"`, "", 1), statelessOnly: true},
			{name: "missing terminal ID", body: strings.Replace(sdkStreamFixture(), `"id":"interaction_stream","status":"completed"`, `"status":"completed"`, 1), statelessOnly: true},
		} {
			name := "default"
			if store != nil {
				name = fmt.Sprint(*store)
			}
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				p := requestProvider()
				params, err := p.buildRequest(requestFixture())
				if err != nil {
					t.Fatal(err)
				}
				params.Store = store
				state := streamState{stateless: store != nil && !*store}
				body := strings.Split(tc.body, "event: done")[0]
				body = strings.TrimSuffix(body, "data: [DONE]\n\n")
				events := newInteractionEventStream(context.Background(), strings.NewReader(body))
				defer events.Close()
				for events.Next() {
					if err = state.consume(events.Value().Data, func(adapter.StreamEvent) bool { return true }); err != nil {
						break
					}
				}
				if err == nil {
					err = events.Err()
				}
				if err == nil {
					_, err = state.finish(p, params)
				}
				wantSuccess := !tc.statelessOnly || (store != nil && !*store)
				if (err == nil) != wantSuccess {
					t.Fatalf("incorrect stored stream validation: %v", err)
				}
			})
		}
	}
}

func TestStatelessStreamRejectsTruncationAndUnclosedSteps(t *testing.T) {
	for _, tc := range []struct {
		name, remove string
	}{
		{name: "no terminal event", remove: "event: interaction.completed\ndata: " + statelessCompletedEvent + "\n\n"},
		{name: "unclosed step", remove: "event: step.stop\ndata: {\"event_type\":\"step.stop\",\"index\":1}\n\n"},
		{name: "missing signature", remove: "event: step.delta\ndata: {\"event_type\":\"step.delta\",\"index\":0,\"delta\":{\"type\":\"thought_signature\",\"signature\":\"stateless-signature\"}}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Replace(statelessTextStream, tc.remove, "", 1)
			if body == statelessTextStream {
				t.Fatal("fixture replacement did not match")
			}
			var closed atomic.Int32
			backend := streamWithReader(t, strings.NewReader(body), "text/event-stream", &closed)
			events := collectStream(t, backend.Stream(context.Background(), requestFixture()))
			if tc.name == "missing signature" {
				for _, event := range events {
					if event.Type == adapter.EventError {
						t.Fatal(event.Error)
					}
				}
				return
			}
			requireStreamFailure(t, events)
		})
	}
}

func TestStatelessToolContinuationWithoutInteractionIDs(t *testing.T) {
	fixture := strings.ReplaceAll(toolStreamFixture(t), `"id":"interaction_stream_tools",`, "")
	fixture = strings.ReplaceAll(fixture, `"interaction_id":"interaction_stream_tools",`, "")
	fixture = strings.Replace(fixture, "event: done", "event: interaction.completed\ndata: {\"event_type\":\"interaction.completed\",\"interaction\":{\"status\":\"requires_action\"}}\n\nevent: done", 1)
	var requests atomic.Int32
	captured := make(chan recordedParameters, 2)
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		count := requests.Add(1)
		var params recordedParameters
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil || params.Store == nil || *params.Store || params.PreviousInteractionID != nil {
			t.Errorf("tool continuation enabled server-managed history: %+v %v", params, err)
		}
		captured <- params
		if count == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, fixture)
		} else {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, statelessTextResponse)
		}
	})
	req := requestFixture()
	req.Tools = requestTools()
	message := adapter.Message{Role: "assistant"}
	for _, event := range collectStream(t, backend.Stream(context.Background(), req)) {
		switch event.Type {
		case adapter.EventError:
			t.Fatal(event.Error)
		case adapter.EventContent:
			message.Content += event.Data
		case adapter.EventThinking:
			message.ThinkingContent += event.Data
		case adapter.EventResponseState:
			message.State = event.State
		case adapter.EventToolCallEnd:
			message.ToolCalls = append(message.ToolCalls, adapter.ToolCall{ID: event.Delta.ToolID, Name: event.Delta.ToolName, Arguments: event.Delta.ArgumentsJSON})
		}
	}
	if message.State == nil || message.State.TurnComplete || len(message.ToolCalls) != 2 || message.ToolCalls[0].ID != "a" || message.ToolCalls[1].ID != "b" {
		t.Fatalf("stateless stream lost tool IDs or waiting state: %+v", message)
	}
	stored, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var loaded adapter.Message
	if err := json.Unmarshal(stored, &loaded); err != nil {
		t.Fatal(err)
	}
	req.Messages = append(req.Messages, loaded, adapter.Message{Role: "tool", ToolCallID: "b", Content: "第二章"}, adapter.Message{Role: "tool", ToolCallID: "a", Content: "第一章"})
	response, err := backend.Generate(context.Background(), req)
	if err != nil || response.Content != "OK." || response.State == nil || !response.State.TurnComplete || requests.Load() != 2 {
		t.Fatalf("stateless tool continuation failed: response=%+v err=%v", response, err)
	}
	<-captured
	replayed := (<-captured).Input
	var payload replayPayload
	if err := json.Unmarshal(loaded.State.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	var steps []json.RawMessage
	if err := json.Unmarshal(payload.Steps, &steps); err != nil || len(replayed) != len(steps)+3 {
		t.Fatalf("incorrect stateless replay length: %d %v", len(replayed), err)
	}
	for i, step := range steps {
		if !sameJSON(step, replayed[i+1]) {
			t.Fatalf("native step %d changed during stateless continuation", i)
		}
	}
	for i, callID := range []string{"a", "b"} {
		var result struct {
			Type   string `json:"type"`
			CallID string `json:"call_id"`
		}
		if err := json.Unmarshal(replayed[len(steps)+1+i], &result); err != nil || result.Type != "function_result" || result.CallID != callID {
			t.Fatalf("tool result lost call pairing: %+v %v", result, err)
		}
	}
}
