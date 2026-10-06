# 双向 MCP 与低频工具按需调用设计

> 状态：设计草案，待 review；尚未实现。本文面向 v1.7.0 的实现讨论，不代表已开放任何对外接口。

## 背景与目标

当前 `internal/mcp_tools` 是 Goink 进程内的 Function Calling 工具注册表。`Registry.OpenAI` 把工具定义交给模型，`Registry.Execute` 校验参数并调用 Go 工具；它尚未实现 MCP 的 client、server 或传输协议。`internal/mcp_tools/DESIGN.md` 只讨论 Goink 消费外部 MCP server，且其中的磁盘 schema 读取方案与现有 `read` 工具的路径限制不兼容。

目标是同时提供以下三种能力：

1. Goink agent 通过稳定的通用入口，按需调用卷管理等内部低频工具。
2. Goink 作为 MCP client，调用用户显式启用的外部 MCP server 工具。外部工具也走 agent 的通用入口。
3. Goink 作为 MCP server，让外部 agent 通过标准 MCP 协议发现并调用经过授权的 Goink 工具；操作仍在 Goink 进程内执行。

本方案不提供任意 SQL、数据库文件访问或默认对外开放全部工具。常用内部工具可继续直接出现在 Function Calling 的 `tools` 数组中；是否迁移到通用入口按工具逐项决定。

## 总体结构

```text
Goink agent
  ├─ 常用工具：直接 Function Calling ──────────→ 本地 Registry
  ├─ search_mcp / read(mcp/...) ────────────────→ toolrouter：统一目录
  └─ run_mcp ───────────────────────────────────→ toolrouter：权限与执行路由
                                                 ├─ 内部低频工具 ─→ 本地 Registry
                                                 └─ 外部工具 ─→ mcpclient ─→ 外部 server

外部 agent ─→ MCP 协议 ─→ mcpserver ─→ 本地 Registry / 领域服务
```

工具目录、参数 schema、权限判断、执行结果转换是两条方向可以复用的基础。`run_mcp` 是 **Goink 内部 agent 看见的 Function Calling 包装工具**，不是 MCP 协议方法。对外的 MCP server 应在 `tools/list` 中列出具体工具及其 `inputSchema`，通过 `tools/call` 执行；外部客户端不需要调用 `run_mcp`。Goink 程序内部若要确定性地使用外部工具，也可以直接调用 MCP client，不经过模型或 `run_mcp`。

### 包职责与依赖

| 包 | 职责 |
| --- | --- |
| `internal/mcp_tools` | Goink 内部工具与 `Registry`，以及模型直接看到的 `search_mcp`、`run_mcp` 包装工具；现有 `read` 增加虚拟目录读取入口。 |
| `internal/toolrouter` | 按需工具的目录、搜索、稳定 ID，以及内部／外部调用分派；内部执行使用单独的隐藏工具白名单。 |
| `internal/mcpclient` | 连接用户配置的外部 server，处理 `tools/list`、`tools/call`、连接与工具列表生命周期。 |
| `internal/mcpserver` | 对外提供 MCP 协议端点，把获准的调用适配到内部 Registry 或领域服务。 |

`mcp_tools` 通过由 App 注入的小接口使用 `toolrouter` 的目录、搜索、schema 读取和调用能力，不直接 import `toolrouter`；`toolrouter` 可依赖现有 `mcp_tools.Registry` 和 `mcpclient`，由 App 负责组装，避免 Go 包循环。`Registry.Execute("run_mcp")` 只执行包装工具；`toolrouter` 查到内部工具后再次调用 `Registry.Execute(实际工具名)` 完成现有参数校验，查到外部工具则交给 `mcpclient`。常用内部工具仍直接经 Registry 执行，不经过 Router。`mcpserver` 对外只适配获准的 Goink 工具，也不经 `run_mcp` 或 `toolrouter`。外部工具描述只登记在 Router 目录和 MCP client 中，不伪装成 Registry 的本地 `Tool`。

