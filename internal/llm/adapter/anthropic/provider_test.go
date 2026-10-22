package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestSDKClientsPreserveRoutesAndInstanceCredentials(t *testing.T) {
	tests := []struct {
		name, suffix string
	}{
		{"root-candidate", ""},
		{"version-candidate", "/v1/"},
		{"endpoint", "/v1/messages"},
		{"custom-version", "/v8/messages"},
		{"custom-route", "/proxy/chat"},
		{"trailing-slash", "/gateway/tenant/v1/messages/"},
		{"escaped-path", "/tenant%2Fname/messages"},
		{"query", "/messages?token=a%2Fb&tag=1&tag=2"},
		{"empty-query", "/messages?"},
		{"dot-path", "/prefix/../messages"},
	}
	type received struct {
		method, uri, key, auth, version string
	}
	requests := make(chan received, len(tests)*2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- received{r.Method, r.RequestURI, r.Header.Get("X-Api-Key"), r.Header.Get("Authorization"), r.Header.Get("Anthropic-Version")}
		var params struct {
			Model     string
			MaxTokens int `json:"max_tokens"`
			Stream    bool
		}
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil || params.Model != "test-model" || params.MaxTokens != 16 {
			t.Error("SDK message request was not received as expected")
		}
		if params.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: message_start\ndata: "+`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"test-model","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`+"\n\n")
			fmt.Fprint(w, "event: content_block_start\ndata: "+`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`+"\n\n")
			fmt.Fprint(w, "event: content_block_delta\ndata: "+`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`+"\n\n")
			fmt.Fprint(w, "event: content_block_stop\ndata: "+`{"type":"content_block_stop","index":0}`+"\n\n")
			fmt.Fprint(w, "event: message_delta\ndata: "+`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`+"\n\n")
			fmt.Fprint(w, "event: message_stop\ndata: "+`{"type":"message_stop"}`+"\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"msg_test","type":"message","role":"assistant","model":"test-model","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer server.Close()
	var transportCalls atomic.Int32
	transport := server.Client().Transport
	shared := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		transportCalls.Add(1)
		return transport.RoundTrip(r)
	})}
	configs := make(adapter.ProviderMap, len(tests))
	for _, tt := range tests {
		configs[tt.name] = adapter.Provider{Name: tt.name, ProviderType: "anthropic", ChatURL: server.URL + tt.suffix, APIKey: "key-" + tt.name}
	}
	backend, err := New(configs, shared, nil)
	if err != nil {
		t.Fatal(err)
	}
	if transportCalls.Load() != 0 {
		t.Fatal("constructing SDK clients performed network I/O")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := backend.provider(tt.name)
			if err != nil {
				t.Fatal(err)
			}
			if p.ChatURL != configs[tt.name].ChatURL {
				t.Fatal("construction overwrote the public configuration URL")
			}
			urlOption, err := messagesURLOption(p.ChatURL)
			if err != nil {
				t.Fatal(err)
			}
			params := sdk.MessageNewParams{
				Model: "test-model", MaxTokens: 16,
				Messages: []sdk.MessageParam{sdk.NewUserMessage(sdk.NewTextBlock("hello"))},
			}
			message, err := p.client.Messages.New(ctx, params, urlOption)
			if err != nil || len(message.Content) != 1 || message.Content[0].Text != "ok" {
				t.Fatalf("SDK Messages request failed: %v", err)
			}
			stream := p.client.Messages.NewStreaming(ctx, params, urlOption)
			defer stream.Close()
			var accumulated sdk.Message
			sawDelta, sawStop := false, false
			for stream.Next() {
				event := stream.Current()
				if err := accumulated.Accumulate(event); err != nil {
					t.Fatal(err)
				}
				if event.Type == "content_block_delta" && event.Delta.Text == "ok" {
					sawDelta = true
				}
				sawStop = sawStop || event.Type == "message_stop"
			}
			if err := stream.Err(); err != nil {
				t.Fatal(err)
			}
			if !sawDelta || !sawStop || len(accumulated.Content) != 1 || accumulated.Content[0].Text != "ok" {
				t.Fatal("SDK stream did not preserve events and accumulated response")
			}
			wantURI := tt.suffix
			if wantURI == "" {
				wantURI = "/"
			}
			for range 2 {
				got := <-requests
				if got.method != http.MethodPost || got.uri != wantURI || got.key != "key-"+tt.name || got.auth != "" || got.version == "" {
					t.Fatalf("wrong route or instance authentication: %s %s (want POST %s)", got.method, got.uri, wantURI)
				}
			}
		})
	}
	if transportCalls.Load() != int32(len(tests)*2) {
		t.Fatal("SDK clients did not share the injected transport")
	}
}

func TestSDKClientsIgnoreEnvironmentDefaults(t *testing.T) {
	t.Setenv("ANTHROPIC_BASE_URL", "http://environment.invalid/wrong-prefix")
	t.Setenv("ANTHROPIC_PROFILE", "goink-test-nonexistent-profile")
	for _, source := range []string{"api-key", "auth-token", "profile"} {
		t.Run(source, func(t *testing.T) {
			t.Setenv("ANTHROPIC_API_KEY", "")
			t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
			switch source {
			case "api-key":
				t.Setenv("ANTHROPIC_API_KEY", "environment-api-key")
			case "auth-token":
				t.Setenv("ANTHROPIC_AUTH_TOKEN", "environment-auth-token")
			}
			var requests atomic.Int32
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests.Add(1)
				if r.URL.String() != "https://configured.invalid/prefix/messages?route=custom" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" {
					t.Error("SDK loaded another address or credential source")
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"msg_test","type":"message","role":"assistant","content":[]}`))}, nil
			})}
			// 空密钥也不允许退回环境或本机 profile；是否允许调用由配置层和服务端决定。
			backend, err := New(adapter.ProviderMap{"relay": {Name: "relay", ProviderType: "anthropic", ChatURL: "https://configured.invalid/prefix/messages?route=custom"}}, client, nil)
			if err != nil {
				t.Fatal(err)
			}
			p, err := backend.provider("relay")
			if err != nil {
				t.Fatal(err)
			}
			urlOption, err := messagesURLOption(p.ChatURL)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.client.Messages.New(context.Background(), sdk.MessageNewParams{Model: "test-model", MaxTokens: 16}, urlOption); err != nil || requests.Load() != 1 {
				t.Fatalf("SDK did not use only the configured instance: requests=%d error=%v", requests.Load(), err)
			}
		})
	}
}

