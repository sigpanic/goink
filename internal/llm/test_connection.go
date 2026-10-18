package llm

import (
	"context"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// ProviderProbeInput 是尚未保存的服务商配置，探测只使用临时适配器。
type ProviderProbeInput struct {
	AdapterName  adapter.Protocol `json:"adapter_name"`
	ProviderName string           `json:"provider_name"`
	ProviderType string           `json:"provider_type"`
	ChatURL      string           `json:"chat_url"`
	APIKey       string           `json:"api_key"`
}

// TestConnectionInput 是连通性测试的参数。
type TestConnectionInput struct {
	ProviderProbeInput
	ModelID string `json:"model_id"`
}

// TestConnectionResult 是连通性测试的结果。
// URL 是验证通过的实际端点（可能和入参不同，是 fallback 探测到的）。
// Warning 是非致命提示（如 429 限流视为通过但提示用户当前被限速）。
type TestConnectionResult = adapter.TestConnectionResult

// TestConnection 发送最小化请求验证 provider 连通性。
// 多层 fallback：对候选 URL 逐个真测（真 key + model + stream + max_tokens=1），
// 找到第一个返回 200 + text/event-stream + 有效 SSE chunk（含 choices 字段）的候选，
// 返回该 URL。全部失败返回 error，附带最后一个候选的错误信息。
//
// 返回值：
//   - (TestConnectionResult{URL:...}, nil)：验证通过，URL 是实际可用的端点（可能和入参不同，是 fallback 探测到的）
//   - (TestConnectionResult{URL:..., Warning:...}, nil)：验证通过但有警告（如 429 限流视为通过）
//   - (TestConnectionResult{}, error)：所有候选均失败
//
// 设计目标：用户填啥都能 work。
//   - 裸域名 https://1024token.club → 探测到 /v1/chat/completions
//   - base URL https://api.deepseek.com/v1 → 探测到 /v1/chat/completions
//   - 完整端点 https://x.com/proxy/chat → 原样通过
//
// 调用方（前端）应将返回的 URL 回写到 provider.chat_url，确保保存的 URL 和测试时一致。
// Warning 非空时也应展示给用户（不影响 ok 判定，但提示当前被限流等）。
func TestConnection(ctx context.Context, input TestConnectionInput) (TestConnectionResult, error) {
	client := newHTTPClient()
	defer client.CloseIdleConnections()
	backend, err := newProtocolAdapter(input.AdapterName, ProviderMap{
		input.ProviderName: {Name: input.ProviderName, ProviderType: input.ProviderType, ChatURL: input.ChatURL, APIKey: input.APIKey},
	}, client, nil)
	if err != nil {
		return TestConnectionResult{}, err
	}
	return backend.TestConnection(ctx, input.ProviderName, input.ModelID)
}
