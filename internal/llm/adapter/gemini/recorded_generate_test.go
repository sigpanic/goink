package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestRecordedStatelessGeneratePreservesSignedHistoryWithoutIDs(t *testing.T) {
	exchanges := recordedExchanges(t, "interactions-stateless-unary.yaml", 2)
	var requests int
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.RequestURI() != "/v1/interactions" || r.Header.Get("Accept") != "application/json" ||
			r.Header.Get("X-Goog-Api-Key") != "replay-only" || r.Header.Get("Api-Revision") != "2026-05-20" {
			t.Error("recorded Generate changed the endpoint, credentials or JSON request mode")
		}
		if requests >= len(exchanges) {
			return nil, fmt.Errorf("unexpected recorded Generate request %d", requests+1)
		}
		exchange := exchanges[requests]
		requests++
		var got, want recordedParameters
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(exchange.When.Body), &want); err != nil {
			return nil, err
		}
		if got.Stream == nil || *got.Stream || got.Store == nil || *got.Store || got.Background == nil || *got.Background ||
			got.PreviousInteractionID != nil || got.Model != want.Model || len(got.Tools) != 0 || len(got.Input) != len(want.Input) {
			t.Error("recorded Generate did not retain stateless full-history request semantics")
		}
		for i := range want.Input {
			if i >= len(got.Input) || !sameJSON(got.Input[i], want.Input[i]) {
				t.Errorf("request %d changed recorded input step %d", requests, i)
			}
		}
		header := make(http.Header)
		for _, item := range exchange.Then.Header {
			header.Add(item.Name, item.Value)
		}
		return &http.Response{StatusCode: exchange.Then.Status, Header: header, Body: io.NopCloser(strings.NewReader(exchange.Then.Body))}, nil
	})}
	backend, err := New(adapter.ProviderMap{"relay": {
		Name: "relay", ProviderType: "google", APIKey: "replay-only", ChatURL: "https://recording.invalid/v1/interactions",
	}}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := recordedRequest(t, exchanges[0])
	req.Options = &adapter.CallOptions{MaxTokens: ptr(2048)}
	for i, exchange := range exchanges {
		var original struct {
			ID     json.RawMessage   `json:"id"`
			Status string            `json:"status"`
			Steps  []json.RawMessage `json:"steps"`
		}
		if err := json.Unmarshal([]byte(exchange.Then.Body), &original); err != nil || len(original.ID) != 0 || original.Status != "completed" || len(original.Steps) != 2 {
			t.Fatalf("recorded stateless response shape changed: %v", err)
		}
		response, err := backend.Generate(context.Background(), req)
		if err != nil {
			t.Fatalf("recorded Generate round %d: %v", i+1, err)
		}
		wantText := []string{"289", "290"}[i]
		if requests != i+1 || response.Content != wantText || len(response.ToolCalls) != 0 || response.StopReason != "completed" ||
			response.State == nil || !response.State.Complete || !response.State.TurnComplete {
			t.Fatalf("recorded Generate round %d lost text or completion", i+1)
		}
		input, output, total := []float64{14, 31}[i], []float64{148, 58}[i], []float64{162, 89}[i]
		if response.Usage["prompt_tokens"] != input || response.Usage["completion_tokens"] != output || response.Usage["total_tokens"] != total {
			t.Fatalf("recorded Generate round %d miscounted usage: %v", i+1, response.Usage)
		}
		var thought struct {
			Signature string `json:"signature"`
			Summary   []struct{ Type, Text string }
		}
		if err := json.Unmarshal(original.Steps[0], &thought); err != nil || thought.Signature == "" || len(thought.Summary) != 1 || thought.Summary[0].Type != "text" {
			t.Fatalf("recorded thought shape changed: %v", err)
		}
		if response.ThinkingContent != thought.Summary[0].Text {
			t.Fatalf("recorded Generate round %d changed its thinking summary", i+1)
		}
		message := adapter.Message{Role: "assistant", Content: response.Content, ThinkingContent: response.ThinkingContent, State: response.State}
		var payload replayPayload
		if err := json.Unmarshal(response.State.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		var native []json.RawMessage
		if err := json.Unmarshal(payload.Steps, &native); err != nil || len(native) != len(original.Steps) {
			t.Fatalf("recorded Generate round %d lost native steps: %v", i+1, err)
		}
		for j, step := range original.Steps {
			if !sameJSON(step, native[j]) {
				t.Fatalf("recorded Generate round %d changed native step %d", i+1, j)
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
		var next recordedParameters
		if err := json.Unmarshal([]byte(exchanges[i+1].When.Body), &next); err != nil || len(next.Input) != 4 {
			t.Fatalf("recorded follow-up shape changed: %v", err)
		}
		var user struct {
			Type    string
			Content []struct{ Type, Text string }
		}
		if err := json.Unmarshal(next.Input[3], &user); err != nil || user.Type != "user_input" || len(user.Content) != 1 || user.Content[0].Type != "text" {
			t.Fatalf("recorded follow-up is not a single text input: %v", err)
		}
		req.Messages = append(req.Messages, loaded, adapter.Message{Role: "user", Content: user.Content[0].Text})
	}
}
