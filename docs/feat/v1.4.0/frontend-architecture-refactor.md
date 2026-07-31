# 前端架构改造设计

> **调查稿**（2026-07-31，激进版）
> 目标：解决以 `WorkspaceView` 巨石组件为核心的 UI 状态膨胀问题，**引入 Zustand（UI 状态）+ TanStack Query（数据获取）双库**，分阶段重构，提升可维护性与 AI 协作清晰度。本文不涉及后端改动，所有方案均为前端纯重构或增量基建。

## 背景与动机

Goink 前端（React 19 + TypeScript + Tailwind 4 + shadcn/ui）目前功能完整，但随着面板和实体类型持续增加，几个结构性问题开始反复发作：

1. **`WorkspaceView` 膨胀到 786 行、34 个 `useState`**，承担 7 类互不相干的职责，每次加功能都要在这个文件里找透传链
2. **`activePanel` 用字符串字面量散落全文件**，新增面板时极易漏改否定链
3. **8 个 `*FocusId` 状态 + switch 分支**，搜索导航是关键路径但实现脆弱
4. **兄弟组件数据不同步靠 `refreshNonce` bump 数字兜底**，15 个文件在用这个反模式
5. **核心路径零测试**，任何重构都靠手动 `wails dev` 点遍验证

这些问题里，1-3 是高频痛点（每次加功能都痛），4 是低频但扩散面广，5 是阻塞所有重构的基建缺口。本文给出**激进方案**：UI 状态和数据获取两个层面同时引入库，一次性根治。

## 现状调查

### 1. WorkspaceView 巨石组件