func TestMessagesURLOverrideIsScopedToEachCall(t *testing.T) {
	var requests []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.Method+" "+r.URL.String())
		if (r.Host != "" && r.Host != r.URL.Host) || r.Header.Get("X-Api-Key") != "relay-key" {
			t.Error("URL override lost the target host or instance credentials")
		}
		body := `{"id":"msg_test","type":"message","role":"assistant","content":[]}`
		if r.Method == http.MethodGet {
			body = `{"data":[],"has_more":false}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	const savedURL = "https://configured.invalid/proxy/v8/messages?route=saved"
	backend, err := New(adapter.ProviderMap{"relay": {Name: "relay", ProviderType: "anthropic", ChatURL: savedURL, APIKey: "relay-key"}}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := backend.provider("relay")
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"https://candidate.invalid/custom/messages?route=probe", savedURL} {
		urlOption, err := messagesURLOption(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.client.Messages.New(context.Background(), sdk.MessageNewParams{Model: "test-model", MaxTokens: 16}, urlOption); err != nil {
			t.Fatal(err)
		}
	}
	// 这里只验证 Messages 的覆盖不会污染其他调用，不为模型发现推导地址规则。
	if _, err := p.client.Models.List(context.Background(), sdk.ModelListParams{}, option.WithBaseURL("https://discovery.invalid/catalog/")); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST https://candidate.invalid/custom/messages?route=probe",
		"POST " + savedURL,
		"GET https://discovery.invalid/catalog/v1/models",
	}
	if len(requests) != len(want) {
		t.Fatalf("unexpected request count: %v", requests)
	}
	for i := range want {
		if requests[i] != want[i] {
			t.Fatalf("request %d = %q, want %q", i, requests[i], want[i])
		}
	}
	if backend.Providers()["relay"].ChatURL != savedURL {
		t.Fatal("probe candidate overwrote the saved endpoint")
	}
}

func TestSDKClientDoesNotRetry(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.Header().Set("Retry-After", "0")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"test failure"}}`)
			}))
			defer server.Close()
			backend, err := New(adapter.ProviderMap{"relay": {Name: "relay", ProviderType: "anthropic", ChatURL: server.URL, APIKey: "test-key"}}, server.Client(), nil)
			if err != nil {
				t.Fatal(err)
			}
			p, err := backend.provider("relay")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			urlOption, err := messagesURLOption(p.ChatURL)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.client.Messages.New(ctx, sdk.MessageNewParams{Model: "test-model", MaxTokens: 16}, urlOption); err == nil || attempts.Load() != 1 {
				t.Fatalf("SDK retry policy changed: attempts=%d error=%v", attempts.Load(), err)
			}
		})
	}
}

func TestMessagesURLRejectsInvalidAddresses(t *testing.T) {
	for _, raw := range []string{
		"", "relay.invalid", "/relative", "ftp://relay.invalid", "https:///missing-host", "https://relay.invalid/%zz",
		"https://user:password@relay.invalid", "https://relay.invalid#fragment",
	} {
		t.Run(raw, func(t *testing.T) {
			config := adapter.Provider{Name: "relay", ProviderType: "anthropic", ChatURL: raw}
			if _, err := New(adapter.ProviderMap{config.Name: config}, &http.Client{}, nil); err == nil {
				t.Fatal("unsupported URL was silently changed or accepted")
			}
			if _, err := messagesURLOption(raw); err == nil {
				t.Fatal("invalid probe candidate URL was accepted")
			}
		})
	}
	got, err := parseMessagesURL("  https://relay.invalid/prefix/v8/messages/?token=a%2Fb  ")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "https://relay.invalid/prefix/v8/messages/?token=a%2Fb" {
		t.Fatalf("URL path or query changed: %q", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSDKClientUsesRequestContext(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		close(stopped)
		return nil, r.Context().Err()
	})}
	backend, err := New(adapter.ProviderMap{"relay": {Name: "relay", ProviderType: "anthropic", ChatURL: "https://relay.invalid", APIKey: "test-key"}}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := backend.provider("relay")
	if err != nil {
		t.Fatal(err)
	}
	urlOption, err := messagesURLOption(p.ChatURL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := p.client.Messages.New(ctx, sdk.MessageNewParams{Model: "test-model", MaxTokens: 16}, urlOption)
		done <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("SDK request did not reach the injected transport")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("SDK request did not stop on cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SDK request remained blocked after cancellation")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("injected transport did not observe cancellation")
	}
}
