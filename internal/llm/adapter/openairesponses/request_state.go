package openairesponses

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// replayPayload 保存有序原生 output；后续入站转换写入，公共层仅透传。
type replayPayload struct {
	Output      json.RawMessage `json:"output"`
	RequestHash string          `json:"request_hash"`
}

// replayFamily 只合并官方明确的模型族，其他 ID 保守要求完全相同。
// 6.1 不推断与 6 的密文兼容；Cyber 是独立配置的服务别名。
func replayFamily(model string) string {
	switch model {
	case "gpt-5.6", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna":
		return "gpt-5.6"
	case "gpt-6-astra", "gpt-6-sol", "gpt-6-luna":
		return "gpt-6"
	default:
		return model
	}
}

// requestHash 绑定历史、工具、模型族及服务实例；凭据只参与摘要，不写入 payload。
// 不绑定采样或推理等级，允许后续轮次修改选项；摘要不代替服务端密文校验。
func (p Provider) requestHash(params responses.ResponseNewParams) (string, error) {
	hash := sha256.New()
	if err := json.NewEncoder(hash).Encode(struct {
		Input    responses.ResponseNewParamsInputUnion
		Tools    []responses.ToolUnionParam
		Family   string
		Endpoint string
		APIKey   string
	}{params.Input, params.Tools, replayFamily(params.Model), p.ChatURL, p.APIKey}); err != nil {
		return "", fmt.Errorf("encode OpenAI Responses request prefix: %w", err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (p Provider) replayOutput(params responses.ResponseNewParams, message adapter.Message) ([]responses.ResponseInputItemUnionParam, bool, error) {
	state := message.State
	if state == nil || !state.Complete || state.Origin.AdapterName != adapter.ProtocolOpenAIResponses ||
		state.Origin.ProviderName != p.Name || state.Origin.ProviderType != p.ProviderType ||
		replayFamily(state.Origin.Model) != replayFamily(params.Model) {
		return nil, false, nil
	}
	var payload replayPayload
	if err := json.Unmarshal(state.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode OpenAI Responses replay state: %w", err)
	}
	hash, err := p.requestHash(params)
	if err != nil {
		return nil, false, err
	}
	if payload.RequestHash == "" || payload.RequestHash != hash {
		return nil, false, nil
	}
	items, content, calls, err := replayProjection(payload.Output)
	if err != nil {
		return nil, false, err
	}
	if content != message.Content || len(calls) != len(message.ToolCalls) {
		return nil, false, nil
	}
	for i, call := range calls {
		common := message.ToolCalls[i]
		if call.ID != common.ID || call.Name != common.Name || !sameJSON(call.Arguments, common.Arguments) {
			return nil, false, nil
		}
	}
	return items, true, nil
}

// replayProjection 验证本阶段支持的 output items；原文通道保留顺序、密文及扩展。
// 显示用 ThinkingContent 不用于创建 reasoning item，函数关联使用 call_id 而非 item.id。
func replayProjection(output json.RawMessage) ([]responses.ResponseInputItemUnionParam, string, []adapter.ToolCall, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(output, &raw); err != nil || len(raw) == 0 {
		return nil, "", nil, fmt.Errorf("OpenAI Responses replay output must be a nonempty item array")
	}
	var content strings.Builder
	var calls []adapter.ToolCall
	items := make([]responses.ResponseInputItemUnionParam, 0, len(raw))
	seen := make(map[string]bool)
	itemIDs := make(map[string]bool)
	for _, data := range raw {
		var item struct {
			Type             string            `json:"type"`
			ID               string            `json:"id"`
			Status           string            `json:"status"`
			Role             string            `json:"role"`
			Content          []json.RawMessage `json:"content"`
			EncryptedContent string            `json:"encrypted_content"`
			CallID           string            `json:"call_id"`
			Name             string            `json:"name"`
			Arguments        string            `json:"arguments"`
			Async            bool              `json:"async"`
			Namespace        string            `json:"namespace"`
			Caller           *struct {
				Type string `json:"type"`
			} `json:"caller"`
		}
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, "", nil, fmt.Errorf("decode OpenAI Responses replay item: %w", err)
		}
		if (item.Status != "" && item.Status != "completed") || (item.ID != "" && itemIDs[item.ID]) {
			return nil, "", nil, fmt.Errorf("incomplete or duplicate OpenAI Responses replay item %q", item.ID)
		}
		if item.ID != "" {
			itemIDs[item.ID] = true
		}
		switch item.Type {
		case "reasoning":
			if item.ID == "" || item.EncryptedContent == "" {
				return nil, "", nil, fmt.Errorf("OpenAI Responses reasoning replay requires original ID and encrypted content")
			}
		case "message":
			if item.Role != "assistant" || item.Content == nil {
				return nil, "", nil, fmt.Errorf("OpenAI Responses replay message requires assistant content")
			}
			for _, rawPart := range item.Content {
				var part struct {
					Type    string  `json:"type"`
					Text    *string `json:"text"`
					Refusal *string `json:"refusal"`
				}
				if err := json.Unmarshal(rawPart, &part); err != nil {
					return nil, "", nil, fmt.Errorf("decode OpenAI Responses replay content: %w", err)
				}
				switch {
				case part.Type == "output_text" && part.Text != nil:
					content.WriteString(*part.Text)
				case part.Type == "refusal" && part.Refusal != nil:
					content.WriteString(*part.Refusal)
				default:
					return nil, "", nil, fmt.Errorf("unsupported OpenAI Responses replay content %q", part.Type)
				}
			}
		case "function_call":
			if item.Async || item.Namespace != "" || (item.Caller != nil && item.Caller.Type != "direct") {
				return nil, "", nil, fmt.Errorf("provider-executed, asynchronous or namespaced OpenAI function calls are not supported")
			}
			arguments := json.RawMessage(item.Arguments)
			if strings.TrimSpace(item.CallID) == "" || strings.TrimSpace(item.Name) == "" || !jsonObject(arguments) || seen[item.CallID] {
				return nil, "", nil, &adapter.APIError{
					Kind:      "tool_args_invalid",
					Retryable: true,
					Message:   fmt.Sprintf("invalid or duplicate OpenAI Responses function call %q", item.CallID),
				}
			}
			seen[item.CallID] = true
			calls = append(calls, adapter.ToolCall{ID: item.CallID, Name: item.Name, Arguments: arguments})
		default:
			return nil, "", nil, fmt.Errorf("unsupported OpenAI Responses replay item %q", item.Type)
		}
		items = append(items, param.Override[responses.ResponseInputItemUnionParam](slices.Clone(data)))
	}
	return items, content.String(), calls, nil
}
