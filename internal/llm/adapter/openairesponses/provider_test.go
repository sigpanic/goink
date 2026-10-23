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
	"sync/atomic"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func newTestProvider(t *testing.T, endpoint, key string, client *http.Client) Provider {
	t.Helper()
	backend, err := New(adapter.ProviderMap{"relay": {
		Name: "relay", ProviderType: "openai", ChatURL: endpoint, APIKey: key,
	}}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := backend.provider("relay")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func sdkRequest() responses.ResponseNewParams {
	return responses.ResponseNewParams{
		Model: "test-model", MaxOutputTokens: param.NewOpt(int64(16)), Store: param.NewOpt(false),
		Input: responses.ResponseNewParamsInputUnion{OfString: param.NewOpt("hello")},
	}
}

func TestSDKClientsPreserveCompleteEndpointsAndCredentials(t *testing.T) {
	paths := []string{
		"", "/v1/", "/v1/responses", "/v8/responses", "/proxy/chat",
		"/gateway/tenant/v1/responses/", "/tenant%2Fname/responses",
		"/responses?token=a%2Fb&tag=1&tag=2", "/responses?", "/prefix/../responses",
	}
	type received struct{ uri, key string }
	requests := make(chan received, len(paths)*2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- received{r.RequestURI, r.Header.Get("Authorization")}
		var body struct {
			Model  string
			Store  *bool
			Stream bool
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "test-model" || body.Store == nil || *body.Store || r.Method != http.MethodPost {
			t.Error("SDK did not send the expected stateless request")
		}
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sdkStreamFixture())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, sdkResponse)
	}))
	defer server.Close()
	var transportCalls atomic.Int32
	transport := server.Client().Transport
	shared := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		transportCalls.Add(1)
		return transport.RoundTrip(r)
	})}
	configs := make(adapter.ProviderMap, len(paths))
	for i, path := range paths {
		name := fmt.Sprintf("relay-%d", i)
		configs[name] = adapter.Provider{Name: name, ProviderType: "openai", ChatURL: server.URL + path, APIKey: "key-" + name}
	}
	backend, err := New(configs, shared, nil)
	if err != nil {
		t.Fatal(err)
	}
	if transportCalls.Load() != 0 {
		t.Fatal("construction performed network I/O")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for i, path := range paths {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			name := fmt.Sprintf("relay-%d", i)
			p, err := backend.provider(name)
			if err != nil {
				t.Fatal(err)
			}
			opts, err := responsesURLOptions(p.ChatURL)
			if err != nil {
				t.Fatal(err)
			}
			result, err := p.client.New(ctx, sdkRequest(), opts...)
			if err != nil || result.OutputText() != "ok" {
				t.Fatalf("SDK JSON response failed: %v", err)
			}
			stream := p.client.NewStreaming(ctx, sdkRequest(), opts...)
			defer stream.Close()
			text, terminal := "", false
			for stream.Next() {
				event := stream.Current()
				if event.Type == "response.output_text.delta" {
					text += event.Delta
				}
				terminal = terminal || event.Type == "response.completed"
			}
			if err := stream.Err(); err != nil || text != "ok" || !terminal {
				t.Fatalf("SDK SSE response failed: text=%q terminal=%v err=%v", text, terminal, err)
			}
			wantURI := path
			if wantURI == "" {
				wantURI = "/"
			}
			for range 2 {
				got := <-requests
				if got.uri != wantURI || got.key != "Bearer key-"+name {
					t.Fatalf("wrong endpoint or instance credential for %s: URI=%q", name, got.uri)
				}
			}
		})
	}
	if transportCalls.Load() != int32(len(paths)*2) {
		t.Fatal("SDK clients did not share the injected transport")
	}
}

