package anthropic

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// TestConnection 原地址优先；只有临时探测可尝试候选，不改写 provider 快照。
func (c *Adapter) TestConnection(ctx context.Context, providerName, model string) (adapter.TestConnectionResult, error) {
	p, err := c.provider(providerName)
	if err != nil {
		return adapter.TestConnectionResult{}, err
	}
	if strings.TrimSpace(model) == "" {
		return adapter.TestConnectionResult{}, fmt.Errorf("model must not be empty")
	}
	endpoint, err := adapter.ParseChatURL(p.ChatURL)
	if err != nil {
		return adapter.TestConnectionResult{}, err
	}
	var failures []error
	for _, candidate := range messagesURLCandidates(endpoint) {
		if err := ctx.Err(); err != nil {
			return adapter.TestConnectionResult{}, err
		}
		probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		warning, err := p.probeMessages(probeCtx, candidate, model)
		cancel()
		if err == nil {
			return adapter.TestConnectionResult{URL: candidate, Warning: warning}, nil
		}
		failures = append(failures, fmt.Errorf("candidate %s: %w", candidate, err))
	}
	if err := ctx.Err(); err != nil {
		return adapter.TestConnectionResult{}, err
	}
	return adapter.TestConnectionResult{}, fmt.Errorf("all Messages URL candidates failed: %w", errors.Join(failures...))
}

func messagesURLCandidates(endpoint *url.URL) []string {
	var candidates []string
	seen := map[string]bool{}
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
	if base, ok := strings.CutSuffix(path, "/messages"); ok {
		if root, v1 := strings.CutSuffix(base, "/v1"); v1 {
			add(root + "/messages")
		} else {
			add(base + "/v1/messages")
		}
	} else {
		add(path + "/messages")
		if !strings.HasSuffix(path, "/v1") {
			add(path + "/v1/messages")
		}
	}
	add("/v1/messages")
	return candidates
}

func (p Provider) probeMessages(ctx context.Context, candidate, model string) (string, error) {
	urlOption, err := messagesURLOption(candidate)
	if err != nil {
		return "", err
	}
	var response *http.Response
	stream := p.client.Messages.NewStreaming(ctx, sdk.MessageNewParams{
		Model: sdk.Model(model), MaxTokens: 64,
		Messages: []sdk.MessageParam{sdk.NewUserMessage(sdk.NewTextBlock("hi"))},
	}, urlOption, option.WithResponseInto(&response))
	defer stream.Close()
	if err := stream.Err(); err != nil {
		converted := convertSDKError(err)
		var apiErr *adapter.APIError
		if errors.As(converted, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests && apiErr.Kind == "rate_limit_error" {
			return "429 限流：端点返回 Anthropic 限流错误，尚未验证密钥、模型和生成能力，请稍后重试", nil
		}
		return "", converted
	}
	if response == nil {
		return "", fmt.Errorf("missing Messages response")
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "text/event-stream" {
		return "", fmt.Errorf("messages response is not an SSE stream")
	}
	if stream.Next() {
		event := stream.Current()
		var state streamState
		if event.Type != "message_start" {
			return "", fmt.Errorf("SSE stream did not begin with message_start")
		}
		if _, _, err := state.consume(event); err != nil {
			return "", err
		}
		// 仅验证协议握手；立即关闭生成，不等待整轮输出或宣称完整生成成功。
		return "", nil
	}
	if err := stream.Err(); err != nil {
		return "", convertSDKError(err)
	}
	return "", fmt.Errorf("SSE stream contains no valid message_start")
}
