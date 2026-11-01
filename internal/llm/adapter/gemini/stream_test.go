package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func toolStreamFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/interactions-tools.sse")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func collectStream(t *testing.T, ch <-chan adapter.StreamEvent) []adapter.StreamEvent {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	var events []adapter.StreamEvent
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, event)
		case <-timer.C:
			t.Fatal("stream channel did not close")
		}
	}
}

func TestStreamFixtureMatchesGeneratedSDKSchema(t *testing.T) {
	fixture := toolStreamFixture(t)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(fixture))}, nil
	})}
	p := newTestProvider(t, "https://relay.invalid/v8/interactions", "key", client)
	response, err := p.client.Create(context.Background(), sdkRequest(true))
	if err != nil {
		t.Fatal(err)
	}
	stream := response.InteractionSSEStreamEvent
	defer stream.Close()
	count := 0
	for stream.Next() {
		count++
	}
	if stream.Err() != nil || count != 20 {
		t.Fatalf("fixture does not match the installed SDK schema: count=%d err=%v", count, stream.Err())
	}
}

func TestStreamProducesReusableStateAndAtomicToolCompletion(t *testing.T) {
	fixture := toolStreamFixture(t)
	var requests atomic.Int32
	var replayed []json.RawMessage
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPost || r.RequestURI != "/relay/v8/interactions?route=gemini" || r.Header.Get("X-Goog-Api-Key") != "private-key" {
			t.Error("stream request changed the complete endpoint or credentials")
		}
		var request struct {
			Input                     []json.RawMessage `json:"input"`
			Stream, Store, Background *bool
			PreviousInteractionID     *string `json:"previous_interaction_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Stream == nil || request.Store == nil || *request.Store || request.Background == nil || *request.Background || request.PreviousInteractionID != nil {
			t.Errorf("invalid stateless request: %+v err=%v", request, err)
			return
		}
		if *request.Stream {
			if r.Header.Get("Accept") != "text/event-stream" {
				t.Error("stream request did not negotiate SSE")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, fixture)
			return
		}
		replayed = request.Input
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, sdkTextResponse)
	})
	req := requestFixture()
	req.Tools = requestTools()
	before, _ := json.Marshal(req)
	events := collectStream(t, backend.Stream(context.Background(), req))
	message := adapter.Message{Role: "assistant"}
	var usage map[string]any
	var arguments []string
	for _, event := range events {
		switch event.Type {
		case adapter.EventError:
			t.Fatalf("stream failed: %v", event.Error)
		case adapter.EventContent:
			message.Content += event.Data
		case adapter.EventThinking:
			message.ThinkingContent += event.Data
		case adapter.EventToolCallArguments:
			arguments = append(arguments, event.Delta.ArgumentsText)
		case adapter.EventToolCallEnd:
			message.ToolCalls = append(message.ToolCalls, adapter.ToolCall{ID: event.Delta.ToolID, Name: event.Delta.ToolName, Arguments: event.Delta.ArgumentsJSON})
		case adapter.EventUsage:
			usage = event.Usage
		case adapter.EventResponseState:
			message.State = event.State
		}
	}
	if requests.Load() != 1 || message.Content != "先读后" || message.ThinkingContent != "先思后想" || len(message.ToolCalls) != 2 {
		t.Fatalf("stream content or call group changed: %+v", message)
	}
	if message.ToolCalls[0].ID != "a" || string(message.ToolCalls[0].Arguments) != `{"chapter":9007199254740993}` || message.ToolCalls[1].ID != "b" {
		t.Fatal("parallel tool order or argument precision changed")
	}
	if !reflect.DeepEqual(arguments, []string{`{"chapter":`, `{"chapter":9007199254740993}`}) {
		t.Fatalf("arguments events are not cumulative: %q", arguments)
	}
	if message.State == nil || !message.State.Complete || message.State.TurnComplete || message.State.StopReason != "requires_action" {
		t.Fatalf("incorrect tool-waiting state: %+v", message.State)
	}
	wantUsage := map[string]any{
		"prompt_tokens": float64(7), "completion_tokens": float64(42), "total_tokens": float64(49),
		"prompt_cache_hit_tokens": float64(3), "prompt_cache_miss_tokens": float64(4),
		"completion_tokens_details": map[string]any{"reasoning_tokens": float64(22)}, "total_tool_use_tokens": float64(50),
	}
	if !reflect.DeepEqual(usage, wantUsage) {
		t.Fatalf("cumulative or step usage was added twice: %v", usage)
	}
	if len(events) < 4 || events[len(events)-4].Type != adapter.EventUsage || events[len(events)-3].Type != adapter.EventResponseState || events[len(events)-2].Type != adapter.EventToolCallEnd || events[len(events)-1].Type != adapter.EventToolCallEnd {
		t.Fatal("usage, state and tool group were not published together at successful completion")
	}
	after, _ := json.Marshal(req)
	if string(before) != string(after) {
		t.Fatal("Stream changed caller inputs")
	}
	stored, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), "private-key") {
		t.Fatal("stream state contains credentials")
	}
	var loaded adapter.Message
	if err := json.Unmarshal(stored, &loaded); err != nil {
		t.Fatal(err)
	}
	req.Model = "gemini-3.6-flash"
	req.Messages = append(req.Messages, loaded, adapter.Message{Role: "tool", ToolCallID: "b", Content: "第二章"}, adapter.Message{Role: "tool", ToolCallID: "a", Content: "第一章"})
	response, err := backend.Generate(context.Background(), req)
	if err != nil || response.Content != "ok" || requests.Load() != 2 {
		t.Fatalf("Stream -> storage -> Generate continuation failed: %+v err=%v", response, err)
	}
	var payload replayPayload
	if err := json.Unmarshal(message.State.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	var native []json.RawMessage
	if err := json.Unmarshal(payload.Steps, &native); err != nil || len(replayed) != len(native)+3 {
		t.Fatalf("incorrect native step count: replayed=%d err=%v", len(replayed), err)
	}
	for i, step := range native {
		if !sameJSON(step, replayed[i+1]) {
			t.Fatalf("native step %d changed after storage and model switch: %s", i, replayed[i+1])
		}
	}
	for _, value := range []string{"opaque-stream-signature", "9007199254740995", "9007199254740997", "url_citation"} {
		if !strings.Contains(string(payload.Steps), value) {
			t.Fatalf("stream dropped original signature or extensions: %s", value)
		}
	}
}

func TestStreamPublishesBeforeTheResponseCompletes(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fixture := sdkStreamFixture()
		parts := strings.SplitN(fixture, `data: {"event_type":"step.stop"`, 2)
		fmt.Fprint(w, parts[0])
		w.(http.Flusher).Flush()
		select {
		case <-release:
			fmt.Fprint(w, `data: {"event_type":"step.stop"`+parts[1])
		case <-r.Context().Done():
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := backend.Stream(ctx, requestFixture())
	select {
	case event := <-ch:
		if event.Type != adapter.EventContent || event.Data != "ok" {
			t.Fatalf("expected live text before terminal response: %+v", event)
		}
	case <-ctx.Done():
		t.Fatal("Stream waited for the full response before publishing text")
	}
	cancel()
	collectStream(t, ch)
}

func TestStreamAcceptsSSEFramingAndLargeUTF8Content(t *testing.T) {
	text := strings.Repeat("正文🙂", 10000)
	encoded, _ := json.Marshal(text)
	fixture := strings.Replace(sdkStreamFixture(), `"text":"ok"`, `"text":`+string(encoded), 1)
	// SDK 负责 BOM、注释、无 data 保活、多行 data、CRLF 和跨读取边界的 UTF-8。
	fixture = strings.ReplaceAll(fixture, `,"index":`, ",\ndata: \"index\":")
	fixture = "\ufeff: heartbeat\n\nid: keepalive\n\n" + fixture
	fixture = strings.ReplaceAll(fixture, "\n", "\r\n")
	var closed atomic.Int32
	backend := streamWithReader(t, strings.NewReader(fixture), "text/event-stream", &closed)
	events := collectStream(t, backend.Stream(context.Background(), requestFixture()))
	var got strings.Builder
	for _, event := range events {
		if event.Type == adapter.EventError {
			t.Fatal(event.Error)
		}
		if event.Type == adapter.EventContent {
			got.WriteString(event.Data)
		}
	}
	if got.String() != text || closed.Load() != 1 {
		t.Fatal("SDK SSE framing truncated UTF-8 content or leaked its connection")
	}
}
