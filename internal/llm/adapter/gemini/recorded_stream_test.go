package gemini

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	types "google.golang.org/genai/interactions/models/interactions"
	"google.golang.org/genai/interactions/models/operations"
	"gopkg.in/yaml.v3"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

type recordedExchange struct {
	When struct {
		Body string `yaml:"body"`
	} `yaml:"when"`
	Then struct {
		Status int `yaml:"status"`
		Header []struct {
			Name  string `yaml:"name"`
			Value string `yaml:"value"`
		} `yaml:"header"`
		Body string `yaml:"body"`
	} `yaml:"then"`
}

func recordedExchanges(t *testing.T, name string, count int) []recordedExchange {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "rig", name))
	if err != nil {
		t.Fatal(err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var exchanges []recordedExchange
	for {
		var exchange recordedExchange
		if err := decoder.Decode(&exchange); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if exchange.Then.Status != 200 || exchange.Then.Body == "" {
			t.Fatal("recording has no successful response body")
		}
		exchanges = append(exchanges, exchange)
	}
	if len(exchanges) != count {
		t.Fatalf("recording contains %d exchanges; want %d", len(exchanges), count)
	}
	return exchanges
}

type recordedParameters struct {
	Model                     string            `json:"model"`
	SystemInstruction         string            `json:"system_instruction"`
	Input                     []json.RawMessage `json:"input"`
	Store, Background, Stream *bool
	PreviousInteractionID     *string `json:"previous_interaction_id"`
	Tools                     []struct {
		Name, Description string
		Parameters        json.RawMessage
	}
}

func recordedRequest(t *testing.T, exchange recordedExchange) adapter.Request {
	t.Helper()
	var params recordedParameters
	if err := json.Unmarshal([]byte(exchange.When.Body), &params); err != nil {
		t.Fatal(err)
	}
	req := adapter.Request{ProviderName: "relay", Model: params.Model}
	if params.SystemInstruction != "" {
		req.Messages = append(req.Messages, adapter.Message{Role: "system", Content: params.SystemInstruction})
	}
	for _, raw := range params.Input {
		var step struct {
			Type    string
			Content []struct{ Type, Text string }
		}
		if err := json.Unmarshal(raw, &step); err != nil || step.Type != "user_input" {
			t.Fatalf("initial recorded request is not a user input: %v", err)
		}
		var text strings.Builder
		for _, content := range step.Content {
			if content.Type != "text" {
				t.Fatal("recorded user input is not text")
			}
			text.WriteString(content.Text)
		}
		req.Messages = append(req.Messages, adapter.Message{Role: "user", Content: text.String()})
	}
	for _, tool := range params.Tools {
		req.Tools = append(req.Tools, adapter.ToolDefinition{Name: tool.Name, Description: tool.Description, InputSchema: tool.Parameters})
	}
	return req
}

func recordedAdapter(t *testing.T, path string, exchanges []recordedExchange) (*Adapter, <-chan recordedParameters) {
	t.Helper()
	captured := make(chan recordedParameters, len(exchanges))
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.RequestURI() != path+"?alt=sse" || r.Header.Get("Api-Revision") != "2026-05-20" ||
			r.Header.Get("X-Goog-Api-Key") != "replay-only" || r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("recorded replay changed the endpoint, revision or credentials: %s %v", r.URL, r.Header)
		}
		var params recordedParameters
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			return nil, err
		}
		if params.Stream == nil || !*params.Stream || params.Store == nil || *params.Store || params.Background == nil || *params.Background || params.PreviousInteractionID != nil {
			t.Error("replaying a response enabled provider-managed state or disabled streaming")
		}
		index := int(calls.Add(1)) - 1
		if index >= len(exchanges) {
			return nil, fmt.Errorf("replay made an unexpected request %d", index)
		}
		captured <- params
		exchange := exchanges[index]
		header := make(http.Header)
		for _, item := range exchange.Then.Header {
			header.Add(item.Name, item.Value)
		}
		return &http.Response{StatusCode: exchange.Then.Status, Header: header, Body: io.NopCloser(strings.NewReader(exchange.Then.Body))}, nil
	})}
	backend, err := New(adapter.ProviderMap{"relay": {Name: "relay", ProviderType: "google", APIKey: "replay-only", ChatURL: "https://recording.invalid" + path + "?alt=sse"}}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	return backend, captured
}

