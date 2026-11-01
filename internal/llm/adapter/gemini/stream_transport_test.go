package gemini

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func requireStreamFailure(t *testing.T, events []adapter.StreamEvent) error {
	t.Helper()
	var failure error
	for _, event := range events {
		switch event.Type {
		case adapter.EventError:
			if failure != nil {
				t.Fatal("stream reported more than one error")
			}
			failure = event.Error
		case adapter.EventToolCallEnd, adapter.EventUsage, adapter.EventResponseState:
			t.Fatalf("failed stream delivered a successful final event: %+v", event)
		}
	}
	if failure == nil {
		t.Fatal("stream did not report an error")
	}
	return failure
}

func streamWithReader(t *testing.T, reader io.Reader, contentType string, closed *atomic.Int32) *Adapter {
	t.Helper()
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: countedResponseBody{Reader: reader, closed: closed}}, nil
	})}
	p := requestProvider()
	backend, err := New(adapter.ProviderMap{p.Name: p.Provider}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	return backend
}

func TestStreamRequiresProtocolCompletionAndPreservesReadErrors(t *testing.T) {
	complete := strings.TrimSuffix(sdkStreamFixture(), "data: [DONE]\n\n")
	for _, tc := range []struct {
		name      string
		reader    io.Reader
		wantError bool
		cause     error
	}{
		{name: "terminal plus sentinel", reader: strings.NewReader(sdkStreamFixture())},
		{name: "terminal plus clean EOF", reader: strings.NewReader(complete)},
		{name: "sentinel without terminal", reader: strings.NewReader("data: [DONE]\n\n"), wantError: true, cause: io.ErrUnexpectedEOF},
		{name: "EOF before terminal", reader: strings.NewReader(strings.Split(complete, `data: {"event_type":"interaction.completed"`)[0]), wantError: true, cause: io.ErrUnexpectedEOF},
		{name: "broken connection after terminal", reader: io.MultiReader(strings.NewReader(complete), iotest.ErrReader(io.ErrUnexpectedEOF)), wantError: true, cause: io.ErrUnexpectedEOF},
		{name: "cancelled read", reader: iotest.ErrReader(context.Canceled), wantError: true, cause: context.Canceled},
		{name: "malformed tail after terminal", reader: strings.NewReader(complete + "data: {\n\n"), wantError: true},
		{name: "unknown event after terminal", reader: strings.NewReader(complete + "data: {\"event_type\":\"future_event\"}\n\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var closed atomic.Int32
			backend := streamWithReader(t, tc.reader, "text/event-stream", &closed)
			events := collectStream(t, backend.Stream(context.Background(), requestFixture()))
			if closed.Load() != 1 {
				t.Fatalf("response body was closed %d times", closed.Load())
			}
			if tc.wantError {
				err := requireStreamFailure(t, events)
				if tc.cause != nil && !errors.Is(err, tc.cause) {
					t.Fatalf("read error chain lost: %v", err)
				}
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
			if state == nil || !state.Complete || !state.TurnComplete {
				t.Fatalf("completed text stream did not produce complete state: %+v", state)
			}
		})
	}
}

func TestStreamMalformedHistoriesNeverDeliverPartOfAToolGroup(t *testing.T) {
	fixture := toolStreamFixture(t)
	for _, tc := range []struct {
		name, old, replacement string
	}{
		{name: "missing signature", old: `"signature":"opaque-stream-signature"`, replacement: `"signature":""`},
		{name: "null initial text", old: `"text":"先"`, replacement: `"text":null`},
		{name: "array arguments", old: `"id":"b","name":"read","arguments":{"chapter":2}`, replacement: `"id":"b","name":"read","arguments":[]`},
		{name: "invalid argument fragments", old: `"arguments":"9007199254740993}"`, replacement: `"arguments":"oops}"`},
		{name: "duplicate tool ID", old: `"id":"b","name":"read"`, replacement: `"id":"a","name":"read"`},
		{name: "complete initial arguments and fragments", old: `"id":"a","name":"read","arguments":{}`, replacement: `"id":"a","name":"read","arguments":{"chapter":1}`},
		{name: "unknown tool step", old: `"type":"function_call","id":"b"`, replacement: `"type":"google_search_call","id":"b"`},
		{name: "negative index", old: `"index":0`, replacement: `"index":-1`},
		{name: "missing index", old: `"index":0,`, replacement: ``},
		{name: "nonconsecutive index", old: `"index":1,"step"`, replacement: `"index":10,"step"`},
		{name: "delta on stopped step", old: `"index":2,"delta":{"type":"arguments_delta","arguments":"9007199254740993}"}`, replacement: `"index":3,"delta":{"type":"arguments_delta","arguments":"9007199254740993}"}`},
		{name: "interaction ID changed", old: `"interaction_id":"interaction_stream_tools"`, replacement: `"interaction_id":"other"`},
		{name: "completed with functions", old: `"status":"requires_action"`, replacement: `"status":"completed"`},
		{name: "incomplete", old: `"status":"requires_action"`, replacement: `"status":"incomplete"`},
		{name: "cancelled", old: `"status":"requires_action"`, replacement: `"status":"cancelled"`},
		{name: "failed", old: `"status":"requires_action"`, replacement: `"status":"failed"`},
		{name: "invalid usage", old: `"total_cached_tokens":3`, replacement: `"total_cached_tokens":8`},
		{name: "missing step stop", old: `data: {"event_type":"step.stop","index":4}`, replacement: ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Replace(fixture, tc.old, tc.replacement, 1)
			if body == fixture {
				t.Fatal("fixture edit did not match")
			}
			var closed atomic.Int32
			backend := streamWithReader(t, strings.NewReader(body), "text/event-stream", &closed)
			events := collectStream(t, backend.Stream(context.Background(), requestFixture()))
			switch tc.name {
			case "missing signature", "unknown tool step", "interaction ID changed", "completed with functions", "invalid usage":
				complete := false
				for _, event := range events {
					if event.Type == adapter.EventError {
						t.Fatal(event.Error)
					}
					complete = complete || event.Type == adapter.EventToolCallEnd
				}
				if !complete {
					t.Fatal("auxiliary data blocked valid tools")
				}
			default:
				requireStreamFailure(t, events)
			}
			if closed.Load() != 1 {
				t.Fatalf("failed stream leaked its response body: %d", closed.Load())
			}
		})
	}
}

