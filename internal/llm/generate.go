package llm

import (
	"context"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// GenerateText 发起非流式对话，返回模型生成的纯文本。
// messages 由调用方自行组装，格式与 ChatStream 一致。
func (c *Client) GenerateText(ctx context.Context, adapterName adapter.Protocol, providerName, model string, messages []Message, opts *CallOptions) (string, error) {
	backend, err := c.protocolAdapter(adapterName)
	if err != nil {
		return "", err
	}
	result, err := backend.Generate(ctx, adapter.Request{ProviderName: providerName, Model: model, Messages: messages, Options: opts})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}
