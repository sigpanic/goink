# 阶段 3：小说领域先行（P3 模板）

> 前置条件：[阶段 2](./02-workspaceview.md) 完成（usePanelStore/useFocusStore 就位）。
> 完成后：小说领域完全自治，WorkspaceView 不再管小说业务；模板成型，阶段 4 套用。
> 关键里程碑：3.7 switchNovel 从命令式协调变成 store action + 声明式订阅（干掉 flushSync）。

## 进度勾选

- [ ] 3.1 useNovels query（WorkspaceView 1 处试用）
- [ ] 3.2 useNovelStore（activeNovelId + 对话框开关）
- [ ] 3.3 useCreateNovel mutation
- [ ] 3.4 useUpdateNovel mutation
- [ ] 3.5 useDeleteNovel mutation
- [ ] 3.6 抽 <NovelDialogs> 组件
- [ ] 3.7 switchNovel 迁到 store action
- [ ] 3.8 ContentPanel 订阅 activeNovelId（干掉 flushSync）
- [ ] 3.9 其余 3 处 GetNovels 消费方迁移

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

**怎么做**：mutationFn 接收 `{ id, input }` 调 `UpdateNovel(id, input)`；`onSuccess` 失效 `novelKeys.all`。`handleUpdateNovel` 改用 mutation，删 `loadNovels`。

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

## 3.7 switchNovel 迁到 store action（里程碑）

**目标**：`switchToNovel`（2.5 抽的函数）从 WorkspaceView 迁到 `useNovelStore` 的 action。范式转变——从「父组件命令式协调」到「store action + 子组件订阅」。

**改动文件**：改 `frontend/src/components/novel/useNovelStore.ts`；改 `frontend/src/views/WorkspaceView.tsx`

**怎么做**：
- store 加 `switchNovel(id)` action：`set({ activeNovelId: id })` + 调 `SetActiveNovel({ novel_id: id })`（直接 import wailsjs）。
- `closeAllTabs`/`setTabTarget`/`setActiveContent`/`setSelectedGitFile` 这些 ContentPanel 相关的重置**不放进 store action**——它们是 ContentPanel 的职责，3.8 让 ContentPanel 订阅 activeNovelId 变化自行重置。store action 只管自己的 state + 后端调用。
- WorkspaceView 删 `switchToNovel`，4 处调用改 `useNovelStore` 的 `switchNovel(id)`。

**验证**：build + lint + test。注意本步 ContentPanel 重置逻辑还没改（3.8 才改），**本步与 3.8 应紧接做**，否则中间状态 tabs 可能残留。

**风险**：高。关键路径改造。

**手测点**：切小说后 activeNovelId 更新；ContentPanel 重置（3.8 完成后验证）。

**commit**：`refactor(novel): move switchNovel to store action`

---

## 3.8 ContentPanel 订阅 activeNovelId（干掉 flushSync）

**目标**：ContentPanel 订阅 `activeNovelId` 变化，自动 `closeAllTabs` + 重置状态。干掉 WorkspaceView 的 `flushSync`（L309）和命令式 `contentRef.current?.closeAllTabs()`。

**改动文件**：改 `frontend/src/components/content/ContentPanel.tsx`；改 `frontend/src/views/WorkspaceView.tsx`

**怎么做**：
- ContentPanel 内 `useNovelStore` 订阅 `activeNovelId`，用 ref 记录 prev，变化时调用内部 `closeAllTabs` + 重置自身 state（activeContent 等）。
- WorkspaceView 的 `handleSearchNavigateChapter`（L302-320）：`flushSync(() => setActivePanel("chapters"))` 改成普通 `setActivePanel("chapters")`（panel 路由已类型化，不需要同步 flush）。
- 4 处 switchNovel 调用里删 `contentRef.current?.closeAllTabs()`、`setTabTarget(null)`、`setActiveContent("")`、`setSelectedGitFile(null)`（归 ContentPanel 自己管）。
- `tabTarget`/`activeContent`/`selectedGitFile` state 评估是否还用，能删则删。

**验证**：build + lint + test（1.5/1.7 测试必须仍绿）。

**风险**：高。审批关键路径 + ContentPanel 改造，三方协议（WorkspaceView + ChatPanel + ContentPanel）。

**手测点**：
- 切小说 → ContentPanel tabs 全部关闭，内容清空
- 搜索章节跳转 → openFileWithHighlight 正常（flushSync 删除后无闪烁）
- 审批流 → approve/reject 后 diff tab 正常
- 导入小说 → tabs 重置

**commit**：`refactor(content): subscribe activeNovelId changes and drop flushSync`

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
- switchNovel 是 store action，ContentPanel 声明式订阅
- flushSync 删除
- 4 处 GetNovels 消费方共享缓存
- WorkspaceView 不再管小说 CRUD（除面板路由 1 行）
- 1.4-1.7 测试全绿
- 手测小说 CRUD + 切小说 + 4 处同步全通过

→ 模板成型，进入 [04-entities-batch.md](./04-entities-batch.md) 套用到 7 个实体领域。
