# 阶段 3：小说领域先行（P3 模板）

> 前置条件：[阶段 2](./02-workspaceview.md) 完成（usePanelStore/useFocusStore 就位）。
> 完成后：小说领域完全自治，WorkspaceView 不再管小说业务；模板成型，阶段 4 套用。
> 关键里程碑：3.7 switchNovel 抽到 store action（switchToNovel 改瘦 wrapper）；3.8 ContentPanel 订阅 activeNovelId。完整删 switchToNovel/flushSync 依赖 tabTarget 等迁领域 store（见 3.8 后续）。

## 进度勾选

- [x] 3.1 useNovels query（WorkspaceView 1 处试用）
- [x] 3.2 useNovelStore（activeNovelId + 对话框开关）
- [x] 3.3 useCreateNovel mutation
- [x] 3.4 useUpdateNovel mutation
- [x] 3.5 useDeleteNovel mutation
- [x] 3.6 抽 <NovelDialogs> 组件
- [x] 3.7 switchNovel 迁到 store action
- [x] 3.8 ContentPanel 订阅 activeNovelId
- [x] 3.9 其余 3 处 GetNovels 消费方迁移

---

## 3.1 useNovels query（WorkspaceView 1 处试用）

**目标**：引入第一个 query，替换 WorkspaceView 的 `novels` state + `loadNovels` + `useEffect`。其余 3 处消费方（StyleView/PatternExtractView/GeneralConfigTab）3.9 再迁。

**改动文件**：新建 `frontend/src/components/novel/useNovels.ts`；改 `frontend/src/views/WorkspaceView.tsx`

**怎么做**：
- 新建 `useNovels` hook：`useQuery`，queryKey 用 `novelKeys.all`（1.3 建的常量），queryFn 直接 import `GetNovels` from wailsjs（不用 useApp），返回值兜底空数组。
- WorkspaceView 删 L72 `novels` state、L170-174 `loadNovels`、L192-194 `useEffect`，改用 `const { data: novels = [] } = useNovels()`。
- mutation 还没建，`handleImportedNovel` 里临时用 `queryClient.invalidateQueries({ queryKey: novelKeys.all })` 触发刷新（3.3 mutation 建好后改回标准模式）。

**验证**：build + lint + test。手测小说列表显示正常。

**风险**：中。首次引入 query 替代 state，缓存机制开始生效。

**手测点**：启动看小说列表；切面板回来不重复 fetch（30s staleTime）。

**commit**：`feat(novel): add useNovels query and replace WorkspaceView novels state`

---

## 3.2 useNovelStore（activeNovelId + 对话框开关）

**目标**：把小说相关的 UI 状态（activeNovelId + 4 个对话框开关）提到 store。

**改动文件**：新建 `frontend/src/components/novel/useNovelStore.ts`；改 `frontend/src/views/WorkspaceView.tsx`、`frontend/src/components/novel/BookshelfView.tsx`

**怎么做**：
- store 状态：`activeNovelId`、`editingNovel`、`deletingNovel`、`showCreateDialog`、`exportNovelId`；对应 5 个 setter。
- WorkspaceView 删 L73/L132-135 的 5 个 state，改用 store selector；`initialNovelId` 在 mount 时一次性 `setActiveNovelId`。
- `switchToNovel`（2.5 抽的）暂留 WorkspaceView，本步只把 state 外移，3.7 才整体迁到 store action。
- BookshelfView 删 novels/activeNovelId/各 onXxx props，组件内自己 `useNovels()` + `useNovelStore()`。

**验证**：build + lint + test（1.7 switchNovel 测试可能要调整 mock，因 state 外移）。

**风险**：中。state 外移 + props 减少，测试可能需调。

**手测点**：打开/关闭编辑/删除/导出对话框；切小说 activeNovelId 更新。

**commit**：`feat(novel): add useNovelStore for activeNovelId and dialog state`

---

## 3.3 useCreateNovel mutation

**目标**：把创建小说的 try/catch + 刷新逻辑封进 mutation。

**改动文件**：新建 `frontend/src/components/novel/useCreateNovel.ts`；改 `frontend/src/views/WorkspaceView.tsx`（`handleCreateNovelFromDialog`）

**怎么做**：
- `useMutation`，mutationFn 调 `CreateNovel(input)`（直接 import）；`onSuccess` 调 `qc.invalidateQueries({ queryKey: novelKeys.all })`。
- `handleCreateNovelFromDialog` 改用 `createNovel.mutateAsync(input)`，成功后 `switchToNovel(n.id)` + 关闭对话框，删掉手动的 `loadNovels` 调用。
- `handleCreateNovel`（旧 showCreate 流，L336-355）同理改造或后续废弃。
- 移除 3.1 临时加的 `invalidateQueries` 手动调用，改由 mutation onSuccess 接管。

**验证**：build + lint + test。

