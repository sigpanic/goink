package openairesponses

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

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

const sdkReasoning = `{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"plan"}],"encrypted_content":"opaque-test-state","extension":{"integer":9007199254740993}}`
const sdkMessage = `{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok","annotations":[]}]}`
const sdkFunction = `{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read","arguments":"{\"chapter\":9007199254740993}","status":"completed","extension":null}`
const sdkResponse = `{"id":"resp_1","object":"response","created_at":1,"status":"completed","model":"test-model","output":[` + sdkReasoning + `,` + sdkMessage + `,` + sdkFunction + `],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5},"extension":{"integer":9007199254740993}}`

// 本地合成事件只用于验证固定版本 SDK 的行为，不作为真实服务录制资料。
func sdkStreamEvents() []string {
	return []string{
		`{"type":"response.created","sequence_number":0,"response":{"id":"resp_1","object":"response","status":"in_progress","output":[]}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[]}}`,
		`{"type":"response.reasoning_summary_text.delta","output_index":0,"item_id":"rs_1","summary_index":0,"delta":"plan"}`,
		`{"type":"response.output_item.done","output_index":0,"item":` + sdkReasoning + `}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`,
		`{"type":"response.content_part.added","output_index":1,"item_id":"msg_1","content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`,
		`{"type":"response.output_text.delta","output_index":1,"item_id":"msg_1","content_index":0,"delta":"o"}`,
		`{"type":"response.output_text.delta","output_index":1,"item_id":"msg_1","content_index":0,"delta":"k"}`,
		`{"type":"response.output_text.done","output_index":1,"item_id":"msg_1","content_index":0,"text":"ok"}`,
		`{"type":"response.output_item.done","output_index":1,"item":` + sdkMessage + `}`,
		`{"type":"response.output_item.added","output_index":2,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read","arguments":""}}`,
		`{"type":"response.function_call_arguments.delta","output_index":2,"item_id":"fc_1","delta":"{\"chapter\":"}`,
		`{"type":"response.function_call_arguments.delta","output_index":2,"item_id":"fc_1","delta":"9007199254740993}"}`,
		`{"type":"response.function_call_arguments.done","output_index":2,"item_id":"fc_1","arguments":"{\"chapter\":9007199254740993}"}`,
		`{"type":"response.output_item.done","output_index":2,"item":` + sdkFunction + `}`,
		`{"type":"response.completed","response":` + sdkResponse + `}`,
	}
}

func sdkStreamFixture() string { return sseEvents(sdkStreamEvents()) }

func sseEvents(events []string) string {
	var out strings.Builder
	for _, event := range events {
		out.WriteString("data: " + event + "\n\n")
	}
	return out.String()
}

func TestSDKPreservesRawResponseAndReplayItems(t *testing.T) {
	var input json.RawMessage
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body struct{ Input json.RawMessage }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		input = body.Input
		return jsonResponse(sdkResponse), nil
	})}
	p := newTestProvider(t, "https://configured.invalid/v1/responses", "key", client)
	result, err := p.client.New(context.Background(), sdkRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.RawJSON() != sdkResponse || result.OutputText() != "ok" || result.Output[2].CallID != "call_1" || result.Output[2].ID != "fc_1" {
		t.Fatal("SDK lost native fields or confused item ID with call ID")
	}
	if result.Output[0].RawJSON() != sdkReasoning || result.Output[2].RawJSON() != sdkFunction {
		t.Fatal("SDK output items no longer retain their native JSON")
	}
	params := sdkRequest()
	params.Input = responses.ResponseNewParamsInputUnion{OfInputItemList: []responses.ResponseInputItemUnionParam{
		param.Override[responses.ResponseInputItemUnionParam](json.RawMessage(result.Output[0].RawJSON())),
		param.Override[responses.ResponseInputItemUnionParam](json.RawMessage(result.Output[2].RawJSON())),
	}}
	if _, err := p.client.New(context.Background(), params); err != nil {
		t.Fatal(err)
	}
	if string(input) != "["+sdkReasoning+","+sdkFunction+"]" {
		t.Fatalf("SDK replay changed signatures, numbers or extension fields: %s", input)
	}
}

