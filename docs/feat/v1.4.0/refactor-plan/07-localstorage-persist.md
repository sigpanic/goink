# 阶段 7：localStorage 持久化迁移到 Zustand persist

> 前置条件：[阶段 5](./05-misc-modules.md) 完成（useApp 已删，store 模式稳定）。
> 正交于 [阶段 6](./06-monolith-optional.md)（拆巨石），可与阶段 6 并行。
> 完成后：全项目持久化机制统一为「zustand store + persist 中间件」，删除手写 localStorage 读写 + beforeunload batch + MutationObserver 同步等 workaround。
> 范围：`useTheme` / `useLayoutState` / `useWindowState`。**`useEditorTabs` 不在本阶段**，与 6.5 的 useTabStore 合并执行。

## 进度勾选

- [ ] 7.1 useTheme → useThemeStore（删 MutationObserver）
- [ ] 7.2 useLayoutState → useLayoutStore（删手写防抖）
- [ ] 7.3 useWindowState → useWindowStateStore（persist + 保留 beforeunload 触发）
- [ ] 7.4 删除 `frontend/src/hooks/` 下被替换的旧 hook 文件

---

## 4 个 hook 现状

| Hook | localStorage key | 存什么 | 写时机 | 跨组件共享 | 迁移难度 |
|---|---|---|---|---|---|
| `useEditorTabs` | `goink_tabs_all` | `Record<novelId, TabMeta[]>` | beforeunload batch | 否（仅 ContentPanel） | **高**（与 6.5 合并，不在本阶段） |
| `useLayoutState` | `goink_sidepanel_width` / `goink_chatpanel_width` | 两个 number | 300ms 防抖 | 否 | **低** |
| `useWindowState` | `goink_window_*`（5 字段） | 窗口几何 | beforeunload | 否 | **中**（与 Wails API 耦合） |
| `useTheme` | `theme` | `"light" \| "dark"` | 立即写 | **是**（7 个组件 + graphColors） | **低-中**（删 MutationObserver hack） |

---

## 7.1 useTheme → useThemeStore

**目标**：把 `theme` state 提到 zustand store + persist，删 MutationObserver 跨组件同步 hack。

**改动文件**：新建 `frontend/src/stores/useThemeStore.ts`；改 `frontend/src/hooks/useTheme.ts`；改 7 个消费方（WorkspaceView/InitView/ArcListView/StoryArcGraph/GitCommitView/ContentPanel/graphColors）；改测试 mock。

**怎么做**：
- store 状态 `theme: Theme`；actions `setTheme(t)` / `toggle()`。
- `persist` 配置：`name: "goink-theme"`、`partialize: (s) => ({ theme: s.theme })`。
- `applyTheme(t)`（设 `data-theme` DOM attribute）放 store 的 `onRehydrateStorage` + action 内调用。
- 删 `useTheme.ts` 里的 MutationObserver 整段。
- `matchMedia("(prefers-color-scheme: dark)")` 监听 effect 保留，搬 App 顶层。

**风险点**：原 `localStorage.getItem("theme") === null` 时跟随系统主题。persist 总会写 store，会导致「首次启动后不再跟随系统」。执行前与用户确认：保留 system fallback（matchMedia effect 不写 store）或接受行为变更。

**commit**：`refactor(theme): migrate useTheme to useThemeStore with persist`

---

## 7.2 useLayoutState → useLayoutStore

**目标**：把两个面板宽度提到 store + persist，删手写 300ms 防抖。

**改动文件**：新建 `frontend/src/stores/useLayoutStore.ts`；改 `frontend/src/hooks/useLayoutState.ts`；改 `WorkspaceView.tsx`；改测试 mock。

