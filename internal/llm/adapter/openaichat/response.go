package openaichat

import "github.com/sigpanic/goink/internal/llm/adapter"

// responseState 记录已完整接收的响应；Chat 无需保存原生回放 payload。
func (p Provider) responseState(model, stopReason string, hasTools bool) *adapter.ResponseState {
	turnComplete := false
	switch stopReason {
	case "", "stop", "content_filter":
		turnComplete = !hasTools
	}
	return &adapter.ResponseState{
		Origin: adapter.MessageOrigin{
			AdapterName: adapter.ProtocolOpenAIChat, ProviderName: p.Name, ProviderType: p.ProviderType, Model: model,
		},
		Complete: true, TurnComplete: turnComplete, StopReason: stopReason,
	}
}
