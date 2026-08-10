# 阶段 5：其他模块收尾 + useApp 废弃

> 前置条件：[阶段 4](./04-entities-batch.md) 完成（7 实体领域四件套化 + refreshNonce 删除）。
> 完成后：全项目统一 query/store/mutation 模式，useApp 删除。
> 顺序纪律（重要）：先改 EventsOn 用 invalidateQueries → 再删 loadXxx → 最后删 useApp。不能跳序，否则重现丢事件 bug（详见设计文档 useApp 章节）。
>
> **文件位置（领域聚合）**：query/store/mutation 放对应 `components/{domain}/`（chat 的放 `components/chat/`、content 的放 `components/content/` 或业务归属目录等）。useApp.ts 删除前留 `hooks/`。
>
> **本阶段文档讲「要求」不讲「怎么做」**：每个领域列出改造要求清单 + commit 拆分次序，具体行号/实现细节由执行时核对当前代码决定（步骤文档是参考而非权威，动手前务必核对当前代码）。
>
> **04 重构经验对齐（每个领域都遵守）**：
> - query 错误走全局中间件 [queryErrorToast.ts](../../frontend/src/lib/queryErrorToast.ts)，组件不再挂 useEffect 监听 query.error
> - mutation 错误由调用方 try/catch + toastError，mutation 不挂 onError
> - mutation onSuccess 失效对应 queryKey
> - update mutation payload 全量回传 input 所有字段（§6）
> - store 只放跨组件 UI 状态，组件内自用状态留 useState（规则 10）
> - queryKey 从 [queryKeys.ts](../../frontend/src/lib/queryKeys.ts) 集中常量引入，禁止裸写
> - queryFn 直接 import wailsjs 函数，不经 useApp
> - 三分支渲染：isLoading / isError / data；isError 内连错误显示（对齐 ReaderList/PreferenceList）
> - i18n key 命名：query 失败 `<domain>.<noun>LoadFailed`，mutation 失败 `<domain>.<verb>Failed`
> - **重复 toast 检查**：迁移前 grep 领域内 toastError，GET 错误只 inline（无 toastError）→ 中间件接管不重复；mutation/校验保留组件级 toastError
> - **错误降级检查**：每步迁移后自检 toast 是否仍弹、UI 是否仍显示、错误消息是否仍具体
> - **delete 双删除统一**：参考 character/location，List + ListView 两处删除各自实现 → 统一为 store 共享 deletingXxxId + 集中 ConfirmDialog + mutation（侧栏只 dispatch 不执行）

## 进度勾选

### 5.1 chat（4 commit）
- [ ] commit 1: GET 端点 query 化（GetModels/GetSettings/GetSessions/GetSession/GetSessionMessages/ListSlashCommands）
- [ ] commit 2: useChatStore + 结构化 selectedModel（废弃拼接 key）
- [ ] commit 3: useDeleteSession mutation + 删除合并（store + 集中 ConfirmDialog）
- [ ] commit 4: 剩余 mutation 迁移（CompressContext/SetLastSession/SetSelectedModel/SetReasoningEffort/SetApprovalMode/CancelChat）

### 5.2 content（3 commit）
- [ ] commit 1: useChapters + useChapter query
- [ ] commit 2: useSaveContent + useCreateChapter + useUpdateChapterTitle mutation
- [ ] commit 3: file:changed 改 qc.invalidateQueries + 删 useApp + max-chapter 失效收尾

### 5.3 pattern / style / extract（3 commit）
- [x] commit 1: useStyleSamples + useStyleSample query
- [x] commit 2: useCreate/Update/DeleteStyleSample mutation
- [x] commit 3: 收尾（确认 useApp 残留仅剩流式/命令/非本领域调用）

### 5.4 skill / git（4 commit + 1 可选）
- [ ] commit 1: wailsResult util + useSkills query
- [ ] commit 2: useDeleteSkill mutation
- [ ] commit 3: SkillMarketplace query + apperr 适配（useRemoteSkills/useRemoteSkillContent）
- [ ] commit 4: useInstallRemoteSkill mutation
- [ ] commit 5（可选）: GitHistoryList 走 query

### 5.5 search（2 commit）
- [ ] commit 1: SearchAll query 化 + 错误处理对齐
- [ ] commit 2: 搜索 state 下沉/外置 + 删透传

### 5.6 删除 useApp.ts
- [ ] 全部 EventsOn 订阅改 qc.invalidateQueries 依赖后，删 useApp.ts

---

## 5.1 chat

**特殊点**：
1. **流式数据不走 query 缓存**：`Chat` API + `chat:started`/`agent:${turn_id}` 事件订阅 + `turns`/`sessionId`/`isLoading`/事件队列 refs 全部保持本地 state（5.1 与实体领域最大不同）。
2. **GetModels 拼接 key 问题（必做）**：现状选中模型用拼接字符串 `Key`（`ProviderName + "/" + ModelID`），`handleSend`/`handleCompress` 用 `splitModelKey` 拆回 `provider_name` + `model_id`，**服务商名含 `/` 会出错**。要求：后端 `AvailableModel` 补独立 `ModelID` 字段，前端用全局 store 持结构化 `selectedModel = { providerName, modelId, key, ... }`，chat 领域内废弃 `splitModelKey` 调用（util 函数保留给其他领域后续清理）。
3. **delete 双删除统一（必做）**：现状 `useDeleteSession` 是手写 promise hook + `onDeleted` 回调，SessionHistory + RecentSessions **两处各自挂 ConfirmDialog**。要求统一为 store 共享 `deletingSessionId` + ChatPanel 集中唯一 ConfirmDialog + mutation（参考 character/location）。
4. **sessionKeys.detail 类型错误**：当前 `(id: number)`，但 `session_id` 是 `string`，迁移时必须修正为 `(id: string)`。
5. **跨目录联动**：`SettingsDialog`/`ModelConfigTab`（settings 目录）的 SaveLLMConfig 保存后需刷新 models query，可通过 mutation onSuccess invalidate 实现，不再依赖 ChatPanel onSaved 回调。

**改动文件**：
- `frontend/src/components/chat/ChatPanel.tsx`（1533 行，主改造对象，仅迁数据层不拆组件，拆组件留阶段 6）
- `frontend/src/components/chat/SessionHistory.tsx`、`RecentSessions.tsx`、`ChatControls.tsx`
- 新增 `frontend/src/components/chat/useModels.ts` / `useSettings.ts` / `useSessions.ts` / `useSession.ts` / `useSessionMessages.ts` / `useSlashCommands.ts` / `useChatStore.ts` / `useDeleteSession.ts`（从 hooks/ 迁入）/ `useCompressContext.ts` / `useSetLastSession.ts` / `useSetSelectedModel.ts` / `useSetReasoningEffort.ts` / `useSetApprovalMode.ts` / `useCancelChat.ts`
- `frontend/src/lib/queryKeys.ts`（扩展 sessionKeys + 新增 modelKeys/settingsKeys/slashCommandKeys/sessionMessagesKeys）
- `frontend/src/lib/queryErrorToast.ts`（QUERY_ERROR_I18N 补 chat 前缀）
- `frontend/src/i18n/locales/zh-CN.json` + `en.json`（新增 `chat.*LoadFailed` key）
- `frontend/src/components/settings/ModelConfigTab.tsx`（SaveLLMConfig 后 invalidate models query，替代 onSaved 回调链路）