现有 Agent 的三个 `AllowedTools` 白名单继续控制直接调用，只给需要按需调用能力的 Agent 增加 `run_mcp`（及发现入口），不把隐藏工具名加入这些白名单。内部低频工具在 `mcp_tools` 注册并设置 `ExposeToLLM()==false`，因此 `Registry.OpenAI` 不列出它们；直接调用也会被现有 Agent 白名单拦住。Router 另维护一份仅含获准内部低频工具名的非空白名单；`run_mcp` 解析到内部目标后，把这份白名单传给现有 `Registry.Execute`。这样隐藏工具可以经 Router 执行，而 `run_mcp` 等包装工具不在该名单中，无法作为内部目标递归调用。不新增 `ExecuteDirect` 或另一套 Registry 权限规则。

内部低频工具走本地调用，不为它们额外建立 MCP 网络连接。工具目录为每个工具生成一个路径安全的稳定 `tool_id`：本地工具由内部名称确定，外部工具由稳定的 `server_id` 和原始工具名确定，必要时对不适合作路径的字符编码。这个 **同一个 ID** 出现在目录、`mcp/schemas/<tool_id>.json` 和 `run_mcp(tool_id, args)` 中，不引入第二套模型需记忆的标识。`server_id` 不依赖可修改的显示名称；路由表负责从 ID 找回外部 server 和原始工具名。对外 MCP server 仍使用其公布的具体工具名。

## 内部 agent 如何发现和调用工具

### 工具目录

- 本地工具：从已有 `Registry` 读取名称、说明和 schema；按需目录只收录 Router 隐藏工具白名单中的工具。当前生产工具的 `ExposeToLLM()` 全部返回 `true`，协议接入与路由基建阶段可用测试工具验证；真实目录在后续卷工具落地后才出现。`run_mcp` 执行它们时仍走 Registry 参数校验和 Router 传入的白名单。
- 外部工具：用户启用 server 后，MCP client 按需连接，调用该 server 的 `tools/list`（处理分页），取得名称、说明和 `inputSchema`，存入进程内目录。
- 目录记录来源、稳定 `tool_id`、描述、schema 和启用状态。内部目录取 Router 的隐藏工具白名单；外部目录取已启用的 server 与逐工具许可。禁用 server 或工具后立即从可发现目录移除；执行时再次检查名单和启用状态，不能只依靠目录过滤。
- 不把外部工具 schema 全量加入模型的 Function Calling `tools` 数组，也不把全量目录拼入每轮提示词。`mcp/index.md` 只列出当前 agent 允许使用的内部低频工具和已启用外部工具；每项只有作为调用 ID 的名称和一句简短描述。完整 schema 只在选中工具时由 `read` 返回给 agent。

为让 agent 能主动发现能力，新增 `search_mcp` 和 `run_mcp`，并复用现有 `read`：

| 入口 | 用途 |
| --- | --- |
| `search_mcp(queries)` | 接受 1～5 条相近的自然语言能力意图，在当前 agent 可用的内部低频工具和已启用外部工具中搜索，返回少量候选的稳定 ID、名称、简短描述、来源和 schema 路径，不返回完整 schema。 |
| `read("mcp/index.md")` | 查看所有已启用且当前 agent 有权使用的工具 ID／名称与简短描述，不含 schema。 |
| `read("mcp/schemas/<tool_id>.json")` | 查看单个工具的完整说明与参数 schema；路径中的 ID 与 `run_mcp` 参数相同。 |
| `run_mcp(tool_id, args)` | 路由到本地 Registry 或对应外部 MCP server。 |

例如用户说“把第二卷改叫北境篇”，模型传 `queries: ["分卷重命名", "卷改名", "volume rename"]`，不把具体卷名塞进工具搜索；搜索返回候选后，模型再读取 schema 并决定参数。搜索“生成插图”得到候选 `mcp.srv42.generate_image — 根据文字描述生成插图` 时，agent 读 `mcp/schemas/mcp.srv42.generate_image.json`，最后以 `mcp.srv42.generate_image` 调用 `run_mcp`。

