package llm

import (
	"context"
	"strings"

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
// Warning 是非致命提示；限流提示不代表已验证密钥、模型或完整生成。
type TestConnectionResult = adapter.TestConnectionResult

// TestConnection 发送最小化请求验证 provider 连通性。
// 按所选协议对候选 URL 逐个真测，验证鉴权、模型和协议流的开头，
// 返回第一个可用端点；限流按协议返回告警，不等同于完成生成。
// 全部失败返回 error，包含候选的失败信息；正式生成不执行该探测。
//
// 返回值：
//   - (TestConnectionResult{URL:...}, nil)：验证通过，URL 是实际可用的端点（可能和入参不同，是 fallback 探测到的）
//   - (TestConnectionResult{URL:..., Warning:...}, nil)：端点有警告，验证范围以 Warning 为准
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
	input.ChatURL = normalizeProbeURL(input.ChatURL)
	backend, err := newProtocolAdapter(input.AdapterName, ProviderMap{
		input.ProviderName: {Name: input.ProviderName, ProviderType: input.ProviderType, ChatURL: input.ChatURL, APIKey: input.APIKey},
	}, client, nil)
	if err != nil {
		return TestConnectionResult{}, err
	}
	return backend.TestConnection(ctx, input.ProviderName, input.ModelID)
}

// normalizeProbeURL 只处理尚未保存的输入，不改变正式调用的完整端点语义。
func normalizeProbeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw != "" && !strings.Contains(raw, "://") {
		return "https://" + raw
	}
	return raw
}
