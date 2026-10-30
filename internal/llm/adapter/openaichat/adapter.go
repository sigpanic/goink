// Package openaichat 封装现有 OpenAI Chat Completions 兼容调用链。
package openaichat

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Hooks 处理 OpenAI 兼容协议内的厂商差异，不参与配置序列化。
type Hooks struct {
	BuildRequest func(map[string]any) map[string]any
	BuildHeaders func(*adapter.CallOptions, map[string]string) map[string]string
	ParseError   func([]byte) error
}

// Adapter 负责单次模型调用及协议转换，复用上层注入的 HTTP client。
type Adapter struct {
	providers map[string]Provider
	http      *http.Client
	logger    *slog.Logger
}

var _ adapter.ProtocolAdapter = (*Adapter)(nil)

// New 接收配置层已合并的完整实例配置，创建独立的 providers 快照。
// 构造后配置只读；热重载应创建新实例，让进行中的请求继续使用旧实例。
func New(configs adapter.ProviderMap, client *http.Client, logger *slog.Logger) (*Adapter, error) {
	providers := make(map[string]Provider, len(configs))
	for providerName, config := range configs {
		if providerName != config.Name {
			return nil, fmt.Errorf("OpenAI Chat provider map name %q does not match configuration name %q", providerName, config.Name)
		}
		p, err := assembleProvider(config)
		if err != nil {
			return nil, err
		}
		providers[providerName] = p
	}
	if client == nil {
		return nil, fmt.Errorf("OpenAI Chat adapter requires an injected HTTP client")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{providers: providers, http: client, logger: logger}, nil
}

// Providers 返回公共配置副本，不暴露本包的运行时 hooks 或内部可变字段。
func (c *Adapter) Providers() adapter.ProviderMap {
	providers := make(adapter.ProviderMap, len(c.providers))
	for providerName, p := range c.providers {
		providers[providerName] = p.Clone()
	}
	return providers
}

func (c *Adapter) provider(providerName string) (Provider, error) {
	p, ok := c.providers[providerName]
	if !ok {
		return Provider{}, fmt.Errorf("unknown OpenAI Chat provider: %s", providerName)
	}
	return p, nil
}

// presetHooks 根据明确的服务商类型选择协议规则，不猜测 URL 或模型名称。
// provider_type 校验由 assembleProvider 完成，不依赖内置实例的名称。
func presetHooks(providerType string) Hooks {
	switch providerType {
	case "opencode", "opencode-go":
		return Hooks{BuildHeaders: opencodeBuildHeaders}
	case "minimax":
		return Hooks{BuildRequest: minimaxBuildRequest}
	case "alibaba":
		return Hooks{BuildRequest: qwenBuildRequest}
	case "moonshot":
		return Hooks{BuildRequest: moonshotBuildRequest}
	case "xiaomi":
		return Hooks{BuildHeaders: mimoBuildHeaders}
	default:
		return Hooks{}
	}
}

type APIError = adapter.APIError
type StreamEvent = adapter.StreamEvent
type ToolCallDelta = adapter.ToolCallDelta
type CallOptions = adapter.CallOptions
type ModelInfo = adapter.ModelInfo

const (
	EventThinking          = adapter.EventThinking
	EventContent           = adapter.EventContent
	EventToolCallStart     = adapter.EventToolCallStart
	EventToolCallArguments = adapter.EventToolCallArguments
	EventToolCallEnd       = adapter.EventToolCallEnd
	EventUsage             = adapter.EventUsage
	EventError             = adapter.EventError
)
