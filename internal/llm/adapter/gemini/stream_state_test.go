package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestStreamVerifiesCanonicalTerminalSnapshots(t *testing.T) {
	fixture := toolStreamFixture(t)
	var closed atomic.Int32
	backend := streamWithReader(t, strings.NewReader(fixture), "text/event-stream", &closed)
	var payload replayPayload
	for _, event := range collectStream(t, backend.Stream(context.Background(), requestFixture())) {
		if event.Type == adapter.EventError {
			t.Fatal(event.Error)
		}
		if event.Type == adapter.EventResponseState {
			if err := json.Unmarshal(event.State.Payload, &payload); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(payload.Steps) == 0 {
		t.Fatal("baseline stream did not produce native steps")
	}
	for _, tc := range []struct {
		name      string
		edit      func([]map[string]json.RawMessage)
		wantError bool
	}{
		{name: "original snapshot", edit: func(steps []map[string]json.RawMessage) {
			steps[0]["snapshot_extension"] = json.RawMessage(`{"n":9007199254740999}`)
		}},
		{name: "different signature", edit: func(steps []map[string]json.RawMessage) {
			steps[0]["signature"] = json.RawMessage(`"changed-signature"`)
		}},
		{name: "different arguments", edit: func(steps []map[string]json.RawMessage) {
			steps[2]["arguments"] = json.RawMessage(`{"chapter":9007199254740992}`)
		}, wantError: true},
		{name: "different text", edit: func(steps []map[string]json.RawMessage) {
			steps[1]["content"] = json.RawMessage(`[{"type":"text","text":"changed"}]`)
		}},
		{name: "different step type", edit: func(steps []map[string]json.RawMessage) { steps[2]["type"] = json.RawMessage(`"google_search_call"`) }, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var steps []map[string]json.RawMessage
			if err := json.Unmarshal(payload.Steps, &steps); err != nil {
				t.Fatal(err)
			}
			tc.edit(steps)
			snapshot, err := json.Marshal(steps)
			if err != nil {
				t.Fatal(err)
			}
			terminal := fmt.Sprintf("data: {\"event_type\":\"interaction.completed\",\"interaction\":{\"id\":\"interaction_stream_tools\",\"status\":\"requires_action\",\"steps\":%s}}\n\n", snapshot)
			body := strings.Replace(fixture, "event: done\ndata: [DONE]", terminal+"event: done\ndata: [DONE]", 1)
			var closed atomic.Int32
			backend := streamWithReader(t, strings.NewReader(body), "text/event-stream", &closed)
			events := collectStream(t, backend.Stream(context.Background(), requestFixture()))
			if tc.wantError {
				requireStreamFailure(t, events)
				return
			}
			var state *adapter.ResponseState
			for _, event := range events {
				if event.Type == adapter.EventError {
					t.Fatal(event.Error)
				}
				if event.Type == adapter.EventResponseState {
					state = event.State
				}
			}
			if state == nil || (tc.name == "original snapshot" && !strings.Contains(string(state.Payload), "snapshot_extension")) {
				t.Fatal("canonical terminal steps lost their original extensions")
			}
			if tc.name != "original snapshot" && strings.Contains(string(state.Payload), "changed") {
				t.Fatal("divergent terminal metadata replaced the accumulated native state")
			}
		})
	}
}

func TestStreamRejectsChangesAfterASignatureOrTerminalStatus(t *testing.T) {
	for _, events := range [][]string{
		{`{"event_type":"step.start","index":0,"step":{"type":"thought"}}`},
		{
			`{"event_type":"interaction.created","interaction":{"id":"test","status":"in_progress"}}`,
			`{"event_type":"interaction.created","interaction":{"id":"test","status":"in_progress"}}`,
		},
		{
			`{"event_type":"interaction.created","interaction":{"id":"test","status":"in_progress"}}`,
			`{"event_type":"step.start","index":0,"step":{"type":"thought"}}`,
			`{"event_type":"step.delta","index":0,"delta":{"type":"thought_signature","signature":"first"}}`,
			`{"event_type":"step.delta","index":0,"delta":{"type":"thought_signature","signature":"second"}}`,
		},
		{
			`{"event_type":"interaction.created","interaction":{"id":"test","status":"in_progress"}}`,
			`{"event_type":"step.start","index":0,"step":{"type":"thought","signature":"final"}}`,
			`{"event_type":"step.delta","index":0,"delta":{"type":"thought_summary","content":{"type":"text","text":"late summary"}}}`,
		},
		{
			`{"event_type":"interaction.created","interaction":{"id":"test","status":"in_progress"}}`,
			`{"event_type":"interaction.status_update","interaction_id":"test","status":"requires_action"}`,
			`{"event_type":"interaction.completed","interaction":{"id":"test","status":"completed"}}`,
		},
	} {
		var body strings.Builder
		for _, event := range events {
			fmt.Fprintf(&body, "data: %s\n\n", event)
		}
		body.WriteString("data: [DONE]\n\n")
		var closed atomic.Int32
		backend := streamWithReader(t, strings.NewReader(body.String()), "text/event-stream", &closed)
		requireStreamFailure(t, collectStream(t, backend.Stream(context.Background(), requestFixture())))
	}
}
