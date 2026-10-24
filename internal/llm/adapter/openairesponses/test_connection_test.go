package openairesponses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

const probeStart = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"probe\",\"object\":\"response\",\"status\":\"in_progress\",\"output\":[]}}\n\n"

func TestResponsesURLCandidatesPreserveFullEndpointEscapingAndQuery(t *testing.T) {
	for _, tt := range []struct {
		path string
		want []string
	}{
		{"", []string{"", "/responses", "/v1/responses"}},
		{"/v1", []string{"/v1", "/v1/responses"}},
		{"/v1/responses", []string{"/v1/responses", "/responses"}},
		{"/responses", []string{"/responses", "/v1/responses"}},
		{"/tenant%2Fname/v8/responses", []string{"/tenant%2Fname/v8/responses", "/tenant%2Fname/v8/v1/responses", "/v1/responses"}},
		{"/proxy/chat", []string{"/proxy/chat", "/proxy/chat/responses", "/proxy/chat/v1/responses", "/v1/responses"}},
		{"/v1/", []string{"/v1/", "/v1", "/v1/responses"}},
		{"/proxy/responses/", []string{"/proxy/responses/", "/proxy/responses", "/proxy/v1/responses", "/v1/responses"}},
	} {
		for _, query := range []string{"?route=a%2Fb&x=1&x=2", "?"} {
			u, err := parseResponsesURL("https://relay.invalid" + tt.path + query)
			if err != nil {
				t.Fatal(err)
			}
			var paths []string
			for _, candidate := range responsesURLCandidates(u) {
				parsed, err := url.Parse(candidate)
				if err != nil || parsed.RawQuery != u.RawQuery || parsed.ForceQuery != u.ForceQuery || parsed.Host != u.Host {
					t.Fatalf("candidate changed origin, escaping or query: %s", candidate)
				}
				paths = append(paths, parsed.EscapedPath())
			}
			if !reflect.DeepEqual(paths, tt.want) {
				t.Fatalf("candidates for %s = %v, want %v", tt.path, paths, tt.want)
			}
		}
	}
}

