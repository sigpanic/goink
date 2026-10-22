package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const probeStart = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_probe\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"relay-model\",\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"

func TestMessagesURLCandidatesPreserveEscapingAndQuery(t *testing.T) {
	for _, tt := range []struct {
		path string
		want []string
	}{
		{"", []string{"", "/messages", "/v1/messages"}},
		{"/v1", []string{"/v1", "/v1/messages"}},
		{"/v1/messages", []string{"/v1/messages", "/messages"}},
		{"/tenant%2Fname/v8/messages", []string{"/tenant%2Fname/v8/messages", "/tenant%2Fname/v8/v1/messages", "/v1/messages"}},
		{"/proxy/chat", []string{"/proxy/chat", "/proxy/chat/messages", "/proxy/chat/v1/messages", "/v1/messages"}},
		{"/v1/", []string{"/v1/", "/v1/messages"}},
	} {
		u, err := parseMessagesURL("https://relay.invalid" + tt.path + "?route=a%2Fb&x=1&x=2")
		if err != nil {
			t.Fatal(err)
		}
		var paths []string
		for _, candidate := range messagesURLCandidates(u) {
			parsed, err := url.Parse(candidate)
			if err != nil || parsed.RawQuery != u.RawQuery {
				t.Fatalf("candidate lost query: %s", candidate)
			}
			paths = append(paths, parsed.EscapedPath())
		}
		if !reflect.DeepEqual(paths, tt.want) {
			t.Fatalf("candidates for %s = %v; want %v", tt.path, paths, tt.want)
		}
	}
}

func TestConnectionProbesOriginalAndFallbackWithoutMutatingProvider(t *testing.T) {
	for _, successPath := range []string{"/relay/v8/messages", "/v1/messages"} {
		t.Run(successPath, func(t *testing.T) {
			var paths []string
			closed := make(chan struct{})
			backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.Method != http.MethodPost || r.Header.Get("X-Api-Key") != "key" || r.URL.RawQuery != "route=claude" {
					t.Errorf("probe lost route or authentication: %s %v", r.URL, r.Header)
				}
				var params struct {
					Model     string
					Stream    bool
					MaxTokens int64 `json:"max_tokens"`
					Tools     []any
				}
				if err := json.NewDecoder(r.Body).Decode(&params); err != nil || params.Model != "relay-model" || !params.Stream || params.MaxTokens != 64 || len(params.Tools) != 0 {
					t.Errorf("invalid minimal probe: %+v %v", params, err)
				}
				if r.URL.Path != successPath {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
				fmt.Fprint(w, streamFrame("ping", `{"type":"ping"}`)+probeStart)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(closed)
			})
			before := backend.Providers()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			result, err := backend.TestConnection(ctx, "relay", "relay-model")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(result.URL, successPath+"?route=claude") || result.Warning != "" {
				t.Fatalf("unexpected probe result: %+v", result)
			}
			select {
			case <-closed:
			case <-ctx.Done():
				t.Fatal("probe waited for complete generation or leaked response body")
			}
			if paths[0] != "/relay/v8/messages" || successPath == "/relay/v8/messages" && len(paths) != 1 || !reflect.DeepEqual(before, backend.Providers()) {
				t.Fatalf("probe changed provider or skipped original: %v", paths)
			}
		})
	}
}

func TestConnectionRejectsInvalidProtocolAndReportsFailures(t *testing.T) {
	for _, tt := range []struct {
		name, contentType, body string
		status                  int
	}{
		{"HTML", "text/html", "<html>SPA</html>", 200},
		{"empty SSE", "text/event-stream", "", 200},
		{"OpenAI", "text/event-stream", "data: {\"choices\":[]}\n\n", 200},
		{"bad start", "text/event-stream", streamFrame("message_start", `{"type":"message_start","message":{"id":123,"type":"message","role":"assistant","content":[]}}`), 200},
		{"wrong first event", "text/event-stream", streamFrame("message_stop", `{"type":"message_stop"}`), 200},
		{"SSE error", "text/event-stream", streamFrame("error", `{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`), 200},
		{"unauthorized", "application/json", `{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`, 401},
		{"generic 429", "text/html", "<html>rate limited</html>", 429},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			})
			result, err := backend.TestConnection(context.Background(), "relay", "relay-model")
			if err == nil || result.URL != "" || !strings.Contains(err.Error(), "/relay/v8/messages") || requests.Load() != 3 {
				t.Fatalf("invalid probe accepted, SDK retried, or failures lost: %+v %v (%d requests)", result, err, requests.Load())
			}
		})
	}
}

func TestConnectionRateLimitWarningDoesNotClaimGenerationSucceeded(t *testing.T) {
	var requests atomic.Int32
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"limited"}}`)
	})
	result, err := backend.TestConnection(context.Background(), "relay", "relay-model")
	if err != nil || requests.Load() != 1 || result.URL != backend.Providers()["relay"].ChatURL || !strings.Contains(result.Warning, "尚未验证") {
		t.Fatalf("rate limit must report limited verification: %+v %v", result, err)
	}
}

func TestConnectionCancellationStopsReadingAndFallback(t *testing.T) {
	started, closed := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(closed)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := backend.TestConnection(ctx, "relay", "relay-model")
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
		if !errors.Is(err, context.Canceled) || requests.Load() != 1 {
			t.Fatalf("cancellation lost or another candidate attempted: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("probe ignored cancellation")
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("probe leaked connection")
	}
	if _, err := backend.TestConnection(context.Background(), "missing", "model"); err == nil {
		t.Fatal("unknown provider fell back")
	}
	if _, err := backend.TestConnection(context.Background(), "relay", " "); err == nil {
		t.Fatal("empty model issued a request")
	}
}