func recordedStreamMessage(t *testing.T, events []adapter.StreamEvent) (adapter.Message, map[string]any) {
	t.Helper()
	message := adapter.Message{Role: "assistant"}
	var usage map[string]any
	thinking := false
	for _, event := range events {
		switch event.Type {
		case adapter.EventError:
			t.Fatal(event.Error)
		case adapter.EventContent:
			message.Content += event.Data
		case adapter.EventThinking:
			message.HasThinking, thinking = true, true
			message.ThinkingContent += event.Data
		case adapter.EventThinkingDone:
			thinking = false
		case adapter.EventToolCallEnd:
			message.ToolCalls = append(message.ToolCalls, adapter.ToolCall{ID: event.Delta.ToolID, Name: event.Delta.ToolName, Arguments: event.Delta.ArgumentsJSON})
		case adapter.EventResponseState:
			message.State = event.State
		case adapter.EventUsage:
			usage = event.Usage
		}
	}
	if message.State == nil || !message.State.Complete || usage == nil {
		t.Fatal("recorded stream did not publish complete state and usage")
	}
	if thinking {
		t.Fatal("recorded stream did not close its thinking phase")
	}
	return message, usage
}

type recordedSDKOutput struct {
	text, signature, arguments, callID, toolName, status string
}

// recordedSDKOutputFor 独立走生成的 SDK 事件解码器，以原录制内容核对本包的归一结果。
func recordedSDKOutputFor(t *testing.T, exchange recordedExchange) recordedSDKOutput {
	t.Helper()
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(exchange.Then.Body))}, nil
	})}
	p := newTestProvider(t, "https://recording.invalid/v1beta/interactions", "replay-only", client)
	var params types.CreateModelInteraction
	if err := json.Unmarshal([]byte(exchange.When.Body), &params); err != nil {
		t.Fatal(err)
	}
	result, err := p.client.Create(context.Background(), operations.CreateInteractionRequest{Body: operations.NewCreateInteractionRequestBody(params)})
	if err != nil {
		t.Fatal(err)
	}
	if result.InteractionSSEStreamEvent == nil {
		t.Fatal("recorded response was not decoded as SDK SSE")
	}
	stream := result.InteractionSSEStreamEvent
	defer stream.Close()
	var output recordedSDKOutput
	for stream.Next() {
		event := stream.Value()
		if start := event.GetDataStepStart(); start != nil && start.GetStepFunctionCall() != nil {
			call := start.GetStepFunctionCall()
			output.callID, output.toolName = call.ID, call.Name
		}
		if delta := event.GetDataStepDelta(); delta != nil {
			if text := delta.GetDeltaText(); text != nil {
				output.text += text.Text
			}
			if signature := delta.GetDeltaThoughtSignature(); signature != nil && signature.Signature != nil {
				output.signature = *signature.Signature
			}
			if arguments := delta.GetDeltaArgumentsDelta(); arguments != nil && arguments.Arguments != nil {
				output.arguments += *arguments.Arguments
			}
		}
		if completed := event.GetDataInteractionCompleted(); completed != nil {
			output.status = string(completed.Interaction.Status)
		}
	}
	if stream.Err() != nil || output.signature == "" || output.status == "" {
		t.Fatalf("SDK did not decode the recorded signature and terminal status: %v", stream.Err())
	}
	return output
}

func recordedNativeSteps(t *testing.T, message adapter.Message, signature string) []json.RawMessage {
	t.Helper()
	var payload replayPayload
	if err := json.Unmarshal(message.State.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	var steps []json.RawMessage
	if err := json.Unmarshal(payload.Steps, &steps); err != nil || len(steps) != 2 {
		t.Fatalf("recorded native steps were lost: %v", err)
	}
	var thought struct {
		Type, Signature string
		Summary         []json.RawMessage
	}
	if err := json.Unmarshal(steps[0], &thought); err != nil || thought.Type != "thought" || thought.Signature != signature || len(thought.Summary) != 0 {
		t.Fatalf("signature-only thought was changed or a summary was fabricated: %v", err)
	}
	return steps
}

func TestRecordedFixturesMatchPinnedSources(t *testing.T) {
	raw, err := os.ReadFile("testdata/rig/sources.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Files []struct {
			File, SHA256 string
		}
	}
	if err := json.Unmarshal(raw, &manifest); err != nil || len(manifest.Files) != 4 {
		t.Fatalf("invalid recorded fixture manifest: %v", err)
	}
	for _, file := range manifest.Files {
		data, err := os.ReadFile(filepath.Join("testdata", "rig", file.File))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != file.SHA256 {
			t.Fatalf("%s differs from the pinned upstream source", file.File)
		}
	}
}