`search_mcp` 与 index 使用同一份目录和权限过滤。初版不在搜索工具内再次调用 LLM：本地工具额外登记能力组、同义词和动作词（现有 `Tool.Category()` 只有 `novel_management` 等大类，不能表示“分卷管理”）；外部工具先索引 `tools/list` 的名称、描述和 server 名称。各条查询与元数据归一化后，匹配中文短语、中文相邻字词组和英文单词，对工具名、能力组与动作词、描述分别加权；相同工具在多条查询中的得分取最大值，再合并去重并限制结果数，避免堆叠宽泛词抬高排名。不接受正则语法。这样可以接受自然语言意图，但仍是词项与别名检索，不能保证理解任意同义表达或跨语种描述；英文外部工具可能需要模型补充英文查询，或读取 index 浏览。只有实测召回不足时才考虑语义向量检索；现有正文向量模型偏中文，不能直接假设适合外部工具的多语言描述。目录可按 server 分组，仍保持一行一个工具；工具很多时支持 `read` 行范围翻页，必要时再增加按 server 的子索引，不把长描述或 schema 塞入根索引。

这些路径是工具目录提供的虚拟内容，不对应小说 Git 仓库或 DataDir 中的文件。`ReadTool.Execute` 在现有文件路径分支之前识别精确的 `mcp/` 路径，由 `ToolContext` 注入的小接口读取目录；`ReadArgs.Path` 的 schema 说明和 `read` 描述也要列出这个只读命名空间。`edit` 不接受它。路径中的 ID 必须由目录查表，不直接拼接未校验的外部工具名。普通文件仍走现有路径校验与 `git.ReadFile`；仅放宽 `plainPathRe` 无法读取 DataDir 下的 schema，因为 `git.ReadFile` 会把普通相对路径解析到小说仓库。

系统提示只注入当前 agent 实际可用的能力类别，例如“分卷管理”，以及“可搜索已启用的外部工具”，不注入逐个工具的元数据；能力组和别名由内部工具目录维护。提示 agent 在需要额外能力时把 1～5 条“能力 + 动作”的简短近义意图传给 `search_mcp`，选定候选后 `read` 其 schema，再调用 `run_mcp`；搜索不到时可换词重搜或 `read mcp/index.md` 浏览。虚拟 `.json` 路径默认无行号、完整返回单工具 schema，不套用普通文件默认 2000 行截断；若超过明确的结果大小上限，返回可识别的超限信息，再允许显式按行读取，不能静默返回不完整 JSON。agent 看过的 schema 会留在本轮对话历史；目录刷新后执行时仍须检查工具是否存在及获准使用。

首次搜索或读取 `mcp/index.md` 时可按并发和超时上限获取尚未缓存的已启用 server 工具列表；某个 server 不可用时，结果标明该来源暂不可用，不让它阻塞其余工具。启用或禁用 server、工具列表变化时刷新目录。Goink 不因每轮对话而预先连接全部 server；提示词也不注入每个工具的元数据。

`run_mcp` 的外层参数 schema 只约束 `tool_id` 和 JSON 对象 `args`。外层调用仍经过 Agent 现有白名单；Router 解析 ID 后，内部目标交给 `Registry.Execute`，传入仅含隐藏工具的白名单，由 Registry 继续做参数反序列化与 `validate`。外部目标检查 server 启用状态和逐工具许可，按目录中的 schema 检查可检查的字段后交给 server 做最终校验。不要为了动态外部参数修改所有现有工具的强类型校验路径。

### 外部 MCP client 生命周期

用户为每个外部 server 显式配置 stdio 命令或 Streamable HTTP 地址，并启用它。Goink 保存配置和所需凭据；凭据不出现在工具目录、模型提示词和工具返回值中。首次构建索引时按需连接尚未缓存的 server，获取工具列表；连接失败只影响该 server 的工具。对工具列表变化通知和缓存提示作刷新处理；断线重连后重新核对列表。调用时由 `tool_id` 找到连接和原始工具名，发送标准 `tools/call`，并传播取消和超时。

