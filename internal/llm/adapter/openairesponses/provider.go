package openairesponses

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Provider 组合公共配置和私有 SDK service；实例名索引配置，类型选择协议内行为。
type Provider struct {
	adapter.Provider
	client responses.ResponseService
}

func assembleProvider(config adapter.Provider, client *http.Client) (Provider, error) {
	if config.Name == "" || strings.TrimSpace(config.Name) != config.Name {
		return Provider{}, fmt.Errorf("OpenAI Responses provider name must not be empty or have surrounding whitespace")
	}
	if err := ValidateProviderType(config.ProviderType); err != nil {
		return Provider{}, fmt.Errorf("provider %q: %w", config.Name, err)
	}
	endpoint, err := parseResponsesURL(config.ChatURL)
	if err != nil {
		return Provider{}, fmt.Errorf("provider %q: %w", config.Name, err)
	}
	origin := (&url.URL{Scheme: endpoint.Scheme, Host: endpoint.Host, Path: "/"}).String()
	return Provider{
		Provider: config.Clone(),
		// service 构造器不读取环境默认值；只使用当前实例和根包注入的 HTTP client。
		client: responses.NewResponseService(
			option.WithBaseURL(origin),
			option.WithAPIKey(config.APIKey),
			option.WithHTTPClient(client),
			option.WithMaxRetries(0),
		),
	}, nil
}

// ValidateProviderType 不依赖实例名称，官方及兼容中转均使用 openai 行为类型。
func ValidateProviderType(providerType string) error {
	if providerType != "openai" {
		return fmt.Errorf("unknown OpenAI Responses provider type %q", providerType)
	}
	return nil
}

// parseResponsesURL 只校验地址，不补路径；根地址可留待连接测试探测。
func parseResponsesURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("chat_url requires an absolute HTTP(S) URL")
	}
	if u.User != nil || u.Fragment != "" {
		return nil, fmt.Errorf("chat_url must not contain user info or a fragment")
	}
	return u, nil
}

// responsesURLOptions 用于 New / NewStreaming 的单次调用，正式请求与探测共用完整 URL 语义。
// 同时设置本次调用的 origin，满足 SDK 的来源校验，不污染实例或其他 API 的路由。
func responsesURLOptions(rawURL string) ([]option.RequestOption, error) {
	endpoint, err := parseResponsesURL(rawURL)
	if err != nil {
		return nil, err
	}
	origin := (&url.URL{Scheme: endpoint.Scheme, Host: endpoint.Host, Path: "/"}).String()
	return []option.RequestOption{
		option.WithBaseURL(origin),
		option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			request := req.Clone(req.Context())
			target := *endpoint
			request.URL = &target
			request.Host = target.Host
			return next(request)
		}),
	}, nil
}