**改造要求清单**：

### A. useApp 调用迁移分类

| 类别 | API | 处理 |
|---|---|---|
| 迁 query（GET） | GetModels / GetSettings / GetSessions / GetSession / GetSessionMessages / ListSlashCommands | 6 个 GET 端点全部 query 化，queryFn 直接 import wailsjs |
| 迁 mutation | DeleteSession / CompressContext / SetLastSession / SetSelectedModel / SetReasoningEffort / SetApprovalMode / CancelChat | 7 个 mutation 端点全部 useMutation 化，onSuccess 失效对应 queryKey |
| 保持本地 | Chat（流式）+ turns/sessionId/isLoading/isCompressing/事件队列 refs/滚动 refs/拖拽 refs | 流式核心不改 |

### B. useChatStore 字段（按需，规则 10）

- `deletingSessionId: string | null` + setter — 删除合并用（SessionHistory + RecentSessions + ChatPanel 三处协调）
- `selectedModel: { providerName, modelId, key, reasoningLevels } | null` + setter — **结构化选中模型**，废弃拼接 key；ChatControls/handleSend/handleCompress 共享
- `reasoningEffort` + `approvalMode` + setter — 与 selectedModel 强相关（选模型时重置 effort），一起进 store 更内聚，ChatControls 直接订阅减少 props drilling

**不进 store**：`activeSessionId`（ChatPanel 内协调即可）、`turns`/`sessionId`/`isLoading`/`isCompressing`（流式本地）、`showSettings`/`showHistoryPanel`（纯 UI 开关）、拖拽/滚动 refs。

### C. 事件订阅 → qc.invalidateQueries

- Chat 完成后手动 GetSessions 刷新 → `qc.invalidateQueries(sessionKeys.list(...))`
- handleNewChat 手动 GetSessions 刷新 → `qc.invalidateQueries(...)`
- handleSessionDeleted 手动 setSessions 过滤 → 删除 mutation onSuccess invalidate，自动 refetch
- SettingsDialog onSaved 回调 GetModels → ModelConfigTab SaveLLMConfig mutation onSuccess invalidate models query
- refreshModels（PopSelect 打开）→ `qc.invalidateQueries(modelKeys.all)` 或 query 自动 refetch
- **流式事件订阅（chat:started / agent:turn_id）保留 EventsOn，不改 invalidate**

### D. GetModels 拼接 key 处理（必做）

- 后端 `AvailableModel` 补独立 `ModelID` 字段（与用户确认，后端可加）
- store 持结构化 `selectedModel = { providerName, modelId, key, ... }`
- `handleSend`/`handleCompress` 从 store 取 `providerName` + `modelId`，不再调 `splitModelKey`
- `SetSelectedModel(key, effort)` mutation 调用时用 `selectedModel.key` 传入（后端 API 入参仍字符串 key，不改后端）
- `GetSettings` 恢复时 `selected_model_key` 字符串与 `GetModels` 列表按 `Key` 匹配，匹配中转结构化（直接用 `AvailableModel.ModelID` + `ProviderName`）
- `splitModelKey` 在 chat 领域内废弃使用（util 保留给 StyleView/Pattern/useImportNovel 后续清理，不在 5.1 范围）

### E. delete 统一（参考 character/location）

- `useDeleteSession` 改 mutation（`mutationFn: DeleteSession(id)`，onSuccess invalidate `sessionKeys.list` 全前缀；若删活跃会话则 invalidate `sessionMessagesKeys`）
- `useChatStore.deletingSessionId` 共享删除目标
- ConfirmDialog 集中到 ChatPanel 唯一入口，`open={deletingSessionId !== null}`，`loading={deleteMutation.isPending}`
- SessionHistory/RecentSessions 点删除只 `setDeletingSessionId(id)` dispatch，**不执行、不挂 ConfirmDialog**
- 移除 `onSessionDeleted` 父回调链路（列表刷新靠 invalidate）
- 删除当前活跃会话时清空 `activeSessionId`/`turns` 逻辑保留（mutation 调用方判断 `deletingSessionId === activeSessionId` 后处理）
- 删除失败 toastError `chat.deleteSessionFailed` 保留（调用方 try/catch + toastError，mutation 不挂 onError）

### F. 错误处理

- **GET 类**：错误 toast 由中间件接管，组件移除 `console.error` + 单独 toast；isError 内连显示错误文案 + retry 用 `refetch()`
- **mutation 类**：mutation 不挂 onError；调用方 try/catch + toastError
  - `CompressContext` 失败 → toastError `chat.compressFailed`（保留）
  - `DeleteSession` 失败 → toastError `chat.deleteSessionFailed`（保留）
  - `Chat`（send）失败 → UI 内连 interrupted（保留，不 toast）
  - `CancelChat`/`SetLastSession`/`SetSelectedModel`/`SetReasoningEffort`/`SetApprovalMode` 失败 → 当前 `.catch(()=>{})` 静默，**保留静默**（或加 console.error 便于排查）

### G. i18n / queryKey / 中间件映射

- queryKeys.ts：修正 `sessionKeys.detail(id: string)`；`sessionKeys.list` 含分页/搜索参数（区分最近会话 page=1,size=5 与历史面板分页）；新增 `modelKeys.all` / `settingsKeys.all` / `slashCommandKeys.list(novelId)` / `sessionMessagesKeys.detail(sessionId)`
- queryErrorToast.ts：补 `models`/`settings`/`sessions`/`session`/`session-messages`/`slash-commands` 前缀映射
- i18n：新增 `chat.modelsLoadFailed`/`chat.settingsLoadFailed`/`chat.sessionsLoadFailed`/`chat.sessionLoadFailed`/`chat.messagesLoadFailed`/`chat.slashCommandsLoadFailed`（现有 `chat.loadSettingsFailed`/`chat.loadMessagesFailed` 评估是否统一规范命名或保留作 inline 文案）

**验证**：build + lint + test。手测：模型列表/设置/会话列表/历史消息/slash 命令加载；GET 失败 toast + 内连错误；SessionHistory 分页/搜索；切 novelId 会话刷新；选模型/推理程度/审批模式持久化；发送/压缩用结构化 providerName/modelId；**服务商名含 `/` 的模型不再出错**；SessionHistory/RecentSessions 删除走同一 ConfirmDialog；删除当前会话清空 turns。

**风险**：高。ChatPanel 1533 行巨石 + 流式核心 + 跨目录联动（settings/ModelConfigTab）。每 commit 独立验证，commit 1（query）和 commit 3（delete 统一）风险最高。

### commit 拆分

**commit 1: GET 端点 query 化**
- `feat(chat): migrate GET endpoints to tanstack query`
- 6 个 GET 端点 query 化 + queryKeys/queryErrorToast/i18n 扩展 + ChatPanel/SessionHistory 改造（删 load 三件套，三分支渲染，isError 内连）+ 选中态恢复从 Promise.all 改 useEffect 监听 query data ready
- 风险：useSessions queryKey 含分页/搜索参数，SessionHistory 与最近会话查询参数不同缓存不共享；GetSettings 恢复选中态依赖 useModels data ready（query 间依赖）