func TestRecordedToolLoopSurvivesMessageStorage(t *testing.T) {
	exchanges := recordedExchanges(t, "interactions-tool-loop.yaml", 2)
	first, second := recordedSDKOutputFor(t, exchanges[0]), recordedSDKOutputFor(t, exchanges[1])
	for _, path := range []string{"/v1beta/interactions", "/v1/interactions"} {
		t.Run(path, func(t *testing.T) {
			backend, captured := recordedAdapter(t, path, exchanges)
			req := recordedRequest(t, exchanges[0])
			message, usage := recordedStreamMessage(t, collectStream(t, backend.Stream(context.Background(), req)))
			<-captured
			if message.Content != first.text || message.ThinkingContent != "" || !message.HasThinking || message.State.TurnComplete || message.State.StopReason != first.status || first.status != "requires_action" || len(message.ToolCalls) != 1 {
				t.Fatal("recorded tool response lost its text, call or requires_action status")
			}
			call := message.ToolCalls[0]
			if call.ID != first.callID || call.Name != first.toolName || call.Name != "add" || !sameJSON(call.Arguments, []byte(first.arguments)) || !sameJSON(call.Arguments, []byte(`{"x":17,"y":25}`)) {
				t.Fatal("recorded provider call ID or streamed arguments were changed")
			}
			if usage["prompt_tokens"] != float64(102) || usage["completion_tokens"] != float64(95) || usage["total_tokens"] != float64(197) {
				t.Fatalf("recorded tool-turn usage was miscounted: %v", usage)
			}
			native := recordedNativeSteps(t, message, first.signature)
			stored, err := json.Marshal(message)
			if err != nil {
				t.Fatal(err)
			}
			var loaded adapter.Message
			if err := json.Unmarshal(stored, &loaded); err != nil {
				t.Fatal(err)
			}
			req.Messages = append(req.Messages, loaded, adapter.Message{Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: "42"})
			answer, usage := recordedStreamMessage(t, collectStream(t, backend.Stream(context.Background(), req)))
			sent := <-captured
			if answer.Content != second.text || answer.Content != "42" || answer.ThinkingContent != "" || !answer.HasThinking || len(answer.ToolCalls) != 0 || !answer.State.TurnComplete || answer.State.StopReason != "completed" {
				t.Fatal("recorded tool continuation did not complete with the original answer")
			}
			if usage["prompt_tokens"] != float64(211) || usage["completion_tokens"] != float64(2) || usage["total_tokens"] != float64(213) {
				t.Fatalf("recorded continuation usage was miscounted: %v", usage)
			}
			recordedNativeSteps(t, answer, second.signature)
			if len(sent.Input) != 4 {
				t.Fatalf("expected user input, original thought/call and tool result; got %d steps", len(sent.Input))
			}
			for i, step := range native {
				if !sameJSON(step, sent.Input[i+1]) {
					t.Fatalf("native step %d changed after message storage", i)
				}
			}
			var result struct {
				Type, Name string
				CallID     string                        `json:"call_id"`
				Content    []struct{ Type, Text string } `json:"result"`
			}
			if json.Unmarshal(sent.Input[3], &result) != nil || result.Type != "function_result" || result.CallID != call.ID || result.Name != call.Name || len(result.Content) != 1 || result.Content[0].Type != "text" || result.Content[0].Text != "42" {
				t.Fatal("outbound tool result lost its association or text")
			}
		})
	}
}

func TestRecordedThoughtWithoutSummaryRetainsSignature(t *testing.T) {
	exchanges := recordedExchanges(t, "interactions-signature-only.yaml", 1)
	want := recordedSDKOutputFor(t, exchanges[0])
	for _, path := range []string{"/v1beta/interactions", "/v1/interactions"} {
		t.Run(path, func(t *testing.T) {
			backend, _ := recordedAdapter(t, path, exchanges)
			req := recordedRequest(t, exchanges[0])
			message, usage := recordedStreamMessage(t, collectStream(t, backend.Stream(context.Background(), req)))
			if message.Content != want.text || message.Content != "11" || message.ThinkingContent != "" || len(message.ToolCalls) != 0 || !message.State.TurnComplete || message.State.StopReason != want.status || want.status != "completed" {
				t.Fatal("signature-only recording changed the answer or fabricated reasoning text")
			}
			if usage["prompt_tokens"] != float64(33) || usage["completion_tokens"] != float64(1598) || usage["total_tokens"] != float64(1631) {
				t.Fatalf("recorded signature-only usage was miscounted: %v", usage)
			}
			recordedNativeSteps(t, message, want.signature)
		})
	}
}
