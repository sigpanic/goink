package anthropic

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// replayPayload 是本包的私有存储约定；公共层只透传 JSON。
// 后续入站转换保存完整 content 和生成该响应的请求指纹。
type replayPayload struct {
	Content     json.RawMessage `json:"content"`
	RequestHash string          `json:"request_hash"`
}

// requestHash 保守绑定完整请求、端点及凭据；密钥只参与摘要，不写入 payload。
// 它只证明本地输入未改，不能代替服务端签名校验。
func (p Provider) requestHash(params sdk.MessageNewParams) (string, error) {
	hash := sha256.New()
	if err := json.NewEncoder(hash).Encode(struct {
		Params   sdk.MessageNewParams
		Endpoint string
		APIKey   string
	}{params, p.ChatURL, p.APIKey}); err != nil {
		return "", fmt.Errorf("encode Anthropic request prefix: %w", err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (p Provider) replayContent(params sdk.MessageNewParams, message adapter.Message) ([]sdk.ContentBlockParamUnion, bool, error) {
	state := message.State
	if state == nil || !state.Complete || state.Origin != (adapter.MessageOrigin{
		AdapterName: adapter.ProtocolAnthropicMessages, ProviderName: p.Name, ProviderType: p.ProviderType, Model: params.Model,
	}) {
		return nil, false, nil
	}
	var payload replayPayload
	if err := json.Unmarshal(state.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode Anthropic replay state: %w", err)
	}
	hash, err := p.requestHash(params)
	if err != nil {
		return nil, false, err
	}
	if payload.RequestHash == "" || payload.RequestHash != hash {
		return nil, false, nil
	}
	var content []json.RawMessage
	if err := json.Unmarshal(payload.Content, &content); err != nil || len(content) == 0 {
		return nil, false, fmt.Errorf("anthropic replay content must be a nonempty block array")
	}
	var text strings.Builder
	var calls []adapter.ToolCall
	blocks := make([]sdk.ContentBlockParamUnion, 0, len(content))
	for _, raw := range content {
		var block struct {
			Type      string          `json:"type"`
			Text      string          `json:"text"`
			Thinking  *string         `json:"thinking"`
			Signature string          `json:"signature"`
			Data      string          `json:"data"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Input     json.RawMessage `json:"input"`
		}
		if err := json.Unmarshal(raw, &block); err != nil {
			return nil, false, fmt.Errorf("decode Anthropic replay block: %w", err)
		}
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "thinking":
			if block.Thinking == nil || block.Signature == "" {
				return nil, false, fmt.Errorf("anthropic thinking replay requires original thinking and signature")
			}
		case "redacted_thinking":
			if block.Data == "" {
				return nil, false, fmt.Errorf("anthropic redacted thinking replay requires original data")
			}
		case "tool_use":
			calls = append(calls, adapter.ToolCall{ID: block.ID, Name: block.Name, Arguments: block.Input})
		default:
			return nil, false, fmt.Errorf("unsupported Anthropic replay block %q", block.Type)
		}
		blocks = append(blocks, param.Override[sdk.ContentBlockParamUnion](slices.Clone(raw)))
	}
	if text.String() != message.Content || len(calls) != len(message.ToolCalls) {
		return nil, false, nil
	}
	for i, call := range calls {
		common := message.ToolCalls[i]
		if call.ID != common.ID || call.Name != common.Name || !sameJSON(call.Arguments, common.Arguments) {
			return nil, false, nil
		}
	}
	return blocks, true, nil
}
