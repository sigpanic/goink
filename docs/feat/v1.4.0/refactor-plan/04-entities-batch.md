# 阶段 4：7 实体领域套模板

> 前置条件：[阶段 3](./03-novel-template.md) 完成，小说领域模板验证手感。
> 完成后：7 个实体领域全部四件套化，refreshNonce 整套删除。
> 模式：每个领域 = useXxxList query + useXxxStore + useCreate/Update/Delete mutation + <XxxDialogs>。同构迁移，套阶段 3 模板，不再重复详述每步，只列各领域差异点和批次。

## 进度勾选

- [ ] 4.1 character（List + Graph 共享，收益最大）
- [ ] 4.2 location（含 location-relations）
- [ ] 4.3 storyarc（含 arc-nodes 子资源）
- [ ] 4.4 timeline
- [ ] 4.5 reader
- [ ] 4.6 preference（含 is_global 全局/小说级区分）
- [ ] 4.7 novel-setting
- [ ] 4.8 删除 useRefresh / refreshNonce 整套机制

---

## 通用做法（每个领域都走这套，不重复写）

每个领域按小说模板（阶段 3）套 4 步，建议每个领域拆成 4 个 commit。

**文件位置（领域聚合，见 [00-conventions.md §4](./00-conventions.md#4-目录约定领域聚合)）**：所有 query/store/mutation 都放对应 `components/{domain}/`，与该领域的 UI 组件同目录。应用级 store（panel/focus/tab）放 `src/stores/`。

1. **useXxxList query** + 在 List 组件试用，替换手 fetch 三件套（state + load + useEffect）。queryKey 用 1.3 常量。
2. **useXxxStore**（activeId + 对话框开关 + 必要的协调 action）。
3. **useCreateXxx / useUpdateXxx / useDeleteXxx mutation**，onSuccess 失效对应 queryKey。替换组件内的 try/catch + bumpRefresh。useUpdateXxx 的 payload 全量回传 input 所有字段（见 [00-conventions.md §6](./00-conventions.md)）。
4. **<XxxDialogs> 抽出**，消费 store；原组件删 dialog state。

每个领域完成后手测：List 计数与 View 数据同步（mutation 后 invalidateQueries 自动刷新，不再靠 bumpRefresh）。

---

## 4.1 character

**特殊点**：有 CharacterGraph（关系图），List 和 Graph 都消费角色数据，是收益最大的领域（共享缓存消除重复 fetch）。

**改动文件**：`frontend/src/components/character/CharacterListView.tsx`、`CharacterGraph.tsx`、`CharacterList.tsx`

**怎么做**：
- `useCharacters(novelId)` query（key `characterKeys.list(novelId)`），List 和 Graph 共享。
- 额外 `useCharacterRelations(novelId)` query（key `characterKeys.relations(novelId)`）供 Graph。
- CharacterListView 当前 9 个 state（参考读码：characters/loading/loadFailed/viewTab/editMode/form/saving/deleteTarget/deleting），query 接管 characters/loading/loadFailed，store 接管 editMode/deleteTarget/dialog，mutation 接管 saving/deleting。
- 删 `useRefresh` 调用（L43、L117、L137、L157）。

**验证**：build + lint + test（CharacterList.test.tsx 必须仍绿，可能需调整 mock 从 useApp 改成 mock query 或保持 useApp mock）。

**手测点**：List 编辑/删除后 Graph 同步；Graph 改关系后 List 计数同步。

**commit**（4 个）：`feat(character): add useCharacters query` / `... useCharacterStore` / `... useCreateCharacter mutation`（+ update/delete） / `refactor(character): extract CharacterDialogs`

---

## 4.2 location

**特殊点**：有 location-relations（空间关系图）。

**改动文件**：`frontend/src/components/location/LocationListView.tsx`、相关 View

**怎么做**：`useLocations(novelId)` + `useLocationRelations(novelId)` 两个 query。其余同模板。

**手测点**：List 和关系图数据同步。

**commit**：同 4.1 拆 4 个。

**进度**：
- [x] commit 1: useLocations + useLocationRelations query + LocationListView/LocationGraph/LocationList 改造 + 测试适配 + 中间件映射（queryErrorToast 补 locations/location-relations）+ i18n 补 locationsLoadFailed/relationsLoadFailed
- [x] commit 1.5: LocationList/CharacterList 侧边栏加 isError 内连错误显示（commit 1 漏补）
- [x] commit 2: useLocationStore + useDeleteLocation mutation + 删除合并（LocationList 侧边栏 dispatch store，LocationListView 集中 ConfirmDialog）+ i18n 文案融合（新建 confirmDeleteLocation，删 confirmDeleteWithChildren/confirmDeleteIrreversible，含 character 块清理 ee13b0b）
- [x] commit 3: useCreate/UpdateLocation mutation（handleCreate/handleUpdate 改 mutateAsync，saving 由 mutation.isPending 推导，删 useApp + setSaving useState）
- [ ] commit 4: LocationDialogs 抽出

---

## 4.3 storyarc

**特殊点**：arc-nodes 原文档写按 arcId 切分，但后端 `GetArcNodes(novelId, fromChapter, toChapter)` 第二三参数是章节窗口非 arcId，无按 arcId 拉取的 API，故 queryKey 改为 `["arc-nodes", novelId]` 全量（已同步改 00-conventions.md §1.2 + queryKeys.ts）。额外有 `GetMaxChapterNumber(novelId)` 用于章节窗口中心 windowCenter，抽 `useMaxChapterNumber` query。ArcListView 1130 行是巨石，本阶段**只迁移数据层**，不拆组件（拆组件留阶段 6）。

**改动文件**：`frontend/src/components/storyarc/ArcList.tsx`、`ArcListView.tsx`、`StoryArcGraph.tsx`

**怎么做**：`useStoryArcs(novelId)` + `useArcNodes(novelId)` + `useMaxChapterNumber(novelId)` 三个 query。arc CRUD 和 node CRUD 各一组 mutation。

**手测点**：arc/node CRUD 后列表同步；focusArcId 联动（自动展开+定位章节窗口）；章节窗口前/后翻；快速状态切换。

**commit**：同 4.1 拆 4 个（storyarc 跳过 store commit，因 ArcList 侧边栏只读无跨组件 state，按规则 10 不建 store）。

**进度**：
- [x] commit 1: useStoryArcs + useArcNodes + useMaxChapterNumber query + ArcList/ArcListView/StoryArcGraph 改造（删 useApp/useRefresh/load 三件套，改用 query data；CRUD 后由 bumpRefresh → refreshNonce → invalidateQueries 刷新，commit 2/3 改 mutation 后改 onSuccess invalidate）+ 中间件映射（queryErrorToast 补 storyarcs/arc-nodes/max-chapter）+ i18n 补 arcsLoadFailed/nodesLoadFailed/maxChapterLoadFailed + queryKeys.ts 改 arcNodeKeys.list(novelId) + 新增 maxChapterKeys + 00-conventions.md §1.2 同步
- [x] commit 2: useDeleteStoryArc + useDeleteArcNode mutation + confirmDelete 改 mutateAsync（deleting 由 mutation.isPending 推导，删 setDeleting useState + bumpRefresh；onSuccess 失效对应 query：删 arc 失效 storyarcs + arc-nodes，删 node 失效 arc-nodes）
- [x] commit 3: useCreateStoryArc + useUpdateStoryArc + useCreateArcNode + useUpdateArcNode mutation + handleCreateArc/handleUpdateArc/handleCreateNode/handleUpdateNode/handleQuickNodeStatus 改 mutateAsync（saving 由 4 个 mutation.isPending 推导，删 setSaving useState + bumpRefresh + useRefresh + refreshNonce useEffect；全量回传 input 所有字段 §6：openEditArc/openEditNode 补全 status/reactivate_at/actual_chapter，handleQuickNodeStatus 从 node 构造全量 input 仅替换 status）+ 失效范围（create/update arc 失效 storyarcs；create/update node 失效 arc-nodes；create arc 无 node 不失效 arc-nodes）

**遗留（章节领域改造时处理）**：`max-chapter` query 当前无消费方主动 invalidate。章节增删后 `GetMaxChapterNumber` 返回值变化，但 storyarc 的 `useMaxChapterNumber` 缓存不刷新，导致 `windowCenter` 不更新（既有行为，非本次改造引入）。阶段 5 章节领域 query 化时，在章节 create/delete mutation 的 `onSuccess` 里补 `qc.invalidateQueries({ queryKey: maxChapterKeys.detail(novelId) })`，或通过 `EventsOn("file:changed")` 触发 invalidate。

---

## 4.4 timeline

**特殊点**：TimelineView 的 `load()` 用 `Promise.all` 拉 3 个数据（同 storyarc 三 query 同构）：①`GetTimelineEntries(novelId, 0, 0)` → entries（第二三参数是章节窗口，传 0,0 拿全量，同 `GetArcNodes` 模式，queryKey 用 `["timeline", novelId]` 全量）；②`GetChapterPlans(novelId)` → plans（章节计划 3-slot next/near/far，独立 API，新建 `useChapterPlans` + `chapterPlanKeys`）；③`GetMaxChapterNumber(novelId)` → windowCenter（复用 storyarc 4.3 已建 `useMaxChapterNumber`，同 novelId 共享 `maxChapterKeys` 缓存）。TimelineList 侧边栏独立 fetch `GetTimelineEntries`，改造后与 TimelineView 共享 entries 缓存（同 character/location/storyarc 的 List+View 模式）。TimelineView 934 行巨石，本阶段**只迁数据层**不拆组件（拆组件留阶段 6）。

**改动文件**：`frontend/src/components/timeline/TimelineView.tsx`、`TimelineList.tsx`

**怎么做**：`useTimelineEntries(novelId)` + `useChapterPlans(novelId)` + 复用 `useMaxChapterNumber(novelId)` 三个 query。entry CRUD（create/update/delete/handleQuickStatus PATCH 单字段）+ plan CRUD（handleSavePlan）各一组 mutation。handleQuickStatus 全量回传（§6，同 storyarc handleQuickNodeStatus）。

**手测点**：entry/plan CRUD 后列表同步；focusEntryId 联动；章节窗口前/后翻；快速状态切换。

**commit**：同 4.1 拆 4 个（timeline 跳过 store commit，因 TimelineList 侧边栏只读无跨组件 state，按规则 10 不建 store，同 storyarc）。

**进度**：
- [x] commit 1: useTimelineEntries + useChapterPlans query（复用 useMaxChapterNumber）+ TimelineView/TimelineList 改造（删 load() 三件套，改用 query data；CRUD 后由 bumpRefresh → refreshNonce → invalidateQueries 刷新，commit 2/3 改 mutation 后改 onSuccess invalidate；useApp/useRefresh 暂保留供 CRUD handler 过渡）+ 中间件映射（queryErrorToast 启用 timeline + chapter-plans）+ i18n 补 chapterPlansLoadFailed + queryKeys.ts 新增 chapterPlanKeys + 00-conventions.md §1.2 同步
- [x] commit 2: useDeleteTimelineEntry mutation + confirmDelete 改 mutateAsync（deleting 由 mutation.isPending 推导，删 setDeleting useState + bumpRefresh；onSuccess 失效 timeline；useApp/useRefresh/saving 暂保留供 commit 3）
- [ ] commit 3: useCreate/UpdateTimelineEntry mutation（含 handleQuickStatus 全量回传，§6）+ useSaveChapterPlan mutation（plan CRUD）+ saving 由 mutation.isPending 推导 + 删 bumpRefresh/useRefresh

---

## 4.5 reader

**改动文件**：`frontend/src/components/reader/ReaderView.tsx`

**怎么做**：`useReaderPerspectives(novelId)` query + CRUD mutation。

**手测点**：perspective CRUD 同步。

**commit**：同 4.1 拆 4 个。

---

## 4.6 preference

**特殊点**：preference 有 `is_global` 字段区分全局/小说级。queryKey 用 `preferences` + novelId（小说级）或全局。注意后端 PATCH 语义（参考 project_memory：Category/Content 必填值类型，IsGlobal 指针类型 omitempty）。

**改动文件**：`frontend/src/components/preference/PreferenceList.tsx`、PreferenceView

**怎么做**：`usePreferences(novelId)` query。mutation 处理 is_global 归属（全局时 novelId=0）。迁移后 PreferenceList 的 bumpRefresh 删除。

**手测点**：全局/小说级 preference CRUD 同步。

**commit**：同 4.1 拆 4 个。

---

## 4.7 novel-setting

**改动文件**：`frontend/src/components/novel-setting/NovelSettingView.tsx`

**怎么做**：`useNovelSettings(novelId)` query + CRUD mutation。模式同 preference。

**手测点**：setting CRUD 同步。

**commit**：同 4.1 拆 4 个。

---

## 4.8 删除 useRefresh / refreshNonce 整套机制

**目标**：所有领域迁移完后，refreshNonce 已无消费方，整体删除。

**前置条件**：4.1-4.7 全部完成，确认无组件再调 bumpRefresh。

**改动文件**：删 `frontend/src/hooks/useRefresh.ts`；改 `frontend/src/views/WorkspaceView.tsx`（删 RefreshContext.Provider L434 + L119-124 的 refreshNonce state）；删各组件残留的 `useRefresh` import。

**怎么做**：
- 全局搜 `useRefresh` / `refreshNonce` / `bumpRefresh`，确认无引用（之前各领域迁移时已逐步删，本步扫尾）。
- 删 useRefresh.ts 文件。
- WorkspaceView 删 Provider 包裹 + 相关 state。

**验证**：build + lint + test 全绿，确认无残留引用。

**风险**：低（前提是 4.1-4.7 迁干净）。

**手测点**：各领域 CRUD 后兄弟组件自动同步（靠 invalidateQueries，不靠 nonce）。

**commit**：`refactor(frontend): remove refreshNonce mechanism superseded by query invalidation`

---

## 阶段 4 完成标准

- 7 个实体领域全部四件套化
- refreshNonce / useRefresh 整套删除（16 个消费点清零）
- 所有 List/View 数据靠 query 缓存自动同步
- 现有测试全绿
- 手测各领域 CRUD + 兄弟组件同步

完成后进入 [05-misc-modules.md](./05-misc-modules.md)。