**怎么做**：
- store 状态 `sidePanelWidth`（默认 224）/ `chatPanelWidth`（默认 360）。
- actions `setSidePanelWidth(w)` / `setChatPanelWidth(w)` —— **clamp 逻辑必须搬进 action**（`Math.min(480, Math.max(180, Math.round(w)))` / `Math.min(800, Math.max(280, Math.round(w)))`），否则写越界值。
- `persist` 配置：`name: "goink-layout"`、`partialize: (s) => ({ sidePanelWidth, chatPanelWidth })`。
- persist 默认每改必写；原 300ms 防抖是为了拖拽时不频繁 I/O。localStorage 写两个 number 成本可忽略，**建议接受写频率上升**（行为等价）。若严格保持原行为，action 内包 setTimeout 防抖。

**commit**：`refactor(layout): migrate useLayoutState to useLayoutStore with persist`

---

## 7.3 useWindowState → useWindowStateStore

**目标**：把窗口几何状态持久化提到 store + persist，**保留 beforeunload 触发写盘**语义。

**改动文件**：新建 `frontend/src/stores/useWindowStateStore.ts`；改 `frontend/src/hooks/useWindowState.ts`；改 `WorkspaceView.tsx`；改测试 mock。

**怎么做**：
- store 状态 `width`/`height`/`x`/`y`/`isMaximised`（5 字段）。
- `persist` 配置：`name: "goink-window"`、`partialize: (s) => s`。
- **关键**：persist 默认 setState 即写。仍保留 beforeunload effect 主动调 Wails API + `setState`（语义不变）。mount 时 restore 逻辑保持原样：从 store 读 5 字段 → 调 Wails `WindowSetSize/Position/ToggleMaximise`。
- 屏幕边界 clamp 逻辑保留。
- store 本身不调 Wails API（纯状态）；所有 Wails 调用留 hook/effect 层。

**风险点**：restore 顺序错会导致窗口位置异常。必须手测真实窗口。

**commit**：`refactor(window): migrate useWindowState to useWindowStateStore with persist`

---

## 7.4 删除旧 hook 文件

**前置条件**：7.1-7.3 全部完成，确认无组件再 import `@/hooks/useTheme` / `useLayoutState` / `useWindowState`。

**改动文件**：删除 `useTheme.ts`、`useLayoutState.ts`、`useWindowState.ts`；保留 `useEditorTabs.ts`（待 6.5 处理）。

**commit**：`refactor(frontend): remove legacy localStorage hooks after store migration`

---

## 阶段 7 完成标准

- useTheme / useLayoutState / useWindowState 三个 hook 全部迁到 zustand store + persist
- MutationObserver 跨组件同步 hack 删除
- 手写 localStorage 读写代码全部删除（除 `useEditorTabs` 留待 6.5）
- 所有测试全绿，测试 mock 全部改指向 store
- 手测主题切换、面板宽度、窗口几何恢复全通过

## 不在本阶段范围

- **`useEditorTabs`**：与阶段 6.5 的 useTabStore 合并执行。6.5 启动时一并设计 persist（partialize 只持久化 `Record<novelId, TabMeta[]>`，不持久化 `tabs`/`activeTabId`/`idSeq`）。
- **i18n 语言设置**：`frontend/src/i18n/index.ts` 由 i18next 自管理，不动。

## 风险红线

- **`useTheme` 的 system fallback 行为**：原 `localStorage.getItem("theme") === null` 时跟随系统主题。persist 总会写 store，会改变「首次启动跟随系统」的行为。执行前与用户确认。
- **`useWindowState` 的 restore 顺序**：必须严格保留原顺序（先 maximised，再 size/position），否则窗口闪到错误位置。
- **`useLayoutState` 的 clamp 边界**：180-480 / 280-800，必须搬进 store action，否则越界值写盘。
- **persist key 命名**：建议统一 `goink-` 前缀（kebab-case），与现有 `goink_xxx` snake_case key 不同。可接受首次升级丢失一次设置，或 `onRehydrateStorage` 做旧 key 迁移。
- **测试 mock 改动量**：3 个 hook 被 `WorkspaceView.test.tsx` mock、`useTheme`/`useEditorTabs` 被 `ContentPanel.test.tsx` mock。迁移后 `vi.mock("@/hooks/useXxx")` 改成 `vi.mock("@/stores/useXxxStore")` 或 `useXxxStore.setState(...)` 注入。
