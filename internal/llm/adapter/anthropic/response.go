package anthropic

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/respjson"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// convertResponse 供完整响应和后续流式收尾复用；所有校验通过后才交付工具和回放状态。
func (p Provider) convertResponse(params sdk.MessageNewParams, message *sdk.Message) (adapter.Response, error) {
	if message == nil || message.Type != "message" || message.Role != "assistant" || message.ID == "" || !stringField(message.JSON.ID) ||
		!message.JSON.Content.Valid() || len(message.Content) == 0 || !message.JSON.StopReason.Valid() {
		return adapter.Response{}, invalidResponse("missing or invalid message envelope")
	}
	stop := string(message.StopReason)
	switch stop {
	case "end_turn", "stop_sequence", "max_tokens", "refusal", "model_context_window_exceeded", "tool_use":
	default:
		return adapter.Response{}, invalidResponse("unsupported stop_reason " + stop)
	}
	var text, thinking strings.Builder
	var calls []adapter.ToolCallDelta
	seen := make(map[string]bool)
	for _, block := range message.Content {
		switch block.Type {
		case "text":
			if !stringField(block.JSON.Text) {
				return adapter.Response{}, invalidResponse("text block requires text")
			}
			text.WriteString(block.Text)
		case "thinking":
			if !stringField(block.JSON.Thinking) || !stringField(block.JSON.Signature) || block.Signature == "" {
				return adapter.Response{}, invalidResponse("thinking block requires original thinking and signature")
			}
			thinking.WriteString(block.Thinking)
		case "redacted_thinking":
			if !stringField(block.JSON.Data) || block.Data == "" {
				return adapter.Response{}, invalidResponse("redacted thinking block requires original data")
			}
		case "tool_use":
			if block.ToolsetName != "" {
				return adapter.Response{}, invalidResponse("toolset member calls are not supported")
			}
			sdkCall := block.AsToolUse()
			if raw := sdkCall.JSON.Caller.Raw(); raw != "" && raw != "null" {
				if !sdkCall.JSON.Caller.Valid() || sdkCall.Caller.Type != "direct" {
					return adapter.Response{}, invalidResponse("provider-executed function calls are not supported")
				}
			}
			if block.ID == "" || block.Name == "" || !stringField(block.JSON.ID) || !stringField(block.JSON.Name) || seen[block.ID] || !jsonObject(block.Input) {
				return adapter.Response{}, &adapter.APIError{
					Kind: "tool_args_invalid", Retryable: true,
					Message: fmt.Sprintf("invalid tool call %q: expected a unique ID, name and object input", block.ID),
				}
			}
			seen[block.ID] = true
			calls = append(calls, adapter.ToolCallDelta{
				ToolID: block.ID, ToolName: block.Name, ArgumentsText: string(block.Input), ArgumentsJSON: slices.Clone(block.Input),
			})
		default:
			return adapter.Response{}, invalidResponse("unsupported content block " + block.Type)
		}
	}
	if (len(calls) > 0) != (stop == "tool_use") {
		return adapter.Response{}, invalidResponse("tool calls do not match stop_reason " + stop)
	}
	usage, err := normalizeUsage(message.Usage)
	if err != nil {
		return adapter.Response{}, err
	}
	hash, err := p.requestHash(params)
	if err != nil {
		return adapter.Response{}, err
	}
	// 使用 SDK 保存的原始 content，不经 ToParam 重建或从扁平字段合成签名块。
	payload, err := json.Marshal(replayPayload{Content: json.RawMessage(message.JSON.Content.Raw()), RequestHash: hash})
	if err != nil {
		return adapter.Response{}, fmt.Errorf("encode Anthropic response state: %w", err)
	}
	return adapter.Response{
		Content: text.String(), ThinkingContent: thinking.String(), ToolCalls: calls, Usage: usage, StopReason: stop,
		State: &adapter.ResponseState{
			Origin:   adapter.MessageOrigin{AdapterName: adapter.ProtocolAnthropicMessages, ProviderName: p.Name, ProviderType: p.ProviderType, Model: params.Model},
			Complete: true, TurnComplete: stop == "end_turn" || stop == "stop_sequence" || stop == "refusal",
			StopReason: stop, Payload: payload,
		},
	}, nil
}

func invalidResponse(message string) error {
	return &adapter.APIError{Kind: "invalid_response", Message: "invalid Anthropic response: " + message}
}

// SDK 会宽松转换标量；原生回放字段必须真的是字符串，避免正文与原始块不一致。
func stringField(field respjson.Field) bool {
	return field.Valid() && strings.HasPrefix(strings.TrimSpace(field.Raw()), `"`)
}

// normalizeUsage 将三个互斥的输入计数归为输入总量；缓存、推理明细不再额外加进总量。
// 数字沿用当前 Goink 消费路径要求的 float64。
func normalizeUsage(usage sdk.Usage) (map[string]any, error) {
	if !usage.JSON.InputTokens.Valid() || !usage.JSON.OutputTokens.Valid() || usage.InputTokens < 0 || usage.OutputTokens < 0 ||
		usage.CacheCreationInputTokens < 0 || usage.CacheReadInputTokens < 0 {
		return nil, invalidResponse("missing or invalid usage counters")
	}
	for _, field := range []respjson.Field{
		usage.JSON.InputTokens, usage.JSON.OutputTokens,
		usage.JSON.CacheCreationInputTokens, usage.JSON.CacheReadInputTokens,
		usage.OutputTokensDetails.JSON.ThinkingTokens,
	} {
		if raw := field.Raw(); raw != "" && raw != "null" {
			if _, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err != nil || !field.Valid() {
				return nil, invalidResponse("usage token counters must be integers")
			}
		}
	}
	if raw := usage.JSON.OutputTokensDetails.Raw(); raw != "" && raw != "null" && !usage.JSON.OutputTokensDetails.Valid() {
		return nil, invalidResponse("invalid output token details")
	}
	uncached := float64(usage.InputTokens)
	created := float64(usage.CacheCreationInputTokens)
	cached := float64(usage.CacheReadInputTokens)
	input, output := uncached+created+cached, float64(usage.OutputTokens)
	result := map[string]any{
		"prompt_tokens": input, "completion_tokens": output, "total_tokens": input + output,
		"prompt_cache_hit_tokens": cached, "prompt_cache_miss_tokens": uncached + created,
		"cache_creation_input_tokens": created,
	}
	if usage.JSON.OutputTokensDetails.Valid() && usage.OutputTokensDetails.JSON.ThinkingTokens.Valid() {
		reasoning := usage.OutputTokensDetails.ThinkingTokens
		if reasoning < 0 || reasoning > usage.OutputTokens {
			return nil, invalidResponse("invalid thinking token counter")
		}
		result["completion_tokens_details"] = map[string]any{"reasoning_tokens": float64(reasoning)}
	}
	return result, nil
}
