package openairesponses

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// TestConnection 原地址优先，只在临时探测中尝试候选，返回可保存的完整端点。
func (c *Adapter) TestConnection(ctx context.Context, providerName, model string) (adapter.TestConnectionResult, error) {
	p, err := c.provider(providerName)
	if err != nil {
		return adapter.TestConnectionResult{}, err
	}
	if strings.TrimSpace(model) == "" {
		return adapter.TestConnectionResult{}, fmt.Errorf("model must not be empty")
	}
	endpoint, err := parseResponsesURL(p.ChatURL)
	if err != nil {
		return adapter.TestConnectionResult{}, err
	}
	var failures []error
	for _, candidate := range responsesURLCandidates(endpoint) {
		if err := ctx.Err(); err != nil {
			return adapter.TestConnectionResult{}, err
		}
		probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		warning, err := p.probeResponse(probeCtx, candidate, model)
		cancel()
		if err == nil {
			return adapter.TestConnectionResult{URL: candidate, Warning: warning}, nil
		}
		failures = append(failures, fmt.Errorf("candidate %s: %w", candidate, err))
	}
	if err := ctx.Err(); err != nil {
		return adapter.TestConnectionResult{}, err
	}
	return adapter.TestConnectionResult{}, fmt.Errorf("all Responses URL candidates failed: %w", errors.Join(failures...))
}

func responsesURLCandidates(endpoint *url.URL) []string {
	var candidates []string
	seen := make(map[string]bool)
	add := func(path string) {
		u := *endpoint
		u.RawPath = path
		u.Path, _ = url.PathUnescape(path)
		if candidate := u.String(); !seen[candidate] {
			seen[candidate] = true
			candidates = append(candidates, candidate)
		}
	}
	add(endpoint.EscapedPath())
	path := strings.TrimRight(endpoint.EscapedPath(), "/")
	add(path)
	if base, ok := strings.CutSuffix(path, "/responses"); ok {
		if root, v1 := strings.CutSuffix(base, "/v1"); v1 {
			add(root + "/responses")
		} else {
			add(base + "/v1/responses")
		}
	} else {
		add(path + "/responses")
		if !strings.HasSuffix(path, "/v1") {
			add(path + "/v1/responses")
		}
	}
	add("/v1/responses")
	return candidates
}

func (p Provider) probeResponse(ctx context.Context, candidate, model string) (string, error) {
	opts, err := responsesURLOptions(candidate)
	if err != nil {
		return "", err
	}
	var response *http.Response
	opts = append(opts, option.WithHeader("Accept", "text/event-stream"), option.WithResponseInto(&response))
	stream := p.client.NewStreaming(ctx, responses.ResponseNewParams{
		Model:           model,
		Input:           responses.ResponseNewParamsInputUnion{OfString: param.NewOpt("hi")},
		MaxOutputTokens: param.NewOpt(int64(64)),
		Store:           param.NewOpt(false),
		Background:      param.NewOpt(false),
	}, opts...)
	defer stream.Close()
	if err := stream.Err(); err != nil {
		converted := convertSDKError(err, response)
		var fault *adapter.APIError
		if errors.As(converted, &fault) && fault.StatusCode == http.StatusTooManyRequests {
			switch fault.Kind {
			case "rate_limit_exceeded", "rate_limit_error", "insufficient_quota":
				return "429 限流或配额不足：端点返回 OpenAI 错误，尚未验证密钥、模型和完整生成能力，请稍后重试或检查配额", nil
			}
		}
		return "", converted
	}
	if response == nil {
		return "", fmt.Errorf("missing Responses HTTP response")
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "text/event-stream" {
		return "", fmt.Errorf("responses endpoint did not return an SSE stream")
	}
	if stream.Next() {
		event := stream.Current()
		if event.Type == "error" {
			return "", streamError([]byte(event.RawJSON()))
		}
		if event.Type != "response.created" {
			return "", fmt.Errorf("SSE stream did not begin with response.created")
		}
		var state streamState
		if err := state.consume(event, func(adapter.StreamEvent) bool { return true }); err != nil {
			return "", err
		}
		// 只验证协议握手，立即关闭请求；不等待输出，也不将握手视为完整生成成功。
		return "", nil
	}
	if err := stream.Err(); err != nil {
		return "", convertSDKError(err, response)
	}
	return "", fmt.Errorf("SSE stream contains no valid response.created")
}