**commit 2: useChatStore + 结构化 selectedModel**
- `feat(chat): add useChatStore with structured selected model`
- 新建 useChatStore（deletingSessionId + selectedModel 结构化 + reasoningEffort + approvalMode）+ ChatPanel/ChatControls 订阅 store + 废弃 chat 领域内 splitModelKey
- 风险：后端 AvailableModel.ModelID 字段需先补；store 化后选中态恢复 useEffect 依赖 query data 时序；ChatControls props 接口变更

**commit 3: useDeleteSession mutation + 删除合并**
- `refactor(chat): unify session delete via store and mutation`
- useDeleteSession 改 mutation（从 hooks/ 迁到 components/chat/）+ ChatPanel 集中 ConfirmDialog + SessionHistory/RecentSessions 只 dispatch + 移除 onSessionDeleted 父回调
- 风险：SessionHistory 自管分页 state，invalidate refetch 后分页状态一致性；活跃会话清空逻辑需另找触发点

**commit 4: 剩余 mutation 迁移**
- `refactor(chat): migrate set/cancel mutations and cleanup`
- CompressContext/SetLastSession/SetSelectedModel/SetReasoningEffort/SetApprovalMode/CancelChat 6 个 mutation 化 + ModelConfigTab SaveLLMConfig 后 invalidate models + ChatPanel 内 useApp 调用清零（除流式 Chat 保留直接调）
- 风险：ModelConfigTab 跨目录改动；Chat 流式仍直接调 `app.Chat`，useApp 不能完全移除（可改直接 import wailsjs Chat 函数）

---

## 5.2 content

**特殊点**：
1. **ChapterList 归属**：ChapterList 物理在 `sidebar/` 但业务属 content 领域（章节增删失效 max-chapter 必做就是指这里）。query/store 文件按领域聚合建议放 `sidebar/`（按物理位置）或 `content/`（按业务），执行时与用户商讨决定。
2. **useChapter 命名问题**：`GetContent` 读的不止 chapter（还有 outline/goink.md/skill 内容），叫 `useChapter` 名不副实。可改叫 `useFileContent` + 新增 `contentKeys`，或复用 `chapterKeys.detail` 容忍命名偏差，执行时与用户商讨决定。
3. **max-chapter 失效（必做，storyarc 4.3 遗留）**：`useMaxChapterNumber` query 当前无消费方主动 invalidate，章节增删后 `windowCenter` 不更新。要求 `useCreateChapter` onSuccess 失效 `maxChapterKeys.detail(novelId)` + `file:changed` 收到 `chapters/` 路径变更时两处订阅都失效 max-chapter。
4. **ContentPanel 多 tab 编辑器**：tabs 是本地 state（`useEditorTabs`），GetContent 的角色是"按需 fetch 后塞进 tab.content"，**query 是 fetch 缓存通道而非直接驱动 UI**——fetch 回来的数据仍要回填 tab。这点与 04 模式有差异，改造要求需照顾到。
5. **无 DeleteChapter API**：App.d.ts 无 DeleteChapter 方法，章节不可删除，只有 create + update title。
6. **跨领域复用**：useSaveContent 会被 5.3 StyleView/PatternSessionView 复用，hook 位置和签名通用性需在 commit 2 定。

**改动文件**：
- `frontend/src/components/content/ContentPanel.tsx`（836 行，多 tab 编辑器）
- `frontend/src/components/sidebar/ChapterList.tsx`（343 行，章节 CRUD 入口）
- 新增 `frontend/src/components/content/useChapter.ts`（或 useFileContent）+ `useSaveContent.ts`
- 新增 `frontend/src/components/sidebar/useChapters.ts` + `useCreateChapter.ts` + `useUpdateChapterTitle.ts`（或放 content/ 按领域聚合）
- `frontend/src/lib/queryKeys.ts`（chapterKeys 已存在，确认 detail 是否够用；maxChapterKeys 已存在）
- `frontend/src/lib/queryErrorToast.ts`（启用 `chapters` + 新增 `chapter` 映射）
- `frontend/src/components/content/ContentPanel.test.tsx`（mock 适配）

**改造要求清单**：

### A. useApp 调用迁移分类

| 类别 | API | 处理 |
|---|---|---|
| 迁 query | GetContent（5 处）/ GetChapters（ChapterList） | useChapter(filePath, novelId) + useChapters(novelId)；ContentPanel 5 处 GetContent 走共享缓存，数据回填 tab 语义保留 |
| 迁 mutation | SaveContent / CreateChapter / UpdateChapterTitle | useSaveContent（onSuccess 失效 chapterKeys.detail(path)）+ useCreateChapter（onSuccess 失效 chapterKeys.list + maxChapterKeys.detail，**必做**）+ useUpdateChapterTitle（onSuccess 失效 chapterKeys.list；payload 全量回传 chapter_number + title，§6） |
| 改 invalidate | file:changed（2 处订阅） | ContentPanel L355 + ChapterList L60 handler 改 qc.invalidateQueries；path 属 chapters/ 时同时失效 maxChapterKeys |

### B. useContentStore 评估

按规则 10，**无需 useContentStore/useChapterStore**：
- ContentPanel 的 tabs 已在 `useEditorTabs`（本地），novelId 在 `useNovelStore`，编辑器协调 ref 全是局部态
- ChapterList 的 `showCreateChapter`/`expandedBlocks`/`editingId`/`editTitle` 全是局部 UI 态，章节创建是内联 input 不是 Dialog 组件
- 与 storyarc/timeline 跳过 store commit 同理（侧边栏只读无跨组件 state）

### C. 错误处理

- **query 错误**：走中间件，组件不 toast。ContentPanel tab 回填场景降级检查（query isError 时 tab 塞兜底文案，沿用 `content.loadFailedCloseTab`）
- **mutation 错误**：调用方 try/catch + toastError，沿用 `common.saveFailed` 或 `chapter.saveFailed`
- **重复 toast 检查**：ContentPanel 当前 GetContent 失败处全静默（无 toast），query 化后由中间件接管不重复；唯一 toastError 在 doSave 失败（L224），mutation 化后保留调用方 toast 不重复

### D. i18n / queryKey / 中间件映射

- queryKeys.ts：`chapterKeys.list`/`chapterKeys.detail`/`maxChapterKeys.detail` 已存在，**无需新增**（除非决定 useChapter 改名 useFileContent + 新增 contentKeys）
- queryErrorToast.ts：启用 `chapters: "chapter.loadFailed"`；新增 `chapter: "chapter.loadFailed"`（detail query）
- i18n：`chapter.loadFailed`（L701）+ `chapter.saveFailed`（L702）已存在，无需新增

**验证**：build + lint + test（ContentPanel.test.tsx 必须仍绿）。手测：打开章节/大纲/goink.md/skill 内容正常显示；新建章节 → storyarc 章节窗口中心更新（验证 max-chapter 失效）；改标题 → 列表同步；编辑保存 → tab isDirty 清除；外部改文件 → 事件触发刷新；切面板回来事件不丢。

