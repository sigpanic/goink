# 阶段 2：拆 WorkspaceView + 外置 UI 状态

> 前置条件：[阶段 1](./01-foundation.md) 完成（尤其 1.4-1.7 测试就位）。
> 完成后：WorkspaceView 786 行 → ~400 行，消掉高频痛点，行为不变。
> 参考文件：`frontend/src/views/WorkspaceView.tsx`（当前 786 行）。

## 进度勾选

- [ ] 2.1 PanelId 联合类型
- [ ] 2.2 CONTENT_PANEL_IDS Set
- [ ] 2.3 替换 activePanel 类型 + 删否定链
- [ ] 2.4 抽 WindowControls 组件
- [ ] 2.5 switchNovel 抽函数
- [ ] 2.6 FocusId 对象化
- [ ] 2.7 usePanelStore
- [ ] 2.8 useFocusStore

---

## 2.1 定义 PanelId 联合类型

**目标**：把散落全文件的字符串字面量收敛成联合类型。纯类型定义，先不替换使用点。

**改动文件**：新建 `frontend/src/types/panel.ts`；改 `frontend/src/views/WorkspaceView.tsx`（仅 import，不替换使用）

**怎么做**：定义 `PanelId` 联合类型，包含 12 个值：novels、chapters、characters、locations、storyarcs、timeline、reader、preferences、novel-settings、profile、git、style-samples。再定义 `SidebarPanelId = PanelId | "search"`（侧栏可额外为 search）。

PanelId 来源核对（执行时验证）：activePanel 默认值 L74-76（novels/chapters）、否定链 L635-644（10 个）、三元链 L670-721、style-samples 分支 L654-669，合计 12 个。

WorkspaceView 顶部加 `import type { PanelId }`，本步不替换使用点。

**验证**：`npm run build` + `npm run lint` 通过。无行为变化。

**风险**：零。

**手测点**：无。

**commit**：`refactor(workspace): introduce PanelId union type`

---

## 2.2 建 CONTENT_PANEL_IDS Set

**目标**：为下一步删否定链做准备，定义「哪些面板走 ContentPanel」的集合。

**改动文件**：改 `frontend/src/views/WorkspaceView.tsx`

**怎么做**：在模块顶层定义 `CONTENT_PANEL_IDS = new Set<PanelId>(["chapters"])`。

> ⚠️ 设计文档原文写 `chapters/skills/git`，但核对代码后 git 走 GitCommitView（L713）、skills 是 SidePanel 内部行为非 activePanel 路由，实际只有 chapters 走 ContentPanel。执行时再确认一遍。

本步先只建 Set，下一步用它替换否定链。`PANEL_RENDERERS` record 要消费各 View 的 props（focusId 等），等 2.6 focusId 对象化后再建，本步不做。

**验证**：build + lint。

**风险**：零。

**手测点**：无。

**commit**：`refactor(workspace): add CONTENT_PANEL_IDS set for panel routing`

---

## 2.3 替换 activePanel 类型 + 删否定链

**目标**：把 `activePanel` 的 `useState` 类型注解为 `PanelId`，用 `Set.has` 替换 L635-644 的 10 行否定链。

**改动文件**：改 `frontend/src/views/WorkspaceView.tsx`

**怎么做**：
- L74-76 给 `useState` 加类型注解 `useState<PanelId>(...)`。
- L635-644 的 10 行否定链改成 `!CONTENT_PANEL_IDS.has(activePanel) && activePanel !== "novels"`（保留 novels 守卫，因 novels 分支在 L622 已处理）。
- `handleActivitySelect`（L249-263）参数类型 `PanelId`；`sidebarPanel` state 类型注解为 `SidebarPanelId | null`。

**验证**：build + lint + `npm run test`（1.4 面板切换测试必须仍绿）。

**风险**：低。纯重构，类型收紧。若有遗漏字符串值，TS 会报。

**手测点**：逐个点 ActivityBar 所有面板，确认渲染正确；切到 novels 看 BookshelfView。

**commit**：`refactor(workspace): type activePanel as PanelId and replace negation chain`

---

## 2.4 抽 WindowControls 组件

**目标**：把 L481-561 的 80 行内联 SVG（最小化/最大化/关闭按钮）抽成独立组件。

**改动文件**：新建 `frontend/src/components/shell/WindowControls.tsx`；改 `frontend/src/views/WorkspaceView.tsx`

**怎么做**：
- 新组件接收 props：`platformOS`、`isMaximised`、`setIsMaximised`。
- 把 `winBtn`/`closeBtn` 样式常量、3 个 SVG 按钮（最小化/最大化/关闭）、`platformOS !== "darwin"` 判断搬进新组件。
- 组件内自己 `useTranslation()` 取 `t`，不透传；`WindowMinimise/WindowToggleMaximise/Quit` 直接 import wailsjs runtime（不用 useApp）。
- WorkspaceView L448-563 区域换成 `<WindowControls platformOS=... isMaximised=... setIsMaximised=... />`。
- header 双击最大化逻辑（L439-443）保留在 WorkspaceView。

**验证**：build + lint + test。手测窗口按钮（Linux 上才显示，macOS 走原生）。

**风险**：低。纯抽组件。

**手测点**：点最小化/最大化/关闭按钮行为不变；双击 header 最大化不变。

**commit**：`refactor(workspace): extract WindowControls component`

---

## 2.5 switchNovel 抽函数

**目标**：把 4 处重复的「切小说重置」逻辑收敛成一个 `switchToNovel` 函数。

**改动文件**：改 `frontend/src/views/WorkspaceView.tsx`