当前 `ToolResult` 和 agent 的 tool 消息主要是 JSON 文本。第一阶段应明确支持文本和结构化数据，并保留 MCP 内容块的类型信息；图片、音频及其他二进制结果在 agent 消息链路支持前不得静默转成空文本或宣称处理成功。要启用生图等工具，需单独完成内容块保存、展示和模型输入的设计。

外部工具描述和返回内容均视为不可信数据。Goink 负责用户是否启用该 server、agent 是否允许调用工具，以及调用记录；外部工具实际会产生哪些副作用、能否预览或回滚，取决于该 server，Goink 不作统一保证。

### 前端：外部 server 管理

在现有 `SettingsDialog` 增加“MCP”页签，外部连接与 Goink 对外服务分别放在该页签的独立区块。外部连接区列出已配置 server、传输方式、启用状态、连接状态、最近一次错误和已发现工具数；可新增、编辑、删除配置，启用或禁用 server，手动连接测试与刷新工具列表。选中一个 server 后可查看工具名和简短描述，并逐项允许或禁用供 Goink agent 使用的工具；禁用后搜索目录与执行权限应立即同步。stdio 表单提供命令、参数和必要的环境变量；Streamable HTTP 表单提供地址和认证配置。连接失败只影响对应 server，界面给出可操作的错误信息，不把原始凭据带入报错。

配置保存、连接测试、工具列表刷新与开关操作通过独立的 App API 暴露，App 只做 DTO、校验入口和错误适配，连接生命周期仍由 `mcpclient` 管理。server ID 在编辑名称或地址后保持稳定；删除配置时撤销对应工具目录项并关闭连接。配置需持久化逐工具许可；凭据单独安全存放，列表与设置查询只返回“已配置”状态或脱敏值，不能把密钥放进会被前端通用 `GetSettings` 返回的 `AppSettings`。前端调用外部工具的许可与审计信息可以展示，但不称为可预览或可回滚的本地写入审批。

## Goink 对外提供 MCP server

### 协议与进程

在 Goink 进程内使用现成的 Go MCP SDK 增加 Streamable HTTP endpoint。server **默认关闭**，用户在设置中显式开启后、应用核心模块初始化完成时才启动；关闭开关或退出应用时，先停止接收新请求，再取消或限时等待在途调用结束，最后关闭 DB。启用本地服务时只监听回环地址；远程使用须另加认证和安全连接。若某个外部客户端只能启动 stdio server，可提供轻量 CLI 桥接到运行中的 Goink endpoint；桥接程序不打开数据库，也不复制领域逻辑。Goink 没有运行时，这个内嵌 server 不可用。

server 的 `tools/list` 按对外白名单公布 **具体工具** 及其 schema；`tools/call` 用同一份策略再次校验。`ExposeToLLM()` 只表示是否向 Goink 内部模型展示，不自动表示可对外开放。应单独维护对外工具清单，初期不暴露 `run_subagent`、原始网络工具、任意文件路径操作等能力。

对外小说工具默认操作 **Goink 界面当前打开的小说**，不要求外部 agent 在每个工具参数中传 `novel_id`。外部请求与内部 agent 一样，经同一 Registry 执行：MCP server 适配层在每次 `tools/call` 开始时读取并固定当前小说 ID，校验小说仍存在且客户端获准调用，再注入 `ToolContext.NovelID`，并传显式工具白名单。当前小说为空时拒绝调用。后端需维护与界面切书动作同步的实时当前小说状态；`settings.LastNovelID` 只是持久化的上次选择，不应在界面尚未打开该小说时直接当作实时状态。提供一个无参数的只读 `get_current_novel`，返回当前小说的 ID 和书名，供外部 agent 确认上下文；不需要 `list_novels` 和逐工具 `novel_id` 参数。全局工具与小说工具分别定义作用范围。未知工具、非法参数、无权访问和业务失败要映射为清楚的 MCP 结果；系统错误记录完整日志，返回受限摘要。

