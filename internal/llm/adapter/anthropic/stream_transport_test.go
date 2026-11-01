package anthropic

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func liveStreamPrefix() string {
	return streamFrame("message_start", `{"type":"message_start","message":{"id":"msg_live","type":"message","role":"assistant","content":[],"usage":{"input_tokens":2,"output_tokens":0}}}`) +
		streamFrame("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
		streamFrame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"live"}}`)
}

func liveStreamEnd() string {
	return streamFrame("content_block_stop", `{"type":"content_block_stop","index":0}`) +
		streamFrame("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`) +
		streamFrame("message_stop", `{"type":"message_stop"}`)
}

func TestStreamPublishesTextBeforeResponseFinishes(t *testing.T) {
	release := make(chan struct{})
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, liveStreamPrefix())
		w.(http.Flusher).Flush()
		select {
		case <-release:
			fmt.Fprint(w, liveStreamEnd())
		case <-r.Context().Done():
		}
	})
	_, req := testRequest()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := backend.Stream(ctx, req)
	select {
	case event := <-ch:
		if event.Type != adapter.EventContent || event.Data != "live" {
			t.Fatalf("first event is not live text: %+v", event)
		}
	case <-ctx.Done():
		t.Fatal("text was buffered until response completion")
	}
	close(release)
	var tail []adapter.StreamEventType
	for event := range ch {
		if event.Type == adapter.EventError {
			t.Fatal(event.Error)
		}
		tail = append(tail, event.Type)
	}
	if len(tail) != 2 || tail[0] != adapter.EventUsage || tail[1] != adapter.EventResponseState {
		t.Fatalf("unexpected text-only completion: %v", tail)
	}
}

func TestStreamCancellationClosesReadAndBlockedSend(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(fmt.Sprintf("backpressure=%v", blocked), func(t *testing.T) {
			written, stopped := make(chan struct{}), make(chan struct{})
			backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				defer close(stopped)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, liveStreamPrefix())
				if blocked {
					for range 100 {
						fmt.Fprint(w, streamFrame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"more"}}`))
					}
				}
				w.(http.Flusher).Flush()
				close(written)
				<-r.Context().Done()
			})
			_, req := testRequest()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ch := backend.Stream(ctx, req)
			select {
			case <-written:
			case <-time.After(3 * time.Second):
				t.Fatal("test server did not write the stream")
			}
			if blocked {
				deadline := time.NewTimer(3 * time.Second)
				defer deadline.Stop()
				ticker := time.NewTicker(time.Millisecond)
				defer ticker.Stop()
				for len(ch) < cap(ch) {
					select {
					case <-ticker.C:
					case <-deadline.C:
						t.Fatal("stream never reached consumer backpressure")
					}
				}
			} else {
				select {
				case event := <-ch:
					if event.Type != adapter.EventContent {
						t.Fatal("stream failed before cancellation")
					}
				case <-time.After(3 * time.Second):
					t.Fatal("stream did not produce text")
				}
			}
			cancel()
			deadline := time.NewTimer(3 * time.Second)
			defer deadline.Stop()
			draining := true
			for draining {
				select {
				case event, open := <-ch:
					if !open {
						draining = false
					} else if event.Type == adapter.EventResponseState || event.Type == adapter.EventToolCallEnd {
						t.Fatal("cancelled stream published complete state")
					}
				case <-deadline.C:
					t.Fatal("cancelled stream goroutine did not exit")
				}
			}
			select {
			case <-stopped:
			case <-time.After(3 * time.Second):
				t.Fatal("cancelled SDK stream did not close its response body")
			}
		})
	}
}

func TestStreamPreservesHTTPErrorsAndRejectsNonSSE(t *testing.T) {
	for _, tt := range []struct {
		name, contentType, body, kind string
		status                        int
	}{
		{"http-error", "application/json", `{"type":"error","error":{"type":"rate_limit_error","message":"limited"}}`, "rate_limit_error", 429},
		{"html", "text/html", "<html>relay homepage</html>", "invalid_response", 200},
		{"json", "application/json", generatedTextResponse, "invalid_response", 200},
		{"empty-stream", "text/event-stream", "", "incomplete_response", 200},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", tt.contentType)
				w.Header().Set("Retry-After", "12")
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			})
			_, req := testRequest()
			events := collectStream(t, backend, req)
			if len(events) != 1 || events[0].Type != adapter.EventError || requests.Load() != 1 {
				t.Fatalf("unexpected failed stream events: %v", events)
			}
			var apiErr *adapter.APIError
			if !errors.As(events[0].Error, &apiErr) || apiErr.Kind != tt.kind {
				t.Fatalf("wrong error: %v", events[0].Error)
			}
			if tt.status == 429 && (apiErr.StatusCode != 429 || apiErr.RetryAfter != 12*time.Second || !apiErr.Retryable) {
				t.Fatal("HTTP retry metadata was lost")
			}
			if tt.status == 429 {
				var cause *sdk.Error
				if !errors.As(events[0].Error, &cause) || cause.StatusCode != 429 || cause.Response == nil {
					t.Fatal("original SDK HTTP error was lost")
				}
			}
		})
	}
}

func TestStreamPreservesBodyReadErrors(t *testing.T) {
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Length", "99999")
		fmt.Fprint(w, liveStreamPrefix())
	})
	_, req := testRequest()
	events := collectStream(t, backend, req)
	if len(events) == 0 || events[len(events)-1].Type != adapter.EventError || !errors.Is(events[len(events)-1].Error, io.ErrUnexpectedEOF) {
		t.Fatalf("body read error was hidden: %v", events)
	}
	for _, event := range events {
		if event.Type == adapter.EventResponseState || event.Type == adapter.EventToolCallEnd {
			t.Fatal("read failure published complete state")
		}
	}
}

func TestStreamPreservesEmptyThinkingAndSignature(t *testing.T) {
	fixture := strings.Replace(streamToolsFixture(), streamFrame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"summary"}}`), "", 1)
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, fixture)
	})
	_, req := testRequest()
	var state *adapter.ResponseState
	for _, event := range collectStream(t, backend, req) {
		if event.Type == adapter.EventError {
			t.Fatal(event.Error)
		}
		if event.Type == adapter.EventThinking {
			if event.Data != "" {
				t.Fatal("signature or redacted thinking was emitted as visible text")
			}
		}
		if event.Type == adapter.EventResponseState {
			state = event.State
		}
	}
	if state == nil || !strings.Contains(string(state.Payload), `"thinking":""`) || !strings.Contains(string(state.Payload), `"signature":"sig-part"`) {
		t.Fatal("empty signed thinking block was lost")
	}
}

func TestStreamPreservesMaxTokensStopForText(t *testing.T) {
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, liveStreamPrefix()+strings.Replace(liveStreamEnd(), `"end_turn"`, `"max_tokens"`, 1))
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
	if state == nil || !state.Complete || state.TurnComplete || state.StopReason != "max_tokens" {
		t.Fatal("text max_tokens stop was confused with a transport failure or completed turn")
	}
}
