# 阶段 5：其他模块收尾 + useApp 废弃

> 前置条件：[阶段 4](./04-entities-batch.md) 完成。
> 完成后：全项目统一 query/store 模式，useApp 删除。
> 顺序纪律（重要）：先改 EventsOn 用 invalidateQueries → 再删 loadXxx → 最后删 useApp。不能跳序，否则重现丢事件 bug（详见设计文档 useApp 章节）。
>
> **文件位置（领域聚合）**：query/store/mutation 放对应 `components/{domain}/`（chat 的放 `components/chat/`、content 的放 `components/content/` 等）。useApp.ts 删除前留 `hooks/`。

## 进度勾选

- [ ] 5.1 chat sessions（messages 流式保持本地）
- [ ] 5.2 content（GetContent/SaveContent 配 file:changed）
- [ ] 5.3 pattern / style / extract
- [ ] 5.4 skill / git
- [ ] 5.5 search（跟随实体迁移）
- [ ] 5.6 删除 useApp.ts

---

## 5.1 chat sessions

**特殊点**：sessions 列表可走 query，但 **messages 流式数据不走 query 缓存**（流式数据不适合缓存），保持 ChatPanel 本地 state。

**改动文件**：`frontend/src/components/chat/ChatPanel.tsx`（1532 行，仅迁数据层不拆组件，拆组件留阶段 6）

**怎么做**：
- `useSessions(novelId)` query（key `["sessions", novelId]`）替换手 fetch。
- session messages 流式接收逻辑保持本地 state，不动。
- chat 事件订阅（`chat:started`/`tool_call` 等 L1077/L1094）**不走 query**，保持 ChatPanel 本地 state。
- session 创建/删除后 invalidateQueries(["sessions", novelId])。

**验证**：build + lint + test。手测聊天 session 切换、消息收发。

**风险**：中。ChatPanel 巨石且事件驱动，谨慎改数据层。

**手测点**：新建/删除 session 列表同步；流式消息正常接收；切 session 不丢消息。

**commit**：`feat(chat): add useSessions query for session list`

---

## 5.2 content（GetContent/SaveContent 配 file:changed）

**目标**：章节内容文件 I/O 走 query，配对 `file:changed` 事件失效。

**改动文件**：`frontend/src/components/content/ContentPanel.tsx`

**怎么做**：
- `useChapter(filePath)` query（key `["chapter", filePath]`）替换手 fetch GetContent。
- `useSaveContent` mutation，onSuccess 失效 `["chapter", path]`。
- `file:changed` 事件订阅（L353）：handler 调 `qc.invalidateQueries({ queryKey: ["chapter", path] })` + `invalidateQueries(["chapters", novelId])`（章节列表也要刷）。
- EventsOn 订阅依赖从 `loadXxx` 改成 `qc`（这是 useApp 废弃顺序纪律的第一步）。

**验证**：build + lint + test（ContentPanel.test.tsx 必须仍绿）。

**风险**：中。文件 I/O + 事件订阅，注意事件去重。

**手测点**：编辑保存章节 → ContentPanel 同步；外部改文件 → 事件触发刷新。

**commit**：`feat(content): add useChapter query and useSaveContent mutation`

---

## 5.3 pattern / style / extract

**改动文件**：`frontend/src/components/style/StyleView.tsx`、`frontend/src/components/extract/ExtractWorkspaceView.tsx`、相关 hook（usePatternProgress 等）

**怎么做**：
- style-samples 走 query（key `["style-samples", novelId]`）。
- pattern 进度（usePatternProgress L108 的事件订阅）保持本地 state（流式进度不走 query）。
- extract 的 CRUD mutation 后失效对应 key。

**验证**：build + lint + test（StyleView.test.tsx 必须仍绿）。

**手测点**：style sample CRUD；pattern 提取进度更新。

**commit**：`feat(style): migrate style-samples to query` 等

---

## 5.4 skill / git

