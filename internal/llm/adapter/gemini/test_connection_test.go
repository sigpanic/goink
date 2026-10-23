package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

const createdProbeEvent = `{"event_type":"interaction.created","interaction":{"id":"probe","status":"in_progress"}}`

func TestInteractionsURLCandidates(t *testing.T) {
	for raw, paths := range map[string][]string{
		"":                               {"", "/interactions", "/v1/interactions"},
		"/v1":                            {"/v1", "/v1/interactions"},
		"/v1beta":                        {"/v1beta", "/v1beta/interactions", "/v1/interactions"},
		"/v1/interactions":               {"/v1/interactions", "/interactions"},
		"/v1beta/interactions":           {"/v1beta/interactions", "/interactions", "/v1/interactions"},
		"/tenant%2Fname/v8/interactions": {"/tenant%2Fname/v8/interactions", "/tenant%2Fname/v8/v1/interactions", "/v1/interactions"},
		"/proxy/chat/":                   {"/proxy/chat/", "/proxy/chat/interactions", "/proxy/chat/v1/interactions", "/v1/interactions"},
	} {
		t.Run(raw, func(t *testing.T) {
			endpoint, err := url.Parse("https://relay.invalid" + raw + "?route=a%2Fb&tag=1&tag=2")
			if err != nil {
				t.Fatal(err)
			}
			var want []string
			for _, path := range paths {
				want = append(want, "https://relay.invalid"+path+"?route=a%2Fb&tag=1&tag=2")
			}
			if got := interactionsURLCandidates(endpoint); !reflect.DeepEqual(got, want) {
				t.Fatalf("candidates=%v, want=%v", got, want)
			}
		})
	}
}

