# v1.8.0 协议适配器与同会话切换 Commit 路线

> 2026-10-16 修订，配套 [llm-protocol-design.md](./llm-protocol-design.md)。顺序为公共 adapter 基建 → OpenAI Chat → Claude Messages → Gemini GenerateContent。调用按 adapterName → providerName → model，适配器持有其 providers，具体服务商模板与规则归入对应子包。同会话切换目标及允许丢弃不兼容推理状态的原则保留。

## 当前范围与工作区状态

- 当前只完成并 review 公共 adapter 基建；OpenAI 迁移不属于本次范围。公共包不导入任何具体适配器或 SDK，不在本单元调整 Client / Agent / Session / UI。
- 设计文档基线已提交（`d38f6ed1`）。本单元按用户确认移除公共 Base，提交公共基建和三份配套文档。
- 工作区此前产生的单 provider 绑定式 OpenAI 子包、Client / factory、配置引用和测试迁移草稿保持现状，不继续修改、不回滚、不暂存，也不计入本次验收。
- SSE 资料已分供应商目录提交（`e1bfb0a9`）。Anthropic SDK `v1.81.0`、Google SDK `v1.75.0` 的依赖改动保留在未暂存区。
- 用户允许按新设计先调整公共签名、后续接通上层；阶段间允许编译失败。此前旧实例方案的生产包 / race / lint 通过记录不代表本方案已验收。当前只验证公共包及文档，并分别记录冻结草稿的接口不匹配。
- 来源 / 原生状态持久化、三级选择接线、原生协议和 UI 尚未实现；真实模型 API 烟测未进行。既存 dev_test 调试程序编译失败仍单独记录。

## 实施阶段

| 阶段 | 目标 | 完成条件 |
|---|---|---|
| A0：公共基建，当前唯一范围 | 可供具体 Provider 嵌入的公共配置、含 ProviderName / Model 的 Request，以及统一的生成 / 发现 / 探测接口；公共层不提供 Base | 公共包独立验证通过，不接入具体协议或上层；providers 实际持有与快照隔离随具体实现验收 |
| A：OpenAI Chat，后续单独开始 | 适配器内部管理本协议的 providers；迁移模板、hooks、HTTP / SSE，再接通上层与共用状态 / 切换预检 | Client 按协议选择 adapter，adapter 按 providerName 选择 provider，再解析 model；现有功能及配置回归通过 |
| B：Claude Messages | 官方 SDK 及完整产品流程，providers 与客户端留在该子包 | 工具、多轮 / 重启、取消、压缩、辅助调用及 OpenAI ↔ Claude 切换通过 |
| C：Gemini GenerateContent | 官方 SDK 及完整产品流程，复用已确认的公共能力 | Gemini 全流程及三协议六个有向切换方向通过，覆盖工具历史、推理丢弃、切回 / 重启 |

A0 验收后才开始 A。随后每个协议完成完整调用链和切换验收后，再进入下一个协议。

## Commit 拆分与文件范围

计划为 1 个文档基线单元和 8 个代码单元，另有已提交的 SSE 资料。以下为 review / commit 路线，不能据此自动开始后续单元或提交代码。

