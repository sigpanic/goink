package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/genai/interactions/models/apierrors"
	types "google.golang.org/genai/interactions/models/interactions"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func convertSDKError(err error) error {
	var clientErr *apierrors.CreateInteractionClientError
	var serverErr *apierrors.CreateInteractionServerError
	var apiErr *apierrors.APIError
	var validationErr *apierrors.ResponseValidationError
	var response *http.Response
	var native types.Error
	var raw string
	var sdkMessage string
	status := 0
	switch {
	case errors.As(err, &clientErr):
		response, native = clientErr.HTTPMeta.Response, clientErr.Error_
	case errors.As(err, &serverErr):
		response, native = serverErr.HTTPMeta.Response, serverErr.Error_
	case errors.As(err, &apiErr):
		response, status, raw = apiErr.RawResponse, apiErr.StatusCode, apiErr.Body
		sdkMessage = apiErr.Message
	case errors.As(err, &validationErr):
		response, status, raw = validationErr.RawResponse, validationErr.StatusCode, validationErr.Body
		sdkMessage = validationErr.Message
	default:
		// 保留 context、读取和传输错误链，供调用方 errors.Is / errors.As 判断。
		return fmt.Errorf("gemini request failed: %w", err)
	}
	if response != nil {
		status = response.StatusCode
	}
	message, kind := http.StatusText(status), ""
	if status < http.StatusBadRequest && sdkMessage != "" {
		message = sdkMessage
	}
	if native.Code != nil {
		kind = *native.Code
	}
	if native.Message != nil && *native.Message != "" {
		message = *native.Message
	}
	if raw != "" {
		// 代理也可能返回旧式 Google 错误（numeric code + status），不能因此丢失 HTTP 状态。
		var envelope struct {
			Error struct {
				Code    json.RawMessage `json:"code"`
				Status  string          `json:"status"`
				Message string          `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(raw), &envelope) == nil {
			kind = envelope.Error.Status
			var code string
			if json.Unmarshal(envelope.Error.Code, &code) == nil && code != "" {
				kind = code
			}
			if envelope.Error.Message != "" {
				message = envelope.Error.Message
			}
		}
	}
	var retryAfter time.Duration
	if response != nil {
		retryAfter = adapter.ParseRetryAfter(response.Header.Get("Retry-After"))
	}
	return &adapter.APIError{StatusCode: status, Message: message, Kind: kind, Retryable: adapter.StatusRetryable(status), RetryAfter: retryAfter, Cause: err}
}