**怎么做**：
- 抽 `switchToNovel(id)`，包含：`setActiveNovelId` + `setActivePanel("chapters")` + `contentRef.current?.closeAllTabs()` + `setTabTarget(null)` + `setActiveContent("")` + `setSelectedGitFile(null)` + `app.SetActiveNovel`。依赖 `[app]`。
- 4 处改成调 `switchToNovel(id)`：`handleImportedNovel`（L176-188）、`handleSelectNovel`（L322-334）、`handleCreateNovel`（L336-355）、`handleCreateNovelFromDialog`（L357-379）。
- 各处保留独有的后续动作（如 `handleCreateNovel` 还要 `setTitle("")`/`setDescription("")`/`setShowCreate(false)`）。

**验证**：build + lint + test（1.7 switchNovel 测试必须仍绿）。

**风险**：低。行为等价收敛。

**手测点**：切小说、创建小说、导入小说，确认 tabs 清空、内容重置、Git 文件选中清空。

**commit**：`refactor(workspace): consolidate switchNovel logic into single function`

---

## 2.6 FocusId 对象化

**目标**：8 个 `*FocusId` state → 1 个 `focusMap`。

**改动文件**：改 `frontend/src/views/WorkspaceView.tsx`

**怎么做**：
- L80-89 的 7 个 number 型 focusId 合并成 `focusMap: Partial<Record<PanelId, number>>`（character/location/timeline/arc/reader/preference/setting）。
- `styleSampleFocusId`（number | null，L87-89）单独保留——它用 null 语义表示「已处理」，与其他不同。
- `handleSearchNavigateEntity`（L269-300）的 8 分支 switch 收敛成：`setFocusMap({ [panelId]: entityId })` + `setActivePanel(panelId)`（之前是先清 8 个再 set 一个，合并后一次 set 即可）。
- 各 View 的 props 从 focusMap 取值（如 `focusId={focusMap.characters ?? 0}`）。

**验证**：build + lint + test（1.5 搜索导航测试必须仍绿）。

**风险**：中。focusId 传递链改动，搜索导航是关键路径。

**手测点**：搜索实体（角色/地点/时间线/弧线/读者/偏好/设置）跳转，确认对应 View 收到 focusId 并定位/高亮。

**commit**：`refactor(workspace): collapse 8 FocusId states into focusMap`

---

## 2.7 引入 usePanelStore（activePanel 外置）

> 从本步起用 zustand（1.1 已装）。

**目标**：把 `activePanel`/`sidebarPanel`/`sidebarClosed` 从 WorkspaceView 提到 zustand store，SidePanel 等组件直接订阅，消掉透传。

**改动文件**：新建 `frontend/src/stores/usePanelStore.ts`；改 `frontend/src/views/WorkspaceView.tsx`、`frontend/src/components/sidebar/SidePanel.tsx`、`frontend/src/components/shell/ActivityBar.tsx`

**怎么做**：
- store 状态：`activePanel`、`sidebarPanel`、`sidebarClosed`；actions：`setActive`、`setSidebarPanel`、`setSidebarClosed`、`toggleSidebar`。`activePanel` 默认 `"novels"`，由 WorkspaceView mount 时用 `initialNovelId` 覆盖（`initialNovelId ? "chapters" : "novels"`）。
- WorkspaceView 删这 3 个 state，改用 store selector；`handleActivitySelect` 改用 store actions。
- SidePanel/ActivityBar 删对应 props，改 `usePanelStore` 订阅。

> 注意 activePanel 默认值依赖 `initialNovelId` prop，初始化要在 WorkspaceView mount 时一次性 `setActive`，避免循环更新。

**验证**：build + lint + test（1.4 面板切换测试必须仍绿）。

**风险**：中。首次引入 store，多个组件改订阅，props 透传减少。

**手测点**：面板切换、侧栏开关、搜索侧栏切换。

**commit**：`refactor(workspace): extract activePanel state to usePanelStore`

---

## 2.8 引入 useFocusStore（focusMap 外置）

**目标**：把 `focusMap` 提到 store，搜索导航的 entity 跳转直接 dispatch，各 View 自己订阅自己的 focusId。

**改动文件**：新建 `frontend/src/stores/useFocusStore.ts`；改 `frontend/src/views/WorkspaceView.tsx`、各 View（CharacterListView/LocationListView/ArcListView/TimelineView/ReaderView/PreferenceView/NovelSettingView）

**怎么做**：
- store 状态：`focusMap`；actions：`focusEntity(panelId, id)`（设单个）、`clear()`。
- `handleSearchNavigateEntity` 改成调 `focusEntity`。
- 各 View 用 selector 订阅自己的（如 `useFocusStore((s) => s.focusMap.characters ?? 0)`）。

**验证**：build + lint + test（1.5 搜索导航测试必须仍绿）。

**风险**：中。多 View 改订阅，但模式统一。

**手测点**：搜索实体跳转 + 各 View 内编辑后 focusId 清除（`onFocusSampleHandled` 类回调）。

**commit**：`refactor(workspace): extract focusMap to useFocusStore`

---

## 阶段 2 完成标准

- WorkspaceView 行数从 786 降到 ~400
- activePanel 类型化，否定链删除
- WindowControls 独立组件
- switchNovel 单函数
- 8 FocusId 收敛成 focusMap
- usePanelStore / useFocusStore 就位，SidePanel 透传 props 大幅减少
- 1.4-1.7 测试全绿
- 手测面板切换 + 搜索导航 + 审批 + 切小说全通过

完成后进入 [03-novel-template.md](./03-novel-template.md)。