**风险**：低。mutation 封装完整。

**手测点**：创建小说 → 列表自动出现 → 切到新小说。

**commit**：`feat(novel): add useCreateNovel mutation`

---

## 3.4 useUpdateNovel mutation

**目标**：更新小说的 mutation。

**改动文件**：新建 `frontend/src/components/novel/useUpdateNovel.ts`；改 `handleUpdateNovel`（L381-395）

**怎么做**：mutationFn 接收 `{ id, input }` 调 `UpdateNovel(id, input)`，payload 全量回传 input 所有字段（见 [00-conventions.md §6](./00-conventions.md)）；`onSuccess` 失效 `novelKeys.all`。`handleUpdateNovel` 改用 mutation，删 `loadNovels`。

**验证**：build + lint + test。手测编辑小说标题，列表同步更新。

**风险**：低。

**commit**：`feat(novel): add useUpdateNovel mutation`

---

## 3.5 useDeleteNovel mutation

**目标**：删除小说的 mutation。

**改动文件**：新建 `frontend/src/components/novel/useDeleteNovel.ts`；改 `handleDeleteNovel`（L397-407）

**怎么做**：mutationFn 调 `DeleteNovel(id)`；`onSuccess` 失效 `novelKeys.all`。`handleDeleteNovel` 改用 mutation。删 novel 后若删的是当前 activeNovelId，由 L236-247 的 useEffect（「当前小说不存在时选第一个」）自动接管。

**验证**：build + lint + test。手测删除小说 → 列表更新 → 若删的是当前小说自动切到第一个。

**风险**：低。

**commit**：`feat(novel): add useDeleteNovel mutation`

---

## 3.6 抽 <NovelDialogs> 组件

**目标**：把 4 个小说对话框（NovelEditDialog×2 / NovelDeleteDialog / ExportDialog）从 WorkspaceView 抽到独立组件，消费 useNovelStore。

**改动文件**：新建 `frontend/src/components/novel/NovelDialogs.tsx`；改 `frontend/src/views/WorkspaceView.tsx`（删 L745-768 对话框 JSX）

**怎么做**：
- NovelDialogs 内部：从 `useNovelStore` 取对话框状态 + setter；从 `useNovels` 取小说列表（用于 ExportDialog 显示标题）；用 3.3-3.5 的 mutation 处理 onSave/onConfirm。
- WorkspaceView 把 L745-768 换成 `<NovelDialogs />`。
- ExportDialog 的 `onExport` 调 `app.ExportNovel`（可单独留或后续抽 mutation）。

**验证**：build + lint + test。

**风险**：中。对话框逻辑搬家，需确认 onSave 回调等价。

**手测点**：创建/编辑/删除/导出小说全流程。

**commit**：`refactor(novel): extract NovelDialogs component consuming store`

---

## 3.7 switchNovel 迁到 store action

**目标**：把 `switchToNovel` 的「store state + 后端调用」部分抽到 `useNovelStore.switchNovel` action。`switchToNovel` 改成瘦 wrapper，删冗余的 `closeAllTabs`。

**改动文件**：改 `frontend/src/components/novel/useNovelStore.ts`；改 `frontend/src/views/WorkspaceView.tsx`；改 `frontend/src/views/WorkspaceView.test.tsx`

**怎么做**：
- store 加 `switchNovel(id)` action：`set({ activeNovelId: id })` + `await SetActiveNovel({ novel_id: id })`（直接 import wailsjs，绕过 useApp）。
- `switchToNovel` 改瘦 wrapper：`await switchNovel(id)` 后照旧 `setActivePanel("chapters") + setTabTarget(null) + setActiveContent("") + setSelectedGitFile(null)`。**删 `contentRef.current?.closeAllTabs()`** —— `useEditorTabs` 的 novelId effect（`useEditorTabs.ts:64-83`）已自动切换 per-novel tabs，closeAllTabs 冗余。
- 调用方不变（4 处仍调 `switchToNovel`）。
- **不删 switchToNovel** —— `tabTarget`/`activeContent`/`selectedGitFile` 是 WorkspaceView 本地 state，分别喂给 SidePanel/StatusBar/GitCommitView，删了没人重置。完整删 `switchToNovel` 需先迁这些 state 到对应领域 store（见 3.8 后续）。
- 测试：删 `WorkspaceView.test.tsx` 里 `expect(contentRefSpies.closeAllTabs).toHaveBeenCalled()` 断言 —— closeAllTabs 不再被命令式调用，tab 切换由 useEditorTabs 内部 effect 接管。

**验证**：build + lint + test。

**风险**：低。switchNovel 是纯抽取；closeAllTabs 删除有 useEditorTabs 兜底；调用方不变。

**手测点**：切小说后 activeNovelId 更新；tabs 自动切到新小说（useEditorTabs 接管）；SidePanel tab 高亮、StatusBar 内容、GitCommitView 文件选择重置。