func TestSDKStreamsExposeLiveEventsAndAccumulateNativeOutput(t *testing.T) {
	for _, sentinel := range []string{"", "data: [DONE]\n\n"} {
		t.Run(fmt.Sprint(sentinel != ""), func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				result := jsonResponse(sdkStreamFixture() + sentinel)
				result.Header.Set("Content-Type", "text/event-stream")
				return result, nil
			})}
			p := newTestProvider(t, "https://configured.invalid/v1/responses", "key", client)
			stream := p.client.NewStreaming(context.Background(), sdkRequest())
			defer stream.Close()
			var acc responses.ResponseAccumulator
			var terminal responses.Response
			text, thinking, arguments := "", "", ""
			for stream.Next() {
				event := stream.Current()
				// 固定版本累积器接收 WebSocket 事件类型；HTTP 事件原文可解码为同形事件。
				var observed responses.ResponsesServerEventUnion
				if err := json.Unmarshal([]byte(event.RawJSON()), &observed); err != nil {
					t.Fatal(err)
				}
				if err := acc.AddEvent(observed); err != nil {
					t.Fatalf("SDK accumulator rejected SSE event %s: %v", event.Type, err)
				}
				switch event.Type {
				case "response.output_text.delta":
					text += event.Delta
				case "response.reasoning_summary_text.delta":
					thinking += event.Delta
				case "response.function_call_arguments.delta":
					arguments += event.Delta
				case "response.completed":
					terminal = event.Response
					if terminal.RawJSON() != sdkResponse {
						t.Fatal("terminal response no longer retains its original JSON")
					}
				}
			}
			if err := stream.Err(); err != nil || text != "ok" || thinking != "plan" || arguments != `{"chapter":9007199254740993}` {
				t.Fatalf("live SDK events mismatch: text=%q thinking=%q args=%q error=%v", text, thinking, arguments, err)
			}
			snapshot := acc.Snapshot()
			if snapshot.TerminalEvent != "response.completed" || snapshot.OutputText() != "ok" || len(snapshot.Output) != 3 {
				t.Fatalf("unexpected SDK accumulated output: %+v", snapshot)
			}
			call := snapshot.Output[2]
			if call.ItemID != "fc_1" || call.CallID != "call_1" || call.Name != "read" || call.Arguments != arguments {
				t.Fatalf("SDK accumulated function call mismatch: %+v", call)
			}
			details := acc.DetailedSnapshot()
			if string(details.Output[0].Item["encrypted_content"]) != `"opaque-test-state"` || string(details.Output[0].Item["extension"]) != `{"integer":9007199254740993}` || string(details.Output[2].Item["extension"]) != "null" {
				t.Fatal("SDK detailed snapshot lost native state or exact extension fields")
			}
		})
	}
}

func TestSDKStreamEOFAndFailureDoNotImplySuccess(t *testing.T) {
	prefix := sseEvents(sdkStreamEvents()[:15])
	for _, tt := range []struct {
		name, suffix, terminal string
		readErr                error
	}{
		{name: "missing terminal"},
		{name: "failed", suffix: sseEvents([]string{`{"type":"response.failed","response":{"id":"resp_1","object":"response","status":"failed","error":{"code":"server_error","message":"failure"}}}`}), terminal: "response.failed"},
		{name: "incomplete", suffix: sseEvents([]string{`{"type":"response.incomplete","response":{"id":"resp_1","object":"response","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`}), terminal: "response.incomplete"},
		{name: "read failure after terminal", suffix: sseEvents(sdkStreamEvents()[15:]), terminal: "response.completed", readErr: io.ErrUnexpectedEOF},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				result := jsonResponse("")
				result.Header.Set("Content-Type", "text/event-stream")
				var body io.Reader = strings.NewReader(prefix + tt.suffix)
				if tt.readErr != nil {
					body = io.MultiReader(body, errorReader{tt.readErr})
				}
				result.Body = io.NopCloser(body)
				return result, nil
			})}
			p := newTestProvider(t, "https://configured.invalid/v1/responses", "key", client)
			stream := p.client.NewStreaming(context.Background(), sdkRequest())
			defer stream.Close()
			var acc responses.ResponseAccumulator
			for stream.Next() {
				var observed responses.ResponsesServerEventUnion
				if err := json.Unmarshal([]byte(stream.Current().RawJSON()), &observed); err != nil {
					t.Fatal(err)
				}
				if err := acc.AddEvent(observed); err != nil {
					t.Fatal(err)
				}
			}
			if !errors.Is(stream.Err(), tt.readErr) || acc.Snapshot().TerminalEvent != tt.terminal {
				t.Fatalf("stream termination changed: terminal=%q error=%v", acc.Snapshot().TerminalEvent, stream.Err())
			}
		})
	}
}

func TestSDKRequestAndStreamCancellationReleaseConnection(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
			started, stopped := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, sseEvents(sdkStreamEvents()[:1]))
					w.(http.Flusher).Flush()
				} else {
					close(started)
				}
				select {
				case <-r.Context().Done():
				case <-time.After(5 * time.Second):
				}
				close(stopped)
			}))
			defer server.Close()
			p := newTestProvider(t, server.URL+"/v1/responses", "key", server.Client())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if !streaming {
					_, err := p.client.New(ctx, sdkRequest())
					done <- err
					return
				}
				stream := p.client.NewStreaming(ctx, sdkRequest())
				defer stream.Close()
				for stream.Next() {
					if stream.Current().Type == "response.created" {
						// 服务端还在等待取消，此事件已可被调用方实时读取。
						close(started)
					}
				}
				done <- stream.Err()
			}()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("SDK did not expose the request or event before completion")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("SDK lost cancellation: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("SDK ignored cancellation")
			}
			select {
			case <-stopped:
			case <-time.After(3 * time.Second):
				t.Fatal("SDK did not release the HTTP connection")
			}
		})
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }
