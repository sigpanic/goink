# 阶段 5 补遗：profile / settings 缺口 + 5.6 删 useApp

> 前置条件：[阶段 5](./05-misc-modules.md) 5.1-5.5 全部完成（chat/content/pattern/style/extract/skill/git/search 已 query/mutation 化或保持本地 state）。
> 本文档补遗 5.x 文档遗漏的两个领域（profile / settings）+ 命令式调用收尾 + 类型 import 迁移，最终完成 5.6 删 useApp.ts。
>
> **执行参照**：每个领域走「query + store + mutation + dialogs」四件套，参照已重构模块（character/location/storyarc/preference/novel-setting 等）。文档讲「要求」不讲「怎么做」，具体行号/实现细节由执行时核对当前代码决定。
>
> **04 重构经验对齐**（每个领域都遵守，摘自 05-misc-modules.md）：
> - query 错误走全局中间件 [queryErrorToast.ts](../../frontend/src/lib/queryErrorToast.ts)，组件不再挂 useEffect 监听 query.error
> - mutation 错误由调用方 try/catch + toastError，mutation 不挂 onError
> - mutation onSuccess 失效对应 queryKey
> - store 只放跨组件 UI 状态，组件内自用状态留 useState（规则 10）
> - queryKey 从 [queryKeys.ts](../../frontend/src/lib/queryKeys.ts) 集中常量引入，禁止裸写
> - queryFn 直接 import wailsjs 函数，不经 useApp
> - 三分支渲染：isLoading / isError / data；isError 内连错误显示
> - i18n key 命名：query 失败 `<domain>.<noun>LoadFailed`，mutation 失败 `<domain>.<verb>Failed`

## 进度勾选

### 5.7 profile（2 commit）
- [ ] commit 1: useWritingActivity + useWritingStats + useProfileSettings query
- [ ] commit 2: useSaveAvatar + useSaveUserName mutation + 删 useApp

### 5.8 settings 收尾（1 commit）
- [ ] commit 1: useSaveGitConfig + useRebuildNovelIndex + useTestConnection mutation + 删 useApp

### 5.9 命令式调用收尾（1 commit）
- [ ] commit 1: WorkspaceView/App/InitView 命令式调用改直接 import wailsjs + SaveCover mutation 化

### 5.10 类型 import 迁移 + 删 useApp.ts（1 commit）
- [ ] commit 1: 类型 import 全量迁移 `@/hooks/useApp` → `@/lib/wailsjs/go/models` + 删 useApp.ts

---

## 5.7 profile

**现状调研**（ProfileView.tsx，2026-08-11 核对）：
- 完全未改造：`useApp()` + load 三件套（loading/loadFailed/activity/stats/settings useState + useEffect + load()）
- 5 个 API 全走 useApp：GetWritingActivity / GetWritingStats / GetSettings（读 user_name/avatar）/ SaveAvatar / SaveUserName
- 错误处理：load 失败 toastError + inline loadFailed；SaveAvatar/SaveUserName 失败 inline error（无 toast）
- 无跨组件 state（ProfileView 是独立视图，无侧栏 List + 主区 View 拆分）

**特殊点**：
1. **GetSettings 与 chat 共享 API 但消费不同字段**：profile 读 `user_name` / `avatar`，chat 读 `selected_model_key` / `reasoning_effort` / `approval_mode`。query 化后两处共享同一 queryKey 缓存（settingsKeys.all），互不干扰（各取各字段）。
2. **SaveAvatar/SaveUserName 成功后需刷新 settings query**：mutation onSuccess invalidate settingsKeys.all，让 useProfileSettings refetch 拿到新 user_name/avatar，替代当前 `setSettings(prev => ...)` 局部更新。
3. **avatar 显示靠 avatarKey state 强刷 `<img>`**：SaveAvatar 成功后 `setAvatarKey(prev => prev + 1)` 让 `<img key={avatarKey}>` 重新加载。mutation 化后保留此机制（query refetch 只刷 settings 数据，img 缓存靠 key 破坏）。
4. **无需 useProfileStore**（规则 10）：ProfileView 单视图无跨组件共享 state，editingName/nameDraft/avatarKey 全是组件内 UI state。

