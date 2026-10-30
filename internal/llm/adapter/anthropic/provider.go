package anthropic

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Provider 组合公共配置和私有 SDK client；实例名负责索引，类型只选择协议内行为。
type Provider struct {
	adapter.Provider
	client sdk.Client
}

func assembleProvider(config adapter.Provider, client *http.Client) (Provider, error) {
	if config.Name == "" || strings.TrimSpace(config.Name) != config.Name {
		return Provider{}, fmt.Errorf("anthropic provider name must not be empty or have surrounding whitespace")
	}
	if err := ValidateProviderType(config.ProviderType); err != nil {
		return Provider{}, fmt.Errorf("provider %q: %w", config.Name, err)
	}
	endpoint, err := adapter.ParseChatURL(config.ChatURL)
	if err != nil {
		return Provider{}, fmt.Errorf("provider %q: %w", config.Name, err)
	}
	// SDK 的默认地址只保留本实例的 origin；Messages 调用通过 messagesURLOption 指定完整端点。
	// Models 等其他 API 由各自调用指定路由，不复用 Messages 的完整端点覆盖。
	origin := (&url.URL{Scheme: endpoint.Scheme, Host: endpoint.Host}).String()
	return Provider{
		Provider: config.Clone(),
		client: sdk.NewClient(
			// Goink 已完成实例配置解析，禁止 SDK 从环境或本机 profile 补入其他凭据及地址。
			option.WithoutEnvironmentDefaults(),
			option.WithBaseURL(origin),
			option.WithAPIKey(config.APIKey),
			option.WithHTTPClient(client),
			option.WithMaxRetries(0),
		),
	}, nil
}

// ValidateProviderType 独立于内置实例名称，官方及兼容中转均使用 anthropic 行为类型。
func ValidateProviderType(providerType string) error {
	if providerType != "anthropic" {
		return fmt.Errorf("unknown Anthropic provider type %q", providerType)
	}
	return nil
}

// messagesURLOption 仅用于 Messages.New / NewStreaming 的单次调用，不能挂在整个 SDK client 上。
// 正式请求传保存的 ChatURL，连接测试传候选 URL；两者均保留完整路径、转义及查询参数。
func messagesURLOption(rawURL string) (option.RequestOption, error) {
	endpoint, err := adapter.ParseChatURL(rawURL)
	if err != nil {
		return nil, err
	}
	return option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		request := req.Clone(req.Context())
		target := *endpoint
		request.URL = &target
		request.Host = target.Host
		return next(request)
	}), nil
}
