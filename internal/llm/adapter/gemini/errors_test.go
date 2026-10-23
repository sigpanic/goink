package gemini

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"google.golang.org/genai/interactions/models/apierrors"
	"google.golang.org/genai/interactions/models/components"
	types "google.golang.org/genai/interactions/models/interactions"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestSDKErrorKeepsWrappedHTTPMetadata(t *testing.T) {
	response := &http.Response{StatusCode: 503, Header: http.Header{"Retry-After": {"5"}}}
	cause := errors.New("schema mismatch")
	for _, err := range []error{
		&apierrors.CreateInteractionServerError{Error_: types.Error{Code: ptr("unavailable"), Message: ptr("overloaded")}, HTTPMeta: components.HTTPMetadata{Response: response}},
		apierrors.NewAPIError("unknown content type", 503, `{"error":{"code":"unavailable","message":"overloaded"}}`, response),
		apierrors.NewResponseValidationError("schema error", 503, `{"error":{"code":503,"status":"unavailable","message":"overloaded"}}`, response, cause),
	} {
		converted := convertSDKError(errors.Join(errors.New("wrapper"), err))
		var apiErr *adapter.APIError
		if !errors.As(converted, &apiErr) || apiErr.StatusCode != 503 || apiErr.Kind != "unavailable" || apiErr.Message != "overloaded" || !apiErr.Retryable || apiErr.RetryAfter != 5*time.Second {
			t.Fatalf("SDK error metadata lost: %+v", converted)
		}
	}
}

func TestRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want time.Duration
	}{
		{raw: "0"}, {raw: "-1"}, {raw: "invalid"}, {raw: "9223372036854775807"},
		{raw: "9223372036854775808"}, {raw: " 12 ", want: 12 * time.Second},
		{raw: time.Now().Add(-time.Minute).UTC().Format(http.TimeFormat)},
	} {
		if got := parseRetryAfter(tc.raw); got != tc.want {
			t.Fatalf("parseRetryAfter(%q)=%v, want %v", tc.raw, got, tc.want)
		}
	}
	got := parseRetryAfter(time.Now().Add(time.Minute).UTC().Format(http.TimeFormat))
	if got < 58*time.Second || got > time.Minute {
		t.Fatalf("future HTTP date lost: %v", got)
	}
}
