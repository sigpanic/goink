package openairesponses

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	sdk "github.com/openai/openai-go/v3"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func convertSDKError(err error, response *http.Response) error {
	var fault *sdk.Error
	if errors.As(err, &fault) && fault.Response != nil {
		response = fault.Response
	}
	if response == nil || response.StatusCode < 400 || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		// 保留取消及非 HTTP 错误链；本包主动校验的 APIError 也可继续由 errors.As 识别。
		return fmt.Errorf("OpenAI Responses request failed: %w", err)
	}
	message, kind := http.StatusText(response.StatusCode), ""
	if fault != nil {
		if fault.Message != "" {
			message = fault.Message
		}
		kind = fault.Code
		if kind == "" {
			kind = fault.Type
		}
	}
	return &adapter.APIError{
		StatusCode: response.StatusCode,
		Message:    message,
		Kind:       kind,
		Retryable:  adapter.StatusRetryable(response.StatusCode),
		RetryAfter: parseRetryAfter(response.Header.Get("Retry-After")),
	}
}

func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds >= 0 && seconds <= int64((1<<63-1)/time.Second) {
			return time.Duration(seconds) * time.Second
		}
		return 0
	}
	if date, err := http.ParseTime(value); err == nil {
		if delay := time.Until(date); delay > 0 {
			return delay
		}
	}
	return 0
}

func invalidResponse(message string) error {
	return &adapter.APIError{Kind: "invalid_response", Message: "invalid OpenAI Responses response: " + message}
}