func TestStreamReportsNativeErrorsAndHTTPMetadata(t *testing.T) {
	for _, status := range []int{200, 400, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var requests atomic.Int32
			backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Retry-After", "12")
				if status == 200 {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"event_type\":\"error\",\"error\":{\"code\":\"model_fault\",\"message\":\"test failure\"}}\n\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":{"code":"model_fault","message":"test failure"}}`)
			})
			err := requireStreamFailure(t, collectStream(t, backend.Stream(context.Background(), requestFixture())))
			var apiErr *adapter.APIError
			if !errors.As(err, &apiErr) || apiErr.Kind != "model_fault" || apiErr.Message != "test failure" || requests.Load() != 1 {
				t.Fatalf("native error lost or retried: %v", err)
			}
			if status != 200 && (apiErr.StatusCode != status || apiErr.RetryAfter != 12*time.Second || apiErr.Retryable != adapter.StatusRetryable(status)) {
				t.Fatalf("HTTP error metadata lost: %+v", apiErr)
			}
		})
	}
	var closed atomic.Int32
	backend := streamWithReader(t, strings.NewReader(sdkTextResponse), "application/json", &closed)
	requireStreamFailure(t, collectStream(t, backend.Stream(context.Background(), requestFixture())))
	if closed.Load() != 1 {
		t.Fatal("unexpected JSON response body was not released")
	}
}

func TestStreamCancelReleasesBlockedReaderAndConsumer(t *testing.T) {
	for _, blockedConsumer := range []bool{false, true} {
		t.Run(fmt.Sprint(blockedConsumer), func(t *testing.T) {
			started, released := make(chan struct{}), make(chan struct{})
			backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"event_type\":\"interaction.created\",\"interaction\":{\"id\":\"test\",\"status\":\"in_progress\"}}\n\n")
				fmt.Fprint(w, "data: {\"event_type\":\"step.start\",\"index\":0,\"step\":{\"type\":\"model_output\"}}\n\n")
				if blockedConsumer {
					for i := 0; i < 100; i++ {
						fmt.Fprint(w, "data: {\"event_type\":\"step.delta\",\"index\":0,\"delta\":{\"type\":\"text\",\"text\":\"x\"}}\n\n")
					}
				}
				w.(http.Flusher).Flush()
				close(started)
				select {
				case <-r.Context().Done():
				case <-time.After(5 * time.Second):
				}
				close(released)
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ch := backend.Stream(ctx, requestFixture())
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("stream did not start")
			}
			cancel()
			collectStream(t, ch)
			select {
			case <-released:
			case <-time.After(3 * time.Second):
				t.Fatal("cancelled stream did not release its connection")
			}
		})
	}
}