**改动文件**：
- `frontend/src/components/profile/ProfileView.tsx`（核心：删 load 三件套，改 query/mutation）
- 新增 `frontend/src/components/profile/useWritingActivity.ts` / `useWritingStats.ts` / `useProfileSettings.ts` / `useSaveAvatar.ts` / `useSaveUserName.ts`
- `frontend/src/lib/queryKeys.ts`（新增 profileKeys / writingActivityKeys / writingStatsKeys；settingsKeys 复用 chat 已有的）
- `frontend/src/lib/queryErrorToast.ts`（补 `profile-activity` / `profile-stats` / `profile-settings` 映射；settings 映射 chat 已有）
- `frontend/src/i18n/locales/zh-CN.json` + `en.json`（profile 命名空间补 `activityLoadFailed` / `statsLoadFailed`；`loadFailed` 已存在）

**改造要求清单**：

### A. useApp 调用迁移分类

| 类别 | API | 处理 |
|---|---|---|
| 迁 query | GetWritingActivity(12) / GetWritingStats / GetSettings | useWritingActivity(months) + useWritingStats() + useProfileSettings()；queryFn 直接 import wailsjs；enabled 守卫；GetSettings 复用 settingsKeys.all 与 chat 共享缓存 |
| 迁 mutation | SaveAvatar / SaveUserName | useSaveAvatar + useSaveUserName；onSuccess invalidate settingsKeys.all（让 useProfileSettings refetch 拿新 user_name/avatar） |
| 删 useApp | 全部 5 个 | ProfileView 内 useApp 调用清零 |

### B. useProfileStore 评估

按规则 10，**不需要**：
- ProfileView 是单视图，无侧栏 List + 主区 View 拆分
- editingName/nameDraft/avatarKey/avatarError/nameError 全是组件内 UI state
- 与 style/reader/preference 跳过 store 同理

### C. 错误处理

- **GET 错误**（GetWritingActivity/GetWritingStats/GetSettings）：走中间件，删组件级 toastError（L56）+ console.error；isError 内连显示 `profile.loadFailed`（保留现有 loadFailed UI 文案）
- **mutation 错误**（SaveAvatar/SaveUserName）：调用方 try/catch + inline error 保留（avatarError/nameError）；**评估是否补 toast**——当前无 toast 只有 inline，按规则 9「不静默」inline 已满足，可不补 toast
- **三分支渲染**：ProfileView 主区改 isLoading/isError/data 三分支（替代 loading/loadFailed useState）
- **重复 toast 检查**：GET 错误当前有 toastError（L56）→ 迁 query 后删组件级 toastError 由中间件接管不重复；mutation 无 toast 保留 inline 不重复

### D. i18n / queryKey / 中间件映射

- queryKeys.ts：新增 `writingActivityKeys.detail(months)` + `writingStatsKeys.all` + `profileKeys`（或复用 settingsKeys.all）；GetSettings 复用 chat 的 `settingsKeys.all`
- queryErrorToast.ts：补 `profile-activity: "profile.activityLoadFailed"` + `profile-stats: "profile.statsLoadFailed"`；`settings` 映射 chat 已有（`chat.settingsLoadFailed`）——评估是否改通用 key 或保留 chat 前缀
- i18n：补 `profile.activityLoadFailed` / `profile.statsLoadFailed`（zh-CN + en）；`profile.loadFailed` 已存在

**验证**：build + lint + test。手测：profile 页面加载（activity grid + stats + 用户名/头像显示）；GET 失败 toast + inline 错误；SaveAvatar 成功后头像刷新；SaveUserName 成功后用户名更新；切走再切回 query 缓存命中不 spinner。

**风险**：低。ProfileView 是独立视图，无跨组件联动；GetSettings 与 chat 共享缓存需确认 queryKey 一致（settingsKeys.all）。

### commit 拆分

**commit 1: useWritingActivity + useWritingStats + useProfileSettings query**
- `feat(profile): migrate profile data loading to query hooks`
- 3 个 GET query 化 + ProfileView 删 load 三件套 + 三分支渲染 + 删 GET toastError（中间件接管）+ queryKeys/中间件映射/i18n 补齐
- 风险：GetSettings 与 chat 共享 settingsKeys.all 缓存，确认不互相干扰

