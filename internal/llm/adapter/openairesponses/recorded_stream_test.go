package openairesponses

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

type recordedExchange struct {
	When struct {
		Path, Method, Body string
	}
	Then struct {
		Status int
		Header []struct{ Name, Value string }
		Body   string
	}
}

type recordedParameters struct {
	Model, Instructions       string
	Input, Tools              []json.RawMessage
	Include                   []string
	Store, Stream, Background *bool
	PreviousResponseID        json.RawMessage `json:"previous_response_id"`
	Conversation              json.RawMessage
}

func TestRecordedFixturesMatchPinnedSources(t *testing.T) {
	raw, err := os.ReadFile("testdata/rig/sources.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Files []struct{ File, SHA256 string }
	}
	if err := json.Unmarshal(raw, &manifest); err != nil || len(manifest.Files) != 2 {
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

func recordedExchanges(t *testing.T) []recordedExchange {
	t.Helper()
	raw, err := os.ReadFile("testdata/rig/responses-stateless-tools.yaml")
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
		if exchange.When.Path != "/v1/responses" || exchange.When.Method != "POST" || exchange.Then.Status != 200 {
			t.Fatal("recorded HTTP contract changed")
		}
		exchanges = append(exchanges, exchange)
	}
	if len(exchanges) != 2 {
		t.Fatalf("expected two recorded exchanges, got %d", len(exchanges))
	}
	return exchanges
}

func recordedOutput(t *testing.T, wire string) []json.RawMessage {
	t.Helper()
	var output []json.RawMessage
	var complete bool
	for _, line := range strings.Split(wire, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event struct {
			Type string
			Item json.RawMessage
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal("invalid recorded SSE JSON:", err)
		}
		if event.Type == "response.output_item.done" {
			output = append(output, event.Item)
		}
		complete = complete || event.Type == "response.completed"
	}
	if !complete || len(output) == 0 {
		t.Fatal("recording has no completed output")
	}
	return output
}

func TestRecordedStatelessStreamReplaysEncryptedParallelToolHistory(t *testing.T) {
	exchanges := recordedExchanges(t)
	var source [2]recordedParameters
	for i, exchange := range exchanges {
		if err := json.Unmarshal([]byte(exchange.When.Body), &source[i]); err != nil {
			t.Fatal(err)
		}
		if source[i].Store == nil || *source[i].Store || source[i].Stream == nil || !*source[i].Stream ||
			source[i].PreviousResponseID != nil || source[i].Conversation != nil || len(source[i].Tools) != 2 ||
			!reflect.DeepEqual(source[i].Include, []string{"reasoning.encrypted_content"}) {
			t.Fatal("recording no longer describes the expected stateless request")
		}
	}
	var user struct {
		Role    string
		Content []struct{ Type, Text string }
	}
	if len(source[0].Input) != 1 || json.Unmarshal(source[0].Input[0], &user) != nil || user.Role != "user" ||
		len(user.Content) != 1 || user.Content[0].Type != "input_text" {
		t.Fatal("recorded initial input is not a text user message")
	}
	var requests atomic.Int32
	backend, err := New(adapter.ProviderMap{"relay": {
		Name: "relay", ProviderType: "openai", APIKey: "replay-only", ChatURL: "https://recording.invalid/v1/responses",
	}}, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != "https://recording.invalid/v1/responses" ||
			r.Header.Get("Authorization") != "Bearer replay-only" || r.Header.Get("Accept") != "text/event-stream" {
			t.Error("recorded replay changed the endpoint, credentials or SSE request mode")
		}
		index := int(requests.Add(1)) - 1
		if index >= len(exchanges) {
			return nil, fmt.Errorf("unexpected recorded request %d", index+1)
		}
		var got recordedParameters
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			return nil, err
		}
		want := source[index]
		if got.Store == nil || *got.Store || got.Stream == nil || !*got.Stream || got.Background == nil || *got.Background ||
			got.PreviousResponseID != nil || got.Conversation != nil || got.Model != want.Model {
			t.Error("replay did not preserve stateless full-history request semantics")
		}
		if !reflect.DeepEqual(got.Include, want.Include) {
			t.Error("stateless replay did not request reasoning.encrypted_content")
		}
		if len(got.Tools) != len(want.Tools) || len(got.Input) != len(want.Input)+1 {
			return nil, fmt.Errorf("request %d changed tool or history counts", index+1)
		}
		for i, tool := range want.Tools {
			if !sameJSON(got.Tools[i], tool) {
				t.Errorf("request %d changed recorded tool %d", index+1, i)
			}
		}
		for i, text := range []string{want.Instructions, user.Content[0].Text} {
			var message struct{ Role, Content string }
			role := []string{"system", "user"}[i]
			if json.Unmarshal(got.Input[i], &message) != nil || message.Role != role || message.Content != text {
				t.Errorf("request %d changed recorded %s text", index+1, role)
			}
		}
		for i := 1; i < len(want.Input); i++ {
			var expected struct {
				Type, Output string
				CallID       string `json:"call_id"`
			}
			if err := json.Unmarshal(want.Input[i], &expected); err != nil {
				return nil, err
			}
			if expected.Type == "function_call_output" {
				var actual struct {
					Type, Output string
					CallID       string `json:"call_id"`
				}
				// status=completed 是录制中的可选字段；Goink 按公共文本工具结果编码。
				if json.Unmarshal(got.Input[i+1], &actual) != nil || actual != expected {
					t.Errorf("request %d changed recorded tool result %d", index+1, i)
				}
			} else if !sameJSON(got.Input[i+1], want.Input[i]) {
				t.Errorf("request %d changed native history item %d", index+1, i)
			}
		}
		header := make(http.Header)
		for _, item := range exchanges[index].Then.Header {
			header.Add(item.Name, item.Value)
		}
		return &http.Response{StatusCode: exchanges[index].Then.Status, Header: header, Body: io.NopCloser(strings.NewReader(exchanges[index].Then.Body))}, nil
	})}, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := adapter.Request{ProviderName: "relay", Model: source[0].Model, Messages: []adapter.Message{
		{Role: "system", Content: source[0].Instructions}, {Role: "user", Content: user.Content[0].Text},
	}}
	for _, raw := range source[0].Tools {
		var tool struct {
			Name, Description string
			Parameters        json.RawMessage
		}
		if err := json.Unmarshal(raw, &tool); err != nil {
			t.Fatal(err)
		}
		req.Tools = append(req.Tools, adapter.ToolDefinition{Name: tool.Name, Description: tool.Description, InputSchema: tool.Parameters})
	}
	for i, exchange := range exchanges {
		message := adapter.Message{Role: "assistant"}
		var usage map[string]any
		thinking := false
		for _, event := range collectStream(t, backend, req) {
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
			case adapter.EventResponseState:
				message.State = event.State
			case adapter.EventUsage:
				usage = event.Usage
			case adapter.EventToolCallEnd:
				message.ToolCalls = append(message.ToolCalls, adapter.ToolCall{ID: event.Delta.ToolID, Name: event.Delta.ToolName, Arguments: event.Delta.ArgumentsJSON})
			}
		}
		wantText := []string{"", "The labels are crimson-harbor and silver-orchard."}[i]
		if requests.Load() != int32(i+1) || message.Content != wantText || message.ThinkingContent != "" || !message.HasThinking || thinking || len(message.ToolCalls) != []int{2, 0}[i] ||
			message.State == nil || !message.State.Complete || message.State.TurnComplete != (i == 1) || message.State.StopReason != "completed" {
			t.Fatalf("recorded round %d lost text, tools or completion", i+1)
		}
		if usage["prompt_tokens"] != []float64{160, 402}[i] || usage["completion_tokens"] != []float64{226, 99}[i] || usage["total_tokens"] != []float64{386, 501}[i] ||
			!reflect.DeepEqual(usage["completion_tokens_details"], map[string]any{"reasoning_tokens": []float64{128, 64}[i]}) {
			t.Fatalf("recorded round %d changed usage: %v", i+1, usage)
		}
		var payload replayPayload
		if err := json.Unmarshal(message.State.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		var output []json.RawMessage
		wantOutput := recordedOutput(t, exchange.Then.Body)
		if err := json.Unmarshal(payload.Output, &output); err != nil || len(output) != len(wantOutput) {
			t.Fatalf("recorded round %d lost native output: %v", i+1, err)
		}
		for j, item := range wantOutput {
			if !sameJSON(item, output[j]) {
				t.Fatalf("recorded round %d changed completed output item %d", i+1, j)
			}
		}
		if i == len(exchanges)-1 {
			continue
		}
		stored, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		var loaded adapter.Message
		if err := json.Unmarshal(stored, &loaded); err != nil {
			t.Fatal(err)
		}
		req.Messages = append(req.Messages, loaded)
		for j := len(source[1].Input) - 1; j >= 0; j-- {
			var result struct {
				Type, Output string
				CallID       string `json:"call_id"`
			}
			if err := json.Unmarshal(source[1].Input[j], &result); err != nil {
				t.Fatal(err)
			}
			if result.Type == "function_call_output" {
				req.Messages = append(req.Messages, adapter.Message{Role: "tool", ToolCallID: result.CallID, Content: result.Output})
			}
		}
	}
}
