package openairesponses

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestDiscoverModelsUsesSDKCredentialsAndExactModelMetadata(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "unwanted-environment-key")
	t.Setenv("OPENAI_BASE_URL", "https://unwanted.invalid")
	var attempts atomic.Int32
	backend, err := New(adapter.ProviderMap{"relay": {
		Name:         "relay",
		ProviderType: "openai",
		ChatURL:      "https://configured.invalid/tenant%2Fname/v8/responses/?route=a%2Fb&x=1&x=2",
		APIKey:       "instance-key",
		Models: []adapter.ModelInfo{{
			ID: "custom", Name: "Local model", ContextWindow: 1234, MaxOutputTokens: 456, SupportsThinking: true, ReasoningLevels: []string{"high", "low"},
		}},
	}}, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts.Add(1)
		if r.Method != http.MethodGet || r.URL.String() != "https://configured.invalid/tenant%2Fname/v8/models?route=a%2Fb&x=1&x=2" ||
			r.Header.Get("Authorization") != "Bearer instance-key" || r.Header.Get("Accept") != "application/json" {
			t.Error("Models request lost complete route, query or instance SDK settings")
		}
		return jsonResponse(`{"object":"list","data":[{"id":"custom"},{"id":"gpt-6.1-sol"},{"id":"custom"},{"id":"gpt-6.1-sol-unverified-snapshot"},{"id":"text-embedding-3-small"}]}`), nil
	})}, nil)
	if err != nil {
		t.Fatal(err)
	}
	before := backend.Providers()
	models, err := backend.DiscoverModels(context.Background(), "relay")
	if err != nil || attempts.Load() != 1 || len(models) != 4 {
		t.Fatalf("discovery failed or duplicated IDs: %+v %v", models, err)
	}
	if !reflect.DeepEqual(models[0], before["relay"].Models[0]) || models[1].ID != "gpt-6.1-sol" || !models[1].SupportsThinking || !models[1].SupportsVision || models[1].ContextWindow == 0 ||
		!reflect.DeepEqual(models[2], adapter.ModelInfo{ID: "gpt-6.1-sol-unverified-snapshot", Name: "gpt-6.1-sol-unverified-snapshot"}) || models[3].ID != "text-embedding-3-small" {
		t.Fatalf("discovery guessed model families, lost metadata or claimed catalog filtering: %+v", models)
	}
	models[0].ReasoningLevels[0] = "changed"
	models[1].ReasoningLevels[0] = "changed"
	if !reflect.DeepEqual(before, backend.Providers()) || BuiltinProviders()["OpenAI"].Models[1].ReasoningLevels[0] == "changed" {
		t.Fatal("discovery shared mutable model or provider configuration")
	}
}

func TestDiscoverModelsRejectsInvalidOrPartialCatalogs(t *testing.T) {
	for _, body := range []string{
		`null`, `{}`, `{"data":null}`, `{"data":{}}`, `{"data":true}`,
		`{"object":"response","data":[]}`,
		`{"data":[{"id":3}]}`, `{"data":[{"id":""}]}`, `{"data":[{"id":" model "}]}`,
		`{"data":[{"id":"valid"},{"id":false}]}`,
		`{"data":[{"id":"first"}],"has_more":true,"last_id":"first"}`,
		`{"data":[{"id":"first"}],"has_more":"true"}`,
	} {
		t.Run(body, func(t *testing.T) {
			backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return jsonResponse(body), nil
			}))
			if models, err := backend.DiscoverModels(context.Background(), "relay"); err == nil || models != nil {
				t.Fatalf("invalid catalog produced partial success: %+v %v", models, err)
			}
		})
	}
	backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(`{"object":"list","data":[]}`), nil
	}))
	if models, err := backend.DiscoverModels(context.Background(), "relay"); err != nil || models == nil || len(models) != 0 {
		t.Fatalf("valid empty catalog was rejected: %+v %v", models, err)
	}
}

func TestDiscoverModelsPreservesHTTPAndCancellationErrorsWithoutRetry(t *testing.T) {
	for _, status := range []int{401, 429, 500} {
		var attempts atomic.Int32
		backend := generateAdapter(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if err := r.Context().Err(); err != nil {
				return nil, err
			}
			attempts.Add(1)
			result := jsonResponse(`{"error":{"code":"test_error","message":"failed"}}`)
			result.StatusCode = status
			result.Header.Set("Retry-After", "3")
			return result, nil
		}))
		_, err := backend.DiscoverModels(context.Background(), "relay")
		var fault *adapter.APIError
		if !errors.As(err, &fault) || fault.StatusCode != status || fault.RetryAfter != 3*time.Second || fault.Kind != "test_error" || attempts.Load() != 1 {
			t.Fatalf("discovery lost HTTP failure metadata or retried: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := backend.DiscoverModels(ctx, "relay"); !errors.Is(err, context.Canceled) || attempts.Load() != 1 {
			t.Fatalf("discovery ignored cancellation: %v", err)
		}
	}
}

func TestDiscoverModelsDoesNotGuessAmbiguousRoutesOrProviders(t *testing.T) {
	var attempts atomic.Int32
	backend, err := New(adapter.ProviderMap{"relay": {Name: "relay", ProviderType: "openai", ChatURL: "https://configured.invalid/proxy/chat"}}, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts.Add(1)
		return nil, errors.New("unexpected network request")
	})}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.DiscoverModels(context.Background(), "relay"); err == nil || !strings.Contains(err.Error(), "manually") {
		t.Fatal("ambiguous route must leave model configuration to the user")
	}
	if _, err := backend.DiscoverModels(context.Background(), "missing"); err == nil || attempts.Load() != 0 {
		t.Fatal("unknown provider fell back or issued a request")
	}
}