func TestConnectionProbesOriginalThenFallbackWithoutChangingProvider(t *testing.T) {
	var paths []string
	closed := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		var params struct {
			Model, Input              string
			Store, Background, Stream *bool
			PreviousInteractionID     *string `json:"previous_interaction_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil || params.Model != "relay-model" || params.Input != "hi" || params.Store == nil || *params.Store || params.Background == nil || *params.Background || params.Stream == nil || !*params.Stream || params.PreviousInteractionID != nil {
			t.Errorf("probe request is not a minimal stateless interaction: %+v %v", params, err)
		}
		if r.Header.Get("X-Goog-Api-Key") != "key" || r.URL.RawQuery != "route=a%2Fb&tag=1&tag=2" || r.Header.Get("Accept") != "text/event-stream" {
			t.Error("candidate lost credentials, Accept or raw query")
		}
		if r.URL.EscapedPath() != "/tenant%2Fname/v1/interactions" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		fmt.Fprintf(w, "data: %s\n\n", createdProbeEvent)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			closed <- struct{}{}
		case <-time.After(3 * time.Second):
		}
	}))
	defer server.Close()
	p := newTestProvider(t, server.URL+"/tenant%2Fname?route=a%2Fb&tag=1&tag=2", "key", server.Client())
	backend := &Adapter{providers: map[string]Provider{"relay": p}}
	result, err := backend.TestConnection(context.Background(), "relay", "relay-model")
	if err != nil || result.URL != server.URL+"/tenant%2Fname/v1/interactions?route=a%2Fb&tag=1&tag=2" || result.Warning != "" {
		t.Fatalf("fallback did not return a verified full URL: %+v %v", result, err)
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("probe waited for generation instead of closing at the handshake")
	}
	if !reflect.DeepEqual(paths, []string{"/tenant%2Fname", "/tenant%2Fname/interactions", "/tenant%2Fname/v1/interactions"}) || backend.providers["relay"].client != p.client || backend.Providers()["relay"].ChatURL != p.ChatURL {
		t.Fatalf("candidate order or provider snapshot changed: %v", paths)
	}
}

func TestConnectionValidatesProtocolHandshake(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
		status                  int
		warning, success        bool
	}{
		{"created", "text/event-stream", "data: " + createdProbeEvent + "\n\n", 200, false, true},
		{"html", "text/html", "<html>fallback</html>", 200, false, false},
		{"json", "application/json", sdkTextResponse, 200, false, false},
		{"empty", "text/event-stream", "", 200, false, false},
		{"other protocol", "text/event-stream", "data: {\"choices\":[]}\n\n", 200, false, false},
		{"missing identity", "text/event-stream", "data: {\"event_type\":\"interaction.created\",\"interaction\":{\"status\":\"in_progress\"}}\n\n", 200, false, false},
		{"terminal first", "text/event-stream", "data: {\"event_type\":\"interaction.created\",\"interaction\":{\"id\":\"probe\",\"status\":\"failed\"}}\n\n", 200, false, false},
		{"agent", "text/event-stream", "data: {\"event_type\":\"interaction.created\",\"interaction\":{\"id\":\"probe\",\"status\":\"in_progress\",\"agent\":\"research\"}}\n\n", 200, false, false},
		{"stream error", "text/event-stream", "data: {\"event_type\":\"error\",\"error\":{\"code\":\"authentication\",\"message\":\"bad key\"}}\n\n", 200, false, false},
		{"authentication", "application/json", `{"error":{"code":"authentication","message":"bad key"}}`, 401, false, false},
		{"rate limit", "application/json", `{"error":{"code":"rate_limit_exceeded","message":"slow down"}}`, 429, true, true},
		{"quota", "application/json", `{"error":{"code":"quota_exceeded","message":"daily quota"}}`, 429, true, true},
		{"legacy quota", "application/json", `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"quota"}}`, 429, true, true},
		{"generic 429", "text/html", "<html>gateway rate limit</html>", 429, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			result, err := backend.TestConnection(context.Background(), "relay", "relay-model")
			if (err == nil) != tc.success || (result.Warning != "") != tc.warning {
				t.Fatalf("incorrect probe result: %+v %v", result, err)
			}
			if tc.success && (result.URL != backend.Providers()["relay"].ChatURL || requests.Load() != 1) {
				t.Fatal("valid original endpoint caused a fallback or SDK retry")
			}
			if tc.warning && !strings.Contains(result.Warning, "尚未验证密钥、模型和生成能力") {
				t.Fatal("429 warning claimed generation or authentication success")
			}
			if tc.status == 401 {
				var apiErr *adapter.APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 || apiErr.Kind != "authentication" || requests.Load() != 3 {
					t.Fatalf("candidate failures lost the original error or retried: %v (%d requests)", err, requests.Load())
				}
			}
		})
	}
}

func TestConnectionCancellationAndInvalidInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var requests atomic.Int32
	shared := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		cancel()
		return nil, r.Context().Err()
	})}
	p := newTestProvider(t, "https://configured.invalid/v8/interactions", "key", shared)
	backend := &Adapter{providers: map[string]Provider{"relay": p}}
	for _, input := range [][2]string{{"missing", "model"}, {"relay", " "}} {
		if _, err := backend.TestConnection(context.Background(), input[0], input[1]); err == nil || requests.Load() != 0 {
			t.Fatal("invalid probe input caused network I/O")
		}
	}
	if _, err := backend.TestConnection(ctx, "relay", "model"); !errors.Is(err, context.Canceled) || requests.Load() != 1 {
		t.Fatalf("probe continued candidates after cancellation: %v", err)
	}
	shared.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader("data: " + createdProbeEvent + "\n\n"))}, nil
	})
	if _, err := backend.TestConnection(context.Background(), "relay", "model"); err != nil {
		t.Fatalf("fresh probe could not reuse shared client: %v", err)
	}
}

func TestConnectionCancellationWhileWaitingForHandshake(t *testing.T) {
	started, closed := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		close(started)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		close(closed)
	}))
	defer server.Close()
	p := newTestProvider(t, server.URL+"/v1/interactions", "key", server.Client())
	backend := &Adapter{providers: map[string]Provider{"relay": p}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := backend.TestConnection(ctx, "relay", "model")
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("probe did not open the SSE connection")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || requests.Load() != 1 {
			t.Fatalf("cancelled probe continued candidates or lost context error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("probe ignored cancellation while reading SSE")
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled probe leaked the HTTP connection")
	}
}
