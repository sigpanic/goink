package llm

import "context"

// DiscoverModels 使用所选协议的 Models 路由和能力映射，不在协议之间回退。
// 临时输入可补 https://；地址推导和缺失字段处理由适配器负责。
func DiscoverModels(ctx context.Context, input ProviderProbeInput) ([]ModelInfo, error) {
	client := newHTTPClient()
	defer client.CloseIdleConnections()
	input.ChatURL = normalizeProbeURL(input.ChatURL)
	backend, err := newProtocolAdapter(input.AdapterName, ProviderMap{
		input.ProviderName: {Name: input.ProviderName, ProviderType: input.ProviderType, ChatURL: input.ChatURL, APIKey: input.APIKey},
	}, client, nil)
	if err != nil {
		return nil, err
	}
	return backend.DiscoverModels(ctx, input.ProviderName)
}
