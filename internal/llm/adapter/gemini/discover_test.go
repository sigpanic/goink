package gemini

import (
	"context"
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

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestDiscoverModelsPaginationAndMetadata(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "environment-key")
	t.Setenv("GEMINI_API_KEY", "other-environment-key")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/tenant%2Fname/v8/models" ||
			r.Header.Get("X-Goog-Api-Key") != "instance-key" || r.Header.Get("Authorization") != "" ||
			r.URL.Query().Get("route") != "a/b" || !reflect.DeepEqual(r.URL.Query()["tag"], []string{"1", "2"}) || r.URL.Query().Get("pageSize") != "1000" {
			t.Errorf("Models route, credentials or query changed: %s %v", r.URL, r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("pageToken") {
		case "":
			fmt.Fprint(w, `{"models":[
				{"name":"models/gemini-3.8-flash","displayName":"Server Flash","inputTokenLimit":1234,"outputTokenLimit":456,"supportedGenerationMethods":["generateContent"],"thinking":false},
				{"name":"models/gemini-2.5-pro"},
				{"name":"models/embedding","supportedGenerationMethods":["embedContent"]}
			],"nextPageToken":"cursor+/="}`)
		case "cursor+/=":
			fmt.Fprint(w, `{"models":[
				{"name":"models/gemini-3.8-flash"},
				{"name":"models/configured"},
				{"name":"models/future","thinking":true,"inputTokenLimit":99},
				{"name":"relay-alias","supportedGenerationMethods":["generateContent"]}
			]}`)
		default:
			t.Errorf("unexpected pagination token %q", r.URL.Query().Get("pageToken"))
		}
	}))
	defer server.Close()
	config := adapter.Provider{Name: "relay", ProviderType: "google", APIKey: "instance-key",
		ChatURL: server.URL + "/tenant%2Fname/v8/interactions?route=a%2Fb&tag=1&tag=2&pageToken=stale&pageSize=1",
		Models:  []adapter.ModelInfo{{ID: "configured", Name: "Configured", ContextWindow: 2048, SupportsVision: true, SupportsThinking: true, ReasoningLevels: []string{"low"}}}}
	backend, err := New(adapter.ProviderMap{config.Name: config}, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	models, err := backend.DiscoverModels(context.Background(), "relay")
	if err != nil || len(models) != 5 || requests.Load() != 2 {
		t.Fatalf("pagination/deduplication: %+v %v; requests=%d", models, err, requests.Load())
	}
	if models[0].ID != "gemini-3.8-flash" || models[0].Name != "Server Flash" || models[0].ContextWindow != 1234 || models[0].MaxOutputTokens != 456 || models[0].SupportsThinking || len(models[0].ReasoningLevels) != 0 || !models[0].SupportsVision {
		t.Fatalf("server metadata and exact preset fallback: %+v", models[0])
	}
	if !models[1].SupportsThinking || len(models[1].ReasoningLevels) == 0 || !models[2].SupportsVision || models[2].Name != "Configured" || models[3].SupportsVision || len(models[3].ReasoningLevels) != 0 || !models[3].SupportsThinking || models[4].ID != "relay-alias" {
		t.Fatalf("configured metadata or conservative unknown model mapping: %+v", models)
	}
	models[2].ReasoningLevels[0] = "changed"
	if backend.Providers()["relay"].Models[0].ReasoningLevels[0] != "low" || backend.Providers()["relay"].ChatURL != config.ChatURL {
		t.Fatal("discovery result changed provider snapshot")
	}
}

func TestDiscoverModelsRejectsInvalidResponsesAndCursors(t *testing.T) {
	for _, raw := range []string{
		`<html>proxy</html>`, `{}`, `{"data":[]}`, `{"models":null}`, `{"models":[{}]}`,
		`{"models":[{"name":"models/"}]}`, `{"models":[{"name":"models/a/b"}]}`,
		`{"models":[{"name":" models/a"}]}`, `{"models":[{"name":"models/a","inputTokenLimit":"large"}]}`,
		`{"models":[],"nextPageToken":"next"}`, `{"models":[{"name":"models/a"}],"nextPageToken":"repeat"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			var requests atomic.Int32
			backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				fmt.Fprint(w, raw)
			})
			models, err := backend.DiscoverModels(context.Background(), "relay")
			if err == nil || models != nil || requests.Load() > 2 {
				t.Fatalf("invalid catalog accepted or pagination loop: %+v %v (%d requests)", models, err, requests.Load())
			}
		})
	}
}

func TestDiscoverModelsRoutesErrorsAndEmptyKey(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "environment-key")
	for _, path := range []string{"/v1/interactions", "/v1beta/interactions/", "/proxy/chat"} {
		t.Run(path, func(t *testing.T) {
			var requests atomic.Int32
			shared := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests.Add(1)
				if r.URL.EscapedPath() != strings.TrimSuffix(strings.TrimRight(path, "/"), "/interactions")+"/models" || r.Header.Get("X-Goog-Api-Key") != "" {
					t.Errorf("route or empty key changed: %s %v", r.URL, r.Header)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"models":[]}`))}, nil
			})}
			p := newTestProvider(t, "https://configured.invalid"+path, "", shared)
			backend := &Adapter{providers: map[string]Provider{"relay": p}}
			models, err := backend.DiscoverModels(context.Background(), "relay")
			if path == "/proxy/chat" {
				if err == nil || requests.Load() != 0 {
					t.Fatal("arbitrary route caused a guessed Models request")
				}
			} else if err != nil || models == nil || len(models) != 0 || requests.Load() != 1 {
				t.Fatalf("valid empty catalog: %+v %v", models, err)
			}
		})
	}
	var requests atomic.Int32
	backend := generateAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"quota"}}`)
	})
	_, err := backend.DiscoverModels(context.Background(), "relay")
	var apiErr *adapter.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 || apiErr.Kind != "RESOURCE_EXHAUSTED" || apiErr.RetryAfter != 3*time.Second || requests.Load() != 1 {
		t.Fatalf("Models error lost metadata or retried: %v (%d requests)", err, requests.Load())
	}
	if _, err := backend.DiscoverModels(context.Background(), "missing"); err == nil || requests.Load() != 1 {
		t.Fatal("missing provider caused a fallback request")
	}
}

func TestDiscoverModelsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var requests atomic.Int32
	shared := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		cancel()
		return nil, r.Context().Err()
	})}
	p := newTestProvider(t, "https://configured.invalid/v1/interactions", "key", shared)
	backend := &Adapter{providers: map[string]Provider{"relay": p}}
	if _, err := backend.DiscoverModels(ctx, "relay"); !errors.Is(err, context.Canceled) || requests.Load() != 1 {
		t.Fatalf("discovery ignored cancellation: %v", err)
	}
}
