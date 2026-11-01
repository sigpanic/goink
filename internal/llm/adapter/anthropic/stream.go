package anthropic

import (
	"context"
	"mime"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Stream 实时映射 SDK 事件，完整收尾后才发布回放状态及整组可执行工具。
func (c *Adapter) Stream(ctx context.Context, req adapter.Request) <-chan adapter.StreamEvent {
	ch := make(chan adapter.StreamEvent, 32)
	go func() {
		defer close(ch)
		emitError := func(err error) { adapter.SendEvent(ctx, ch, adapter.StreamEvent{Type: adapter.EventError, Error: err}) }
		p, err := c.provider(req.ProviderName)
		if err != nil {
			emitError(err)
			return
		}
		params, err := p.buildRequest(req)
		if err != nil {
			emitError(err)
			return
		}
		urlOption, err := messagesURLOption(p.ChatURL)
		if err != nil {
			emitError(err)
			return
		}
		var httpResponse *http.Response
		stream := p.client.Messages.NewStreaming(ctx, params, urlOption, option.WithResponseInto(&httpResponse))
		defer stream.Close()
		if err := stream.Err(); err != nil {
			emitError(convertSDKError(err))
			return
		}
		if httpResponse == nil {
			emitError(invalidResponse("missing HTTP response"))
			return
		}
		contentType, _, err := mime.ParseMediaType(httpResponse.Header.Get("Content-Type"))
		if err != nil || contentType != "text/event-stream" {
			emitError(invalidResponse("expected text/event-stream response"))
			return
		}
		var state streamState
		for stream.Next() {
			event, publish, err := state.consume(stream.Current())
			if err != nil {
				emitError(err)
				return
			}
			if publish && !adapter.SendEvent(ctx, ch, event) {
				return
			}
		}
		if err := stream.Err(); err != nil {
			emitError(convertSDKError(err))
			return
		}
		if ctx.Err() != nil {
			return
		}
		response, err := state.finish(p, params)
		if err != nil {
			emitError(err)
			return
		}
		if !adapter.SendEvent(ctx, ch, adapter.StreamEvent{Type: adapter.EventUsage, Usage: response.Usage}) ||
			!adapter.SendEvent(ctx, ch, adapter.StreamEvent{Type: adapter.EventResponseState, State: response.State}) {
			return
		}
		for _, call := range response.ToolCalls {
			if !adapter.SendEvent(ctx, ch, adapter.StreamEvent{Type: adapter.EventToolCallEnd, Delta: &call}) {
				return
			}
		}
	}()
	return ch
}
