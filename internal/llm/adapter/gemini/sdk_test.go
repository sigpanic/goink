package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	types "google.golang.org/genai/interactions/models/interactions"
)

func sdkStreamFixture() string {
	var out strings.Builder
	for _, event := range []string{
		`{"event_type":"interaction.created","interaction":{"id":"interaction_stream","status":"in_progress"}}`,
		`{"event_type":"step.start","index":0,"step":{"type":"model_output"}}`,
		`{"event_type":"step.delta","index":0,"delta":{"type":"text","text":"ok"}}`,
		`{"event_type":"step.stop","index":0}`,
		`{"event_type":"interaction.completed","interaction":{"id":"interaction_stream","status":"completed","usage":{"total_input_tokens":1,"total_output_tokens":1,"total_tokens":2}}}`,
	} {
		fmt.Fprintf(&out, "data: %s\n\n", event)
	}
	out.WriteString("data: [DONE]\n\n")
	return out.String()
}

func TestSDKDecodesV1StreamingEventsAtCompleteEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI != "/v8/interactions?route=stream" || r.Header.Get("X-Goog-Api-Key") != "key" {
			t.Error("SSE request lost the complete endpoint or instance credentials")
		}
		var params struct{ Stream, Store *bool }
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil || params.Stream == nil || !*params.Stream || params.Store == nil || *params.Store {
			t.Errorf("invalid stateless streaming parameters: %+v %v", params, err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sdkStreamFixture())
	}))
	defer server.Close()
	p := newTestProvider(t, server.URL+"/v8/interactions?route=stream", "key", server.Client())
	res, err := p.client.Create(context.Background(), sdkRequest(true))
	if err != nil {
		t.Fatal(err)
	}
	if res.Interaction != nil || res.InteractionSSEStreamEvent == nil {
		t.Fatal("streaming SDK call did not return an event stream")
	}
	stream := res.InteractionSSEStreamEvent
	defer stream.Close()
	var text string
	created, completed, events := false, false, 0
	for stream.Next() {
		event := stream.Value()
		events++
		created = created || event.GetDataInteractionCreated() != nil
		completed = completed || event.GetDataInteractionCompleted() != nil
		if delta := event.GetDataStepDelta(); delta != nil && delta.GetDeltaText() != nil {
			text += delta.GetDeltaText().Text
		}
	}
	if err := stream.Err(); err != nil || events != 5 || text != "ok" || !created || !completed {
		t.Fatalf("SDK SSE shape mismatch: events=%d text=%q created=%v completed=%v err=%v", events, text, created, completed, err)
	}
}

func TestSDKRequestsAndStreamsUseCallerContext(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
			started, closed := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// 读完请求体后，HTTP/1 服务端才能在等待期间观察到客户端断开。
				_, _ = io.Copy(io.Discard, r.Body)
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					w.(http.Flusher).Flush()
				}
				close(started)
				select {
				case <-r.Context().Done():
				case <-time.After(5 * time.Second):
				}
				close(closed)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			p := newTestProvider(t, server.URL+"/v1/interactions", "key", server.Client())
			done := make(chan error, 1)
			go func() {
				res, err := p.client.Create(ctx, sdkRequest(streaming))
				if err == nil && streaming {
					stream := res.InteractionSSEStreamEvent
					defer stream.Close()
					for stream.Next() {
					}
					err = stream.Err()
				}
				done <- err
			}()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("SDK did not reach the shared transport")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("SDK lost context cancellation: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("SDK request ignored cancellation")
			}
			select {
			case <-closed:
			case <-time.After(3 * time.Second):
				t.Fatal("SDK did not release the HTTP connection")
			}
		})
	}
}

func TestSDKTypedStepsRequireSeparateRawPreservation(t *testing.T) {
	const raw = `{"type":"function_call","id":"call_1","name":"read","arguments":{"chapter":9007199254740993},"extension":{"keep":true}}`
	var step types.Step
	if err := json.Unmarshal([]byte(raw), &step); err != nil || step.FunctionCallStep == nil {
		t.Fatalf("SDK could not decode function_call: %v", err)
	}
	encoded, err := json.Marshal(step)
	if err != nil {
		t.Fatal(err)
	}
	// 固定已安装 SDK 的限制，防止后续直接序列化类型对象冒充原生回放。
	if strings.Contains(string(encoded), "9007199254740993") || strings.Contains(string(encoded), "extension") {
		t.Fatal("SDK preservation behavior changed; re-evaluate the raw JSON strategy")
	}
	if !strings.Contains(string(encoded), "9007199254740992") {
		t.Fatalf("unexpected SDK number representation: %s", encoded)
	}
}

func TestSDKRequiresStreamSentinelEvenAfterTerminalEvent(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(strings.TrimSuffix(sdkStreamFixture(), "data: [DONE]\n\n")))}, nil
	})}
	p := newTestProvider(t, "https://configured.invalid/v1/interactions", "key", client)
	res, err := p.client.Create(context.Background(), sdkRequest(true))
	if err != nil {
		t.Fatal(err)
	}
	stream := res.InteractionSSEStreamEvent
	defer stream.Close()
	for stream.Next() {
	}
	if !errors.Is(stream.Err(), io.ErrUnexpectedEOF) {
		t.Fatalf("SDK termination behavior changed: %v", stream.Err())
	}
}
