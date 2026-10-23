package gemini

import (
	"context"
	"encoding/json"
	"io"

	"google.golang.org/genai/interactions/models/operations"
	sse "google.golang.org/genai/interactions/types/stream"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Stream 发布实时显示事件，完整校验成功后才交付状态及整组可执行工具。
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
		streaming := true
		params.Stream = &streaming
		result, err := p.client.Create(ctx, operations.CreateInteractionRequest{
			Body: operations.NewCreateInteractionRequestBody(params),
		}, operations.WithAcceptHeaderOverride(operations.AcceptHeaderEnumTextEventStream))
		if err != nil {
			fail(convertSDKError(err))
			return
		}
		if result == nil || result.InteractionSSEStreamEvent == nil {
			if result != nil && result.HTTPMeta.Response != nil && result.HTTPMeta.Response.Body != nil {
				_ = result.HTTPMeta.Response.Body.Close()
			}
			fail(invalidResponse("expected an SSE interaction response"))
			return
		}
		defer result.InteractionSSEStreamEvent.Close()
		if result.HTTPMeta.Response == nil || result.HTTPMeta.Response.Body == nil {
			fail(invalidResponse("missing SSE response body"))
			return
		}
		// 生成的 SDK 事件类型会丢原文；用 SDK 公开读取器解帧，保留 Data 的原始 JSON。
		// 原 Create stream 持有网络 Body；借用读取器不再次关闭底层连接或发出请求。
		events := sse.NewEventStream(ctx, io.NopCloser(result.HTTPMeta.Response.Body), func(raw []byte) (sse.ServerEvent, error) {
			var event sse.ServerEvent
			err := json.Unmarshal(raw, &event)
			return event, err
		}, "")
		defer events.Close()
		var state streamState
		for events.Next() {
			event := events.Value()
			if string(event.Data) == `"[DONE]"` {
				break
			}
			if err := state.consume(event.Data, emit); err != nil {
				fail(err)
				return
			}
			if ctx.Err() != nil {
				return
			}
		}
		// 空 sentinel 允许协议终态后的正常 EOF；真实读取错误仍必须失败。
		if err := events.Err(); err != nil {
			fail(convertSDKError(err))
			return
		}
		if ctx.Err() != nil {
			return
		}
		response, err := state.finish(p, params)
		if err != nil {
			fail(err)
			return
		}
		if !emit(adapter.StreamEvent{Type: adapter.EventUsage, Usage: response.Usage}) ||
			!emit(adapter.StreamEvent{Type: adapter.EventResponseState, State: response.State}) {
			return
		}
		for _, call := range response.ToolCalls {
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