**commit 2: useSaveAvatar + useSaveUserName mutation + 删 useApp**
- `feat(profile): add avatar and username mutations`
- 2 个 mutation 化 + handleFileChange/handleNameSave 改 mutateAsync + onSuccess invalidate settingsKeys.all（替代 setSettings 局部更新）+ 保留 inline error + 删 useApp import + ProfileView 内 useApp 调用清零
- 风险：SaveAvatar 成功后 avatarKey 强刷 `<img>` 机制保留（query refetch 不影响 img 缓存）

---

## 5.8 settings 收尾

**现状调研**（2026-08-11 核对）：
- `useSaveLLMConfig.ts` 已 mutation 化 ✅（5.1 commit 4 做的）
- `ModelDiscoveryPanel.tsx` 的 DiscoverModels 已直接 import wailsjs ✅
- **剩余未改造**：
  - `GeneralConfigTab.tsx`：useApp 调 RebuildNovelIndex（命令）；直接 import SaveGitConfig / GetVersion / CheckUpdate
  - `ModelConfigTab.tsx`：useApp 调 TestConnection（命令）
- SaveGitConfig 直接 import 但未 mutation 化（命令式 await）
- GetVersion / CheckUpdate 是 GET 但低频（只读一次），query 化收益低

**特殊点**：
1. **SaveGitConfig 应 mutation 化**：保存 git 配置后应刷新相关数据（如 git history），mutation onSuccess 可失效 git 相关 queryKey
2. **RebuildNovelIndex / TestConnection / DiscoverModels 是命令操作**：按文档总原则可走 try/catch + toastError，不必 mutation 化。但为统一模式 + 删 useApp，建议 mutation 化（或直接 import wailsjs + try/catch）
3. **GetVersion / CheckUpdate 保留命令式**：低频 GET，直接 import wailsjs 即可（CheckUpdate 已直接 import）
4. **无需 useSettingsStore**（规则 10）：settings 组件内 state 全是表单 UI，无跨组件共享

**改动文件**：
- `frontend/src/components/settings/GeneralConfigTab.tsx`（删 useApp，RebuildNovelIndex/SaveGitConfig 改 mutation 或直接 import）
- `frontend/src/components/settings/ModelConfigTab.tsx`（删 useApp，TestConnection 改 mutation 或直接 import）
- 新增 `frontend/src/components/settings/useSaveGitConfig.ts` / `useRebuildNovelIndex.ts` / `useTestConnection.ts`（按需）
- `frontend/src/lib/queryKeys.ts`（gitConfigKeys 若需 invalidate）
- `frontend/src/lib/queryErrorToast.ts`（无新增，命令操作不走中间件）

**改造要求清单**：

### A. useApp 调用迁移分类

| 类别 | API | 文件 | 处理 |
|---|---|---|---|
| 迁 mutation | SaveGitConfig | GeneralConfigTab | useSaveGitConfig；onSuccess 评估失效 git 相关 queryKey |
| 命令（直接 import） | RebuildNovelIndex | GeneralConfigTab | useRebuildNovelIndex mutation 或直接 import wailsjs + try/catch + toastError |
| 命令（直接 import） | TestConnection | ModelConfigTab | useTestConnection mutation 或直接 import wailsjs + try/catch |
| 保留命令式 | GetVersion / CheckUpdate | GeneralConfigTab | 已直接 import wailsjs，保留 |
| 保留命令式 | DiscoverModels | ModelDiscoveryPanel | 已直接 import wailsjs，保留 |
| 删 useApp | RebuildNovelIndex / TestConnection | GeneralConfigTab / ModelConfigTab | 两文件 useApp 调用清零 |

### B. useSettingsStore 评估

按规则 10，**不需要**：settings 组件内 state 全是表单 UI（provider/url/key/model 编辑态），无跨组件共享。

### C. 错误处理

- **命令操作**（RebuildNovelIndex/TestConnection/SaveGitConfig）：调用方 try/catch + toastError（保留现有 toastError）
- **无 GET query 化**：GetVersion/CheckUpdate 保留命令式，不走中间件
- **重复 toast 检查**：命令操作保留组件级 toastError，不走中间件，不重复

### D. i18n / queryKey / 中间件映射

