package openairesponses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func convertSDKError(err error, response *http.Response) error {
	var streamed *ssestream.StreamError
	if errors.As(err, &streamed) {
		return streamError(streamed.Event.Data)
	}
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

func streamError(raw json.RawMessage) error {
	var fault struct {
		Code    string          `json:"code"`
		Type    string          `json:"type"`
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(raw, &fault); err != nil {
		return invalidResponse("invalid streamed error")
	}
	if len(fault.Error) != 0 {
		return streamError(fault.Error)
	}
	if fault.Message == "" {
		return invalidResponse("streamed error requires a message")
	}
	if fault.Code == "" {
		fault.Code = fault.Type
	}
	if fault.Code == "" || fault.Code == "error" {
		fault.Code = "response_failed"
	}
	return &adapter.APIError{
		Kind:      fault.Code,
		Message:   fault.Message,
		Retryable: fault.Code == "server_error" || fault.Code == "rate_limit_exceeded" || fault.Code == "vector_store_timeout",
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
