package openairesponses

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestConvertSDKErrorPreservesTypeFallbackAndCancellation(t *testing.T) {
	response := &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"5"}}}
	fault := &sdk.Error{StatusCode: 429, Response: response, Type: "rate_limit_error", Message: "slow down"}
	err := convertSDKError(fault, nil)
	var converted *adapter.APIError
	if !errors.As(err, &converted) || converted.Kind != "rate_limit_error" || converted.Message != "slow down" || !converted.Retryable || converted.RetryAfter != 5*time.Second {
		t.Fatalf("SDK error metadata lost: %v", err)
	}
	var original *sdk.Error
	if !errors.Is(err, fault) || !errors.As(err, &original) || original != fault {
		t.Fatal("original SDK HTTP error was lost")
	}
	for _, sentinel := range []error{context.Canceled, context.DeadlineExceeded} {
		if err := convertSDKError(sentinel, response); !errors.Is(err, sentinel) {
			t.Fatalf("HTTP status hid cancellation: %v", err)
		}
	}
}

func TestConvertStreamErrorPreservesCause(t *testing.T) {
	fault := &ssestream.StreamError{
		Message: "upstream error event",
		Event:   ssestream.Event{Data: []byte(`{"error":{"code":"server_error","message":"overloaded"}}`)},
	}
	err := convertSDKError(fault, nil)
	var converted *adapter.APIError
	if !errors.As(err, &converted) || converted.Kind != "server_error" || converted.Message != "overloaded" || !converted.Retryable || !errors.Is(err, fault) {
		t.Fatalf("stream error metadata or cause lost: %v", err)
	}
}
