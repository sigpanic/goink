package llm

import (
	"maps"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Providers 返回按协议分组的公共配置副本，不暴露具体适配器内部状态。
func (c *Client) Providers() map[adapter.Protocol]ProviderMap {
	c.mu.RLock()
	backends := make(map[adapter.Protocol]adapter.ProtocolAdapter, len(c.adapters))
	maps.Copy(backends, c.adapters)
	c.mu.RUnlock()
	providers := make(map[adapter.Protocol]ProviderMap, len(backends))
	for adapterName, backend := range backends {
		providers[adapterName] = backend.Providers()
	}
	return providers
}

// ProviderModel 在明确指定的协议和 provider 中按 model ID 查找模型元信息。
func (c *Client) ProviderModel(adapterName adapter.Protocol, providerName, modelID string) (*ModelInfo, bool) {
	backend, err := c.protocolAdapter(adapterName)
	if err != nil {
		return nil, false
	}
	p, ok := backend.Providers()[providerName]
	if !ok {
		return nil, false
	}
	for _, model := range p.Models {
		if model.ID == modelID {
			return &model, true
		}
	}
	return nil, false
}
