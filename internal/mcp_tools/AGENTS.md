# MCP 工具开发规范

本目录规则适用于 `internal/mcp_tools/` 下所有代码。详细背景和示例见同目录 `rules.md`。

## 结构与注册

- 一个领域一个 `xxx_tools.go` 文件，公共工具函数放在 `utils.go`。
- 注册函数必须是独立顶层函数，命名为 `RegisterXxxTools(r *Registry)`，并在 `registry.go` 注册。
- 工具实现通常包含 `Name`、`Description`、`Category`、`JSONSchema`、`ExposeToLLM`、`NewArgs` 和 `Execute`。
- `ExposeToLLM()` 默认返回 `true`。

## 参数与校验

- Args 字段同时使用 `jsonschema` 和 `validate` tag；`jsonschema` 面向 LLM，`validate` 面向 Registry。
- 参数校验由 `Registry.Execute` 统一执行。工具不要自行 `json.Unmarshal` 参数或调用 `validate.Struct`，直接使用已校验的 `args.(*XxxArgs)`。
- `tc.RawArgs` 是 Registry 注入的原始 JSON。PATCH 型 update 工具可对允许修改的 Args 使用 `json.Unmarshal(tc.RawArgs, &entity)`，JSON 中未出现的字段保持原值。
- Finder 字段使用清晰的 JSON 名称，如 `character_id`、`entry_id`、`relation_id`，不要为了复用数据库主键字段改成 `id`。

## 错误处理

- 参数不合法、资源不存在、状态冲突等预期业务分支：返回 `&ToolResult{Success: false, Error: "..."}, nil`，错误消息使用中文。消息只说明已知的业务结论；不要附带驱动或网络的原始错误。
- 数据库、磁盘、网络等意外依赖异常：返回包装后的 error，例如 `fmt.Errorf("context: %w", err)`，交给 `Registry.Execute` 兜底。若已发生部分操作，包装错误时必须先说明已确认状态，例如 `fmt.Errorf("正文已回退，标题未更新: %w", err)`。
- 业务与系统的判定标准：原样重试同一个请求会成功属于 system；必须换输入、换来源或改配置才可能成功属于 business。依据是 `ErrKind` 只驱动中断策略，system 连续失败会中断对话，因此只有重试可能恢复或自身依赖/代码故障才归 system。
- `Registry.Execute` 会把所有返回给 LLM 的错误文本压成单行并限制为最多 300 字符；业务/参数 `ToolResult.Error` 只保留其原文摘要，system error 还会保留完整日志并以 `ErrKindSystem` 返回“状态 + 原因”。工具不要自行截断或手工构造 system `ToolResult`。
- 不要在工具中 `recover()`；只有需要把 `gorm.ErrRecordNotFound` 转成业务错误时才在工具内处理。

## 工具行为

- 主 agent 和子 agent 都通过 `AllowedTools` 控制工具白名单；OpenAI 展示和 Execute 阶段都必须做限制。
- Category 仅用于组织，不驱动额外行为：`novel_management`、`writing_assistant`、`memory_retrieval`、`consistency_check`。
- 返回给 LLM 的内容优先使用易读 Markdown，而不是原始 JSON；图结构优先返回邻接表。
- 需要被其他工具引用的 ID 使用 `[args_json_tag:X]` 格式，例如 `[entry_id:5]`；不需要引用的 ID 不重复输出。
- CRUD 工具使用 `create_`、`update_`、`get_` 前缀，禁止用 `add_`、`delete_`、`list_`。
- `place_volume` 与 Volume Store 的放置语义一致，单次创建或移动一个卷，作为命名及批量创建约定的例外。
- 查询工具以外的工具不要重复返回 LLM 刚传入的字段；update 成功时说明修改成功即可。

## create 与事务

- create 工具使用循环 + transaction，不能用 `db.Create(&[]T)` 批量插入，以保证每一行的主键和 operation_log 回调正确。
- Args 用 slice 包裹 Item，使用 `validate:"min=1,max=N,dive"`；先做业务预校验，再在 `db.WithContext(ctx).Transaction(...)` 中逐条创建。
- 失败时返回具体条目的原因，并保证原子性；成功返回 `ids` 和 `count`。
- 单列 IN 查询使用 batch 查询；双列 pair 匹配在小规模 N 下可循环查询。
