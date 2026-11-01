# 结构化消息输入与存储映射实施路线

> 2026-10-18。待实现。本阶段以本文为准：先改 llm 底层及必要的 session 映射，再接上层；不沿用其他设计稿中先转 OpenAI map、再携带内部保留键的输入方案。

## 做什么

将 `Request.Messages` 从 `[]map[string]any` 改为 Goink 结构化消息。数据库 metadata 保存公共业务信息和不透明原生 payload；各适配器自行构造完整的协议请求消息。

```text
DB session.Message
  正文、思考、metadata〔公共工具信息 + 来源 / 原生 payload + 展示等其他信息〕
       ↓ session 映射：提取公共信息，原生 payload 原样搬运
[]adapter.Message
       ↓ 目标适配器：历史处理、原生状态兼容判断、协议转换
OpenAI messages / Anthropic system + blocks / Gemini contents

模型响应 → 适配器 → 公共响应 / 事件 + 原生状态
                         ↓ 后续 Agent 接线
                   session 映射 → DB 新消息
```

本次采用自定义消息契约，继续官方 SDK 路线，不引入统一模型库。公共消息只承载业务数据，不依赖 session、具体适配器或 SDK。

## 数据与职责

- 在 `internal/llm/adapter/message.go` 定义 `Message`、完整 `ToolCall`、来源和原生状态。消息包含角色、正文、显示用思考、工具调用列表、结果关联 ID / 工具名及可选原生状态；工具结果正文继续使用 Content，不新增重复结果 DTO。
- 完整 `ToolCall` 保存 ID、名称和 `json.RawMessage` 参数；`ToolCallDelta` 继续表达流式过程。来源记录协议、provider 实例、providerType 和 model；原生 payload 使用 `json.RawMessage`，不放 SDK 类型或凭据。
- 公共工具 metadata 沿用现有 OpenAI 形状：`tool_calls[].id/type/function`，其中 arguments 仍存为 JSON 字符串；tool 消息沿用 `tool_call_id` / `tool_name`。普通工具调用在入站时已归一化，session 不从厂商 payload 提取工具，也不解析签名。
- `Response` 和最终状态事件承载同一份原生状态；追加事件枚举，不改变已有事件值。响应传输完成与业务 turn 是否结束分别表达，不据此实现服务端工具续接。
- `session` 提供 DB → 公共消息及公共消息 → 新 DB 消息的映射能力。只解析公共 metadata，原生 payload 保真透传；写入保留展示、审计及未知 metadata，不用模型消息覆盖整个 ExtraMetadata，不更新历史记录。
- 既有用户时间前缀规则在 session 映射处统一保留，区分原始存储内容和请求渲染内容；往返不重复加前缀，不把新建消息时间冒充原始发言时间。
- 公共历史辅助函数只处理共同的调用 / 结果关联检查。重复、冲突、孤立和缺失配对分别识别；不建立强制统一过滤流水线，不按全历史 ID 集合随意删消息。具体适配器决定报错、去重或其他处理。
- OpenAI 包把 metadata 之外的协议转换、历史处理及 system 合并收拢到整组 `buildMessages`，返回可直接使用的 `[]map[string]any` 和错误；Stream / Generate 共用，`buildPayload` 不再二次合并。现有 hooks 的作用顺序和行为需回归，不直接套用到其他协议。
- 完整原生回放须经目标适配器确认；来源相同不自动保证兼容。不同来源默认以公共业务内容重建，不兼容推理只在出站副本丢弃。清理改变内容时不得继续使用与之不一致的原生状态。

## 范围与迁移

主要修改 `internal/llm/`，仅为存储映射涉及 `internal/session/`。不修改 App、Agent、pattern、压缩编排、前端、生成绑定、数据库表结构或 migrate 包，不实现 Claude / Gemini 及切换生命周期。

现有工具 metadata 保持不变，无需为普通工具调用迁移。新增可选来源和原生状态不批量补历史；没有状态的消息继续使用公共业务内容。若最终确有其他格式变更，迁移待全部确定后另做；本次不加迁移兼容分支，不扫描或改写用户数据，来源未知的历史不得伪造签名。

允许中间阶段及本阶段结束时上层编译失败，不为通过整仓构建、测试或 hook 扩大范围。DB 映射能力完成不等于生产调用方已经接通，入站实际落库由后续 Agent 阶段完成。

## Commit / review 路线

| 单元 | 建议 subject | 文件范围与交付 |
| --- | --- | --- |
| 1 | `refactor(llm): define structured messages and replay state` | `adapter/message.go`、`adapter/types.go`、必要的根包类型别名及测试。定义消息、公共 metadata 契约、来源、原生状态和响应完成信息，提供必要的 Clone；先不改调用方。 |
| 2 | `refactor(llm): prepare OpenAI message histories inside the adapter` | `adapter/openaichat/messages.go`、stream / generate / 消息相关 hooks 及测试。接收公共消息，收拢历史处理与 system 合并；按需提取 `adapter` 公共关联检查小函数。参照 App 既有过滤行为实现，暂不删除 App 代码，也不顺带制定所有协议的清理策略。 |
| 3 | `refactor(llm): pass structured messages through client entry points` | 根包 stream / generate / calltool / token_counter 及测试。统一使用结构化输入，保持协议路由和工具事件用途；token 统计维持估算语义，不把签名当普通文本计算，不迁移上层调用。 |
| 4 | `refactor(session): map stored metadata to structured LLM messages` | `session` 新映射文件及必要的 types 调整和测试。实现既有公共 metadata 与新增可选状态的双向映射、原生 payload 保真及时间前缀规则；保留其他 metadata。旧 ToAPIFormat 暂不移除，供未接线调用方使用。 |

每个单元先完成局部验证并等待 review；用户明确要求时才暂存或提交。本阶段已授权必要时使用 `git commit --no-verify`，这不等于自动提交授权，也不代表验证通过。

## 验证与完成条件

- 公共消息及 metadata 序列化往返不丢参数、来源和 payload，Clone 不共享可变数据；签名内容、空块和块顺序不被映射层重建。
- OpenAI 文本、工具、错误、历史清理、system 合并和 hooks 经过实际出站请求回归，Stream / Generate 一致且不修改原输入；重复和冲突项分别覆盖。
- session 使用现存工具 metadata 和新增状态样例验证往返、未知 metadata 保留、损坏公共字段报错、无状态消息及时间前缀稳定性；不要求旧消息具备新增来源和原生状态。
- 从仓库根目录执行可独立运行的受影响包测试，例如 `go test ./internal/llm/...`、`go test ./internal/session`；按改动运行相应 build / race 检查。失败时记录是本单元问题还是未接线依赖，不为消除后者修改上层。
- 本阶段交付结构化输入、OpenAI 出站及 DB 映射闭环。上层接线、实际状态持久化、跨协议切换与最终迁移另行 review，不宣称整条产品流程已可用。
