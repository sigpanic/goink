package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func convertSDKError(err error) error {
	var apiErr *sdk.Error
	if !errors.As(err, &apiErr) {
		// 保留 context 和传输错误链，调用方可继续使用 errors.Is / errors.As。
		return fmt.Errorf("anthropic request failed: %w", err)
	}
	var envelope struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	message := http.StatusText(apiErr.StatusCode)
	kind := string(apiErr.Type())
	if json.Unmarshal([]byte(apiErr.RawJSON()), &envelope) == nil {
		if envelope.Error.Message != "" {
			message = envelope.Error.Message
		}
		if envelope.Error.Type != "" {
			kind = envelope.Error.Type
		}
	}
	var retryAfter time.Duration
	if apiErr.Response != nil {
		retryAfter = adapter.ParseRetryAfter(apiErr.Response.Header.Get("Retry-After"))
	}
	return &adapter.APIError{
		StatusCode: apiErr.StatusCode, Message: message, Kind: kind,
		Retryable: adapter.StatusRetryable(apiErr.StatusCode) || (apiErr.StatusCode == http.StatusOK &&
			(kind == "overloaded_error" || kind == "rate_limit_error" || kind == "api_error")), RetryAfter: retryAfter,
		Cause: err,
	}
}
