// Package gemini 封装 Google Interactions 协议和实例配置。
package gemini

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Adapter 持有本协议的 provider 快照，封装生成、发现及探测。
type Adapter struct {
	providers map[string]Provider
	logger    *slog.Logger
}

var _ adapter.ProtocolAdapter = (*Adapter)(nil)

// New 接收完整实例配置和共享 HTTP client；构造不发请求，不自行补入内置实例。
func New(configs adapter.ProviderMap, client *http.Client, logger *slog.Logger) (*Adapter, error) {
	if client == nil {
		return nil, fmt.Errorf("gemini adapter requires an injected HTTP client")
	}
	providers := make(map[string]Provider, len(configs))
	for name, config := range configs {
		if name != config.Name {
			return nil, fmt.Errorf("gemini provider map name %q does not match configuration name %q", name, config.Name)
		}
		p, err := assembleProvider(config, client)
		if err != nil {
			return nil, err
		}
		providers[name] = p
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{providers: providers, logger: logger}, nil
}

// Providers 返回公共配置的独立快照，不暴露 SDK client。
func (c *Adapter) Providers() adapter.ProviderMap {
	providers := make(adapter.ProviderMap, len(c.providers))
	for name, p := range c.providers {
		providers[name] = p.Clone()
	}
	return providers
}

func (c *Adapter) provider(name string) (Provider, error) {
	p, ok := c.providers[name]
	if !ok {
		return Provider{}, fmt.Errorf("unknown Gemini provider: %s", name)
	}
	return p, nil
}
