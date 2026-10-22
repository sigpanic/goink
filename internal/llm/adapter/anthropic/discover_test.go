package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestDiscoverModelsPreservesRouteCredentialsAndPagination(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := requests.Add(1)
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/tenant%2Fname/v8/models" ||
			r.URL.Query().Get("route") != "a+b" || r.URL.Query().Get("limit") != "1000" ||
			r.Header.Get("X-Api-Key") != "secret" || r.Header.Get("Anthropic-Version") == "" {
			t.Errorf("unexpected discovery request: %s %s %v", r.Method, r.URL, r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		if page == 1 {
			if r.URL.Query().Has("after_id") || r.URL.Query().Has("before_id") {
				t.Error("Messages query must not control Models pagination")
			}
			fmt.Fprint(w, `{"data":[{"id":"first","display_name":"Server name","max_input_tokens":123456,"max_tokens":4321,"capabilities":{"image_input":{"supported":true},"thinking":{"supported":true},"effort":{"supported":true,"low":{"supported":true},"high":{"supported":true}}}}],"has_more":true,"last_id":"cursor /+"}`)
		} else {
			if r.URL.Query().Get("after_id") != "cursor /+" || page != 2 {
				t.Errorf("lost or repeated cursor: %s", r.URL)
			}
			fmt.Fprint(w, `{"data":[{"id":"first"},{"id":"unknown"},{"id":"claude-opus-5-5"}],"has_more":false}`)
		}
	}))
	defer server.Close()
	p := adapter.Provider{Name: "relay", ProviderType: "anthropic", APIKey: "secret", ChatURL: server.URL + "/tenant%2Fname/v8/messages?route=a%2Bb&after_id=stale&before_id=stale"}
	backend, err := New(adapter.ProviderMap{p.Name: p}, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	models, err := backend.DiscoverModels(context.Background(), "relay")
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 || len(models) != 3 {
		t.Fatalf("unexpected pages or models: %d %+v", requests.Load(), models)
	}
	want := adapter.ModelInfo{ID: "first", Name: "Server name", ContextWindow: 123456, MaxOutputTokens: 4321, SupportsVision: true, SupportsThinking: true, ReasoningLevels: []string{"low", "high"}}
	if !reflect.DeepEqual(models[0], want) || models[1].Name != "unknown" || models[1].ContextWindow != 0 || !models[2].SupportsThinking {
		t.Fatalf("incorrect server metadata or exact-ID fallback: %+v", models)
	}
	models[2].ReasoningLevels[0] = "mutated"
	if BuiltinProviders()["Anthropic"].Models[0].ReasoningLevels[0] != "medium" || backend.Providers()["relay"].ChatURL != p.ChatURL {
		t.Fatal("discovery mutated the instance or builtin data")
	}
}

func TestDiscoveredCapabilitiesOverridePresetsWithoutGuessingNames(t *testing.T) {
	p := Provider{Provider: adapter.Provider{Models: []adapter.ModelInfo{{ID: "custom", Name: "Local", ContextWindow: 111, SupportsVision: true, SupportsThinking: true, ReasoningLevels: []string{"high", "low"}}}}}
	for _, tt := range []struct {
		raw  string
		want adapter.ModelInfo
	}{
		{`{"id":"custom"}`, p.Models[0]},
		{`{"id":"custom","capabilities":{"image_input":{"supported":false},"thinking":{"supported":false}}}`, adapter.ModelInfo{ID: "custom", Name: "Local", ContextWindow: 111}},
		{`{"id":"custom","capabilities":{"effort":{"supported":true,"low":{"supported":true},"high":{"supported":true}}}}`, p.Models[0]},
		{`{"id":"custom","capabilities":{"effort":{"supported":false}}}`, adapter.ModelInfo{ID: "custom", Name: "Local", ContextWindow: 111, SupportsThinking: true, SupportsVision: true}},
		{`{"id":"unknown","display_name":"Claude Opus 5.5"}`, adapter.ModelInfo{ID: "unknown", Name: "Claude Opus 5.5"}},
	} {
		var model sdk.ModelInfo
		if err := json.Unmarshal([]byte(tt.raw), &model); err != nil {
			t.Fatal(err)
		}
		if got := p.modelInfo(model); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("mapping %s = %+v; want %+v", tt.raw, got, tt.want)
		}
	}
}

func TestDiscoverModelsRejectsBrokenPagesAndPreservesErrors(t *testing.T) {
	for _, body := range []string{
		`{}`, `{"data":null}`, `{"data":{}}`, `{"data":[{"id":3}]}`,
		`{"data":[{"id":"one"}],"has_more":true}`,
		`{"data":[{"id":"one"}],"has_more":true,"last_id":"repeated"}`,
	} {
		t.Run(body, func(t *testing.T) {
			var requests atomic.Int32
			backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				if requests.Add(1) > 2 {
					t.Error("discovery did not stop repeated pagination")
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, body)
			})
			if models, err := backend.DiscoverModels(context.Background(), "relay"); err == nil || models != nil {
				t.Fatalf("broken page returned partial success: %+v %v", models, err)
			}
		})
	}
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"limited"}}`)
	})
	_, err := backend.DiscoverModels(context.Background(), "relay")
	var apiErr *adapter.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 || apiErr.RetryAfter != 2*time.Second {
		t.Fatalf("SDK error metadata lost: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := backend.DiscoverModels(ctx, "relay"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if _, err := backend.DiscoverModels(context.Background(), "missing"); err == nil {
		t.Fatal("unknown provider fell back")
	}
}

func TestDiscoverModelsDoesNotGuessCustomRoutes(t *testing.T) {
	backend, err := New(adapter.ProviderMap{"relay": {Name: "relay", ProviderType: "anthropic", ChatURL: "https://relay.invalid/proxy/chat"}}, &http.Client{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.DiscoverModels(context.Background(), "relay"); err == nil {
		t.Fatal("arbitrary route must require manual model configuration")
	}
}