现有内部聊天并未取消传 `novel_id`：前端 `ChatPanel` 从 `WorkspaceView` 获得当前 ID，调用 `Chat` 时放进 `ChatInput.novel_id`；后端把它传给 `agent.RunOptions.NovelID`，最终注入工具的 `ToolContext.NovelID`。切书时，前端 `switchNovel` 也会调用后端 `SetActiveNovel` 更新 `settings.LastNovelID`；启动时前端从该设置恢复上次选择。MCP 调用没有 `ChatInput`，因此适配层需要从后端已确认的选中状态获取 ID，并做并发安全的读取；可以复用 `SetActiveNovel` 更新的状态，但不能仅凭一次历史设置值推断界面已经完成打开。当前 `switchNovel` 先更新前端状态再等待后端调用，实施时需处理这段短暂不同步及调用失败，避免外部请求操作到界面已切离的小说。

当前小说可能在外部 agent 连续调用工具之间改变，因此每次调用的结果或审批界面应明确目标小说；一次调用开始后始终使用快照中的 ID，不因等待审批或界面切书而改投另一部小说。写操作在实际执行前再次检查当前小说仍是快照中的小说且权限仍有效；若界面已切书，则拒绝该次写入，让外部 agent 重新确认上下文。若以后支持无人值守的连续写操作，应增加固定小说范围或预期小说 ID 校验，避免切书后继续写入另一部小说。

### 数据库和跨介质一致性

Goink 现有 `storage.Open` 设置 `SetMaxOpenConns(1)`，整个 GORM 连接池同时最多只有 **一条** SQLite 连接，并启用 WAL。进程内 MCP server 复用同一 `*gorm.DB`、Store、领域服务以及现有 singleton，不创建第二套持有数据库的 Goink 实例。外部请求占住这条连接时，界面和其他 agent 的 DB 操作都会等待。**任何需要等待用户审批、网络、外部 MCP 响应或其他长时间 I/O 的操作，都不得在持有 DB 事务期间执行**；审批应先完成，再开启短事务，并在执行前重新校验可能变化的数据。外部调用需要并发上限、超时和取消，数据库事务需要保持短暂。当前文件编辑审批在事务开始前等待，但新建章节的事务仍包含本地文件写入，实施时需关注其耗时。

连接数可以在并发实测后小幅提高：WAL 能让不同连接上的读与写并行，但同时仍只有一个写入者；更多连接也不会自动为 UI 保留专用连接。若调整 `SetMaxOpenConns`，必须同时验证 UI 延迟、写竞争与 `SQLITE_BUSY`、每条连接的锁等待配置、事务内始终传 `tx` 的约定和关闭时序。增加连接数不能替代上述短事务约束。

工具不能绕过涉及 Git 文件、向量索引或缓存的业务编排。例子：当前 `app/volume.go` 的删除卷同时修改 DB 并清理卷纲文件。若卷删除也要供工具调用，应先把这段用例迁到 `internal/volume` 的 Service，由 App 和工具共同调用；`mcp_tools` 不反向依赖 `app/`。只涉及单一持久化介质的建卷、重命名、排序仍可复用 Store。

### Goink 本地工具的权限与审批

- 所有对外请求都传显式工具白名单；当前 Registry 的 `allowed=nil` 表示不限制，不能用于 server 入口。
- 工具目录可按授权范围展示，但 `tools/call` 必须再次检查工具、小说范围和操作类型。
- 对外暴露 Goink 自身写工具前经过独立的权限门。文件编辑可在 Goink 内生成预览并交给用户确认；批准后重新检查文件和 DB 状态，再执行写入。其他写操作按自身可预览的内容设计确认方式，不把“审批通过”理解为后续一定可以回滚。
- 当前文件编辑在 `Approver=nil` 时会跳过审批。MCP server 适配层对需要交互确认的工具必须提供有效审批通道；没有通道时直接拒绝，不能把 `nil` 当作拒绝。远程无人值守写入如需支持，应另行设计有期限、限定客户端与小说的授权。
- 本地 HTTP endpoint 校验 `Origin`，默认仅监听 localhost，并验证调用方身份。远程访问另需安全连接、认证、撤销能力和可追溯的调用记录；不能仅把监听地址改为 `0.0.0.0`。

