package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	types "google.golang.org/genai/interactions/models/interactions"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// convertResponse 校验终态、整组工具及原文后，才交付可执行调用和回放状态。
func (p Provider) convertResponse(params types.CreateModelInteraction, raw json.RawMessage) (adapter.Response, error) {
	var interaction struct {
		ID     json.RawMessage `json:"id"`
		Object json.RawMessage `json:"object"`
		Agent  json.RawMessage `json:"agent"`
		Status string          `json:"status"`
		Steps  json.RawMessage `json:"steps"`
		Usage  json.RawMessage `json:"usage"`
		Errors []types.Error   `json:"errors"`
	}
	if err := json.Unmarshal(raw, &interaction); err != nil {
		return adapter.Response{}, invalidResponse("missing or invalid interaction envelope")
	}
	if params.Store == nil || *params.Store {
		if _, err := parseInteractionID(interaction.ID, false); err != nil {
			return adapter.Response{}, err
		}
	}
	if len(interaction.Errors) > 0 {
		fault := interaction.Errors[0]
		kind, message := "interaction_failed", "Gemini interaction reported an error"
		if fault.Code != nil && *fault.Code != "" {
			kind = *fault.Code
		}
		if fault.Message != nil && *fault.Message != "" {
			message = *fault.Message
		}
		return adapter.Response{}, &adapter.APIError{Kind: kind, Message: message}
	}
	switch interaction.Status {
	case "completed", "requires_action", "":
	case "incomplete", "failed", "cancelled":
		return adapter.Response{}, &adapter.APIError{Kind: "interaction_" + interaction.Status, Message: "Gemini interaction is " + interaction.Status}
	default:
		return adapter.Response{}, invalidResponse("unsupported or nonterminal status " + interaction.Status)
	}
	steps, text, calls, err := replayProjection(interaction.Steps)
	if err != nil {
		var apiErr *adapter.APIError
		if errors.As(err, &apiErr) {
			return adapter.Response{}, err
		}
		return adapter.Response{}, invalidResponse(err.Error())
	}
	if interaction.Status == "" {
		interaction.Status = "completed"
		if len(calls) > 0 {
			interaction.Status = "requires_action"
		}
	}
	var thinking strings.Builder
	for _, step := range steps {
		var native struct {
			Type    string          `json:"type"`
			Summary json.RawMessage `json:"summary"`
		}
		if err := json.Unmarshal(step.UnknownRaw, &native); err != nil {
			return adapter.Response{}, invalidResponse("invalid native step")
		}
		if native.Type != "thought" || len(native.Summary) == 0 {
			continue
		}
		var summary []struct {
			Type string  `json:"type"`
			Text *string `json:"text"`
		}
		if err := json.Unmarshal(native.Summary, &summary); err != nil {
			continue
		}
		for _, content := range summary {
			switch content.Type {
			case "text":
				if content.Text != nil {
					thinking.WriteString(*content.Text)
				}
			case "image":
				// 当前只显示文本摘要；图片摘要随原生 thought 原样保存和回放。
			default:
			}
		}
	}
	usage, err := normalizeUsage(interaction.Usage)
	if err != nil {
		return adapter.Response{}, err
	}
	hash, err := p.requestHash(params)
	if err != nil {
		return adapter.Response{}, err
	}
	payload, err := json.Marshal(replayPayload{Steps: interaction.Steps, RequestHash: hash})
	if err != nil {
		return adapter.Response{}, fmt.Errorf("encode Gemini response state: %w", err)
	}
	response := adapter.Response{Content: text, ThinkingContent: thinking.String(), Usage: usage, StopReason: interaction.Status}
	for _, call := range calls {
		response.ToolCalls = append(response.ToolCalls, adapter.ToolCallDelta{
			ToolID: call.ID, ToolName: call.Name, ArgumentsText: string(call.Arguments), ArgumentsJSON: slices.Clone(call.Arguments),
		})
	}
	response.State = &adapter.ResponseState{
		Origin:   adapter.MessageOrigin{AdapterName: adapter.ProtocolGeminiInteractions, ProviderName: p.Name, ProviderType: p.ProviderType, Model: string(params.Model)},
		Complete: true, TurnComplete: len(calls) == 0, StopReason: interaction.Status, Payload: payload,
	}
	return response, nil
}

func invalidResponse(message string) error {
	return &adapter.APIError{Kind: "invalid_response", Message: "invalid Gemini response: " + message}
}

// 显式 store=false 的官方响应实测会省略 ID；字段存在时仍须是非空字符串。
func parseInteractionID(raw json.RawMessage, optional bool) (string, error) {
	if len(raw) == 0 && optional {
		return "", nil
	}
	var id string
	if json.Unmarshal(raw, &id) != nil || strings.TrimSpace(id) == "" {
		return "", invalidResponse("missing or invalid interaction ID")
	}
	return id, nil
}

// normalizeUsage 的 output 不包含 thought；cache 和 tool-use 只是明细，不能重复累加。
// 保留服务端 total_tokens，缺失的字段不冒充零；公共计数仍为 float64。
func normalizeUsage(raw json.RawMessage) (map[string]any, error) {
	var counters map[string]*int64
	var fields map[string]json.RawMessage
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, nil
	}
	counters = make(map[string]*int64)
	for _, key := range []string{"total_input_tokens", "total_output_tokens", "total_thought_tokens", "total_cached_tokens", "total_tool_use_tokens", "total_tokens"} {
		value, present := fields[key]
		if !present {
			continue
		}
		var count *int64
		if err := json.Unmarshal(value, &count); err != nil || count == nil || *count < 0 {
			return nil, nil
		}
		counters[key] = count
	}
	input, output, thought := counters["total_input_tokens"], counters["total_output_tokens"], counters["total_thought_tokens"]
	cached, total := counters["total_cached_tokens"], counters["total_tokens"]
	if cached != nil && input != nil && *cached > *input {
		return nil, nil
	}
	result := make(map[string]any)
	if input != nil {
		result["prompt_tokens"] = float64(*input)
	}
	if output != nil {
		completion := float64(*output)
		if thought != nil {
			completion += float64(*thought)
		}
		result["completion_tokens"] = completion
	}
	if thought != nil {
		result["completion_tokens_details"] = map[string]any{"reasoning_tokens": float64(*thought)}
	}
	if total != nil {
		result["total_tokens"] = float64(*total)
	}
	if cached != nil {
		result["prompt_cache_hit_tokens"] = float64(*cached)
		if input != nil {
			result["prompt_cache_miss_tokens"] = float64(*input - *cached)
		}
	}
	if toolUse := counters["total_tool_use_tokens"]; toolUse != nil {
		result["total_tool_use_tokens"] = float64(*toolUse)
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}