**commit**：`refactor(novel): move switchNovel to store action`

---

## 3.8 ContentPanel 订阅 activeNovelId

**目标**：ContentPanel 直接从 `useNovelStore` 订阅 `activeNovelId`，不再通过 prop 接收 `novelId`。

**改动文件**：改 `frontend/src/components/content/ContentPanel.tsx`；改 `frontend/src/views/WorkspaceView.tsx`

**怎么做**：
- ContentPanel：删 `novelId` prop，改 `const novelId = useNovelStore((s) => s.activeNovelId)`。`useEditorTabs(novelId)` 不变（来源从 prop 变 store，行为等价）。
- WorkspaceView：删 `<ContentPanel novelId={activeNovelId} ...>` 的 `novelId` prop。
- **flushSync 保留** —— `handleSearchNavigateChapter` 的 `flushSync(() => setActivePanel("chapters"))` 确保 ContentPanel 挂载后才调 `contentRef.current?.openFileWithHighlight`（ContentPanel 条件渲染，从非 chapters 面板搜索章节时未挂载）。文档原说「不需要同步 flush」是误判，删了会空指针。
- `tabTarget`/`activeContent`/`selectedGitFile` 暂不迁 —— 迁走需改 SidePanel/StatusBar/GitCommitView 订阅 store，放阶段 4（实体批量时这些组件改造）或单独步骤。

**验证**：build + lint + test（1.5/1.7 测试必须仍绿）。

**风险**：低。novelId 来源从 prop 改 store，行为等价。

**手测点**：切小说 → ContentPanel 显示新小说内容；搜索章节跳转 → openFileWithHighlight 正常（flushSync 保留）；审批流 → diff tab 正常。

**commit**：`refactor(content): subscribe activeNovelId from store`

---

## 3.8 后续：删 switchToNovel + flushSync（依赖前置）

**何时做**：`tabTarget`/`activeContent`/`selectedGitFile` 迁到对应领域 store 后（SidePanel/StatusBar/GitCommitView 订阅 store）。预计阶段 4（实体批量，这些组件改造时）或单独步骤。

**目标**：
- `switchToNovel` 完全删除，4 处调用方直接用 `switchNovel(id) + setActivePanel("chapters")`。
- `flushSync` 评估是否可改声明式 —— 需 ContentPanel 不再条件渲染（始终挂载，CSS 控制显隐），或搜索跳转改成 store/props 驱动（不再 imperative contentRef）。痛点驱动，非必做。

**注**：原 3.8 文档设想「ContentPanel 订阅 + 删 closeAllTabs/setTabTarget/setActiveContent/setSelectedGitFile（归 ContentPanel 自己管）」—— 调研发现 tabTarget 是 SidePanel 的、activeContent 是 StatusBar 的、selectedGitFile 是 GitCommitView 的，**不归 ContentPanel**。故拆分：3.8 只做 ContentPanel 订阅（低风险），完整删 switchToNovel 推迟。

---

## 3.9 其余 3 处 GetNovels 消费方迁移

**目标**：把 StyleView / PatternExtractView（或 ExtractWorkspaceView）/ GeneralConfigTab 各自的手 fetch 换成 `useNovels()`，4 处共享同一缓存。

**改动文件**：`frontend/src/components/style/StyleView.tsx`、`frontend/src/components/extract/ExtractWorkspaceView.tsx`、`frontend/src/components/settings/GeneralConfigTab.tsx`

**怎么做**：每处把 `useState<novel.Novel[]>` + `loadNovels` + `useEffect` 三件套换成 `const { data: novels = [] } = useNovels()`。

**验证**：build + lint + test。手测：4 处都显示小说列表，且其中一处创建/删除小说后**所有 4 处自动同步**（query 失效）。

**风险**：低。模式同 3.1。

**手测点**：在 GeneralConfigTab 切换小说，StyleView 同步；创建小说后所有消费方刷新。

**commit**：`refactor(frontend): migrate 3 remaining GetNovels consumers to useNovels`

---

## 阶段 3 完成标准

- 小说领域四件套就位（useNovels query + useNovelStore + 3 mutation + NovelDialogs）
- switchNovel 是 store action；switchToNovel 是瘦 wrapper（调 switchNovel + 重置本地 state）
- ContentPanel 订阅 activeNovelId（不再走 prop）
- closeAllTabs 命令式调用删除（useEditorTabs 接管）
- flushSync 保留（搜索跳转依赖，完整删除见 3.8 后续）
- 4 处 GetNovels 消费方共享缓存
- WorkspaceView 不再管小说 CRUD（除面板路由 1 行）
- 1.4-1.7 测试全绿
- 手测小说 CRUD + 切小说 + 4 处同步全通过

→ 模板成型，进入 [04-entities-batch.md](./04-entities-batch.md) 套用到 7 个实体领域。
