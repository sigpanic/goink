# v1.8.0 协议适配器与同会话切换 Commit 路线

> 2026-10-16 修订，配套 [llm-protocol-design.md](./llm-protocol-design.md)。实现顺序已确定：先封装现有 OpenAI Chat，再接 Claude Messages，最后接 Gemini GenerateContent。同一会话允许切换，不兼容的推理状态允许在目标请求中丢弃；原始消息继续保留。以下是 review / commit 拆分，不构成代码实现或 Git 写操作授权。

## 当前状态

- 当前分支为 `feat/v1.8.0-llm-protocols`，适配器和模型切换代码均未开始。
- SSE 资料已按供应商分目录提交（`e1bfb0a9`），尚未接入自动化回放测试。
- Anthropic SDK `v1.81.0`、Google SDK `v1.75.0` 已安装并锁定，`go.mod` / `go.sum` 未提交；项目 Go 1.25.0，SDK 及生产包构建通过。整仓仍有既存 dev_test 调试程序编译问题，不能报告完整构建 / 测试通过。
- 设计、协议示例和本路线按最新目标修订；真实模型 API 烟测未进行。

## 三个实施阶段

| 阶段 | 目标 | 完成条件 |
|---|---|---|
| A：现有调用链成为第一个适配器 | `adapter/openaichat` 封装现有 HTTP / SSE / hooks，实现公共 `Stream` / `Generate` 接口；建立共用状态、投影、容量和配置入口 | 现有服务商功能回归通过，新增服务商类型与配置实例分离，统一接口可供后两个适配器复用 |
| B：Claude Messages | 通过官方 SDK 实现第二个适配器，并接通完整产品流程 | 对话、工具、重启、取消、压缩、标题、发现、连接测试可用，OpenAI Chat ↔ Claude 同会话切换通过 |
| C：Gemini GenerateContent | 通过官方 SDK 实现第三个适配器，并复用公共链路 | Gemini 全流程及三协议六个有向切换方向通过，覆盖推理丢弃、工具历史导入和切回 / 重启 |

每个协议都完成其完整调用链和切换验收后，再进入下一个协议；不把三个流式解析器全部写完后才统一补压缩、设置和模型切换。

## 阶段内的 Commit 拆分与文件范围

计划分为 1 个文档单元和 7 个代码单元，另有已提交的 SSE 资料。必要时按实际 review 范围细拆，subject / body 以最终实现为准。

