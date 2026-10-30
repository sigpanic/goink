package llm

import (
	"context"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// ChatStream 发起流式对话，返回 SSE 事件 channel。
//
// adapterName、providerName、model 显式指定调用目标；适配器负责协议转换和厂商规则。
// messages 和 tools 使用结构化输入，调用期间保持只读。
//
// ctx 取消时底层 HTTP 连接被中止，channel 随之关闭。
func (c *Client) ChatStream(
	ctx context.Context,
	adapterName adapter.Protocol,
	providerName, model string,
	messages []Message,
	tools []ToolDefinition,
	opts *CallOptions,
) <-chan StreamEvent {
	backend, err := c.protocolAdapter(adapterName)
	if err != nil {
		ch := make(chan StreamEvent, 1)
		ch <- StreamEvent{Type: EventError, Error: err}
		close(ch)
		return ch
	}

	return backend.Stream(ctx, adapter.Request{ProviderName: providerName, Model: model, Messages: messages, Tools: tools, Options: opts})
}