func TestConnectionProbesOriginalAndFallbackWithoutChangingSavedURL(t *testing.T) {
	for _, successPath := range []string{"/proxy/v8/responses", "/v1/responses"} {
		t.Run(successPath, func(t *testing.T) {
			paths := make(chan string, 8)
			closed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths <- r.URL.Path
				if r.Method != http.MethodPost || r.URL.RawQuery != "route=a%2Fb&x=1&x=2" || r.Header.Get("Authorization") != "Bearer instance-key" || r.Header.Get("Accept") != "text/event-stream" {
					t.Error("probe lost method, complete query or instance credentials")
				}
				var params struct {
					Model           string
					Stream          bool
					Input           string
					MaxOutputTokens int64 `json:"max_output_tokens"`
					Store           *bool
					Background      *bool
					Tools           []any
					Temperature     *float64
				}
				if err := json.NewDecoder(r.Body).Decode(&params); err != nil || params.Model != "gpt-6.1-sol" || params.Input != "hi" || !params.Stream ||
					params.MaxOutputTokens != 64 || params.Store == nil || *params.Store || params.Background == nil || *params.Background || len(params.Tools) != 0 || params.Temperature != nil {
					t.Errorf("invalid minimal probe: %+v %v", params, err)
				}
				if r.URL.Path != successPath {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					fmt.Fprint(w, `{"error":{"code":"route_missing","message":"not found"}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
				fmt.Fprint(w, ": keepalive\n\n"+probeStart)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(closed)
			}))
			defer server.Close()
			temperature := 0.4
			backend, err := New(adapter.ProviderMap{"relay": {
				Name: "relay", ProviderType: "openai", ChatURL: server.URL + "/proxy/v8/responses?route=a%2Fb&x=1&x=2", APIKey: "instance-key", Temperature: &temperature,
			}}, server.Client(), nil)
			if err != nil {
				t.Fatal(err)
			}
			before := backend.Providers()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			result, err := backend.TestConnection(ctx, "relay", "gpt-6.1-sol")
			if err != nil || result.URL != server.URL+successPath+"?route=a%2Fb&x=1&x=2" || result.Warning != "" {
				t.Fatalf("probe did not return verified complete URL: %+v %v", result, err)
			}
			select {
			case <-closed:
			case <-ctx.Done():
				t.Fatal("probe waited for full generation or leaked its connection")
			}
			count := len(paths)
			if <-paths != "/proxy/v8/responses" || (successPath == "/proxy/v8/responses" && count != 1) || (successPath == "/v1/responses" && count != 3) || !reflect.DeepEqual(before, backend.Providers()) {
				t.Fatal("probe skipped original, retried in SDK or mutated the provider snapshot")
			}
		})
	}
}

func TestConnectionRejectsInvalidProtocolAndRetainsAllFailures(t *testing.T) {
	for _, tt := range []struct {
		name, contentType, body string
		status                  int
	}{
		{"HTML", "text/html", "<html>SPA</html>", 200},
		{"empty SSE", "text/event-stream", "", 200},
		{"Chat Completions", "text/event-stream", "data: {\"choices\":[]}\n\n", 200},
		{"Anthropic", "text/event-stream", "data: {\"type\":\"message_start\"}\n\n", 200},
		{"wrong first event", "text/event-stream", sseEvents([]string{`{"type":"response.in_progress","response":{"id":"probe","status":"in_progress"}}`}), 200},
		{"numeric ID", "text/event-stream", strings.Replace(probeStart, `"id":"probe"`, `"id":123`, 1), 200},
		{"missing object", "text/event-stream", strings.Replace(probeStart, `"object":"response",`, "", 1), 200},
		{"nonempty created output", "text/event-stream", strings.Replace(probeStart, `"output":[]`, `"output":[`+sdkMessage+`]`, 1), 200},
		{"SSE error", "text/event-stream", sseEvents([]string{`{"error":{"code":"server_error","message":"busy"}}`}), 200},
		{"unauthorized", "application/json", `{"error":{"code":"invalid_api_key","message":"bad key"}}`, 401},
		{"generic 429", "text/html", "<html>rate limited</html>", 429},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var attempts atomic.Int32
			backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts.Add(1)
				result := jsonResponse(tt.body)
				result.StatusCode = tt.status
				result.Header.Set("Content-Type", tt.contentType)
				return result, nil
			}))
			result, err := backend.TestConnection(context.Background(), "relay", "test-model")
			if err == nil || result.URL != "" || attempts.Load() != 3 || !strings.Contains(err.Error(), "/proxy/v8/responses") || !strings.Contains(err.Error(), "/v1/responses") {
				t.Fatalf("invalid probe accepted or candidate failures lost: %+v %v (%d attempts)", result, err, attempts.Load())
			}
			if tt.status == 401 {
				var fault *adapter.APIError
				if !errors.As(err, &fault) || fault.StatusCode != 401 || fault.Kind != "invalid_api_key" {
					t.Fatalf("joined failures lost the original authentication error: %v", err)
				}
			}
		})
	}
}

func TestConnectionRateLimitWarningDoesNotClaimGenerationSucceeded(t *testing.T) {
	for _, kind := range []string{"rate_limit_exceeded", "insufficient_quota", "rate_limit_error"} {
		t.Run(kind, func(t *testing.T) {
			attempts := 0
			backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts++
				result := jsonResponse(`{"error":{"code":"` + kind + `","message":"limited"}}`)
				result.StatusCode = http.StatusTooManyRequests
				return result, nil
			}))
			result, err := backend.TestConnection(context.Background(), "relay", "test-model")
			if err != nil || result.URL != backend.Providers()["relay"].ChatURL || attempts != 1 || !strings.Contains(result.Warning, "尚未验证") {
				t.Fatalf("rate limit overstated validation or attempted more routes: %+v %v", result, err)
			}
		})
	}
}

func TestConnectionCancellationStopsReadingAndCandidateFallback(t *testing.T) {
	started, closed := make(chan struct{}), make(chan struct{})
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(closed)
	}))
	defer server.Close()
	backend, err := New(adapter.ProviderMap{"relay": {Name: "relay", ProviderType: "openai", ChatURL: server.URL + "/v8/responses"}}, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := backend.TestConnection(ctx, "relay", "test-model")
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("probe did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || attempts.Load() != 1 {
			t.Fatalf("cancellation was hidden or fallback continued: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("probe ignored cancellation")
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled probe leaked connection")
	}
	if _, err := backend.TestConnection(context.Background(), "missing", "model"); err == nil {
		t.Fatal("unknown provider fell back")
	}
	if _, err := backend.TestConnection(context.Background(), "relay", " "); err == nil || attempts.Load() != 1 {
		t.Fatal("empty model issued a request")
	}
}
