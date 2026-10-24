package openairesponses

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	sdk "github.com/openai/openai-go/v3"

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
	for _, sentinel := range []error{context.Canceled, context.DeadlineExceeded} {
		if err := convertSDKError(sentinel, response); !errors.Is(err, sentinel) {
			t.Fatalf("HTTP status hid cancellation: %v", err)
		}
	}
}

func TestRetryAfterHandlesBoundsAndDates(t *testing.T) {
	for value, want := range map[string]time.Duration{
		"0": 0, " 12 ": 12 * time.Second, "-1": 0, "9223372036854775807": 0,
		"9223372036854775808": 0, "not a date": 0, "1.5": 0, "": 0,
	} {
		if got := parseRetryAfter(value); got != want {
			t.Fatalf("Retry-After %q = %v, want %v", value, got, want)
		}
	}
	if got := parseRetryAfter(time.Now().Add(time.Minute).UTC().Format(http.TimeFormat)); got < 58*time.Second || got > time.Minute {
		t.Fatalf("future HTTP date not preserved: %v", got)
	}
	if got := parseRetryAfter(time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)); got != 0 {
		t.Fatalf("past HTTP date produced a delay: %v", got)
	}
}
