package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	types "google.golang.org/genai/interactions/models/interactions"
	"google.golang.org/genai/interactions/models/operations"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// TestConnection 原地址优先，只在临时探测中尝试候选，返回验证过的完整 URL。
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
	for _, candidate := range interactionsURLCandidates(endpoint) {
		if err := ctx.Err(); err != nil {
			return adapter.TestConnectionResult{}, err
		}
		probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		warning, err := p.probeInteraction(probeCtx, candidate, model)
		cancel()
		if err == nil {
			return adapter.TestConnectionResult{URL: candidate, Warning: warning}, nil
		}
		failures = append(failures, fmt.Errorf("candidate %s: %w", candidate, err))
	}
	if err := ctx.Err(); err != nil {
		return adapter.TestConnectionResult{}, err
	}
	return adapter.TestConnectionResult{}, fmt.Errorf("all Interactions URL candidates failed: %w", errors.Join(failures...))
}

func interactionsURLCandidates(endpoint *url.URL) []string {
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
	if base, ok := strings.CutSuffix(path, "/interactions"); ok {
		switch {
		case strings.HasSuffix(base, "/v1"):
			add(strings.TrimSuffix(base, "/v1") + "/interactions")
		case strings.HasSuffix(base, "/v1beta"):
			add(strings.TrimSuffix(base, "/v1beta") + "/interactions")
		default:
			add(base + "/v1/interactions")
		}
	} else {
		add(path + "/interactions")
		if !strings.HasSuffix(path, "/v1") && !strings.HasSuffix(path, "/v1beta") {
			add(path + "/v1/interactions")
		}
	}
	add("/v1/interactions")
	return candidates
}

func (p Provider) probeInteraction(ctx context.Context, candidate, model string) (string, error) {
	client, err := p.clientForURL(candidate)
	if err != nil {
		return "", err
	}
	store, background, streaming, maxTokens := false, false, true, 64
	input := types.NewInteractionsInput("hi")
	result, err := client.Create(ctx, operations.CreateInteractionRequest{
		Body: operations.NewCreateInteractionRequestBody(types.CreateModelInteraction{
			Model: types.Model(model), Input: &input, Store: &store, Background: &background, Stream: &streaming,
			GenerationConfig: &types.GenerationConfig{MaxOutputTokens: &maxTokens},
		}),
	}, operations.WithAcceptHeaderOverride(operations.AcceptHeaderEnumTextEventStream))
	if err != nil {
		converted := convertSDKError(err)
		var apiErr *adapter.APIError
		if errors.As(converted, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests {
			switch apiErr.Kind {
			case "rate_limit_exceeded", "quota_exceeded", "too_many_requests", "RESOURCE_EXHAUSTED":
				return "429 限流或配额不足：端点返回 Google 错误，尚未验证密钥、模型和生成能力，请稍后重试", nil
			}
		}
		return "", converted
	}
	if result == nil || result.InteractionSSEStreamEvent == nil {
		if result != nil && result.HTTPMeta.Response != nil && result.HTTPMeta.Response.Body != nil {
			_ = result.HTTPMeta.Response.Body.Close()
		}
		return "", fmt.Errorf("expected an SSE interaction response")
	}
	stream := result.InteractionSSEStreamEvent
	defer stream.Close()
	if result.HTTPMeta.Response == nil || result.HTTPMeta.Response.Body == nil {
		return "", fmt.Errorf("missing Interactions response")
	}
	mediaType, _, err := mime.ParseMediaType(result.HTTPMeta.Response.Header.Get("Content-Type"))
	if err != nil || mediaType != "text/event-stream" {
		return "", fmt.Errorf("interaction response is not an SSE stream")
	}
	events := newInteractionEventStream(ctx, result.HTTPMeta.Response.Body)
	defer events.Close()
	if events.Next() {
		event := events.Value()
		state := streamState{stateless: true}
		if err := state.consume(event.Data, func(adapter.StreamEvent) bool { return true }); err != nil {
			return "", err
		}
		var created types.InteractionCreatedEvent
		if err := json.Unmarshal(event.Data, &created); err != nil {
			return "", invalidResponse("invalid created interaction")
		}
		if (created.Interaction.Agent != nil && *created.Interaction.Agent != "") ||
			(created.Interaction.Object != nil && *created.Interaction.Object != "interaction") {
			return "", fmt.Errorf("SSE stream did not begin with a model interaction.created")
		}
		// 握手成立后立即关闭，不等待生成，也不把握手当作完整生成验证。
		return "", nil
	}
	if err := events.Err(); err != nil {
		return "", convertSDKError(err)
	}
	return "", fmt.Errorf("SSE stream contains no valid interaction.created")
}