Goink 作为 MCP client 调用外部工具时，只负责启用状态、工具调用许可和审计。可在调用前询问用户是否允许将指定参数交给外部 server，但这属于**调用许可**，不承诺外部副作用的 diff 预览或回滚，也不复用 Goink 本地文件编辑审批的语义。

### 前端：Goink 对外服务管理

同一 MCP 页签的对外服务区提供明确的总开关，默认关闭；显示本地监听地址、运行状态和连接说明，可复制连接地址。启用时创建本机客户端授权凭据，提供一次性复制和后续轮换／撤销入口，普通设置查询只返回凭据状态。用户可查看并选择对外开放的具体 Goink 工具；列表默认只读，写工具须在具备对应确认通道后单独启用，并明确显示目标小说由 Goink 当前打开的小说决定。关闭开关应立即停止接受新请求，界面在后端确认停止后更新状态。认证令牌等敏感信息不得随普通设置查询回显。

远程访问在后续独立提交中扩展此页：增加远程启用状态、客户端授权范围、凭据撤销和调用记录。远程开关不能仅改变监听地址；后端未具备认证和安全连接能力时，前端不得提供看似可用的远程开启入口。页面沿用现有设置页的中英文文案和查询／变更模式；Wails 绑定通过生成命令更新，不手改生成文件。

## 卷管理工具与旧设计边界

`docs/feat/v1.6.0/volume-chapter-management.md` 曾把卷和章节结构操作限定为前端入口。v1.7.0 将卷管理纳入 agent 的内部低频工具，更新这一产品边界。先完成 MCP client/server 协议与 `run_mcp` 路由，再实现卷查询与建卷工具，作为目录和 `run_mcp` 的首批真实目标；随后完成重命名、排序，并在领域服务抽出及失败语义明确后处理删除卷。章节插入、移动、删除继续按 v1.6.0 的前端专属规则执行，是否开放另行讨论。

新增卷工具要和前端使用相同的 `volume.Store` / Service 规则，不允许 agent 直接改 `sort_order` 或拼接 SQL。MCP server 对外公布卷工具时，应在每次调用中验证从当前界面状态取得的小说 ID，并为会改变阅读顺序的操作提供足够明确的结果。

## 与旧 MCP Client 设计稿的关系

本方案沿用“稳定包装工具 + schema 按需给模型看”的目标，但修正以下实现方式：

1. Goink 内部包装工具名称统一为 `run_mcp`，本地与外部工具用不同来源的 `tool_id` 路由。对外 MCP server 公布具体工具，不把 `run_mcp` 当成唯一协议工具。
2. schema 由工具目录在内存中管理，通过现有 `read` 的 `mcp/` 虚拟只读路径返回；根索引只列启用工具的 ID／名称和简短描述。schema 路径与 `run_mcp` 使用同一 `tool_id`，不写入 `{DataDir}/schemas/...`。仅扩展路径正则不足以读取 DataDir，必须在进入小说文件读取分支前路由该虚拟路径。
3. 使用标准 MCP client/server SDK 处理协议版本、stdio 和 Streamable HTTP；旧稿中的 HTTP/SSE 与手写 JSON-RPC 细节不作为实现依据。
4. 动态外部工具参数在路由层处理，不把动态 schema 校验嵌入 `Registry.Execute` 的所有本地工具路径。