| # / 阶段 | 建议 Commit subject | 文件范围与工作 | 验证 |
|---|---|---|---|
| 0 | `docs(llm): define protocol adapters and session model switching` | `docs/feat/v1.8.0/`；明确三个适配器子包、公共 Stream / Generate 契约、统一存储、同会话切换和实现顺序，继承旧稿的容量预检与回放策略。 | 文档一致性、相对链接、格式检查 |
| 1 / A | `refactor(llm): wrap existing calls in the OpenAI chat adapter` | 新增 `internal/llm/adapter` 公共契约、`adapter/openaichat` 实现；调整 `internal/llm/{types,config,stream,generate,calltool,test_connection,discover}.go` 及创建入口和测试。迁移现有请求、HTTP、SSE、错误、hooks 与辅助能力的协议部分；Client 保留配置快照及共用包装，通过类型别名兼容现有事件 / 选项。 | 导入无环、公共类型兼容；已有服务商请求 / headers / 错误 / 事件回归；Stream / Generate 实际路由、非流式完整结果；标题、摘要、CallTool、连接测试和发现经过适配器；热重载、长流取消及提前停止消费 |
| 2 / A | `feat(agent): persist response origin and replay state` | `internal/llm/adapter` 通用状态 / 事件及 OpenAI 响应映射、`internal/session/{types,store}.go`、`internal/agent/{agent,compress,interrupt}.go`、前端 metadata 投影入口及测试；在现有 metadata 保存来源、完整状态和 epoch，接通最终回复、重试清理及完整工具组收尾。 | OpenAI 来源记录、原生状态假适配器生命周期、重启 / 稳定顺序、partial 行为、取消后工具结果配对、压缩副本携带状态、审计原文保留、内部字段不外发 |
| 3 / A | `feat(llm): add provider presets and model switch preflight` | `internal/llm/` 配置与创建入口、`adapter/` 公共投影辅助与 `adapter/openaichat` 厂商规则、`internal/agent/` 容量与按目标压缩、`internal/session/` 选择 / epoch 元数据、`app/chat.go` / `chat_api.go` 输入输出接线、前端设置与模型选择及测试；分离 provider_type / 实例 key，统一 hooks 解析，接通切换边界和预检。 | 内置 / 自定义复用规则、输入预算含本次输出 / system / tools、按目标压缩及失败事务、后端并发检查、预检失败不写用户消息、不复活失效状态；前端 build / lint / test |
| 4 / B | `feat(llm): implement the Anthropic Messages adapter` | 新增 `internal/llm/adapter/anthropic` 实现、请求 / 响应 / 状态转换和原生回放测试，使用已锁定 SDK；实现 Stream / Generate、错误、usage、连接测试和发现能力，注入 HTTP client 并关闭重复重试。 | 真 SDK 经 httptest 的 content / thinking / signature / redacted / 空块、并行工具、全组参数校验、截断 / 200 内错误、取消、非流式完整结果、SDK 序列化 round-trip |
| 5 / B | `feat(llm): complete Claude replay and session switching` | `internal/llm/adapter/anthropic` 历史兼容与投影、公共 epoch / 压缩链路、`internal/llm/providers.go`、配置 DTO、设置 UI、Wails 生成绑定及集成测试；开放 Claude 类型及自定义端点，完成 OpenAI Chat 双向切换。 | 工具调用闭环、主 / 子 Agent、重启、标题 / 摘要、发现 / 连接测试、压缩成功与失败、切回不复活旧推理、旧未知来源消息、原始审计不变；Go / 前端相关检查 |
| 6 / C | `feat(llm): implement the Gemini GenerateContent adapter` | 新增 `internal/llm/adapter/gemini` 实现及原生回放测试，使用已锁定 SDK；实现 Stream / Generate、contents / parts、schema、functionCall / functionResponse、原生 / 本地 ID 对应、签名、错误 / usage 和辅助调用能力。 | 真 SDK 经 httptest 的同名并行工具、无原生 ID、单独签名片段、空文本尾片段、合法 thinking 参数、非流式完整结果、SDK 序列化 round-trip、模型发现分页 |
| 7 / C | `feat(llm): complete Gemini replay and cross-protocol switching` | `internal/llm/adapter/gemini` 外部工具轨迹导入与文本投影、公共 epoch / 压缩链路、Google 模板 / 自定义配置和 UI、Wails 生成绑定、三协议集成测试及状态文档；开放第三个适配器并完成交付验收。 | 三协议六个有向切换方向、同协议跨模型 / provider、历史工具不重复执行、签名缺失与中转不支持导入标记、较小窗口、压缩 / 重启 / 切回、主 / 子 Agent 隔离、完整辅助入口与 usage 回归 |

## 顺序与验收约束

1. A 阶段先封装已工作的实现，再增加公共状态和切换能力。依赖方向为 `llm → 具体适配器子包 → adapter`；公共契约不导入实现，子包不反向导入 llm。三个适配器实现相同 Stream / Generate 契约，GenerateText / CallTool 保留共用包装。公共业务逻辑放在 Agent / Session / llm 的已有职责内；Agent 不增加厂商分支，Session 不生成多套 wire 格式。原生协议尚未完成时不开放对应模板。
2. B 阶段完成后，Claude 已可供用户在同一会话与 OpenAI 模型切换；随后才开始 C 阶段。每个协议的管理 API、设置 UI、压缩和辅助调用都在该协议阶段完成，避免最后集中补齐。
3. 每个新 assistant 保存来源，消息表主体保持现状。出站丢弃不兼容推理状态，原始记录继续 append-only 保存；正文、调用参数和结果仍需保留。切回或重启不自动恢复已退役的推理链。
4. 切换在请求和工具组完整收尾后生效，实际发送前重新做后端预检。容量不足支持按目标预算压缩，压缩提交与随后发消息的成功 / 失败分别报告。
5. 真签名续聊、外部轨迹导入和普通上下文文本投影分别测试；模拟回放不能证明真实服务端验签成功。不承诺固定签名有效期或所有中转站都接受同一规则。
6. SDK 安装已经完成；适配器实现时复核确切版本的重试、超时、端点和依赖行为。原生 SDK 接入前不通过 tidy 删除尚未导入的依赖。
7. Go 验证从仓库根目录执行，前端验证从 frontend 执行；App DTO 变更通过 Wails 命令生成绑定。既存 dev_test 编译失败单独报告，不混入本功能修复。
8. 每个代码单元完成后报告实现、验证、限制和提交状态并等待 review；本次文档修订不包含代码实现授权，也不自动暂存、提交或推送。

## 后续独立功能

- Google Interactions、服务端 compaction / context editing、更多部署平台另行设计。
- 同会话模型切换属于本版本目标；携带摘要创建新会话可作为另一个可选功能，正常切换无需依赖它。
