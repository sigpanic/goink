package adapter

// Provider 是各协议共用的服务商配置，不包含 SDK client 或协议专属 hooks。
// providerName 是所属适配器 providers 集合中的名称；Name 可用于展示。
type Provider struct {
	Name         string      `json:"name"`
	ProviderType string      `json:"provider_type,omitempty"`
	ChatURL      string      `json:"chat_url"`
	APIKey       string      `json:"api_key,omitempty"`
	PlatformURL  string      `json:"-"`
	HelpText     string      `json:"-"`
	Models       []ModelInfo `json:"models"`
	Temperature  *float64    `json:"temperature,omitempty"`
}