**风险**：中。ContentPanel 多 tab + query 缓存的回填语义是难点（query refetch 后数据如何回填对应 tab 且不丢 isDirty/viewMode）；max-chapter 失效是 storyarc 4.3 遗留，验证需切到 storyarc 视图确认 windowCenter 更新；useSaveContent 被 5.3 复用，hook 签名要通用（不绑死 chapter 语义）。

### commit 拆分

**commit 1: useChapters + useChapter query**
- `feat(content): add useChapters and useChapter queries`
- 抽 useChapter/useChapters query + ContentPanel 5 处 GetContent 改读 query 缓存 + ChapterList loadChapters 三件套改 useChapters + 三分支渲染（ChapterList 加 isError 内连，对齐 4.2 commit 1.5）+ 中间件映射补 chapters/chapter + 测试 mock 适配
- 风险：ContentPanel 多 tab + query 缓存的回填语义

**commit 2: useSaveContent + useCreateChapter + useUpdateChapterTitle mutation**
- `feat(content): add useSaveContent, useCreateChapter, useUpdateChapterTitle mutations`
- 三个 mutation 化 + doSave/handleCreateChapter/commitEdit 改 mutateAsync + saving 态由 mutation.isPending 推导 + **useCreateChapter onSuccess 失效 maxChapterKeys.detail（必做）** + useUpdateChapterTitle 全量回传 chapter_number + title（§6）+ useApp 在这些 handler 里删除
- 风险：max-chapter 失效验证需切 storyarc 视图；useSaveContent 签名通用性（5.3 复用）

**commit 3: file:changed 改 qc.invalidateQueries + 删 useApp + max-chapter 收尾**
- `refactor(content): switch file:changed to qc.invalidateQueries and drop useApp`
- ContentPanel L355 + ChapterList L60 EventsOn handler 改 qc.invalidateQueries（path 属 chapters/ 时同时失效 maxChapterKeys）+ 订阅依赖从 app/loadChapters 改成 qc + 两处删 useApp import（对齐 5.6 顺序纪律：先改 EventsOn 依赖 → 再删 useApp）
- 风险：useApp 的 useMemo 防 EventsOn 重订阅 bug 不再依赖 useApp，需确认事件订阅稳定性（长时间操作切面板回来 file:changed 仍触发）

---

## 5.3 pattern / style / extract

**特殊点**：
1. **流式进度/命令操作不走 query**：`usePatternProgress` 的 `pattern:progress` 事件订阅 + `ExtractPattern`/`ExtractStyle`（流式，带 task_id，期间事件推送，async 返回结果）+ `CancelExtract`/`CancelExtractPattern`（取消命令）全部保持本地 state + try/catch + setError，不迁 query/mutation（5.3 文档明确要求 pattern 进度保持本地 state）。
2. **ListStyleSamples 是分页查询**：与 character/location 的全量 list 不同，queryKey 必须含 page/size/search 参数，`styleSampleKeys.list` 签名需扩展。
3. **SaveContent/GetChapters/GetModels/GetSettings 不属 5.3**：分别属 content（5.2）/chapter（5.2）/全局配置领域，5.3 不动，5.3 完成后 StyleView 仍残留 useApp 调用，5.6 时确认对应领域已迁移。
4. **无 apperr 适配需求**：5.3 范围内所有 Wails 方法都是旧 API（reject 字符串），不需要 `unwrapResult` 适配层（区别于 5.4 skill）。
5. **ExtractWorkspaceView 是路由容器**：无数据流，不动；PatternExtractView/PatternSessionView/usePatternProgress 不动（流式/命令操作）。

**改动文件**：
- `frontend/src/components/style/StyleView.tsx`（729 行）
- `frontend/src/components/style/StyleSampleList.tsx`（163 行，独立分页列表，评估是否共用 query）
- 新增 `frontend/src/components/style/useStyleSamples.ts` / `useStyleSample.ts` / `useCreateStyleSample.ts` / `useUpdateStyleSample.ts` / `useDeleteStyleSample.ts`
- `frontend/src/lib/queryKeys.ts`（扩展 `styleSampleKeys.list` 签名含分页参数）
- `frontend/src/lib/queryErrorToast.ts`（启用 `style-samples` + 补 `style-sample` 映射）
- `frontend/src/components/style/StyleView.test.tsx`（测试 mock 适配）

**改造要求清单**：

### A. useApp 调用迁移分类

| 类别 | API | 处理 |
|---|---|---|
| 迁 query | ListStyleSamples / GetStyleSample | useStyleSamples(novelId, page, size, search) + useStyleSample(id)；queryKey 含分页参数；enabled 守卫 |
| 迁 mutation | CreateStyleSample / UpdateStyleSample / DeleteStyleSample | 三个 mutation 化，onSuccess 失效 styleSampleKeys.list（delete/update 还失效 detail）；UpdateStyleSample 全量回传 input（§6） |
| 保持本地 | ExtractStyle / CancelExtract（StyleView）+ ExtractPattern / CancelExtractPattern（PatternSessionView）+ usePatternProgress 事件订阅 | 流式/命令操作，不迁 |
| 不属 5.3 | SaveContent（content 5.2）/ GetChapters（chapter 5.2）/ GetModels + GetSettings（全局配置领域） | 不动，5.6 时确认对应领域已迁移 |

### B. useStyleStore / useExtractStore / usePatternStore 评估

按规则 10，**全部不需要**：
- StyleView 所有 state（phase/selected/loading/deleteTarget/error/result/表单字段/detail 编辑字段）都是组件内自用，无跨组件共享
- StyleSampleList 是独立组件，无与 StyleView 共享删除目标的需求
- 不存在"侧栏 List dispatch + 主区 View 执行删除"的 character 模式
- 与 storyarc/timeline/reader 跳过 store commit 同理

### C. 错误处理

- **GET 错误**（ListStyleSamples/GetStyleSample）：走中间件，删组件级 toastError（StyleView L102/L143）
- **mutation 错误**：调用方 try/catch + toastError（StyleView L239 deleteFailed 保留）+ inline setError（L216 addFailed / L333 saveFailed 保留）
- **流式操作错误**：保留 setError（PatternSessionView L85/L119），不走中间件
- **三分支渲染**：StyleView 列表改 isLoading/isError/data 三分支
- **重复 toast 检查**：GET 错误当前有 toastError（L102/L143）→ 迁 query 后删组件级 toastError 由中间件接管；mutation 错误不走中间件保留组件级不重复

### D. i18n / queryKey / 中间件映射

- queryKeys.ts：扩展 `styleSampleKeys.list` 签名纳入分页/搜索参数；`styleSampleKeys.detail(id)` 已存在
- queryErrorToast.ts：启用 `"style-samples": "styleSample.loadFailed"`；补 `"style-sample": "styleSample.loadFailed"`（detail query）
- i18n：`styleSample.loadFailed`/`deleteFailed`/`addFailed`/`extractFailed`/`saveFailed` + `extract.*` 全部已存在，**无需新增**

**验证**：build + lint + test（StyleView.test.tsx 6 个用例必须仍绿）。手测：style sample CRUD 全流程；pattern 提取进度更新；切小说后 query 自动 refetch。

