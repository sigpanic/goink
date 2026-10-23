package gemini

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	types "google.golang.org/genai/interactions/models/interactions"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// replayPayload 由本包保存和解释；公共层仅透传，后续入站写入有序原始 steps。
type replayPayload struct {
	Steps       json.RawMessage `json:"steps"`
	RequestHash string          `json:"request_hash"`
}

// requestHash 绑定历史、工具、system 和服务凭据，不存明文密钥。
// Google 要求换模型仍传原 thought，由服务端判断兼容；不绑定 model 或生成选项。
// https://ai.google.dev/gemini-api/docs/thinking#thought-signatures
func (p Provider) requestHash(params types.CreateModelInteraction) (string, error) {
	hash := sha256.New()
	if err := json.NewEncoder(hash).Encode(struct {
		System   *string
		Input    *types.InteractionsInput
		Tools    []types.Tool
		Endpoint string
		APIKey   string
	}{params.SystemInstruction, params.Input, params.Tools, p.ChatURL, p.APIKey}); err != nil {
		return "", fmt.Errorf("encode Gemini request prefix: %w", err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (p Provider) replaySteps(params types.CreateModelInteraction, message adapter.Message) ([]types.Step, bool, error) {
	state := message.State
	if state == nil || !state.Complete || state.Origin.AdapterName != adapter.ProtocolGeminiInteractions ||
		state.Origin.ProviderName != p.Name || state.Origin.ProviderType != p.ProviderType {
		return nil, false, nil
	}
	var payload replayPayload
	if err := json.Unmarshal(state.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode Gemini replay state: %w", err)
	}
	hash, err := p.requestHash(params)
	if err != nil {
		return nil, false, err
	}
	if payload.RequestHash == "" || payload.RequestHash != hash {
		return nil, false, nil
	}
	steps, text, calls, err := replayProjection(payload.Steps)
	if err != nil {
		return nil, false, err
	}
	if text != message.Content || len(calls) != len(message.ToolCalls) {
		return nil, false, nil
	}
	for i, call := range calls {
		common := message.ToolCalls[i]
		if call.ID != common.ID || call.Name != common.Name || !sameJSON(call.Arguments, common.Arguments) {
			return nil, false, nil
		}
	}
	return steps, true, nil
}

// replayProjection 验证本阶段支持的模型输出并对照公共字段，SDK 原文通道保留未知扩展。
// thought 的 summary 是可选且不透明的原生状态，不从 ThinkingContent 重建。
func replayProjection(rawSteps json.RawMessage) ([]types.Step, string, []adapter.ToolCall, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(rawSteps, &raw); err != nil || len(raw) == 0 {
		return nil, "", nil, fmt.Errorf("gemini replay steps must be a nonempty array")
	}
	var text strings.Builder
	var calls []adapter.ToolCall
	steps := make([]types.Step, 0, len(raw))
	seen := make(map[string]bool)
	for _, data := range raw {
		var step struct {
			functionCall
			Content   []json.RawMessage `json:"content"`
			Signature string            `json:"signature"`
			Summary   json.RawMessage   `json:"summary"`
			Error     json.RawMessage   `json:"error"`
		}
		if err := json.Unmarshal(data, &step); err != nil {
			return nil, "", nil, fmt.Errorf("decode Gemini replay step: %w", err)
		}
		if len(step.Error) > 0 && !bytes.Equal(bytes.TrimSpace(step.Error), []byte("null")) {
			return nil, "", nil, fmt.Errorf("cannot replay a Gemini step with an error")
		}
		switch step.Type {
		case "thought":
			if step.Signature == "" {
				return nil, "", nil, fmt.Errorf("gemini thought replay requires the original signature")
			}
			if len(step.Summary) > 0 && !bytes.HasPrefix(bytes.TrimSpace(step.Summary), []byte("[")) {
				return nil, "", nil, fmt.Errorf("gemini thought summary must be an array")
			}
		case "model_output":
			for _, content := range step.Content {
				var block struct {
					Type string  `json:"type"`
					Text *string `json:"text"`
				}
				if err := json.Unmarshal(content, &block); err != nil || block.Type != "text" || block.Text == nil {
					return nil, "", nil, fmt.Errorf("gemini replay model_output requires text content")
				}
				text.WriteString(*block.Text)
			}
		case "function_call":
			if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.Name) == "" || !jsonObject(step.Arguments) || seen[step.ID] {
				return nil, "", nil, &adapter.APIError{Kind: "tool_args_invalid", Retryable: true,
					Message: fmt.Sprintf("invalid or duplicate Gemini replay function call %q", step.ID)}
			}
			seen[step.ID] = true
			calls = append(calls, adapter.ToolCall{ID: step.ID, Name: step.Name, Arguments: slices.Clone(step.Arguments)})
		default:
			return nil, "", nil, fmt.Errorf("unsupported Gemini replay step %q", step.Type)
		}
		steps = append(steps, types.NewStepUnknown(slices.Clone(data)))
	}
	return steps, text.String(), calls, nil
}
