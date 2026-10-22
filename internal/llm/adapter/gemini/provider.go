package gemini

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	sdk "google.golang.org/genai/interactions"
	"google.golang.org/genai/interactions/models/components"
	"google.golang.org/genai/interactions/retry"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Provider 组合公共配置、私有 Create 服务及共享传输，不持有会话状态。
type Provider struct {
	adapter.Provider
	client *sdk.Interactions
	http   *http.Client
}

func assembleProvider(config adapter.Provider, client *http.Client) (Provider, error) {
	if config.Name == "" || strings.TrimSpace(config.Name) != config.Name {
		return Provider{}, fmt.Errorf("gemini provider name must not be empty or have surrounding whitespace")
	}
	if err := ValidateProviderType(config.ProviderType); err != nil {
		return Provider{}, fmt.Errorf("provider %q: %w", config.Name, err)
	}
	p := Provider{Provider: config.Clone(), http: client}
	service, err := p.clientForURL(p.ChatURL)
	if err != nil {
		return Provider{}, fmt.Errorf("provider %q: %w", config.Name, err)
	}
	p.client = service
	return p, nil
}

// ValidateProviderType 选择协议内行为，不把官方实例名或模型名当作类型。
func ValidateProviderType(providerType string) error {
	if providerType != "google" {
		return fmt.Errorf("unknown Gemini provider type %q", providerType)
	}
	return nil
}

// clientForURL 为正式端点或单次候选创建独立 SDK 服务，复用相同传输和实例凭据。
// 不改写 p.client / ChatURL；仅用于 Create，Models 等 API 分别处理路由。
func (p Provider) clientForURL(rawURL string) (*sdk.Interactions, error) {
	if p.http == nil {
		return nil, fmt.Errorf("gemini provider requires an injected HTTP client")
	}
	endpoint, err := parseInteractionsURL(rawURL)
	if err != nil {
		return nil, err
	}
	origin := (&url.URL{Scheme: endpoint.Scheme, Host: endpoint.Host}).String()
	return sdk.New(
		sdk.WithServerURL(origin),
		sdk.WithAPIVersion("v1"),
		sdk.WithSecurity(components.Security{APIKey: &p.APIKey}),
		sdk.WithRetryConfig(retry.Config{Strategy: "none"}),
		sdk.WithClient(interactionsEndpointClient{client: p.http, endpoint: *endpoint}),
	).Interactions, nil
}

// parseInteractionsURL 只校验完整地址，不补路径或猜测版本；根地址可用于后续探测。
func parseInteractionsURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("chat_url requires an absolute HTTP(S) URL")
	}
	if u.User != nil || u.Fragment != "" {
		return nil, fmt.Errorf("chat_url must not contain user info or a fragment")
	}
	return u, nil
}

// interactionsEndpointClient 只为 SDK Create 覆盖完整端点，不修改共享 client 或 transport。
// SDK v1.75.0 的 Create 不接受 WithURLOverride，必须在调用共享 client 前保留完整 URL。
type interactionsEndpointClient struct {
	client   *http.Client
	endpoint url.URL
}

func (c interactionsEndpointClient) Do(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodPost || req.URL.Path != "/v1/interactions" {
		return nil, fmt.Errorf("endpoint override only supports Interactions.Create")
	}
	request := req.Clone(req.Context())
	target := c.endpoint
	request.URL, request.Host = &target, target.Host
	return c.client.Do(request)
}
