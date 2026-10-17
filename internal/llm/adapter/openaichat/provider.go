package openaichat

import (
	"fmt"
	"strings"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Provider 组合公共配置和 OpenAI Chat 专属规则，保存在本协议适配器内部。
type Provider struct {
	adapter.Provider
	Hooks `json:"-"`
}

// assembleProvider 接收配置层已合并的实例，只复制配置并绑定本协议的运行时规则。
func assembleProvider(config adapter.Provider) (Provider, error) {
	if config.Name == "" || strings.TrimSpace(config.Name) != config.Name {
		return Provider{}, fmt.Errorf("OpenAI Chat provider name must not be empty or have surrounding whitespace")
	}
	if err := ValidateProviderType(config.ProviderType); err != nil {
		return Provider{}, fmt.Errorf("provider %q: %w", config.Name, err)
	}
	return Provider{Provider: config.Clone(), Hooks: presetHooks(config.ProviderType)}, nil
}

// ValidateProviderType 校验厂商规则类型，独立于内置实例集合；无专属 hook 的类型也合法。
func ValidateProviderType(providerType string) error {
	switch providerType {
	case "openai", "deepseek", "opencode", "opencode-go", "doubao", "qwen", "zhipu", "minimax", "mimo", "moonshot":
		return nil
	default:
		return fmt.Errorf("unknown OpenAI Chat provider type %q", providerType)
	}
}