**风险**：中高。StyleView.test.tsx 当前 mock useApp + 期望组件直接调 toastError，迁 query 后 GET 错误的 toast 由中间件触发，测试需用真实 QueryClient + mock wailsjs 函数让中间件真实触发（参考 queryErrorToast.test.tsx 写法）；分页 queryKey 设计需谨慎（page/size/search 进 key，避免缓存污染）；StyleSampleList 是否共用 query 需评估（共用可共享 cache 但 PAGE_SIZE 不同 15 vs 50）。

### commit 拆分

**commit 1: useStyleSamples + useStyleSample query**
- `feat(style): add useStyleSamples and useStyleSample queries`
- 抽 useStyleSamples/useStyleSample query + StyleView 删 loadRef/load/useEffect 三件套 + 列表 + openDetail 改 query + 三分支渲染 + 删 GET toastError（中间件接管）+ styleSampleKeys.list 扩展分页参数 + 中间件映射 + 测试 mock 适配
- 风险：测试适配是最大风险（mock 模式重构）；分页 queryKey 设计

**commit 2: useCreate/Update/DeleteStyleSample mutation**
- `feat(style): add style sample CRUD mutations`
- 三个 mutation 化 + handleAdd/handleUpdate/confirmDelete 改 mutateAsync + loading/deleting/editSaving 由 mutation.isPending 推导 + 删 `await load(page)` 手动重拉改 onSuccess invalidate + UpdateStyleSample 全量回传 input（§6）+ 保留 mutation 错误组件级 try/catch + toastError/inline setError
- 风险：mutation payload 全量回传需核对 UpdateStyleSample input 字段

**commit 3: 收尾**
- `refactor(style): cleanup useApp residuals in style domain`
- 确认 StyleView 内 useApp 调用清单：保留 ExtractStyle/CancelExtract/SaveContent/GetModels/GetSettings（流式/命令/非本领域），已删 ListStyleSamples/GetStyleSample/CreateStyleSample/UpdateStyleSample/DeleteStyleSample + PatternExtractView/PatternSessionView/usePatternProgress 不动 + 注释清理 + 文档同步
- 风险：低，主要是清理和验证

---

## 5.4 skill / git

**特殊点**：
1. **apperr 新 API 适配（必做）**：`ListRemoteSkills`/`GetRemoteSkillContent`/`InstallRemoteSkill` 是 apperr 新 API（返回 `Result[T]`，HTTP 200，不 throw）。迁移 query 时 queryFn 必须用 `unwrapResult(res)` 解包（err_code 非空且非 "ok" 时 throw AppErr），否则错误静默吞掉（违反规则 8）。**5.4 是 apperr 新 API 首个落地领域，无既有模式可复用**，需先建 `frontend/src/utils/wailsResult.ts` 基建工具。方案详见 [04a-query-error-toast.md](./04a-query-error-toast.md) 的「apperr 新 API 适配」章节。
2. **5.4 文档遗漏 GitHistoryList**：5.4 文档只写 `GitCommitView.tsx`，但 GitCommitView 是纯展示组件（props `file: git.FileDiff | null`，无 useApp/无数据获取），**真正的 git 数据获取在 GitHistoryList.tsx**（已直接 import wailsjs 绕过 useApp，GetCommitLog/GetCommitFileList/GetFileDiff）。重写 5.4 文档时补 GitHistoryList，明确 GitCommitView 不迁。
3. **没有 CreateSkill/UpdateSkill Wails API**：skill 内容 CRUD 走 content 领域的 GetContent/SaveContent（文件路径方式 `skills/{name}.md`），由 ContentPanel 处理。5.4 只涉及 `DeleteSkill` 和 `InstallRemoteSkill` 两个 mutation，无 §6 全量回传问题。
4. **skillKeys.all 与 ListSkillsInput.novel_id 矛盾**：文档说 `["skills"]` 全局，但 API 入参含 `novel_id`，建议改 `["skills", novelId]` 与入参对齐，避免跨 novel 串缓存（不同 novel 的 novel 层 skill 不同）。
5. **SkillMarketplace 是大组件**（931 行，phase 状态机 + 多 query 组合 + debounce + classifyError），commit 3 风险最高。
6. **classifyError 保留**：SkillMarketplace 的 inline error bar（按 err_code 短码映射具体文案如 `errorRateLimited`）保留，中间件只负责兜底 toast。`unwrapResult` throw 的 `AppErr.errCode` 可在组件 catch 块里读，传给 classifyError。

**改动文件**：
- `frontend/src/components/skill/SkillList.tsx`、`SkillMarketplace.tsx`、`SkillList.test.tsx`
- `frontend/src/components/git/GitHistoryList.tsx`（可选，commit 5）
- 新增 `frontend/src/utils/wailsResult.ts`（unwrapResult + AppErr）
- 新增 `frontend/src/components/skill/useSkills.ts` / `useDeleteSkill.ts` / `useRemoteSkills.ts` / `useRemoteSkillContent.ts` / `useInstallRemoteSkill.ts`
- `frontend/src/lib/queryKeys.ts`（skillKeys.all → list(novelId) + 新增 remoteList/remoteContent）
- `frontend/src/lib/queryErrorToast.ts`（启用 `skills` + 补 `remote-skills`/`remote-skill-content`）
- `frontend/src/i18n/locales/zh-CN.json` + `en.json`（补 `skill.loadFailed`/`skill.marketplace.loadFailed`/`skill.marketplace.contentLoadFailed`）
- `docs/feat/v1.4.0/refactor-plan/00-conventions.md` §1.2（同步 skill key）

**改造要求清单**：

### A. useApp 调用迁移分类

| 类别 | API | 处理 |
|---|---|---|
| 迁 query（旧 API） | ListSkills（SkillList L56 + SkillMarketplace L163 已安装索引） | useSkills(novelId) query，两处共享缓存 |
| 迁 query（apperr 新 API） | ListRemoteSkills / GetRemoteSkillContent | useRemoteSkills(input) + useRemoteSkillContent(name)；queryFn 必须用 unwrapResult；enabled 守卫（useRemoteSkillContent 需 `enabled: !!name && phase === "detail"`） |
| 迁 mutation（旧 API） | DeleteSkill | useDeleteSkill(novelId) mutation，onSuccess 失效 `["skills", novelId]` |
| 迁 mutation（apperr 新 API） | InstallRemoteSkill | useInstallRemoteSkill(novelId) mutation，mutationFn 用 unwrapResult 统一错误处理；onSuccess 失效 `["skills", novelId]` + `["remote-skills", ...]` |
| 评估 | GetContent（SkillMarketplace L279 probe local） | 建议保持手 fetch 或复用 content query（5.2 建），不单建 skill probe query |
| 不迁 | SkillEditForm/SkillContributeDialog/GitCommitTooltip（纯展示）/GitCommitView（纯展示 props file） | 无数据获取 |
| 不迁（可选 commit 5） | GetCommitLog/GetCommitFileList/GetFileDiff（GitHistoryList） | 评估后决定，游标分页 + 13 state + 8 ref 巨石，风险高 |

### B. apperr 新 API 适配（必做）