**改动文件**：`frontend/src/components/skill/`、`frontend/src/components/git/GitCommitView.tsx`

**怎么做**：
- skills 走 query（key `["skills"]` 全局）。
- GitCommitView 的 file diff 数据按需走 query 或保持 props（git 操作低频，可评估是否值得迁）。
- skill CRUD mutation。
- **apperr 适配（必做）**：`ListRemoteSkills` / `GetRemoteSkillContent` 是 apperr 新 API（返回 `Result[T]`，HTTP 200，不 throw）。迁移 query 时 queryFn 必须先建 `frontend/src/utils/wailsResult.ts`（`unwrapResult` + `AppErr`），queryFn 用 `unwrapResult(res)` 解包（err_code 非空时 throw AppErr），否则错误静默吞掉（违反规则 8）。`InstallRemoteSkill` 是 mutation 不走 query，无需适配。方案详见 [04a-query-error-toast.md](./04a-query-error-toast.md) 的「apperr 新 API 适配」章节。
- **重复 toast 检查（必做）**：迁移前 grep `frontend/src/components/skill/` 的 `toastError` 调用。GET 错误处理只 inline（无 toastError）→ 中间件接管 toast 不重复；mutation/校验保留组件级 toastError。判断规则详见 [04a-query-error-toast.md](./04a-query-error-toast.md) 的「改造 query 后是否重复 toast」章节。

**验证**：build + lint + test（SkillList.test.tsx 必须仍绿）。

**手测点**：skill 列表/CRUD；git 文件 diff。

**commit**：`feat(skill): migrate skills to query`

---

## 5.5 search（跟随实体迁移）

**目标**：搜索导航在阶段 4 各实体迁移时已部分跟随，本步收尾。

**改动文件**：`frontend/src/components/sidebar/SidePanel.tsx` 搜索部分、`frontend/src/views/WorkspaceView.tsx` 的 handleSearchNavigate*

**怎么做**：确认 search 调用的 GetCharacters/GetLocations 等已走 query 共享缓存；搜索结果跳转用 useFocusStore（阶段 2.8 已建）。

**验证**：build + lint + test（1.5 搜索导航测试仍绿）。

**手测点**：搜索实体/章节跳转正常。

**commit**：`refactor(search): align with query and focus store`

---

## 5.6 删除 useApp.ts

**前置条件**：5.1-5.5 全部完成，确认无组件再调 `useApp()`，且所有 EventsOn 订阅已改成 `qc.invalidateQueries` 依赖（不再间接依赖 app 对象）。

**改动文件**：删 `frontend/src/hooks/useApp.ts`；删各组件残留的 `useApp` import + 类型 import 改从 `@/lib/wailsjs/go/models` 直接取。

**怎么做**：
- 全局搜 `useApp`，确认无引用。
- 类型 re-export（useApp 末尾的 `export type { app, imp, novel, ... }`）改从 `@/lib/wailsjs/go/models` 直接 import（部分组件已这样，统一）。
- 删 useApp.ts 文件。

**验证**：build + lint + test 全绿，确认无残留引用。

**风险**：中。useApp 有修 bug 留下的 useMemo（防 EventsOn 重订阅丢事件），前提是 5.1-5.5 已把 EventsOn 依赖改干净，否则会重现 bug。

**手测点**：重点验证 file:changed 事件、chat 事件不丢（长时间操作切面板回来事件仍触发）。

**commit**：`refactor(frontend): remove useApp aggregation layer`

---

## 阶段 5 完成标准

- chat sessions / content / pattern / style / extract / skill / git / search 全部走 query 或保持本地 state（流式数据）
- EventsOn 订阅全部改成 qc.invalidateQueries 依赖
- useApp.ts 删除
- 全项目统一 query + store 模式
- 现有测试全绿
- 手测重点：事件不丢、流式数据正常、CRUD 同步

完成后进入 [06-monolith-optional.md](./06-monolith-optional.md)（可选，痛点驱动）。