文件：[frontend/src/views/WorkspaceView.tsx](file:///home/nianhe/projects/todo/frontend/src/views/WorkspaceView.tsx) — 786 行，34 个 `useState`。

职责混杂（7 类）：

| 职责 | 涉及代码 | 行数估算 |
|---|---|---|
| 窗口壳（最小化/最大化/关闭、平台检测、拖拽、内联 SVG） | [L481-L561](file:///home/nianhe/projects/todo/frontend/src/views/WorkspaceView.tsx#L481-L561) | ~80 |
| 主题切换 | [L107](file:///home/nianhe/projects/todo/frontend/src/views/WorkspaceView.tsx#L107) + header 按钮 | ~20 |
| 小说 CRUD（创建/编辑/删除/导入/导出/封面） | L170-L422 多个 handler | ~150 |
| 审批流桥接（approve/reject/diff） | L211-L232 | ~40 |
| 搜索导航（entity 跳转 + chapter 高亮） | L269-L320 | ~50 |
| 面板路由（activePanel if-else 渲染） | L622-L721 | ~100 |
| 8 个 FocusId + 7 个对话框开关 + RefreshContext | L80-L135 | ~50 |

**最突出的坏味道**：

**(1) activePanel 否定链**（[L635-L644](file:///home/nianhe/projects/todo/frontend/src/views/WorkspaceView.tsx#L635-L644)）

```tsx
activePanel !== "characters" &&
activePanel !== "locations" &&
activePanel !== "storyarcs" &&
activePanel !== "timeline" &&
activePanel !== "reader" &&
activePanel !== "preferences" &&
activePanel !== "novel-settings" &&
activePanel !== "profile" &&
activePanel !== "git" &&
activePanel !== "style-samples" && (...)
```

**(2) FocusId 8 连发**（[L80-L89](file:///home/nianhe/projects/todo/frontend/src/views/WorkspaceView.tsx#L80-L89)），`handleSearchNavigateEntity` 里 8 分支 switch（[L269-L300](file:///home/nianhe/projects/todo/frontend/src/views/WorkspaceView.tsx#L269-L300)）。

**(3) 嵌套三元渲染链**（[L670-L721](file:///home/nianhe/projects/todo/frontend/src/views/WorkspaceView.tsx#L670-L721)）。

**(4) `flushSync` 逃生舱**（[L309](file:///home/nianhe/projects/todo/frontend/src/views/WorkspaceView.tsx#L309)）——状态模型没想清楚就硬塞。

**(5) 重复的"切小说"逻辑**——4 个 handler 重复同一段重置（`setActiveNovelId + setActivePanel("chapters") + closeAllTabs + setTabTarget(null) + setActiveContent("") + setSelectedGitFile(null) + app.SetActiveNovel`）。

### 2. 数据获取层：手写 fetch + refreshNonce

全项目**无数据获取库**，所有数据靠组件各自 `useState + useEffect` 手拉。

**关键数据消费点调查**（决定 queryKey 设计和迁移优先级）：

| 数据 | 消费点 | 共享度 | 当前重复 fetch |
|---|---|---|---|
| `GetNovels` | WorkspaceView / StyleView / PatternExtractView / GeneralConfigTab | **高（4 处）** | 4 处各 fetch，无共享 |
| `GetCharacters` | CharacterList + CharacterGraph | 中（2 处） | List 和 Graph 各 fetch |
| `GetChapters` | ChapterList + PatternExtractView | 中（2 处） | 各 fetch |
| `GetLocations` / `GetStoryArcs` / `GetTimelineEntries` 等 | 各自 List + View | 中（2 处） | 各 fetch |
| `GetSessions` / `GetSessionMessages` | ChatPanel | 低（1 处，但事件驱动） | — |

**refreshNonce 扩散面**：15 个文件消费 `useRefresh` / `refreshNonce` / `bumpRefresh`，包括 [CharacterList](file:///home/nianhe/projects/todo/frontend/src/components/character/CharacterList.tsx) / [CharacterListView](file:///home/nianhe/projects/todo/frontend/src/components/character/CharacterListView.tsx) / [PreferenceList](file:///home/nianhe/projects/todo/frontend/src/components/preference/PreferenceList.tsx) / [TimelineList](file:///home/nianhe/projects/todo/frontend/src/components/timeline/TimelineList.tsx) / [ArcListView](file:///home/nianhe/projects/todo/frontend/src/components/storyarc/ArcListView.tsx) / [LocationList](file:///home/nianhe/projects/todo/frontend/src/components/location/LocationList.tsx) / [ReaderView](file:///home/nianhe/projects/todo/frontend/src/components/reader/ReaderView.tsx) 等。

机制本身分两层：[useRefresh.ts](file:///home/nianhe/projects/todo/frontend/src/hooks/useRefresh.ts) 全文 22 行，只定义 `RefreshContext` 与 `useRefresh` hook，**不持有任何 state**；真正的 state 持有者是 WorkspaceView 作为 Provider，在 [WorkspaceView.tsx L119-L120](file:///home/nianhe/projects/todo/frontend/src/views/WorkspaceView.tsx#L119-L120)：

```tsx
// useRefresh.ts（仅 Context 定义，无 state）
export const RefreshContext = createContext<RefreshState>(defaultState);
export function useRefresh(): RefreshState { return useContext(RefreshContext); }

// WorkspaceView.tsx L119-L120（Provider 持有 state）
const [refreshNonce, setRefreshNonce] = useState(0);
const bumpRefresh = useCallback(() => setRefreshNonce((n) => n + 1), []);
```

**典型手写 fetch 样板**（[CharacterListView.tsx L55-L77](file:///home/nianhe/projects/todo/frontend/src/components/character/CharacterListView.tsx#L55-L77)）：

```tsx
const [characters, setCharacters] = useState<character.Character[]>([]);
const [loading, setLoading] = useState(false);
const [loadFailed, setLoadFailed] = useState(false);

const load = useCallback(async () => {
  setLoading(true);
  setLoadFailed(false);
  try {
    const list = await app.GetCharacters(novelId);
    setCharacters(list ?? []);
  } catch (err) {
    setLoadFailed(true);
    toastError(...);
  } finally {
    setLoading(false);
  }
}, [app, novelId, t]);

useEffect(() => { load(); }, [load, refreshNonce]);
```

全项目 **26 处 loading state**，20+ 个组件重复这个三件套。CharacterListView 一个组件就管 9 个 state（characters + loading + loadFailed + viewTab + editMode + form + saving + deleteTarget + deleting）。

### 3. 状态管理：零库，纯 useState

全项目**无客户端状态管理库**，唯一的 Context 是 `RefreshContext`（且只为 refreshNonce 服务）。

[useApp.ts](file:///home/nianhe/projects/todo/frontend/src/hooks/useApp.ts) 名字像 store，实际只是把 100+ 个 Wails 绑定函数包进 `useMemo([])`，是 import 聚合，不是状态管理。

后果：所有 UI 状态（面板切换、对话框开关、焦点 id、tab 目标）都堆在 `WorkspaceView` 的 34 个 `useState` 里，靠 props 层层透传。`SidePanel` 接收 20+ 个 props。

### 4. 测试覆盖

全前端 **10 个测试文件**：

| 测试文件 | 覆盖对象 |
|---|---|
| [CharacterList.test.tsx](file:///home/nianhe/projects/todo/frontend/src/components/character/CharacterList.test.tsx) | 角色列表 |
| [ChapterList.test.tsx](file:///home/nianhe/projects/todo/frontend/src/components/sidebar/ChapterList.test.tsx) | 章节列表 |
| [LocationList.test.tsx](file:///home/nianhe/projects/todo/frontend/src/components/location/LocationList.test.tsx) | 地点列表 |
| [SkillList.test.tsx](file:///home/nianhe/projects/todo/frontend/src/components/skill/SkillList.test.tsx) | 技能列表 |
| [ContentPanel.test.tsx](file:///home/nianhe/projects/todo/frontend/src/components/content/ContentPanel.test.tsx) | 内容面板 |
| [StyleView.test.tsx](file:///home/nianhe/projects/todo/frontend/src/components/style/StyleView.test.tsx) | 风格视图 |
| [chat/types.test.ts](file:///home/nianhe/projects/todo/frontend/src/components/chat/types.test.ts) | Chat 类型工具 |
| utils 下 3 个 | cn / error / toast |

**未覆盖的关键路径**：`WorkspaceView` 面板切换、搜索导航、审批桥接；`ChatPanel` 事件队列、turn 重建；`ArcListView` / `TimelineView` 等巨石视图；审批链路；搜索导航（含 `flushSync` 路径）。

pre-commit hook 会跑 `npm run build` / `lint` / `test`，能挡编译和 lint 错误，但**挡不了行为回归**。

### 5. 其他巨石组件

| 文件 | 行数 | 主要问题 |
|---|---|---|
| [ChatPanel.tsx](file:///home/nianhe/projects/todo/frontend/src/components/chat/ChatPanel.tsx) | 1532 | 自维护 `EventQueue` + 重排定时器 + drag resize + session 切换 + 审批 UI + diff tab 触发，27 个 useEffect/useCallback |
| [ArcListView.tsx](file:///home/nianhe/projects/todo/frontend/src/components/storyarc/ArcListView.tsx) | 1129 | List + 编辑表单 + 图 三合一 |
| [TimelineView.tsx](file:///home/nianhe/projects/todo/frontend/src/components/timeline/TimelineView.tsx) | 934 | 同上 |
| [ContentPanel.tsx](file:///home/nianhe/projects/todo/frontend/src/components/content/ContentPanel.tsx) | 835 | `forwardRef + useImperativeHandle` 暴露 8 个方法，父组件命令式驱动子组件 |

### 6. 事件订阅点

`EventsOn` 订阅分布（影响 query 失效策略）：

| 订阅点 | 事件 | 用途 |
|---|---|---|
| [ChapterList.tsx L60](file:///home/nianhe/projects/todo/frontend/src/components/sidebar/ChapterList.tsx#L60) | `file:changed` | 章节文件变更时重载列表 |
| [ContentPanel.tsx L353](file:///home/nianhe/projects/todo/frontend/src/components/content/ContentPanel.tsx#L353) | `file:changed` | 编辑器内容同步 |
| [ChatPanel.tsx L1077/L1094](file:///home/nianhe/projects/todo/frontend/src/components/chat/ChatPanel.tsx#L1077) | chat 事件 | 流式消息接收 |
| [useImportNovel.ts L68](file:///home/nianhe/projects/todo/frontend/src/hooks/useImportNovel.ts#L68) | import 进度 | 导入进度更新 |
| [usePatternProgress.ts L108](file:///home/nianhe/projects/todo/frontend/src/hooks/usePatternProgress.ts#L108) | pattern 进度 | 模式提取进度 |

`file:changed` 事件应配对 `invalidateQueries(['chapter', path])`；chat 流式数据**不走 query 缓存**（流式数据不适合缓存），保持本地 state。

## 痛点评估与优先级

| 痛点 | 发作频率 | 现状危害 | 阻塞性 | 优先级 |
|---|---|---|---|---|
| WorkspaceView 巨石 | 每次加功能 | AI 写新功能迷路、props 透传链长 | 是 | P0 |
| activePanel 字符串 + 否定链 | 加面板时 | 易漏改，潜在 bug | 是 | P0 |
| 核心路径零测试 | 每次重构 | 任何改动靠手测，不敢动 | 是（阻塞重构） | P1 |
| FocusId 8 state + switch | 搜索导航时 | 脆弱但能用 | 否 | P0（顺带） |
| 数据层手写 fetch + refreshNonce | 数据同步时 | 15 文件反模式，重复 fetch | 否 | P3 |
| ContentPanel imperativeHandle | 审批时 | 能用，flushSync 是隐患 | 否 | P4 |
| ChatPanel/ArcListView 巨石 | 改这些模块时 | 局部痛 | 否 | P4 |

## 库选择

### UI 状态：Zustand

| 候选 | 选择理由 |
|---|---|
| **Zustand ✅** | API 极简（`create((set) => ({...}))`），AI 写错率低，bundle ~1KB，selector 订阅避免重渲染 |
| Jotai | 原子化，但项目派生状态不多，Zustand 更直接 |
| Redux Toolkit | 过重，devtools 时间旅行对桌面应用无价值 |
| Valtio | proxy 风格小众，AI 写错概率高 |

### 数据获取：TanStack Query

| 候选 | 选择理由 |
|---|---|
| **TanStack Query ✅** | mutation 语义完整（`useMutation` + `invalidateQueries`），devtools 强，生态成熟，selector 缓存精细 |
| SWR | 更轻量但 mutation 能力弱，`mutate` 语义不如 `invalidateQueries` 明确 |
| RTK Query | 必须先上 Redux，过重 |

**Wails 适配说明**：RPC 函数不是 URL，queryKey 完全靠人工设计。这是引入 Query 的主要风险，通过下文 queryKey 规范 mitigate。

## 耦合根因与解耦本质

### 之前为什么耦合（不是设计失误，是 React 无 store 时代的必然）

| 根因 | 说明 |
|---|---|
| **演进出来** | 早期 WorkspaceView 可能就 100-200 行，小说选择/创建是早期核心功能，放在唯一应用组件里合理。面板从 3 个加到 12 个，小说代码没挪走，一直堆着 |
| **切小说需协调多方** | `switchNovel` 要同时改 `activeNovelId`（state）+ ContentPanel tabs（命令式 `contentRef.current?.closeAllTabs()`）+ 后端 `SetActiveNovel`。无 store 时代，`contentRef` 只有 WorkspaceView 拿得到，**协调者只能落在 WorkspaceView** |
| **共享状态只能往上提** | React 数据流单向，`activeNovelId` 被 SidePanel / ContentPanel / 各 View 共享，无 store 时**共享状态只能提到共同父级**。WorkspaceView 是所有面板的共同父级，状态只能堆这里。**任何无 store 的 React 应用，共享状态都会往父级提，最终堆出巨石组件** |
| **对话框触发与渲染不在同层** | 编辑小说按钮在 SidePanel 的 NovelList 里，对话框渲染在 WorkspaceView 层级。无 store 时，对话框开关状态只能放 WorkspaceView，由 SidePanel 回调触发。props 单向流逼出来的 |

### 现在为什么能解耦（库解决了"状态只能往上提"的根本限制）

| 之前的限制 | 现在的解法 |
|---|---|
| 共享状态只能提到共同父级 | **Zustand**：任何组件 `useStore()` 直接订阅，状态不必往上提 |
| 数据要父组件 fetch 后透传 | **TanStack Query**：任何组件 `useQuery()` 共享缓存，不必父级 fetch |
| CRUD 副作用在父组件写 try/catch/loading | **useMutation**：封装完整，逻辑归位 |
| 命令式 ref 协调子组件 | **声明式订阅**：子组件订阅 store 变化自行响应 |

**解耦的本质**：从"父组件命令式协调子组件"变成"各组件订阅共享状态自行响应"。这是**范式转变**，不是简单挪代码。

### switchNovel 解耦示例

```tsx
// 之前：必须在 WorkspaceView，因为它有 contentRef + activeNovelId state
async function switchNovel(id) {
  setActiveNovelId(id);                    // WorkspaceView 的 state
  contentRef.current?.closeAllTabs();      // 只有它有 contentRef
  await app.SetActiveNovel({ novel_id: id });
}

// 现在：store action + ContentPanel 订阅响应
// useNovelStore
switchNovel: async (id) => {
  set({ activeNovelId: id });               // store 状态
  await app.SetActiveNovel({ novel_id: id });
}

// ContentPanel 自己订阅，自动响应（声明式，干掉 flushSync 和命令式 ref）
const activeNovelId = useNovelStore((s) => s.activeNovelId);
useEffect(() => {
  if (prevRef.current !== activeNovelId) {
    prevRef.current = activeNovelId;
    closeAllTabs();                         // 自动清空
  }
}, [activeNovelId]);
```

## 目标架构：领域自治 + WorkspaceView 纯壳

### 设计原则

1. **领域自治**：每个实体领域（小说/角色/地点/...）自己管数据（query）+ 状态（store）+ 副作用（mutation）+ 对话框，不依赖 WorkspaceView
2. **WorkspaceView 纯壳**：只负责布局（header / activitybar / sidebar / content / chat 的排布）和面板路由，不管任何业务 CRUD
3. **统一模式**：所有领域走同一套模式（query + store + mutation + dialogs），AI 写新领域按模板套，不堆 state 到 WorkspaceView

### 领域统一模式模板

每个领域 = 4 件套：

```tsx
// 1. 数据层：useXxxList query + 单个 useXxx query
export function useNovels() {
  return useQuery({ queryKey: ['novels'], queryFn: () => app.GetNovels() });
}

// 2. 状态层：useXxxStore（activeId + 对话框开关 + 协调 action）
export const useNovelStore = create<NovelState>((set) => ({
  activeNovelId: 0,
  editingNovel: null,
  deletingNovel: null,
  showCreateDialog: false,
  switchNovel: async (id) => { set({ activeNovelId: id }); await app.SetActiveNovel(...); },
  openEditDialog: (n) => set({ editingNovel: n }),
  // ...
}));

// 3. 副作用层：useCreateXxx / useUpdateXxx / useDeleteXxx mutation
export function useCreateNovel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input) => app.CreateNovel(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['novels'] }),
  });
}

// 4. UI 层：<XxxDialogs> 组件消费 store 状态
function NovelDialogs() {
  const { editingNovel, deletingNovel, showCreateDialog } = useNovelStore();
  return (<>
    <NovelEditDialog open={showCreateDialog} ... />
    <NovelEditDialog open={!!editingNovel} novel={editingNovel} ... />
    <NovelDeleteDialog open={!!deletingNovel} ... />
  </>);
}
```

### WorkspaceView 纯壳后的样子

```tsx
// WorkspaceView.tsx（改造后，~150 行）
export default function WorkspaceView() {
  const activePanel = usePanelStore((s) => s.activePanel);
  const focusMap = useFocusStore((s) => s.focusMap);
  // 不再有 novels / activeNovelId / 对话框 state / CRUD handler

  return (
    <div className="h-screen flex flex-col">
      <Header>
        <WindowControls />
        <HeaderToolbar />
      </Header>

      <div className="flex-1 flex">
        <ActivityBar />
        <SidePanel />          {/* 自己从 store/query 取数据 */}
        <PanelRouter />        {/* 按 activePanel 渲染，各 View 自己取数据 */}
        <ChatPanel />          {/* 自己从 store/query 取数据 */}
      </div>

      <StatusBar />

      {/* 对话框归小说领域，由 useNovelStore 控制 */}
      <NovelDialogs />        {/* 内部消费 useNovelStore 的对话框状态 */}
    </div>
  );
}
```

**小说 CRUD 完全不在 WorkspaceView**：
- 数据：`useNovels` query（4 处消费方共享）
- 状态：`useNovelStore`（activeNovelId + 对话框开关 + switchNovel action）
- 逻辑：`useCreateNovel` 等 mutation
- 对话框：`<NovelDialogs>` 组件，消费 store 状态
- 切小说响应：ContentPanel 订阅 `activeNovelId` 变化自动重置 tabs

### 改了之后好在哪里

| 好处 | 说明 |
|---|---|
| 语义清晰 | WorkspaceView = 布局，小说领域 = 自治。看代码知道去哪找 |
| 新功能不再堆 WorkspaceView | 加新实体直接套模板，WorkspaceView 行数不会再膨胀 |
| 数据共享自动同步 | mutation 后 invalidateQueries，所有消费方自动刷新，消掉 refreshNonce |
| 测试边界清晰 | 领域逻辑可单独测 store + mutation，不用渲染整个 WorkspaceView |
| 切小说变声明式 | 干掉 flushSync 和命令式 ref，副作用通过订阅响应 |

### 但也要注意的风险

| 风险 | 缓解 |
|---|---|
| 引入隐性 bug（缓存不失效/订阅未触发） | queryKey 规范 + 测试兜底 |
| 迁移期两套模式并存混乱 | 分批迁移，每批快速合并 |
| 调试链路变长（state→store→subscription→re-render） | devtools + 日志 |
| 学习成本 | Zustand/Query API 简单，但需熟悉 |

**方向是对的，执行决定成败**。对 Goink 这种还在活跃开发、还会加新功能的项目，改了更好——新功能不再堆 WorkspaceView，收益真实。但必须分步 + 测试兜底 + 每步验证手感。

## useApp 模式分析与废弃路径

### 现状

[useApp.ts](file:///home/nianhe/projects/todo/frontend/src/hooks/useApp.ts) 227 行，把 100+ 个 Wails 绑定函数包进 `useMemo(() => ({...}), [])` 返回，顺带 re-export 类型。名字像 store，实际是 import 聚合 + 引用稳定层。

```tsx
export function useApp() {
  return useMemo(() => ({
    CancelChat, Chat, GetNovels, GetCharacters, // ... 100+ 函数
  }), []);
}
```

### useMemo 的历史真相（不是多余的）

> 调查方法：`git blame` + `git log -S "useMemo"` 追溯到 commit `94ae137`（2026-06-05）

commit message 明确写道：

> **"Make useApp() return a stable reference via useMemo to avoid excessive useEffect re-subscriptions causing dropped events"**

**useMemo 是修 bug 留下的，不是性能优化**。真实问题链：

1. 当时拆分 ContentPanel，ChapterList 和 ContentPanel 各自监听 `file:changed` 事件
2. 这些组件的 `useEffect` 依赖 `app` 对象（来自 useApp）
3. 如果 `useApp()` 每次返回新对象（`return { ... }` 会这样），`app` 引用每次变
4. → `useCallback` 的 `load` 函数每次变 → `useEffect` 重新执行 → EventsOn 重新订阅
5. → 旧 unsub + 新 sub 之间有时间窗口，**可能丢事件**
6. 用 `useMemo(() => ({...}), [])` 让 `app` 引用稳定，避免重订阅

**结论：useMemo 有真实作用，不能简单删掉。**

### 根因：useEffect 依赖 app 对象粒度太粗

useMemo 是治标不治本。根因在组件侧：

```tsx
// CharacterList.tsx 当前模式
const load = useCallback(async () => {
  const list = await app.GetCharacters(novelId);  // 用 app
}, [app, novelId]);  // ← app 作为依赖，粒度太粗

useEffect(() => { load(); }, [load, refreshNonce]);  // 间接依赖 app
```

`app` 是 100+ 函数的大对象，作为依赖粒度太粗。正确做法应该是依赖具体函数（ES 模块 import 是永久稳定引用）：

```tsx
// 方案 A：直接 import，不依赖 app 对象
import { GetCharacters } from "@/lib/wailsjs/go/app/App";
const load = useCallback(async () => {
  await GetCharacters(novelId);
}, [novelId]);  // 不依赖 app
```

### 引入 Query 后，问题从根上消失

引入 TanStack Query 后，组件不再写 `useEffect + 依赖 app`：

```tsx
// useQuery 内部管理依赖，不依赖 app 对象
const { data: characters } = useQuery({
  queryKey: ['characters', novelId],
  queryFn: () => GetCharacters(novelId),  // 直接 import
});

// EventsOn 订阅也不依赖 app
useEffect(() => {
  const unsub = EventsOn("file:changed", () => {
    qc.invalidateQueries({ queryKey: ['chapters', novelId] });
  });
  return unsub;
}, [novelId, qc]);  // 依赖 novelId 和 queryClient
```

**useQuery 接管数据获取，useEffect 不再依赖 app 对象**，重订阅丢事件问题从根上消失。useApp 和它的 useMemo 都不再需要。

### 废弃路径（随 P3 自然废弃，零额外成本）

**关键澄清**：废弃 useApp **没有额外成本**。引入 Query 的正常改造走完，useApp 自然就没用了。所谓"改 EventsOn 订阅依赖"不是额外工作，是引入 Query 后的自然结果——`loadXxx` 函数被 useQuery 替代后不存在了，EventsOn 里自然改成 `qc.invalidateQueries()` 刷新缓存，这是 Query 的标准用法。

**唯一要注意的是按顺序改，避免"改一半"的中间状态**：

| 阶段 | useApp 处理 | EventsOn 订阅 | 说明 |
|---|---|---|---|
| 阶段 1（基建） | **保留不动** | 不动 | useApp 还在，useMemo 继续保护 |
| 阶段 1（基建） | 新代码直接 import Wails 函数 | — | query/store 不需要 useApp |
| 阶段 1（基建） | 类型独立到 `@/types/wails` | — | 解耦类型和函数聚合 |
| 阶段 2-5（P3 迁移） | 组件迁移到 useQuery 时自然不再调 `useApp()` | `loadXxx()` → `qc.invalidateQueries()`（引入 Query 的自然结果） | 迁移一个少一个 |
| 阶段 5 后 | **删除 useApp.ts** | 已全部改成 `qc` 依赖，不再依赖 app | 所有消费方已迁移完 |

**顺序纪律**（不是额外工作，是改造顺序）：
1. 先改 EventsOn 用 `invalidateQueries`（此时 loadXxx 还在但不再被调用）
2. 再改数据获取删 loadXxx（改成 useQuery）
3. 最后删 useApp

**为什么不能跳过顺序**：如果在 EventsOn 还依赖 `loadXxx`（间接依赖 app）时就删 useApp，app 引用变 → loadXxx 变 → EventsOn 重订阅 → 丢事件 bug 重现。但只要按顺序改，每步完成再下一步，就不会有这个危险中间状态。

### 废弃动机

保留 useApp 有反复维护成本：**每次 wails 生成新绑定都要手动改 useApp.ts**（加 import + 加到 useMemo 对象），容易忘、烦。wails 官方推荐直接 import 函数调用，useApp 是项目自加的便利层。项目还在加后端方法，这个成本会累积，所以废弃 useApp 长期更划算。

### 教训

代码里看似多余的东西（如 useMemo 包静态 import），往往是修某个 bug 留下的。**下结论前先 `git blame` 调查历史**，避免误判。本次调查修正了"useMemo 毫无意义"的错误判断——它有真实作用（修 EventsOn 重订阅丢事件），只是根因在组件侧的依赖模式。引入 Query 后，组件不再依赖 app 对象，根因消失，useApp 和 useMemo 自然可删。

## 改造方案

### P0：拆 WorkspaceView 巨石 + 类型化

**目标**：786 行 → ~400 行，消掉高频痛点，纯重构行为不变。

#### P0.1 activePanel 联合类型 + record 映射

```tsx
type PanelId =
  | "novels" | "chapters" | "characters" | "locations"
  | "storyarcs" | "timeline" | "reader" | "preferences"
  | "novel-settings" | "profile" | "git" | "style-samples";

const PANEL_RENDERERS: Record<PanelId, () => JSX.Element> = {
  novels: () => <BookshelfView {...} />,
  characters: () => <ErrorBoundary><CharacterListView {...} /></ErrorBoundary>,
  // ...
};

const CONTENT_PANEL_IDS = new Set<PanelId>(["chapters", "skills", "git"]);
// 默认走 ContentPanel 的面板，用 Set 判断，消掉否定链
```

#### P0.2 拆 WindowControls / HeaderToolbar 组件

把 [L481-L561](file:///home/nianhe/projects/todo/frontend/src/views/WorkspaceView.tsx#L481-L561) 80 行内联 SVG 抽成 `<WindowControls>`。

#### P0.3 switchNovel 抽函数

4 个重复 handler 收敛为一个：

```tsx
const switchNovel = useCallback(async (id: number) => {
  setActiveNovelId(id);
  setActivePanel("chapters");
  contentRef.current?.closeAllTabs();
  setTabTarget(null);
  setActiveContent("");
  setSelectedGitFile(null);
  await app.SetActiveNovel({ novel_id: id });
}, [app]);
```

#### P0.4 FocusId 对象化

8 个 state → 1 个：

```tsx
type FocusMap = Partial<Record<PanelId, number>>;
const [focusMap, setFocusMap] = useState<FocusMap>({});

function focusEntity(panelId: PanelId, entityId: number) {
  setFocusMap({ [panelId]: entityId });
}
```

**预期**：WorkspaceView 786 → ~400 行，加面板成本从"改 5+ 处字符串"降到"改类型 + record 2 处"。

**风险**：低。纯重构，行为不变，pre-commit 兜底编译。需手动验证面板切换、搜索导航。

### P1：补关键路径测试 + 测试原则

**目标**：为后续所有重构兜底，零风险（只加测试不改代码）。

#### 测试原则（核心）

> **测行为不测实现。** 这是测试能否跨越重构存活的唯一标准。

**三层区分**：

| 测试类型 | 何时写 | 寿命 |
|---|---|---|
| **纯行为测试**（用户输入 → 可观察输出） | **现在写** | 重构无关，永久有效 |
| **纯函数单元测试**（`rebuildTurns` 等工具） | **现在写** | 与 UI 架构无关 |
| **架构相关测试**（store 订阅、query 缓存） | **重构后写** | 架构稳定后再补 |
| **实现细节测试**（内部 state 值、props 数量） | **永远不写** | 必然失效，无价值 |

**实操三原则**：

1. **测"用户能看到什么"，不测"组件内部状态是什么"**
   - ✅ 用 `getByRole` / `getByText` / `getByLabelText`
   - ❌ 不用 `container.querySelector` 读 DOM 结构、不直接读组件 state

2. **测"调用什么 API"，不测"状态怎么流转"**
   - ✅ mock `useApp` 返回的 Wails 函数，断言"被调用了什么参数"
   - ❌ 不断言中间状态机的值

3. **测"组件渲染了什么"，不测"组件接收了什么 props"**
   - ✅ 断言 DOM 里出现了 `CharacterListView` 的内容
   - ❌ 不断言 `<SidePanel activePanel={...}>` 的 prop 值

#### 测试用例示例

```tsx
// ✅ 行为测试：点 characters 面板 → 主区渲染对应组件
test("切换到 characters 面板渲染角色列表", async () => {
  render(<WorkspaceView />);
  await userEvent.click(screen.getByLabelText("角色"));
  expect(screen.getByText("角色列表")).toBeInTheDocument();
});

// ✅ 行为测试：搜索实体并跳转
test("搜索实体并跳转到对应面板", async () => {
  render(<WorkspaceView />);
  await userEvent.click(screen.getByLabelText("搜索"));
  await userEvent.type(screen.getByPlaceholderText("搜索"), "张三");
  await userEvent.click(screen.getByText("张三"));
  expect(screen.getByText("角色列表")).toBeInTheDocument();
});

// ✅ 行为测试：审批调用后端
test("approve 调用 ApproveTool 且 feedback 透传", async () => {
  const mockApprove = vi.fn().mockResolvedValue(undefined);
  render(<WorkspaceView />);
  await userEvent.click(screen.getByText("批准"));
  expect(mockApprove).toHaveBeenCalledWith(toolId, true, expect.any(String));
});

// ❌ 反例：断言内部 state（重构后必失效）
test("activePanel 初始值", () => {
  expect(result.current.activePanel).toBe("chapters"); // state 外移到 store 后失效
});
```

#### 覆盖范围

1. **WorkspaceView 面板切换**：每个 `PanelId` 渲染对应组件，切换不残留
2. **搜索导航**：entity 跳转设置对应 focusId；chapter 高亮走 `openFileWithHighlight`
3. **审批桥接**：`handleApprove` / `handleReject` 调用 `app.ApproveTool` 且触发 `contentRef` 对应方法
4. **switchNovel**：切换后状态重置齐全（tabs 清空、activeContent 清空等）

测试方式：Vitest + Testing Library，mock `useApp` 返回的 Wails 函数。

### P2：引入 Zustand 外置 UI 状态

**前置条件**：P0 完成（先拆清组件边界再提 store）。

按职责拆 store：

```tsx
// usePanelStore
interface PanelState {
  activePanel: PanelId;
  sidebarPanel: PanelId | null;
  sidebarClosed: boolean;
  setActive: (id: PanelId) => void;
  toggleSidebar: () => void;
}

// useFocusStore
interface FocusState {
  focusMap: Partial<Record<PanelId, number>>;
  focusEntity: (panelId: PanelId, id: number) => void;
  clear: () => void;
}

// useDialogStore
interface DialogState {
  showSettings: boolean;
  showHelp: boolean;
  editingNovel: novel.Novel | null;
  deletingNovel: novel.Novel | null;
  showCreateDialog: boolean;
  exportNovelId: number | null;
  // 各 setter
}
```

各组件 `useStore((s) => s.xxx)` 独立订阅，`SidePanel` 的 20+ 透传 props 大部分消失。

**props 保留原则**：透传进 store，直达留 props。`<WindowControls onMinimise={...}>` 这种纯父子局部仍用 props；`activePanel` 这种跨层共享进 store。

**预期**：WorkspaceView → ~200 行纯渲染逻辑。

**风险**：中。新依赖，但 API 简单、可逆性中。需在 P1 测试兜底下进行。

### P3：引入 TanStack Query 数据层

**前置条件**：P1 测试就位（数据层改动面大，必须有安全网）。

#### 迁移顺序（按共享度排序，收益大的先做）

| 批次 | 数据 | 消费点 | 共享度 | 收益 |
|---|---|---|---|---|
| 1 | `GetNovels` | 4 处 | **高** | 4 处共享缓存，消掉 4 次 fetch |
| 2 | `GetCharacters` | 2 处（List+Graph） | 中 | List 和 Graph 共享，消掉重复 fetch |
| 3 | `GetChapters` | 2 处 | 中 | 同上 |
| 4 | `GetLocations` / `GetStoryArcs` / `GetTimelineEntries` / `GetReaderPerspectives` / `GetPreferences` / `GetNovelSettings` | 各 2 处 | 中 | 同类批量 |
| 5 | `GetSessions` / `GetSessionMessages` | ChatPanel | 低 | 事件驱动，最后迁移 |
| 6 | `GetContent` / `SaveContent` | ContentPanel | 低 | 文件 I/O，配对 `file:changed` 事件 |

#### 迁移示例（以 GetNovels 为例）

**迁移前**（4 处各 fetch）：

```tsx
// WorkspaceView.tsx
const [novels, setNovels] = useState<novel.Novel[]>([]);
const loadNovels = useCallback(async () => {
  const list = await app.GetNovels();
  setNovels(list ?? []);
}, [app]);
useEffect(() => { loadNovels(); }, [loadNovels]);

// StyleView.tsx / PatternExtractView.tsx / GeneralConfigTab.tsx 各写一遍
```

**迁移后**（共享缓存）：

```tsx
// useNovels.ts
export function useNovels() {
  return useQuery({
    queryKey: ['novels'],
    queryFn: () => app.GetNovels(),
  });
}

// 4 处统一调用
const { data: novels = [], isLoading } = useNovels();

// mutation 后失效
const mutation = useMutation({
  mutationFn: (input) => app.CreateNovel(input),
  onSuccess: () => queryClient.invalidateQueries({ queryKey: ['novels'] }),
});
```

4 处共享同一份缓存，创建/编辑/删除小说后 `invalidateQueries(['novels'])` 自动全部刷新，**`refreshNonce` 机制可整体删除**。

#### queryKey 规范

> 严格按规范生成 key，避免 AI/人乱写导致缓存不共享或过度失效。

```tsx
// 实体列表（复数）
['novels']                              // 全局小说列表
['chapters', novelId]                   // 某小说章节列表
['characters', novelId]                 // 某小说角色列表
['character-relations', novelId]        // 角色关系图
['locations', novelId]
['location-relations', novelId]
['storyarcs', novelId]
['arc-nodes', arcId]                    // 子资源用父 id
['timeline', novelId]
['reader', novelId]
['preferences', novelId]
['novel-settings', novelId]
['style-samples', novelId]
['skills']                              // 全局

// 单个实体（单数）
['novel', id]
['chapter', filePath]                   // 章节内容按文件路径
['character', id]

// chat（部分走 query，流式不走）
['sessions', novelId]                   // session 列表可缓存
// session messages 流式数据不走 query，保持本地 state
```

**规则**：
- 实体名复数 = 列表，单数 = 单个
- 父级 id 作为第二段，子级 id 作为第三段
- mutation 后 `invalidateQueries({ queryKey: [实体名, ...] })`

#### EventsOn 配对

| 事件 | 失效策略 |
|---|---|
| `file:changed` | `invalidateQueries({ queryKey: ['chapter', path] })` + `invalidateQueries({ queryKey: ['chapters', novelId] })` |
| chat 事件（`chat:started` / `tool_call` 等） | **不走 query**，保持 ChatPanel 本地 state（流式数据不适合缓存） |
| import 进度 | **不走 query**，保持 `usePatternProgress` 本地 state |

### P4：拆巨石组件 / 去 imperativeHandle（可选，痛点驱动）

**前置条件**：P1 测试扩展覆盖到这些组件。

#### P4.1 拆 ChatPanel / ArcListView / TimelineView

各自 900-1500 行，拆分设计应单独成文。拆成 `<XxxList>` + `<XxxEditor>` + `<XxxGraph>` 三件套。

#### P4.2 去 ContentPanel imperativeHandle

把 `ContentPanelHandle` 的 8 个命令式方法改成声明式 tab store：

```tsx
interface TabState {
  tabs: EditorTab[];
  activeTabId: string | null;
  openFile: (path: string, title: string, opts?: OpenOpts) => void;
  openDiffTab: (data: DiffData) => void;
  closeAllTabs: () => void;
}
```

父组件 dispatch 事件，ContentPanel 订阅。能同时干掉 `flushSync`。

**风险**：高。审批关键路径，三方协议（WorkspaceView + ChatPanel + ContentPanel），必须有测试兜底。

## 风险评估

| 改动 | 回归风险 | 扩散范围 | 测试兜底 | 可逆性 | 建议 |
|---|---|---|---|---|---|
| P0.1 activePanel 类型化 | 低 | 中 | 无 | 高 | 可做，手测面板 |
| P0.2 拆 WindowControls | 低 | 小 | 无 | 高 | 最安全切入点 |
| P0.3 switchNovel 抽函数 | 低 | 小 | 无 | 高 | 安全 |
| P0.4 FocusId 对象化 | 中 | 小 | 无 | 高 | 手测搜索导航 |
| P1 补测试 | 零 | 零 | — | 高 | 无脑做 |
| P2 Zustand | 中 | 中 | 需 P1 | 中 | P0/P1 后做 |
| P3 TanStack Query | **高** | **大（全项目）** | **需 P1** | 中 | **分批迁移，每批独立验证** |
| P4.1 拆巨石组件 | 中-高 | 大 | 需 P1 扩展 | 中 | 先补测试 |
| P4.2 去 imperativeHandle | 高 | 中-大 | 不足 | 中 | 先补测试 |

**最大风险**：P3 数据层迁移。一次性铺开风险不可控，**必须分批**，每批一个模块，每批可独立验证和回退。

## 不建议现在做的工作

1. **引入路由库**（React Router 等）：桌面应用单页面，`activePanel` 类型化（P0.1）已足够。
2. **引入表单库**（React Hook Form）：当前表单复杂度未到痛阈，可后续按需引入。
3. **chat 流式数据走 query 缓存**：流式数据不适合缓存，保持本地 state。
4. **P4 在 P1 测试就位前动**：巨石组件拆分和 imperativeHandle 改造必须有测试兜底。

## 推进路线图

```
阶段 1：基建（零功能风险）
  ├─ P1 补关键路径测试（行为测试，不测实现）
  ├─ 引入 zustand + @tanstack/react-query 依赖
  ├— 制定 queryKey 规范（见上文）
  ├— QueryClientProvider 在 App.tsx 设置
  ├— useApp.ts 保留不动（避免破坏引用稳定性，见 useApp 章节历史真相）
  ├— 类型独立到 @/types/wails（解耦类型和函数聚合）
  └— 新写的 query/store 直接 import Wails 函数，不用 useApp
  → 安全网就位，新旧模式并存

阶段 2：拆 WorkspaceView + 外置 UI 状态（P0+P2）
  ├─ P0.1 activePanel 联合类型 + record 映射
  ├─ P0.2 拆 WindowControls / HeaderToolbar
  ├— P0.3 switchNovel 抽函数（过渡：暂留 WorkspaceView）
  ├— P0.4 FocusId 对象化
  └— P2 usePanelStore / useFocusStore（只外置面板/焦点状态）
  → WorkspaceView 786 → ~400 行

阶段 3：小说领域先行作为模板（P3 核心）
  ├— useNovels query（4 处消费方共享缓存）
  ├— useNovelStore（activeNovelId + 对话框开关 + switchNovel action）
  ├— useCreateNovel / useUpdateNovel / useDeleteNovel mutation
  ├— <NovelDialogs> 组件抽出
  ├— switchNovel 从 WorkspaceView 迁到 store action
  ├— ContentPanel 订阅 activeNovelId 变化自动重置 tabs（干掉 flushSync）
  └— 验证：小说 CRUD + 切小说 + 4 处消费方同步
  → 小说领域完全自治，WorkspaceView 不再管小说业务
  → 模板成型，后续领域套用

阶段 4：7 个实体领域套模板（P3 批量）
  ├— character（List+Graph 共享，收益大）
  ├— location / storyarc / timeline / reader / preference / novel-setting
  ├— 每个领域 = useXxxList + useXxxStore + useCreateXxx mutation + <XxxDialogs>
  └— 同构迁移，套阶段 3 模板
  → refreshNonce 机制整体删除

阶段 5：其他模块（P3 收尾）
  ├— chat（sessions 走 query，messages 流式保持本地 state）
  ├— content（GetContent/SaveContent，配对 file:changed 事件）
  ├— pattern / style / extract / skill / git
  └— search（跟随实体迁移）
  → 全项目统一模式

阶段 6：拆巨石组件（P4，痛点驱动）
  ├— P4.1 拆 ChatPanel / ArcListView / TimelineView（先补测试）
  └— P4.2 去 ContentPanel imperativeHandle（审批关键路径，需测试兜底）
```

### 路线图设计要点

1. **小说领域先行**（阶段 3）：作为模板验证整套模式（query + store + mutation + dialogs + 声明式订阅），做完验证手感后，阶段 4 套模板批量迁移
2. **switchNovel 迁移是阶段 3 的关键里程碑**：从 WorkspaceView 的命令式协调，变成 store action + ContentPanel 订阅响应，标志范式转变落地
3. **阶段 2 和阶段 3 分开**：阶段 2 只外置面板/焦点状态（usePanelStore/useFocusStore），小说状态等阶段 3 完整迁移，避免一次改太多
4. **阶段 4 同构批量**：7 个实体领域模式一致，套模板快速推进
5. **阶段 6 痛点驱动**：拆巨石和去 imperativeHandle 风险高，等真正痛了再动，且有测试兜底

## 验证策略

每个阶段完成后：

1. **pre-commit hook** 自动跑 `npm run build` / `lint` / `test`，挡编译和 lint 错误
2. **P1 测试**自动跑，挡行为回归
3. **手动验证**（`wails dev`）：
   - 面板切换：逐个点 ActivityBar 所有面板，确认渲染正确
   - 搜索导航：搜索 entity 和 chapter，确认跳转和高亮
   - 审批流：触发 AI 工具审批，确认 approve/reject/diff tab
   - 小说切换：切换/创建/导入/删除小说，确认 tab 和内容重置
   - 数据同步：在 View 编辑实体后，侧边栏 List 计数同步更新（验证 query 失效）
4. **回归对比**：重构前后用浏览器开发者工具对比关键操作的网络请求和渲染行为

## 一句话结论

**v1.4.0 前端改造目标：领域自治 + WorkspaceView 纯壳 + 统一模式**。引入 Zustand（UI 状态）+ TanStack Query（数据获取）双库，每个领域走 4 件套（query + store + mutation + dialogs）。核心路径：P0 拆 WorkspaceView 巨石 → P1 补行为测试 → P2 外置面板/焦点状态 → **P3 小说领域先行作为模板（switchNovel 迁移是范式转变里程碑）** → 7 实体领域套模板 → 其他模块收尾。测试原则：**测行为不测实现**。执行分步，每步可回退，小说领域验证手感后再铺开。