func TestSDKServicesIgnoreEnvironmentDefaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "environment-key")
	t.Setenv("OPENAI_BASE_URL", "http://environment.invalid")
	t.Setenv("OPENAI_ORG_ID", "environment-org")
	t.Setenv("OPENAI_PROJECT_ID", "environment-project")
	t.Setenv("OPENAI_CUSTOM_HEADERS", "Authorization: Bearer environment-header\nX-Environment: unwanted")
	for _, key := range []string{"instance-key", ""} {
		t.Run(key, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				wantAuth := ""
				if key != "" {
					wantAuth = "Bearer " + key
				}
				if r.URL.String() != "https://configured.invalid/custom?route=1" || r.Header.Get("Authorization") != wantAuth {
					t.Error("SDK loaded another endpoint or credential source")
				}
				for _, header := range []string{"OpenAI-Organization", "OpenAI-Project", "X-Environment"} {
					if r.Header.Get(header) != "" {
						t.Errorf("SDK loaded environment header %s", header)
					}
				}
				return jsonResponse(sdkResponse), nil
			})}
			p := newTestProvider(t, "https://configured.invalid/custom?route=1", key, client)
			opts, err := responsesURLOptions(p.ChatURL)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.client.New(context.Background(), sdkRequest(), opts...); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEndpointOptionsAreScopedToEachCall(t *testing.T) {
	var requests []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.String())
		if r.Header.Get("Authorization") != "Bearer instance-key" || (r.Host != "" && r.Host != r.URL.Host) {
			t.Error("request lost the configured credential or target host")
		}
		return jsonResponse(sdkResponse), nil
	})}
	const savedURL = "https://saved.invalid/proxy/v8/responses?route=saved"
	p := newTestProvider(t, savedURL, "instance-key", client)
	want := []string{"https://probe.invalid/v9/responses?route=probe", savedURL}
	for _, endpoint := range want {
		opts, err := responsesURLOptions(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.client.New(context.Background(), sdkRequest(), opts...); err != nil {
			t.Fatal(err)
		}
	}
	// 不带端点选项时仍使用原 SDK service 的路由，候选不会改变后续调用。
	if _, err := p.client.New(context.Background(), sdkRequest()); err != nil {
		t.Fatal(err)
	}
	want = append(want, "https://saved.invalid/responses")
	if len(requests) != len(want) {
		t.Fatalf("unexpected request count: %d", len(requests))
	}
	for i := range want {
		if requests[i] != want[i] {
			t.Fatalf("request %d = %q, want %q", i, requests[i], want[i])
		}
	}
	if p.ChatURL != savedURL {
		t.Fatal("probe options changed the saved configuration")
	}
}

func TestSDKRequestsDoNotRetry(t *testing.T) {
	for _, status := range []int{0, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var attempts atomic.Int32
			transportErr := errors.New("test transport failure")
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts.Add(1)
				if status == 0 {
					return nil, transportErr
				}
				result := jsonResponse(`{"error":{"code":"rate_limit_exceeded","message":"test failure","type":"rate_limit_error"}}`)
				result.StatusCode = status
				result.Header.Set("Retry-After", "0")
				return result, nil
			})}
			p := newTestProvider(t, "https://configured.invalid/v1/responses", "key", client)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := p.client.New(ctx, sdkRequest())
			if err == nil || attempts.Load() != 1 {
				t.Fatalf("SDK retry policy changed: attempts=%d error=%v", attempts.Load(), err)
			}
			if status == 0 && !errors.Is(err, transportErr) {
				t.Fatalf("SDK lost the transport error chain: %v", err)
			}
		})
	}
}

func TestResponsesURLRejectsInvalidAddresses(t *testing.T) {
	for _, raw := range []string{
		"", "relay.invalid", "/relative", "ftp://relay.invalid", "https:///missing-host", "https://relay.invalid/%zz",
		"https://user:password@relay.invalid", "https://relay.invalid#fragment",
	} {
		t.Run(raw, func(t *testing.T) {
			p := adapter.Provider{Name: "relay", ProviderType: "openai", ChatURL: raw}
			if _, err := New(adapter.ProviderMap{"relay": p}, &http.Client{}, nil); err == nil {
				t.Fatal("invalid endpoint was accepted")
			}
			if _, err := responsesURLOptions(raw); err == nil {
				t.Fatal("invalid candidate was accepted")
			}
		})
	}
	u, err := parseResponsesURL("  https://relay.invalid/v8/responses/?token=a%2Fb  ")
	if err != nil || u.String() != "https://relay.invalid/v8/responses/?token=a%2Fb" {
		t.Fatalf("endpoint was rewritten: %v %v", u, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