| # / 阶段 | 建议 Commit subject | 文件范围与工作 | 验证 |
|---|---|---|---|
| 0 | `docs(llm): define protocol adapters and session model switching` | 已提交文档基线；当前同步修订层级、providerName 命名、providers 归属和本阶段边界。 | 文档一致性、相对链接、JSON 示例、格式 |
| 1 / A0 | `refactor(llm): define protocol-level adapter contracts` | 仅 `internal/llm/adapter/{adapter,types,provider}.go` 及配套说明；公共 Provider 不含协议 hooks / SDK，具体子包通过匿名嵌入扩展 Provider，自行持有 providers；生成、发现和探测直接放入统一接口。原 llm.Provider、模板及具体协议实现留待后续迁移。 | 公共包 build / test / vet / lint，无具体实现 / SDK 依赖；providers 持有与配置隔离行为测试留在后续具体实现 |
| 2 / A | `refactor(llm): migrate OpenAI providers into the chat adapter` | 后续调整 `adapter/openaichat` 的 providers.go、运行时配置、hooks 和协议实现；根层 `stream/factory/config/generate/calltool/test_connection/discover/lookup` 及相应调用方按三级目标接线。已有草稿到该单元再调整，不按 provider 创建顶层适配器。 | 同一协议下多 provider，协议作用域下同名 provider 隔离，未知 adapter / provider 不回退；流式 / 非流式、错误、工具、辅助入口、热重载、取消、绑定身份与现有服务商回归 |
| 3 / A | `feat(agent): persist response origin and replay state` | 公共状态 / 事件、OpenAI 响应映射、Session metadata、Agent 最终响应 / 重试 / 中断 / 压缩及前端投影；记录来源、完整状态和 epoch。 | 重启、工具组配对、取消 / partial、压缩副本、审计保留及内部字段不外发 |
| 4 / A | `feat(llm): add provider presets and model switch preflight` | 根层配置汇总及各协议子包模板 / 规则、Agent 容量与目标压缩、Session 选择 / epoch、App 接线和设置 / 模型选择；统一保存与恢复 adapterName / providerName / model。 | 内置 / 自定义复用规则，预算含输出 / system / tools，按目标压缩，预检失败不写用户消息，不复活退役推理，Go / 前端相关检查 |
| 5 / B | `feat(llm): implement the Anthropic Messages adapter` | `adapter/anthropic` 的 providers、SDK clients、Stream / Generate、请求 / 事件 / 状态转换、发现 / 探测及回放测试。 | 真 SDK 经 httptest 的 thinking / signature / redacted / 空块、并行工具、参数全组校验、截断 / 错误 / 取消、非流式完整结果及序列化 round-trip |
| 6 / B | `feat(llm): complete Claude replay and session switching` | Claude 前缀兼容与投影、公共 epoch / 压缩链路、`adapter/anthropic/providers.go`、配置汇总、DTO / UI / 生成绑定及集成测试。 | 工具闭环、主 / 子 Agent、多轮 / 重启、辅助入口、压缩成功 / 失败、OpenAI 双向切换、审计原文保留 |
| 7 / C | `feat(llm): implement the Gemini GenerateContent adapter` | `adapter/gemini` 的 providers、SDK clients、Stream / Generate、contents / parts、工具声明 / 结果、ID 对应、签名、usage / 错误、发现 / 探测及回放测试。 | 同名并行工具、无原生 ID、单独签名及空文本片段、合法 thinking 参数、非流式完整结果、模型发现分页及序列化 round-trip |
| 8 / C | `feat(llm): complete Gemini replay and cross-protocol switching` | Gemini 历史导入 / 投影、公共 epoch / 压缩链路、`adapter/gemini/providers.go`、配置汇总与 UI、生成绑定、三协议集成测试及状态文档。 | 六个有向切换方向，同协议跨 provider / model，签名缺失和历史导入，目标窗口缩小，重启 / 切回，工具不重复执行，主 / 子 Agent 隔离 |

## 顺序与验收约束

1. 当前阶段仅 A0：明确公共接口和归属，不迁移 OpenAI，不重做冻结草稿，不为保持旧签名扩大当前范围。允许阶段间编译失败，公共包本身必须通过独立验证。
2. 根层注册表按协议名取 adapter；具体 adapter 内部按 providerName 查 providers，再解析 model。名称保持 providerName，不改称 providerKey。同名 providerName 的作用域由 adapterName 确定。
3. providers.go、内置服务商模板、provider_type 匹配、hooks、运行时 provider 组装及 SDK client 属于各协议子包；根层负责持久化存取、汇总和顶层路由。
4. 依赖为 llm → 具体协议子包 → 公共 adapter；公共包不导入实现，各实现不反向导入上层或相互导入。接口输入输出均为 Goink 类型。
5. 后续单元单独开始、验证并等待 review。B 完成后 Claude 已可与 OpenAI 在同一会话切换，随后才开始 C；原生协议尚未完成时不开放模板。
6. 保存原始消息及审计，出站允许舍弃不兼容推理状态；真实签名续聊、外部轨迹导入和普通文本投影分别验证，不承诺所有中转站兼容。
7. Go 验证从仓库根目录执行，前端验证从 frontend 执行。涉及 DTO 时通过 Wails 命令生成绑定，不手工修改；既存 dev_test 失败不混入本功能修复。
8. SDK 接入前不通过 tidy 删除尚未导入的依赖。暂存、提交、推送分别遵守用户当次授权；本单元只提交公共基建与配套说明，后续实现另行 review。

## 后续独立功能

- Google Interactions、服务端 compaction / context editing、更多部署平台另行设计。
- 同会话模型切换属于本版本目标，接受不兼容推理状态损失，原正文及工具信息继续保留。
