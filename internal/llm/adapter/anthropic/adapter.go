// Package anthropic 封装 Anthropic Messages 协议和实例配置。
package anthropic

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Adapter 持有本协议的 provider 快照；发现及探测方法在后续单元接入。
type Adapter struct {
	providers map[string]Provider
	logger    *slog.Logger
}

// New 接收已合并的完整实例配置，为每个实例创建独立 SDK client。
// 配置构造后只读；热重载通过新建 Adapter 保留进行中请求使用的旧快照。
func New(configs adapter.ProviderMap, client *http.Client, logger *slog.Logger) (*Adapter, error) {
	if client == nil {
		return nil, fmt.Errorf("anthropic adapter requires an injected HTTP client")
	}
	providers := make(map[string]Provider, len(configs))
	for providerName, config := range configs {
		if providerName != config.Name {
			return nil, fmt.Errorf("anthropic provider map name %q does not match configuration name %q", providerName, config.Name)
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

// Providers 返回公共配置的独立副本，不暴露 SDK client。
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
		return Provider{}, fmt.Errorf("unknown Anthropic provider: %s", providerName)
	}
	return p, nil
}
