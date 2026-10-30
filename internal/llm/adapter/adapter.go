// Package adapter 定义 Goink 模型调用契约，不依赖具体协议实现或 SDK。
package adapter

import "context"

// Protocol 标识顶层适配器的协议族；同名 provider 的作用域由协议确定。
type Protocol string

const (
	ProtocolOpenAIChat            Protocol = "openai_chat"
	ProtocolOpenAIResponses       Protocol = "openai_responses"
	ProtocolAnthropicMessages     Protocol = "anthropic_messages"
	ProtocolGeminiGenerateContent Protocol = "gemini_generate_content"
	ProtocolGeminiInteractions    Protocol = "gemini_interactions"
)

// ProtocolAdapter 将一次 Goink 调用转换为对应协议，并返回统一结果。
// Stream 的错误通过 EventError 返回，生产方关闭 channel；提前停止消费须取消 context。
// 两个方法均不执行工具或业务重试。
// 这里的两个生成方法指 Stream / Generate；配置查询和探测使用同一 provider 作用域。
// 每个具体实现持有本协议的 providers，按 Request.ProviderName 查找配置，再解析 Model。
// 顶层调用方先按 Protocol 选择适配器，具体实现之间不查找或回退 provider。
type ProtocolAdapter interface {
	// Providers 返回本适配器的 provider 配置快照，调用方修改副本不影响正在执行的请求。
	Providers() ProviderMap
	Stream(ctx context.Context, req Request) <-chan StreamEvent
	Generate(ctx context.Context, req Request) (Response, error)

	// DiscoverModels 使用指定 provider 发现模型；不支持时返回错误，允许手动配置模型。
	DiscoverModels(ctx context.Context, providerName string) ([]ModelInfo, error)

	// TestConnection 保留协议专属的最小请求、端点探测及结果判定。
	TestConnection(ctx context.Context, providerName, model string) (TestConnectionResult, error)
}
