package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	types "google.golang.org/genai/interactions/models/interactions"
	"google.golang.org/genai/interactions/models/operations"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

const sdkTextResponse = `{"id":"interaction_test","object":"interaction","model":"gemini-3.8-flash","status":"completed","created":"2026-10-22T00:00:00Z","updated":"2026-10-22T00:00:01Z","steps":[{"type":"model_output","content":[{"type":"text","text":"ok"}]}],"usage":{"total_input_tokens":1,"total_output_tokens":1,"total_tokens":2}}`

func sdkRequest(stream bool) operations.CreateInteractionRequest {
	return operations.CreateInteractionRequest{Body: operations.NewCreateInteractionRequestBody(types.CreateModelInteraction{
		Model: "gemini-3.8-flash", Input: ptr(types.NewInteractionsInput("hello")), Store: ptr(false), Stream: ptr(stream),
	})}
}

func ptr[T any](value T) *T { return &value }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newTestProvider(t *testing.T, endpoint, key string, client *http.Client) Provider {
	t.Helper()
	backend, err := New(adapter.ProviderMap{"relay": {Name: "relay", ProviderType: "google", ChatURL: endpoint, APIKey: key}}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := backend.provider("relay")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSDKPreservesCompleteRoutesAndSharedTransport(t *testing.T) {
	var requests atomic.Int32
	var wantURI, wantKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPost || r.RequestURI != wantURI || r.Header.Get("X-Goog-Api-Key") != wantKey ||
			r.Header.Get("Authorization") != "" || r.Header.Get("Api-Revision") != "2026-05-20" {
			t.Errorf("unexpected SDK request: %s %s %v", r.Method, r.RequestURI, r.Header)
		}
		var body struct {
			Model string
			Input string
			Store *bool
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "gemini-3.8-flash" || body.Input != "hello" || body.Store == nil || *body.Store {
			t.Errorf("SDK request did not match the stateless schema: %+v %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, sdkTextResponse)
	}))
	defer server.Close()
	var transportCalls atomic.Int32
	transport := server.Client().Transport
	shared := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		transportCalls.Add(1)
		return transport.RoundTrip(r)
	})}
	transportBefore := reflect.ValueOf(shared.Transport).Pointer()
	for i, path := range []string{
		"", "/v1/interactions", "/v1beta/interactions", "/v8/interactions", "/proxy/chat",
		"/tenant%2Fname/interactions?route=a%2Fb&tag=1&tag=2", "/interactions?", "/prefix/../interactions", "/interactions/",
	} {
		t.Run(path, func(t *testing.T) {
			wantURI, wantKey = path, fmt.Sprintf("key-%d", i)
			if wantURI == "" {
				wantURI = "/"
			}
			before := requests.Load()
			p := newTestProvider(t, server.URL+path, wantKey, shared)
			if requests.Load() != before {
				t.Fatal("constructing SDK client performed network I/O")
			}
			result, err := p.client.Create(context.Background(), sdkRequest(false))
			if err != nil {
				t.Fatal(err)
			}
			if result.Interaction == nil || result.Interaction.Status != types.InteractionStatusCompleted || len(result.Interaction.Steps) != 1 {
				t.Fatalf("SDK did not decode the v1 response shape: %+v", result.Interaction)
			}
			output := result.Interaction.Steps[0].ModelOutputStep
			if output == nil || len(output.Content) != 1 || output.Content[0].TextContent == nil || output.Content[0].TextContent.Text != "ok" {
				t.Fatalf("SDK did not decode model_output content: %+v", output)
			}
		})
	}
	if requests.Load() != 9 || transportCalls.Load() != requests.Load() || reflect.ValueOf(shared.Transport).Pointer() != transportBefore || shared.Timeout != 0 {
		t.Fatal("SDK did not use the injected shared transport")
	}
}

