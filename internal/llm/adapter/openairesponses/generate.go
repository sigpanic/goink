package openairesponses

import (
	"context"
	"mime"
	"net/http"

	"github.com/openai/openai-go/v3/option"

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
	opts, err := responsesURLOptions(p.ChatURL)
	if err != nil {
		return adapter.Response{}, err
	}
	var response *http.Response
	opts = append(opts,
		option.WithJSONSet("stream", false),
		option.WithHeader("Accept", "application/json"),
		option.WithResponseInto(&response),
		option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			result, err := next(req)
			if err == nil && result.StatusCode >= 200 && result.StatusCode < 300 {
				mediaType, _, _ := mime.ParseMediaType(result.Header.Get("Content-Type"))
				if mediaType == "text/event-stream" {
					// 不等无限 SSE 结束后再按 JSON 解码，及时释放连接。
					_ = result.Body.Close()
					return nil, invalidResponse("received SSE for a non-streaming request")
				}
			}
			return result, err
		}),
	)
	result, err := p.client.New(ctx, params, opts...)
	if err != nil {
		return adapter.Response{}, convertSDKError(err, response)
	}
	return p.convertResponse(params, result)
}
