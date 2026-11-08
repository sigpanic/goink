package v180

import (
	"encoding/json"

	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func LLM() seeddata.SeedCase {
	before := seeddata.SeedData{
		LLMConfig: json.RawMessage("{\"adapters\":{\"openai_chat\":{\"providers\":[{\"api_key\":\"fixture-builtin-key\",\"chat_url\":\"https://builtin.example/v8/chat/completions?region=cn\",\"models\":[{\"context_window\":65536,\"id\":\"fixture-deepseek\",\"max_output_tokens\":8192,\"name\":\"内置服务商自定义模型\",\"reasoning_levels\":[\"low\",\"high\"],\"supports_thinking\":true,\"supports_vision\":true}],\"name\":\"DeepSeek\",\"temperature\":0},{\"api_key\":\"fixture-relay-key\",\"chat_url\":\"https://relay.example/custom/chat/completions?tenant=fixture\",\"models\":[{\"context_window\":32768,\"id\":\"vendor/model\",\"max_output_tokens\":4096,\"name\":\"中转模型\",\"supports_thinking\":false,\"supports_vision\":false}],\"name\":\"升级中转站\",\"provider_type\":\"openai\",\"temperature\":0.7}]}},\"version\":2}"),
	}
	return seeddata.SeedCase{
		ID:     "llm-v2",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertConfig(result, json.RawMessage("{\"adapters\":{\"openai_chat\":{\"providers\":[{\"api_key\":\"fixture-builtin-key\",\"chat_url\":\"https://builtin.example/v8/chat/completions?region=cn\",\"models\":[{\"context_window\":65536,\"id\":\"fixture-deepseek\",\"max_output_tokens\":8192,\"name\":\"内置服务商自定义模型\",\"reasoning_levels\":[\"low\",\"high\"],\"supports_thinking\":true,\"supports_vision\":true}],\"name\":\"DeepSeek\",\"temperature\":0},{\"api_key\":\"fixture-relay-key\",\"chat_url\":\"https://relay.example/custom/chat/completions?tenant=fixture\",\"models\":[{\"context_window\":32768,\"id\":\"vendor/model\",\"max_output_tokens\":4096,\"name\":\"中转模型\",\"supports_thinking\":false,\"supports_vision\":false}],\"name\":\"升级中转站\",\"provider_type\":\"openai\",\"temperature\":0.7}]}},\"version\":2}"))
		},
	}
}
