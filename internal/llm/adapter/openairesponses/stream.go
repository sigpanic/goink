package openairesponses

import (
	"context"
	"mime"
	"net/http"

	"github.com/openai/openai-go/v3/option"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Stream 实时发布显示事件；完整终态与传输均成功后才交付状态和整组可执行工具。
func (c *Adapter) Stream(ctx context.Context, req adapter.Request) <-chan adapter.StreamEvent {
	ch := make(chan adapter.StreamEvent, 32)
	go func() {
		defer close(ch)
		emit := func(event adapter.StreamEvent) bool { return sendEvent(ctx, ch, event) }
		fail := func(err error) { emit(adapter.StreamEvent{Type: adapter.EventError, Error: err}) }
		p, err := c.provider(req.ProviderName)
		if err != nil {
			fail(err)
			return
		}
		params, err := p.buildRequest(req)
		if err != nil {
			fail(err)
			return
		}
		opts, err := responsesURLOptions(p.ChatURL)
		if err != nil {
			fail(err)
			return
		}
		var response *http.Response
		opts = append(opts, option.WithHeader("Accept", "text/event-stream"), option.WithResponseInto(&response))
		stream := p.client.NewStreaming(ctx, params, opts...)
		defer stream.Close()
		if err := stream.Err(); err != nil {
			fail(convertSDKError(err, response))
			return
		}
		if response == nil {
			fail(invalidResponse("missing SSE HTTP response"))
			return
		}
		mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if err != nil || mediaType != "text/event-stream" {
			fail(invalidResponse("expected text/event-stream response"))
			return
		}
		var state streamState
		for stream.Next() {
			if err := state.consume(stream.Current(), emit); err != nil {
				fail(err)
				return
			}
			if ctx.Err() != nil {
				return
			}
		}
		if err := stream.Err(); err != nil {
			fail(convertSDKError(err, response))
			return
		}
		if ctx.Err() != nil {
			return
		}
		result, err := state.finish(p, params)
		if err != nil {
			fail(err)
			return
		}
		if !emit(adapter.StreamEvent{Type: adapter.EventUsage, Usage: result.Usage}) ||
			!emit(adapter.StreamEvent{Type: adapter.EventResponseState, State: result.State}) {
			return
		}
		for _, call := range result.ToolCalls {
			if !emit(adapter.StreamEvent{Type: adapter.EventToolCallEnd, Delta: &call}) {
				return
			}
		}
	}()
	return ch
}

func sendEvent(ctx context.Context, ch chan<- adapter.StreamEvent, event adapter.StreamEvent) bool {
	if ctx.Err() != nil {
		return false
	}
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