协议参考：[MCP 工具规范](https://modelcontextprotocol.io/specification/2026-07-28/server/tools)、[Streamable HTTP](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http)、[官方 Go SDK](https://go.sdk.modelcontextprotocol.io/protocol/)。具体 SDK 版本和与旧客户端的协议兼容范围在实施阶段锁定并验证。

## 实施范围、顺序与提交路线

以下工作均纳入 v1.7.0 目标。本文若单独提交，可使用 `docs(mcp): define bidirectional tool architecture`，它只是设计文档提交，不算第一个代码提交。代码先接通双向 MCP 协议，再接 `run_mcp` 和发现入口，最后增加内部低频工具及管理页面。每行是一份可单独 review 的建议 commit；subject 只是路线，真正提交时仍需写清变更原因的 body。文件范围是预计范围，实施前以当时的代码结构核对，不因本表自动暂存或提交：

| 顺序 | 建议 commit subject | 预计文件范围与验收点 |
| --- | --- | --- |
| 1.1 | `feat(mcp): connect to external servers with MCP client` | `internal/mcpclient/`；使用 MCP SDK 接通 stdio 和 Streamable HTTP，接受调用方传入的连接配置，实现 `tools/list`（含分页）与 `tools/call`、取消和超时。用独立测试 server 验证两种传输与结果类型；暂不接 agent 和设置页。 |
| 1.2 | `feat(mcp): serve Goink tools over MCP` | `internal/mcpserver/`、App 生命周期与当前小说状态；使用 MCP SDK 提供默认关闭的回环地址端点，显式启用后以本机凭据和对外白名单提供 `tools/list`／`tools/call`。先接 `get_current_novel` 与获准的只读 Registry 工具；调用时注入当前小说 ID，关闭时先停请求再关 DB。用独立 MCP client 验证真实协议调用。 |
| 1.3 | `feat(mcp): persist server connections and permissions` | 配置／凭据存储、`internal/mcpclient/`、`internal/mcpserver/`、`app/` API；保存外部 server 的稳定 ID、传输配置、启用状态及逐工具许可，以及 Goink server 的开关、凭据和对外白名单；提供连接测试、刷新、启停与凭据轮换接口，普通设置查询不回显密钥。 |
| 2.1 | `feat(tools): route on-demand calls through run_mcp` | `internal/toolrouter/`、`internal/mcp_tools/`、`internal/agentcfg/`、App 组装；只把 `run_mcp` 加进需要它的 Agent 直接白名单。Router 为内部目标向现有 `Registry.Execute` 传非空隐藏工具白名单，为外部目标调用 `mcpclient.tools/call` 并检查启用状态与逐工具许可；稳定 `tool_id` 在两类来源间唯一。用测试隐藏工具和真实协议测试 server 验证两条调用链，不新增 `ExecuteDirect`。 |
| 2.2 | `feat(tools): read on-demand tool schemas` | `internal/toolrouter/`、`internal/mcp_tools/`；从 Router 目录生成轻量 `mcp/index.md` 和单工具 schema，`read` 识别虚拟只读路径；目录、schema 与 `run_mcp` 使用同一个 ID，禁用后不可读取或调用。 |
| 2.3 | `feat(tools): search on-demand tools` | `internal/toolrouter/`、`internal/mcp_tools/`、`internal/agentcfg/`；加入 `search_mcp(queries)`，按 1～5 条近义意图搜索当前可用工具并合并去重；提示词只注入实际可用的能力类别。 |
| 3.1 | `feat(volume): add on-demand volume query and create tools` | `internal/volume/`、`internal/mcp_tools/`、Router 隐藏工具白名单；注册 `ExposeToLLM()==false` 的首批真实工具。它们不进入现有 Agent 直接白名单，可被搜索、读取 schema 并由 `run_mcp` 调用。 |
| 3.2 | `feat(volume): add volume rename and reorder tools` | `internal/volume/`、`internal/mcp_tools/`、Router 隐藏工具白名单；卷改名与排序沿用现有 Store 一致性规则，并能经目录发现与调用。 |
| 3.3 | `refactor(volume): move delete coordination into a service` | `internal/volume/`、`app/volume.go`；删除卷的 DB/Git 编排由 App 下沉为共享领域服务，明确失败补偿。 |
| 3.4 | `feat(volume): add approved volume deletion tool` | `internal/mcp_tools/`、Router 隐藏工具白名单；Goink agent 可按需发现并在本地确认后删除卷，拒绝与取消不产生副作用。 |
| 4.1 | `feat(settings): manage outbound MCP servers` | `frontend/src/components/settings/`、查询 hooks、中英文文案及必要的 App API；新增 MCP 页签，管理外部连接、逐工具开关、状态、测试与刷新。 |
| 4.2 | `feat(settings): configure the Goink MCP server` | 设置页的对外服务区、App 配置 API、中英文文案；管理本地服务开关、状态、地址、凭据轮换和只读工具白名单，关闭后后端停止接收请求。 |
| 5.1 | `feat(mcp): require approval for exposed write tools` | `internal/mcpserver/`、审批适配与必要的前端确认 UI；对外写工具逐项开放，审批明确目标小说，缺少通道时拒绝。 |
| 5.2 | `feat(mcp): authenticate and audit remote clients` | `internal/mcpserver/`、授权配置与记录；远程连接具备安全连接、客户端认证、工具／小说范围、撤销和调用记录。 |
| 5.3 | `feat(settings): manage remote MCP access` | MCP 设置页远程区、中英文文案及必要的 App API；用户可查看客户端授权、撤销凭据并查看调用记录；后端能力未就绪时不提供远程开关。 |

阶段 1 先把 client 与 server 的协议互通做成可测试的能力；1.3 再补持久配置和后端管理接口。阶段 2 只增加 `run_mcp`／发现入口及其所需目录，内部目标由 Router 向现有 `Registry.Execute` 传隐藏工具白名单，外部目标由 MCP client 调用。阶段 3 才注册正式的内部低频卷工具；阶段 4 接前端管理页面；阶段 5 逐步对外开放写操作和远程访问。涉及 App API 的提交需通过 Wails 生成命令更新绑定，不手改生成文件。每个代码阶段完成后报告变更和验证，等待用户 review；只有用户另行明确授权才执行 Git 写操作。

## 验证重点

- 本地低频工具：提示能力组与实际可用工具一致；“分卷”“卷改名”“分卷重命名”等表达能找到对应卷工具，含具体卷名的请求不会把数据误当工具名；搜索无结果时可通过 index 浏览；搜索、目录与执行白名单一致；禁用工具后旧 `tool_id` 不能继续调用；参数错误不会绕过 Registry 校验。
- `read` 虚拟路径：索引只列 ID／名称和简短描述；单工具 schema 默认完整、无行号；索引可按行翻页，超大 schema 明确提示；非法 ID、路径穿越和未授权工具被拒绝；普通小说文件仍按原路径读取，`edit` 不能修改虚拟内容。
- 外部 MCP client：stdio 与 Streamable HTTP 各有一次真实协议互通测试；覆盖 `tools/list` 分页或刷新、同名工具、断线、取消和超时。
- Goink MCP server：用独立 MCP client 验证工具列表、schema 和调用结果；覆盖未授权工具、当前小说为空、界面切书时多次调用的目标变化、单次调用等待审批期间切书会拒绝写入而不会改投新小说、非法参数与缺失审批通道。
- 前端 MCP 管理：外部 server 的新增、修改、删除、启停、连接测试和工具许可与后端状态一致；凭据不随列表回显；Goink server 默认关闭，启停、授权凭据轮换和白名单变化能反映真实监听与权限状态；远程配置未就绪时无可用开关。
- 并发与生命周期：模拟审批与外部 MCP 慢调用时，不能持有 DB 事务阻塞界面查询；界面和 MCP 同时读写时保持排序及文件一致性；未启用时不监听，关停后不再受理请求，数据库在调用结束后关闭。若调整连接数，补充多连接锁竞争和 UI 延迟验证。
- 内容类型：文本和结构化结果可用；未支持的多媒体内容明确报错，不产生误导性的成功结果。
- 执行与测试命令遵循仓库 `AGENTS.md`：Go 命令从根目录执行；前端命令从 `frontend/` 执行。

## 待确认的产品边界

1. v1.7.0 首批对外开放的具体工具清单，以及各本地写操作的确认方式。
2. 卷删除的预览、失败补偿与授权方式；章节结构操作继续维持前端专属，若要改变需单独设计。
3. 远程访问纳入 v1.7.0；无人值守写入是否允许、允许哪些客户端与小说仍需确定，但不影响本地只读 server 和内部工具路由先行实现。