func TestSDKIgnoresEnvironmentCredentialsAndAddresses(t *testing.T) {
	for key, value := range map[string]string{
		"GOOGLE_API_KEY": "environment-key", "GEMINI_API_KEY": "other-key", "GOOGLE_GENAI_USE_VERTEXAI": "true",
		"GOOGLE_GENAI_USE_ENTERPRISE": "true", "GOOGLE_CLOUD_PROJECT": "other-project", "GOOGLE_CLOUD_LOCATION": "other-region",
		"GOOGLE_GEMINI_BASE_URL": "https://environment.invalid", "GOOGLE_VERTEX_BASE_URL": "https://vertex.invalid",
	} {
		t.Setenv(key, value)
	}
	for _, key := range []string{"explicit-key", ""} {
		t.Run(key, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://configured.invalid/proxy/interactions?route=google" || r.Header.Get("X-Goog-Api-Key") != key || r.Header.Get("Authorization") != "" || r.Header.Get("X-Goog-User-Project") != "" {
					t.Error("SDK loaded another instance's address, credential or quota project")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(sdkTextResponse))}, nil
			})}
			p := newTestProvider(t, "https://configured.invalid/proxy/interactions?route=google", key, client)
			if _, err := p.client.Create(context.Background(), sdkRequest(false)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMultipleInstancesKeepIndependentSDKCredentials(t *testing.T) {
	var received []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		received = append(received, r.URL.Host+":"+r.Header.Get("X-Goog-Api-Key"))
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(sdkTextResponse))}, nil
	})}
	configs := adapter.ProviderMap{
		"one": {Name: "one", ProviderType: "google", ChatURL: "https://one.invalid/v1/interactions", APIKey: "one-key"},
		"two": {Name: "two", ProviderType: "google", ChatURL: "https://two.invalid/v8/interactions", APIKey: "two-key"},
	}
	backend, err := New(configs, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	configs["one"] = adapter.Provider{APIKey: "changed after construction"}
	for _, name := range []string{"one", "two", "one", "two"} {
		p, err := backend.provider(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.client.Create(context.Background(), sdkRequest(false)); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(received, []string{"one.invalid:one-key", "two.invalid:two-key", "one.invalid:one-key", "two.invalid:two-key"}) {
		t.Fatalf("SDK instances shared addresses or credentials: %v", received)
	}
}

func TestCandidateSDKClientDoesNotMutateSavedEndpoint(t *testing.T) {
	var received []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		received = append(received, r.URL.String())
		if r.Host != r.URL.Host || r.Header.Get("X-Goog-Api-Key") != "instance-key" {
			t.Error("candidate lost host or credentials")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(sdkTextResponse))}, nil
	})}
	const saved = "https://saved.invalid/proxy/interactions?route=saved"
	const candidate = "https://candidate.invalid/v8/interactions?route=probe"
	p := newTestProvider(t, saved, "instance-key", client)
	probe, err := p.clientForURL(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := probe.Create(context.Background(), sdkRequest(false)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.client.Create(context.Background(), sdkRequest(false)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(received, []string{candidate, saved}) || p.ChatURL != saved {
		t.Fatalf("probe contaminated formal endpoint: %v %s", received, p.ChatURL)
	}
}

func TestSDKDoesNotRetryHTTPOrTransportErrors(t *testing.T) {
	for _, status := range []int{0, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var attempts atomic.Int32
			sentinel := errors.New("transport failure")
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts.Add(1)
				if status == 0 {
					return nil, sentinel
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}, "Retry-After": {"0"}},
					Body: io.NopCloser(strings.NewReader(`{"error":{"code":"rate_limit_exceeded","message":"test failure"}}`))}, nil
			})}
			p := newTestProvider(t, "https://configured.invalid/v1/interactions", "key", client)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := p.client.Create(ctx, sdkRequest(false))
			if err == nil || attempts.Load() != 1 || ctx.Err() != nil || status == 0 && !errors.Is(err, sentinel) {
				t.Fatalf("SDK retry policy or error chain changed: attempts=%d error=%v", attempts.Load(), err)
			}
		})
	}
}

func TestInteractionsURLValidationAndOverrideScope(t *testing.T) {
	for _, raw := range []string{"", "relay.invalid", "/relative", "ftp://relay.invalid", "https:///missing-host", "https://relay.invalid/%zz", "https://user:pass@relay.invalid", "https://relay.invalid#fragment"} {
		if _, err := New(adapter.ProviderMap{"relay": {Name: "relay", ProviderType: "google", ChatURL: raw}}, &http.Client{}, nil); err == nil {
			t.Fatalf("invalid endpoint was accepted: %q", raw)
		}
	}
	endpoint, err := adapter.ParseChatURL(" https://relay.invalid/prefix/v8/interactions/?token=a%2Fb ")
	if err != nil || endpoint.String() != "https://relay.invalid/prefix/v8/interactions/?token=a%2Fb" {
		t.Fatalf("endpoint was changed: %v %v", endpoint, err)
	}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Error("an unrelated SDK operation reached the full-endpoint override")
		return nil, errors.New("unexpected request")
	})}
	delegate := interactionsEndpointClient{client: client, endpoint: *endpoint}
	for _, target := range []struct{ method, path string }{{http.MethodGet, "/v1/interactions"}, {http.MethodPost, "/v1/models"}} {
		req, err := http.NewRequest(target.method, "https://sdk.invalid"+target.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := delegate.Do(req); err == nil {
			t.Fatal("URL override accepted another SDK operation")
		}
	}
}
