package openairesponses

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3/responses"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// convertResponse 供完整响应和流式终态复用；整组校验通过后才交付工具和原生回放状态。
func (p Provider) convertResponse(params responses.ResponseNewParams, result *responses.Response) (adapter.Response, error) {
	if result == nil {
		return adapter.Response{}, invalidResponse("missing SDK response")
	}
	var native struct {
		ID                json.RawMessage `json:"id"`
		Object            json.RawMessage `json:"object"`
		Status            string          `json:"status"`
		Output            json.RawMessage `json:"output"`
		Usage             json.RawMessage `json:"usage"`
		Error             json.RawMessage `json:"error"`
		IncompleteDetails struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
	}
	// 从 SDK 保存的原文检查标量类型，避免宽松解码将数字 ID、非字符串文本伪装成合法数据。
	if err := json.Unmarshal([]byte(result.RawJSON()), &native); err != nil {
		return adapter.Response{}, invalidResponse("invalid response envelope")
	}
	if len(native.Error) > 0 && string(native.Error) != "null" {
		var fault struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(native.Error, &fault); err != nil || fault.Message == "" {
			return adapter.Response{}, invalidResponse("invalid response error")
		}
		if fault.Code == "" {
			fault.Code = "response_failed"
		}
		return adapter.Response{}, &adapter.APIError{
			Kind:      fault.Code,
			Message:   fault.Message,
			Retryable: fault.Code == "server_error" || fault.Code == "rate_limit_exceeded" || fault.Code == "vector_store_timeout",
		}
	}
	switch native.Status {
	case "completed", "":
	case "incomplete", "failed", "cancelled":
		message := "OpenAI response is " + native.Status
		if native.IncompleteDetails.Reason != "" {
			message += ": " + native.IncompleteDetails.Reason
		}
		return adapter.Response{}, &adapter.APIError{Kind: "response_" + native.Status, Message: message}
	default:
		return adapter.Response{}, invalidResponse("unsupported or nonterminal status " + native.Status)
	}
	_, text, calls, err := replayProjection(native.Output)
	if err != nil {
		var apiErr *adapter.APIError
		if errors.As(err, &apiErr) {
			return adapter.Response{}, err
		}
		return adapter.Response{}, invalidResponse(err.Error())
	}
	thinking, err := reasoningSummary(result.Output)
	if err != nil {
		return adapter.Response{}, err
	}
	usage, err := normalizeUsage(native.Usage)
	if err != nil {
		return adapter.Response{}, err
	}
	hash, err := p.requestHash(params)
	if err != nil {
		return adapter.Response{}, err
	}
	payload, err := json.Marshal(replayPayload{Output: native.Output, RequestHash: hash})
	if err != nil {
		return adapter.Response{}, fmt.Errorf("encode OpenAI Responses response state: %w", err)
	}
	response := adapter.Response{Content: text, ThinkingContent: thinking, Usage: usage, StopReason: native.Status}
	for _, call := range calls {
		response.ToolCalls = append(response.ToolCalls, adapter.ToolCallDelta{
			ToolID:        call.ID,
			ToolName:      call.Name,
			ArgumentsText: string(call.Arguments),
			ArgumentsJSON: slices.Clone(call.Arguments),
		})
	}
	response.State = &adapter.ResponseState{
		Origin: adapter.MessageOrigin{
			AdapterName:  adapter.ProtocolOpenAIResponses,
			ProviderName: p.Name,
			ProviderType: p.ProviderType,
			Model:        params.Model,
		},
		Complete:     true,
		TurnComplete: len(calls) == 0,
		StopReason:   native.Status,
		Payload:      payload,
	}
	return response, nil
}

func reasoningSummary(output []responses.ResponseOutputItemUnion) (string, error) {
	var thinking strings.Builder
	for _, item := range output {
		if item.Type != "reasoning" {
			continue
		}
		var native struct {
			Summary []struct {
				Type string  `json:"type"`
				Text *string `json:"text"`
			} `json:"summary"`
		}
		if err := json.Unmarshal([]byte(item.RawJSON()), &native); err != nil {
			continue
		}
		for _, part := range native.Summary {
			if part.Type == "summary_text" && part.Text != nil {
				thinking.WriteString(*part.Text)
			}
		}
	}
	return thinking.String(), nil
}

// normalizeUsage 保留服务端总量；output_tokens 已包含 reasoning，缓存及推理明细均不再次累加。
// 公共计数沿用现有消费者使用的 float64；没有 usage 时不伪造零值。
func normalizeUsage(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var usage struct {
		InputTokens        *int64 `json:"input_tokens"`
		OutputTokens       *int64 `json:"output_tokens"`
		TotalTokens        *int64 `json:"total_tokens"`
		InputTokensDetails *struct {
			CachedTokens     *int64 `json:"cached_tokens"`
			CacheWriteTokens *int64 `json:"cache_write_tokens"`
		} `json:"input_tokens_details"`
		OutputTokensDetails *struct {
			ReasoningTokens *int64 `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
	}
	if err := json.Unmarshal(raw, &usage); err != nil || usage.InputTokens == nil || usage.OutputTokens == nil || usage.TotalTokens == nil ||
		*usage.InputTokens < 0 || *usage.OutputTokens < 0 || *usage.TotalTokens < 0 {
		return nil, nil
	}
	result := map[string]any{
		"prompt_tokens":     float64(*usage.InputTokens),
		"completion_tokens": float64(*usage.OutputTokens),
		"total_tokens":      float64(*usage.TotalTokens),
	}
	if details := usage.InputTokensDetails; details != nil {
		if cached := details.CachedTokens; cached != nil {
			if *cached < 0 || *cached > *usage.InputTokens {
				return nil, nil
			}
			result["prompt_cache_hit_tokens"] = float64(*cached)
			result["prompt_cache_miss_tokens"] = float64(*usage.InputTokens - *cached)
		}
		if written := details.CacheWriteTokens; written != nil {
			if *written < 0 || *written > *usage.InputTokens {
				return nil, nil
			}
			result["cache_creation_input_tokens"] = float64(*written)
		}
	}
	if details := usage.OutputTokensDetails; details != nil && details.ReasoningTokens != nil {
		if *details.ReasoningTokens < 0 || *details.ReasoningTokens > *usage.OutputTokens {
			return nil, nil
		}
		result["completion_tokens_details"] = map[string]any{"reasoning_tokens": float64(*details.ReasoningTokens)}
	}
	return result, nil
}
