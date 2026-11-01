package gemini

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/genai/interactions/models/operations"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Generate 返回单次完整结果，不执行工具或业务重试。
func (c *Adapter) Generate(ctx context.Context, req adapter.Request) (adapter.Response, error) {
	p, err := c.provider(req.ProviderName)
	if err != nil {
		return adapter.Response{}, err
	}
	params, err := p.buildRequest(req)
	if err != nil {
		return adapter.Response{}, err
	}
	stream := false
	params.Stream = &stream
	result, err := p.client.Create(ctx, operations.CreateInteractionRequest{
		Body: operations.NewCreateInteractionRequestBody(params),
	}, operations.WithAcceptHeaderOverride(operations.AcceptHeaderEnumApplicationJson))
	if err != nil {
		return adapter.Response{}, convertSDKError(err)
	}
	if result == nil {
		return adapter.Response{}, invalidResponse("missing SDK response")
	}
	if result.InteractionSSEStreamEvent != nil {
		result.InteractionSSEStreamEvent.Close()
		return adapter.Response{}, invalidResponse("received SSE for a non-streaming request")
	}
	response := result.HTTPMeta.Response
	if response == nil || response.Body == nil {
		return adapter.Response{}, invalidResponse("missing original response body")
	}
	defer response.Body.Close()
	if result.Interaction == nil {
		return adapter.Response{}, invalidResponse("missing interaction")
	}
	// SDK ConsumeRawBody 将原文放回 HTTPMeta.Response.Body；不用有损的 Step 再编码。
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return adapter.Response{}, fmt.Errorf("read Gemini response: %w", err)
	}
	return p.convertResponse(params, raw)
}
