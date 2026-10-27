# LLM 服务商命名规则

本目录沿用仓库根目录协作规则，并遵守以下命名和路由约定。

- `Provider.Name` 是面向用户的实例名称。内置实例优先使用用户熟悉的模型品牌或产品名称，例如 Claude、Gemini、ChatGPT；自定义实例使用用户填写的名称。
- Name 在所属协议内唯一，`ProviderMap` 的 key 必须与实例 Name 一致。Name 同时用于配置合并、实例查找和模型选择引用，不能当成仅影响显示的字段随意改名。
- `ProviderType` 是厂商适配规则的稳定标识，命名优先使用小写公司名称，例如 `anthropic`、`google`、`openai`、`bytedance`、`alibaba`、`xiaomi`。它用于校验及 hooks 选择，不作为实例索引；同一 Type 可以有多个实例。
- 公司与模型品牌同名时直接沿用，例如 DeepSeek、MiniMax。OpenCode Zen / Go 等网关使用服务名称；需要不同适配规则的产品或线路可以保留独立 Type，不强行合并为一个公司类型。
- 协议族由 `adapter.Protocol` 明确指定。不得根据实例名称、模型 ID 或 URL 猜测协议或 ProviderType。
- `ModelInfo.ID` / `ModelInfo.Name` 表示具体模型。修改 ProviderType 时，不修改 API 模型 ID、接口 URL 或模型专属参数名称。
- 前端通过 ProviderType 映射图标，图标优先使用相应模型或产品品牌标识；Type 使用公司名称，不要求图标必须是公司 logo。
- 内置实例的持久化覆盖项不保存 ProviderType，由内置默认配置补齐；自定义实例须保存明确的 ProviderType。
- 修改 Name 或 Type 时同步检查内置 map、查表引用、校验、hooks、前端图标映射、持久化引用与迁移。历史文件中的旧索引必须按原值识别，不能随运行时类型改名直接替换。

当前内置实例命名：

| Name | ProviderType |
| --- | --- |
| Claude | `anthropic` |
| Gemini | `google` |
| ChatGPT | `openai` |
| DeepSeek | `deepseek` |
| Doubao | `bytedance` |
| Qwen | `alibaba` |
| GLM | `zhipu` |
| MiniMax | `minimax` |
| MiMo | `xiaomi` |
| Kimi | `moonshot` |
| OpenCode Zen | `opencode` |
| OpenCode Go | `opencode-go` |
