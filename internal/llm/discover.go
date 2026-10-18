package llm

import (
	"context"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// DiscoverModels 调用 /models 端点自动发现可用模型列表。
// 从 chatURL 推导 modelsURL（去掉 /chat/completions，拼接 /models），
// 解析标准 OpenAI 格式及 Kimi 等扩展字段。返回的 ModelInfo 中未获取到的字段留零值。
func DiscoverModels(ctx context.Context, input ProviderProbeInput) ([]ModelInfo, error) {
	client := newHTTPClient()
	defer client.CloseIdleConnections()
	backend, err := newProtocolAdapter(input.AdapterName, map[string]adapter.Provider{
		input.ProviderName: {Name: input.ProviderName, ProviderType: input.ProviderType, ChatURL: input.ChatURL, APIKey: input.APIKey},
	}, client, nil)
	if err != nil {
		return nil, err
	}
	return backend.DiscoverModels(ctx, input.ProviderName)
}
