package llm

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/llm/adapter/anthropic"
	"github.com/sigpanic/goink/internal/llm/adapter/gemini"
	"github.com/sigpanic/goink/internal/llm/adapter/openaichat"
	"github.com/sigpanic/goink/internal/llm/adapter/openairesponses"
)

// buildAdapters 按协议分组配置，构建独立实例；热重载后已有请求继续持有旧适配器。
func buildAdapters(config *UserLLMConfig, client *http.Client, logger *slog.Logger) (map[adapter.Protocol]adapter.ProtocolAdapter, error) {
	grouped, err := Merge(config)
	if err != nil {
		return nil, err
	}
	adapters := make(map[adapter.Protocol]adapter.ProtocolAdapter, len(grouped))
	for adapterName, providers := range grouped {
		backend, err := newProtocolAdapter(adapterName, providers, client, logger)
		if err != nil {
			return nil, err
		}
		adapters[adapterName] = backend
	}
	return adapters, nil
}

// newProtocolAdapter 将配置层的完整实例交给协议子包，构造其运行时 providers。
func newProtocolAdapter(adapterName adapter.Protocol, providers ProviderMap, client *http.Client, logger *slog.Logger) (adapter.ProtocolAdapter, error) {
	switch adapterName {
	case adapter.ProtocolOpenAIChat:
		return openaichat.New(providers, client, logger)
	case adapter.ProtocolOpenAIResponses:
		return openairesponses.New(providers, client, logger)
	case adapter.ProtocolAnthropicMessages:
		return anthropic.New(providers, client, logger)
	case adapter.ProtocolGeminiInteractions:
		return gemini.New(providers, client, logger)
	default:
		return nil, fmt.Errorf("unsupported adapter: %s", adapterName)
	}
}

func validateProviderType(adapterName adapter.Protocol, providerType string) error {
	switch adapterName {
	case adapter.ProtocolOpenAIChat:
		return openaichat.ValidateProviderType(providerType)
	case adapter.ProtocolOpenAIResponses:
		return openairesponses.ValidateProviderType(providerType)
	case adapter.ProtocolAnthropicMessages:
		return anthropic.ValidateProviderType(providerType)
	case adapter.ProtocolGeminiInteractions:
		return gemini.ValidateProviderType(providerType)
	default:
		return fmt.Errorf("unsupported adapter: %s", adapterName)
	}
}

func builtinProviders() map[adapter.Protocol]ProviderMap {
	return map[adapter.Protocol]ProviderMap{
		adapter.ProtocolOpenAIChat:         openaichat.BuiltinProviders(),
		adapter.ProtocolOpenAIResponses:    openairesponses.BuiltinProviders(),
		adapter.ProtocolAnthropicMessages:  anthropic.BuiltinProviders(),
		adapter.ProtocolGeminiInteractions: gemini.BuiltinProviders(),
	}
}