- 新建 `frontend/src/utils/wailsResult.ts`：导出 `AppErr`（extends Error，带 `errCode: string`）+ `unwrapResult<T>(res: { err_code, err_msg?, data: T }): T`。`err_code` 非空且非 `"ok"` 时 throw `AppErr(err_code, err_msg ?? err_code)`，否则返回 `res.data`
- GET query 适配：`useRemoteSkills`/`useRemoteSkillContent` 的 queryFn 必须 `unwrapResult(await XxxApi(...))`
- mutation 适配：`useInstallRemoteSkill` 的 mutationFn 也用 `unwrapResult`，统一 mutation 错误走 throw（被调用方 try/catch + toastError），**不双轨**（不再在组件里读 `res.err_code`）
- classifyError 保留：inline error bar 按 err_code 短码映射具体文案保留，中间件只负责兜底 toast
- AppErr 文档：wailsResult.ts 顶部注释说明 apperr 新 API 的 HTTP 200 + err_code 模式，指向 [04a-query-error-toast.md](./04a-query-error-toast.md) 与 `internal/apperr/apperr.go`

### C. 重复 toast 检查（必做）

| 调用点 | 类型 | 现有处理 | 迁移后 | 是否重复 |
|---|---|---|---|---|
| SkillList L56 ListSkills | GET | console.error（无 toast） | 走 query 中间件接管 | 不重复（删 console.error） |
| SkillList L101 DeleteSkill | mutation | toastError（L109） | 走 mutation 调用方 try/catch + toastError | 不重复（保留） |
| SkillMarketplace L163 ListSkills（已安装索引） | GET | console.error | 走 query 中间件接管 | 不重复 |
| SkillMarketplace L183 ListRemoteSkills | GET | setError inline（无 toast） | 走 query 中间件接管 + inline 保留 classifyError | 不重复（toast 兜底 + inline 具体文案共存可接受） |
| SkillMarketplace L238 GetRemoteSkillContent | GET | setContentError inline | 走 query 中间件接管 + inline 保留 | 不重复 |
| SkillMarketplace L279 GetContent（probe） | GET（probe） | try-catch silent | 保持手 fetch或复用 content query | N/A |
| SkillMarketplace L293 InstallRemoteSkill | mutation | 读 err_code → toastError + catch → toastError | 走 mutation + unwrapResult，调用方 catch → toastError | 不重复（统一到 catch toast，删 err_code 分支） |
| SkillMarketplace L328 novelRequired | 表单校验 | toastError | 保留 | 不重复（非 API 错误） |
| GitHistoryList L92 GetCommitLog | GET | toastError（L123/L351） | 走 query 中间件接管，**删组件 toastError** | **必查**（否则双 toast） |
| GitHistoryList L213 GetFileDiff | GET | toastError（L222） | 走 query 中间件接管，**删组件 toastError** | **必查** |
| GitHistoryList L192 GetCommitFileList | GET | setExpandedError inline | 走 query 中间件接管 + inline 保留 | 不重复 |

### D. useSkillStore / useGitStore 评估

按规则 10，**全部不需要**：
- SkillList：`activeSkillName` 来自父级 props，`creating/newName/showContribute/marketplaceOpen/deleteTarget/deleting` 全是组件内 UI state
- SkillMarketplace：`phase/selectedSkill/page/...` 全是组件内
- GitHistoryList：`expandedHash/selectedFilePath/...` 全是组件内；`selectedGitFile` 由父级 WorkspaceView 持有 props 下传 GitCommitView
- 与 storyarc/timeline/reader/preference/novel-setting 跳过 store commit 同理

### E. GitHistoryList 迁 query 评估（可选 commit 5）

**支持迁**：GetCommitFileList/GetFileDiff 按 hash+filePath 拉取，缓存有价值；中间件接管 toast 删 L123/L222/L351 三处 toastError 样板；与 5.4 "git 走 query" 文档表述一致。

**反对迁**：GetCommitLog 是游标分页（afterHash），需 `useInfiniteQuery` 或手动合并多页 query；13 state + 8 ref 巨石组件改造面大风险高；git 操作低频缓存收益相对实体领域小。

**建议**：commit 1-4（skill）是 5.4 核心必做；commit 5（git）评估后决定，可推迟到 5.4 之后单独 commit。**最低要求**：skill 部分必做，git 部分评估后决定。

### F. i18n / queryKey / 中间件映射

- queryKeys.ts：`skillKeys.all` → `skillKeys.list(novelId)`（与 ListSkillsInput.novel_id 对齐）；新增 `skillKeys.remoteList(input)` + `skillKeys.remoteContent(name)`；git key（若迁）`gitCommitKeys.list(novelId)`/`commitFileKeys.list(novelId, hash)`/`fileDiffKeys.detail(novelId, hash, filePath)`
- queryErrorToast.ts：启用 `skills: "skill.loadFailed"`；补 `remote-skills: "skill.marketplace.loadFailed"` + `remote-skill-content: "skill.marketplace.contentLoadFailed"`；git（若迁）补 `git-commits`/`commit-files`/`file-diff` 映射
- i18n：补 `skill.loadFailed`/`skill.marketplace.loadFailed`/`skill.marketplace.contentLoadFailed`（zh-CN + en）；git（若迁）补 `git.commitsLoadFailed` 等
- 00-conventions.md §1.2 同步

**验证**：build + lint + test（SkillList.test.tsx 5 用例必须仍绿）。手测：skill 列表/CRUD；marketplace 列表/搜索/分页/卡片详情/安装/网络断开 toast + inline error bar；切换 novel → installed 索引刷新；git（若迁）commit 列表/展开/选文件 diff/切 commit 缓存命中。

**风险**：高。SkillMarketplace 931 行大组件 + phase 状态机 + 多 query 组合 + debounce + classifyError，commit 3 风险最高；apperr 新 API 首个落地领域无先例；GitHistoryList 巨石组件（可选）。

### commit 拆分

**commit 1: wailsResult util + useSkills query**
- `feat(skill): add useSkills query and wailsResult util`
- 新建 wailsResult.ts（unwrapResult + AppErr）+ useSkills query + SkillList 删 load 三件套 + useApp 改 useSkills + 三分支渲染 + SkillList 加 isError 内连（对齐 ReaderList/PreferenceList）+ skillKeys.all → list(novelId) + 中间件启用 skills 映射 + i18n 补 skill.loadFailed + 00-conventions.md 同步 + SkillList.test.tsx mock 适配（QueryClientProvider + mock useSkills hook，参考 CharacterList.test.tsx）
- 风险：skillKeys.all → list(novelId) 改动可能影响其他消费方（grep 确认仅 SkillList/SkillMarketplace 用，marketplace 在 commit 3 才迁，本 commit 内 marketplace 仍用 useApp.ListSkills 不冲突）

**commit 2: useDeleteSkill mutation**
- `feat(skill): add useDeleteSkill mutation`
- useDeleteSkill mutation + SkillList confirmDelete 改 mutateAsync + deleting 由 mutation.isPending 推导删 setDeleting useState + 保留 try/catch + toastError + 删 useApp import + 测试 mock 适配
- 风险：低。DeleteSkill 是单参 mutation，无 §6 全量回传问题