- 无新增 queryKey（命令操作不进 cache；SaveGitConfig onSuccess 若需失效 git history 用现有 gitCommitsKeys）
- 无新增中间件映射（命令操作不走中间件）
- i18n：`settings.*Failed` 评估是否需补（RebuildNovelIndex/TestConnection/SaveGitConfig 失败文案）

**验证**：build + lint + test。手测：RebuildNovelIndex 成功/失败 toast；TestConnection 成功/失败；SaveGitConfig 保存后 git history 刷新；GeneralConfigTab/ModelConfigTab 内 useApp 调用清零。

**风险**：低。settings 是表单操作，无复杂数据流；SaveGitConfig onSuccess 失效范围需确认。

### commit 拆分

**commit 1: useSaveGitConfig + useRebuildNovelIndex + useTestConnection mutation + 删 useApp**
- `refactor(settings): migrate remaining commands to mutations and drop useApp`
- 3 个命令 mutation 化（或直接 import wailsjs）+ GeneralConfigTab/ModelConfigTab 删 useApp + 保留 try/catch + toastError
- 风险：低。命令操作 mutation 化主要是模式统一，无数据流变化

---

## 5.9 命令式调用收尾

**现状调研**（2026-08-11 核对）：
- `App.tsx`：useApp 调 GetSettings（启动检查）
- `InitView.tsx`：useApp 调 GetPlatform / Initialize
- `WorkspaceView.tsx`：useApp 调 GetPlatform / ApproveTool / SetActiveNovel / SaveCover
- `NovelDialogs.tsx`：直接 import ExportNovel（命令，已不走 useApp）✅
- `UpdateDialog.tsx`：直接 import DismissUpdate（命令）✅

**特殊点**：
1. **SetActiveNovel 已在 useNovelStore.switchNovel 里直接 import wailsjs**：WorkspaceView L257 的 `app.SetActiveNovel` 是自动选小说 effect 里的冗余调用，应改用 switchNovel 或直接 import wailsjs
2. **SaveCover 应 mutation 化**：保存封面后应刷新 novel 数据（封面 URL），mutation onSuccess invalidate novelKeys
3. **GetPlatform / GetSettings 启动逻辑保留命令式**：App/InitView 是启动入口，在 QueryClientProvider 挂载前/早期执行，query 化收益低且时序复杂，直接 import wailsjs 即可
4. **ApproveTool 是命令操作**：保留 try/catch + toastError，直接 import wailsjs

**改动文件**：
- `frontend/src/App.tsx`（删 useApp，GetSettings 改直接 import wailsjs）
- `frontend/src/views/InitView.tsx`（删 useApp，GetPlatform/Initialize 改直接 import wailsjs）
- `frontend/src/views/WorkspaceView.tsx`（删 useApp，GetPlatform/ApproveTool/SetActiveNovel 改直接 import wailsjs 或 switchNovel；SaveCover mutation 化）
- 新增 `frontend/src/components/novel/useSaveCover.ts`（mutation）
- `frontend/src/lib/queryKeys.ts`（无新增，novelKeys 已存在）

**改造要求清单**：

### A. useApp 调用迁移分类

| 类别 | API | 文件 | 处理 |
|---|---|---|---|
| 保留命令式（直接 import） | GetSettings | App.tsx | 启动检查，直接 import wailsjs |
| 保留命令式（直接 import） | GetPlatform / Initialize | InitView.tsx | 启动逻辑，直接 import wailsjs |
| 保留命令式（直接 import） | GetPlatform / ApproveTool | WorkspaceView.tsx | 命令操作，直接 import wailsjs + try/catch |
| 改用 store action | SetActiveNovel | WorkspaceView.tsx | 改用 useNovelStore.switchNovel（已封装 SetActiveNovel） |
| 迁 mutation | SaveCover | WorkspaceView.tsx | useSaveCover；onSuccess invalidate novelKeys |

### B. 错误处理

- **命令操作**（GetPlatform/ApproveTool/Initialize/GetSettings）：try/catch + toastError 或静默（按现有行为保留）
- **SaveCover mutation**：调用方 try/catch + toastError
- **重复 toast 检查**：命令操作保留组件级，不走中间件，不重复

### C. i18n / queryKey / 中间件映射

- SaveCover onSuccess invalidate `novelKeys.all`（让 useNovels refetch 拿新封面）
- 无新增中间件映射

