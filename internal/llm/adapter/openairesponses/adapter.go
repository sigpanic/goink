// Package openairesponses 封装 OpenAI Responses 协议和实例配置。
package openairesponses

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Adapter 持有本协议的 provider 快照；生成、发现及连接探测随后在本包实现。
type Adapter struct {
	providers map[string]Provider
	logger    *slog.Logger
}

var _ adapter.ProtocolAdapter = (*Adapter)(nil)

// New 接收已合并的完整实例配置，不补入内置实例，也不发送请求。
func New(configs adapter.ProviderMap, client *http.Client, logger *slog.Logger) (*Adapter, error) {
	if client == nil {
		return nil, fmt.Errorf("OpenAI Responses adapter requires an injected HTTP client")
	}
	providers := make(map[string]Provider, len(configs))
	for providerName, config := range configs {
		if providerName != config.Name {
			return nil, fmt.Errorf("OpenAI Responses provider map name %q does not match configuration name %q", providerName, config.Name)
		}
		p, err := assembleProvider(config, client)
		if err != nil {
			return nil, err
		}
		providers[providerName] = p
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{providers: providers, logger: logger}, nil
}

// Providers 返回公共配置的独立副本，不暴露 SDK service。
func (c *Adapter) Providers() adapter.ProviderMap {
	providers := make(adapter.ProviderMap, len(c.providers))
	for name, p := range c.providers {
		providers[name] = p.Clone()
	}
	return providers
}

func (c *Adapter) provider(providerName string) (Provider, error) {
	p, ok := c.providers[providerName]
	if !ok {
		return Provider{}, fmt.Errorf("unknown OpenAI Responses provider: %s", providerName)
	}
	return p, nil
}
