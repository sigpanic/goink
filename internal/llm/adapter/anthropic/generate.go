package anthropic

import (
	"context"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Generate 返回单次完整生成结果，不执行工具或业务重试。
func (c *Adapter) Generate(ctx context.Context, req adapter.Request) (adapter.Response, error) {
	p, err := c.provider(req.ProviderName)
	if err != nil {
		return adapter.Response{}, err
	}
	params, err := p.buildRequest(req)
	if err != nil {
		return adapter.Response{}, err
	}
	urlOption, err := messagesURLOption(p.ChatURL)
	if err != nil {
		return adapter.Response{}, err
	}
	// 显式超时避免 SDK 因较大的 max_tokens 在发送前强制要求流式。
	// 不降低调用方的输出上限；更短的 context deadline 仍优先生效。
	message, err := p.client.Messages.New(ctx, params, urlOption, option.WithRequestTimeout(10*time.Minute))
	if err != nil {
		return adapter.Response{}, convertSDKError(err)
	}
	return p.convertResponse(params, message)
}