**验证**：build + lint + test（WorkspaceView.test.tsx 18 用例必须仍绿）。手测：启动流程正常；审批 approve/reject 正常；切换小说 SetActiveNovel 正常；保存封面后书架封面刷新。

**风险**：中。WorkspaceView 是核心视图，改 useApp 调用需确认不破坏启动时序；SaveCover mutation onSuccess 失效 novelKeys 让 useNovels refetch 可能触发自动选小说 effect，需确认不循环。

### commit 拆分

**commit 1: 命令式调用改直接 import wailsjs + SaveCover mutation + 删 useApp**
- `refactor(core): switch remaining commands to direct wailsjs imports and drop useApp`
- App/InitView/WorkspaceView 删 useApp + GetPlatform/ApproveTool/Initialize/GetSettings 改直接 import wailsjs + SetActiveNovel 改 switchNovel + SaveCover mutation 化 + useSaveCover 新增
- 风险：WorkspaceView 启动时序；SaveCover invalidate novelKeys 与自动选小说 effect 的交互

---

## 5.10 类型 import 迁移 + 删 useApp.ts

**前置条件**：5.7-5.9 全部完成，确认无组件再调 `useApp()`，且所有 EventsOn 订阅已改成 `qc.invalidateQueries` 依赖（不再间接依赖 app 对象）。

**现状调研**（2026-08-11 核对）：
- 约 20 处 `import type { xxx } from "@/hooks/useApp"`，涉及类型：character / preference / novel / chapter / timeline / setting / storyarc / location / llm / reader / app / session / imp
- 分布：CharacterGraph / CharacterListView / PreferenceView / NovelList / SidePanel / TimelineView / NovelSettingView / ArcListView / StoryArcGraph / LocationList / LocationGraph / LocationListView / SlashMenu / ChatInput / chat/types / BookshelfView / NovelEditDialog / CustomProviderPane / BuiltinProviderPane / ModelEditForm / ModelDiscoveryPanel / ReaderView / WorkspaceView

**改动文件**：
- 上述约 20 个文件的类型 import 改 `from "@/lib/wailsjs/go/models"`
- 删 `frontend/src/hooks/useApp.ts`

**怎么做**：
1. 全局搜 `useApp`，确认无 `useApp()` 函数调用（5.7-5.9 已清零）
2. 逐文件把 `import type { xxx } from "@/hooks/useApp"` 改成 `from "@/lib/wailsjs/go/models"`
3. 删 useApp.ts 文件
4. 确认无残留引用

**验证**：build + lint + test 全绿，确认无残留引用。

**风险**：中。useApp 有修 bug 留下的 useMemo（防 EventsOn 重订阅丢事件），前提是 5.7-5.9 已把 useApp 调用改干净 + EventsOn 依赖已改 qc.invalidateQueries（5.2 commit 3 已完成）。重点手测 file:changed / chat 事件不丢。

**手测点**：重点验证 file:changed 事件、chat 流式事件不丢（长时间操作切面板回来事件仍触发）。

### commit 拆分

**commit 1: 类型 import 迁移 + 删 useApp.ts**
- `refactor(frontend): migrate type imports to wailsjs models and remove useApp`
- 20 处类型 import 改 `@/lib/wailsjs/go/models` + 删 useApp.ts + 确认无残留
- 风险：useApp 的 useMemo 防 EventsOn 重订阅 bug，需确认 EventsOn 订阅不再依赖 app 对象（5.2 commit 3 已改 qc，流式/命令 EventsOn 直接 import wailsjs 不经 useApp）

---

## 阶段 5 补遗完成标准

- profile / settings 领域走 query/mutation（命令操作可保留命令式）
- App/InitView/WorkspaceView 命令式调用改直接 import wailsjs
- 类型 import 全部从 `@/lib/wailsjs/go/models` 取
- useApp.ts 删除
- EventsOn 订阅全部改成 qc.invalidateQueries 依赖（流式/命令保留 EventsOn 但不依赖 app 对象）
- 现有测试全绿
- 手测重点：事件不丢、流式数据正常、CRUD 同步、错误提示完整

完成后进入 [06-monolith-optional.md](./06-monolith-optional.md)（可选，痛点驱动）。