**commit 3: SkillMarketplace query + apperr 适配（核心 commit）**
- `feat(skill): migrate SkillMarketplace to query with apperr unwrap`
- useRemoteSkills/useRemoteSkillContent query（queryFn 用 unwrapResult）+ SkillMarketplace 删 loadRemote/loadRemoteContent/loadInstalledIndex 三件套 + useApp 改 useRemoteSkills/useRemoteSkillContent/useSkills（已安装索引复用 commit 1）+ 保留 phase 状态机 + debounce + classifyError inline 错误展示 + doInstall 仍用 useApp.InstallRemoteSkill（commit 4 才迁 mutation），但本 commit 可先用 unwrapResult 改 doInstall 的 err_code 读法统一错误处理 + queryKeys 加 remoteList/remoteContent + 中间件补 remote-skills/remote-skill-content 映射 + i18n 补 marketplace.loadFailed/contentLoadFailed + 00-conventions.md 同步
- 风险：**高**。SkillMarketplace 931 行大组件 + phase 状态机 + 多 query 组合；debounce + query 的组合需用 `keepPreviousData`/`placeholderData` 防分页闪烁；enabled 控制（useRemoteSkillContent 需 `enabled: !!selectedSkill && phase === "detail"`）；classifyError 与 unwrapResult 的协作（组件 catch 块读 `appErr.errCode` 传给 classifyError）

**commit 4: useInstallRemoteSkill mutation**
- `feat(skill): add useInstallRemoteSkill mutation`
- useInstallRemoteSkill mutation（mutationFn 用 unwrapResult）+ SkillMarketplace doInstall 改 mutateAsync + installing 由 mutation.isPending 推导删 setInstalling useState + onSuccess 失效 `["skills", novelId]` + `["remote-skills", ...]` + 保留 try/catch + toastError + 删 useApp import + 验证 SkillMarketplace 内 `grep useApp` 无残留
- 风险：中。InstallRemoteSkill 返回 `Result_struct____`（data 是空 struct），unwrapResult 后 data 无意义，确认 err_code 处理正确；onSuccess 失效范围需确认（remote 列表的 installed/updatable 标记是前端基于 installedVersions Map 算的，必须失效 `["skills", novelId]` 才能让 installedVersions 刷新）

**commit 5（可选）: GitHistoryList 走 query**
- `feat(git): migrate GitHistoryList to query`
- useCommitLog（考虑 useInfiniteQuery 或分页合并）+ useCommitFiles + useFileDiff query + GitHistoryList 删 load 三件套 + 8 个 ref 中可简化的 + 改用 query + **删 L123/L222/L351 三处 toastError（中间件接管，必查重复 toast）** + 保留 expandedError inline + GitCommitView 不动（保持 props file 模式）+ queryKeys 加 git key + 中间件补 git 映射 + i18n 补 git key
- 风险：**高**。游标分页 + useInfiniteQuery 学习成本；13 state + 8 ref 巨石组件改造面大；闭包过期 ref 与 queryKey 依赖的语义转换需谨慎。建议 commit 1-4（skill）完成且验证稳定后再启动 git 迁移；若风险过高可推迟到 5.4 之后单独 commit

---

## 5.5 search

**特殊点**：
1. **SearchAll 是后端统一 API，不是前端聚合各领域 List**：5.5 文档原表述"确认 search 调用的 GetCharacters/GetLocations 等已走 query 共享缓存"是误解。全局搜索只调 `SearchAll(novelId, query)`（后端 `searchEntities` 跨实体 + 正文 + RAG 统一入口），不调各领域 List。各领域 List 走 query 共享缓存是阶段 4 已完成的事，与 5.5 无关。5.5 的真实改造对象是 `SearchAll` 的前端调用方式（query 化）+ 错误处理对齐 + 搜索 state 透传链路优化。
2. **当前 SearchAll 未走 TanStack Query**：SearchPanel 用 `useState + useRef + setTimeout + reqIdRef` 自管，silent catch 错误（用户无法区分"搜索失败"和"无结果"，违反规则 8）。这是 5.5 的核心改造点。
3. **搜索跳转已用 useFocusStore（阶段 2.8 已建）**：`handleSearchNavigateEntity`/`handleSearchNavigateChapter` 已对齐 focusStore，useFocusStore 已支持 nonce + type（arc/node），**不需要扩展 store**。
4. **flushSync 必须保留**：`handleSearchNavigateChapter` 的 `flushSync(() => setActivePanel("chapters"))` 必须保留（README L72 + 03-novel-template.md L171 明确）。ContentPanel 条件渲染，从非 chapters 面板搜索章节时未挂载，flushSync 确保挂载后才调 `contentRef.current?.openFileWithHighlight`，删了会空指针。完整删除属阶段 6 痛点驱动，非必做。
5. **debounce + 竞态保护**：当前 300ms debounce + `reqIdRef` 手动竞态保护。迁 query 后 debounce 必须保留（UI/UX 不变，规则 7），竞态保护由 query 内置机制接管（替代 reqIdRef）。
6. **staleTime 评估**：搜索结果是动态的，倾向 `staleTime=0` 或很短（始终 refetch），因为搜索是用户主动期望"最新结果"的操作，与领域 List "30s 内切面板不重复 fetch" 场景不同。

**改动文件**：
- `frontend/src/components/search/SearchPanel.tsx`（核心：删 useState+useRef+setTimeout+reqIdRef 自管，改 useQuery）
- `frontend/src/views/WorkspaceView.tsx`（删 searchQuery/searchResults useState + 透传 props，L109-110/L448-455）
- `frontend/src/components/sidebar/SidePanel.tsx`（删搜索 props 透传，L42-56/L82-86/L131-139）
- `frontend/src/lib/queryKeys.ts`（新增 searchKeys）
- `frontend/src/lib/queryErrorToast.ts`（补 `search` 映射）
- `frontend/src/i18n/locales/zh-CN.json` + `en.json`（search 命名空间补 `loadFailed`）

**改造要求清单**：

### A. SearchAll 调用方式改造（核心）

- queryFn 直接 `import { SearchAll } from "@/lib/wailsjs/go/app/App"`（已符合，保持）
- queryKey 从 queryKeys.ts 集中常量引入（需新增 searchKeys）
- `enabled` 守卫：`!!novelId && !!query.trim()`（novelId=0 或空 query 不 fetch）
- 数据兜底：queryFn 返回 `data ?? []`
- **debounce 必须保留**（当前 300ms，迁移后行为等价，UI/UX 不变）
- **竞态保护**：query 自带竞态保护可替代当前 `reqIdRef`，需确认行为等价（快速连续输入时只显示最后一次结果）
- **staleTime 评估**：倾向 staleTime=0 或很短（始终 refetch 最新结果）
- **queryKey 是否编入 query 字符串**：倾向编入（标准做法），但需评估 cache 堆积（用户输入每次字符都生成 key），可能需配 `gcTime` 短

### B. 错误处理改造（必做，规则 8 硬性要求）

- 当前 SearchPanel L127-129 的 silent catch 必须改（用户无法区分"搜索失败"和"无结果"）
- query 化后 GET 错误走全局中间件 `queryErrorToast.ts`
- 中间件按 queryKey 前缀查 QUERY_ERROR_I18N 映射表 → toast `<label>: <err.message>` + console.error
- **必须补 queryErrorToast.ts 的 QUERY_ERROR_I18N 映射**：`search` 前缀 → `search.loadFailed`
- **必须补 i18n**：zh-CN.json + en.json 的 search 命名空间加 `loadFailed`
- **三分支渲染**：SearchPanel 结果区当前是"空 query / loading / 无结果 / 有结果"四态，必须改为"空 query / isLoading / isError / 无结果 / 有结果"五态（或 isError 内联在结果区显示）。`isError` 态需有 inline UI 提示，不能只靠 toast
- **数据加载失败时 UI 正常渲染**（规则 8）：搜索输入框、侧边栏 resize handle、ActivityBar 等周边 UI 必须正常渲染，不能整屏只显示"搜索失败"

### C. 搜索跳转 useFocusStore（已对齐，无需改）

- `handleSearchNavigateEntity`（WorkspaceView L291-298）已调 `focusEntity(panelId, entityId, type)` + `setActivePanel(panelId)`
- `handleSearchNavigateChapter`（WorkspaceView L300-318）已调 contentRef.openFileWithHighlight/openFile
- useFocusStore 已支持 nonce + type（arc/node），不需扩展
- useFocusWithNonce hook 已建，各 View 已接入（04b commit 0 落地）

### D. 是否需要 useSearchStore

按规则 10 评估，搜索 state 的**唯一消费方是 SearchPanel**（SidePanel 纯透传，WorkspaceView 只是 holder）。两种方案（由用户决策）：

- **方案 A（外置 store）**：建 `useSearchStore`（放 `query` + `results` + setter），消除 WorkspaceView→SidePanel→SearchPanel 的透传链路。要求：store 只放跨组件共享的搜索 state，不放 SearchPanel 内部 state（selectedIdx/loading/inputRef 等留 SearchPanel 内部）
- **方案 B（下沉 SearchPanel）**：搜索 state 下沉到 SearchPanel 内部 useState（query 化后 results 由 query data 推导，query state 留 SearchPanel）。要求：WorkspaceView 删 `searchQuery`/`searchResults` useState + 透传 props，SidePanel 删搜索 props 透传，SearchPanel 自管 query state

**倾向方案 B**（更符合规则 10），但需注意：当前 `searchQuery`/`searchResults` 在 WorkspaceView holder 可能是为了"切走搜索面板再切回时保留搜索状态"，若下沉到 SearchPanel 则切走 SearchPanel unmount 时 state 丢失——需确认是否要保留此行为（规则 7 UI/UX 不变）。若现状保留则选方案 A。

### E. flushSync 保留要求（硬性）

- `handleSearchNavigateChapter` 的 `flushSync(() => setActivePanel("chapters"))` 必须保留
- 5.5 不触碰 flushSync。完整删除需 ContentPanel 改为始终挂载或搜索跳转改声明式，属阶段 6 范畴

### F. i18n / queryKey / 中间件映射

- queryKeys.ts：新增 `searchKeys`（key 设计需明确是否把 query 字符串编入 queryKey）
- queryErrorToast.ts：补 `search: "search.loadFailed"`
- i18n：zh-CN.json + en.json 的 search 命名空间补 `loadFailed`（现有 15 个 key 保持不变，UI/UX 不变）

**验证**：build + lint + test（WorkspaceView.test.tsx 的 9 个搜索导航用例 L404-498 必须仍绿，因测试 mock SidePanel 不依赖 props 透传）。手测：搜索输入 debounce 时延不变；快速连续输入只显示最后一次；mock SearchAll 失败弹 1 次 toast + inline 错误 UI；搜索成功正常显示分组；切走搜索面板再切回搜索 state 是否保留（与现状一致）。

**风险**：中。debounce 在 query 模式下的实现方式需谨慎（不能简单删 setTimeout，否则每次按键触发 fetch）；staleTime 选择影响用户体验；queryKey 是否编入 query 字符串影响缓存堆积；isError 内联 UI 文案需与现有"无搜索结果"区分（避免用户混淆）；切走再切回的 state 保留行为可能变化（SearchPanel unmount 丢失 state），若现状保留则下沉后行为变化违反规则 7——这种情况下应选方案 A（外置 store）。

### commit 拆分

**commit 1: SearchPanel query 化 + 错误处理对齐**
- `refactor(search): migrate SearchAll to tanstack query`
- SearchAll 调用从手动 state 管理改为 useQuery + queryKey 集中常量引入（searchKeys）+ debounce 行为等价保留（300ms）+ 竞态保护由 query 内置机制接管（替代 reqIdRef）+ 错误处理从 silent catch 改为走全局中间件 + 三分支渲染补 isError 内联 UI + 中间件映射 + i18n loadFailed 补齐
- 风险：debounce 在 query 模式下的实现；staleTime 选择；queryKey 编入 query 的 cache 堆积

**commit 2: 搜索 state 外置/下沉 + 删透传（可选，取决于 D 决策）**
- `refactor(search): lift search state out of WorkspaceView`
- 消除 WorkspaceView→SidePanel→SearchPanel 的搜索 state 透传链路 + 搜索 state 下沉到 SearchPanel 内部（方案 B）或外置到 useSearchStore（方案 A）+ WorkspaceView/SidePanel 不再持有搜索 props + **UI/UX 完全不变**（切走搜索面板再切回的搜索 state 保留行为需与现状一致，若下沉到 SearchPanel 则 unmount 丢失，需评估是否可接受——规则 7）
- 风险：切走再切回的 state 保留行为可能变化（SearchPanel unmount 丢失 state），若现状保留则下沉后行为变化违反规则 7——这种情况下应选方案 A（外置 store）

---

## 5.6 删除 useApp.ts

**前置条件**：5.1-5.5 全部完成，确认无组件再调 `useApp()`，且所有 EventsOn 订阅已改成 `qc.invalidateQueries` 依赖（不再间接依赖 app 对象）。

**改动文件**：删 `frontend/src/hooks/useApp.ts`；删各组件残留的 `useApp` import + 类型 import 改从 `@/lib/wailsjs/go/models` 直接取。

**怎么做**：
- 全局搜 `useApp`，确认无引用
- 类型 re-export（useApp 末尾的 `export type { app, imp, novel, ... }`）改从 `@/lib/wailsjs/go/models` 直接 import（部分组件已这样，统一）
- 删 useApp.ts 文件
- **chat 流式 Chat 调用**：5.1 commit 4 已将 Chat 改为直接 import wailsjs 函数（不经 useApp），5.6 时确认无残留

**验证**：build + lint + test 全绿，确认无残留引用。

**风险**：中。useApp 有修 bug 留下的 useMemo（防 EventsOn 重订阅丢事件），前提是 5.1-5.5 已把 EventsOn 依赖改干净，否则会重现 bug。

**手测点**：重点验证 file:changed 事件、chat 事件不丢（长时间操作切面板回来事件仍触发）。

**commit**：`refactor(frontend): remove useApp aggregation layer`

---

## 阶段 5 完成标准

- chat / content / pattern / style / extract / skill / git / search 全部走 query 或保持本地 state（流式数据）
- EventsOn 订阅全部改成 qc.invalidateQueries 依赖
- useApp.ts 删除
- 全项目统一 query + store 模式
- 现有测试全绿
- 手测重点：事件不丢、流式数据正常、CRUD 同步、错误提示完整（toast + inline 双重，不静默不重复）

完成后进入 [06-monolith-optional.md](./06-monolith-optional.md)（可选，痛点驱动）。
