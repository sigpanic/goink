# 阶段 7：localStorage 持久化迁移到 Zustand persist

> 前置条件：[阶段 5](./05-misc-modules.md) 完成（useApp 已删，store 模式稳定）。
> 正交于 [阶段 6](./06-monolith-optional.md)（拆巨石），可与阶段 6 并行。
> 完成后：全项目持久化机制统一为「zustand store + persist 中间件」，删除手写 localStorage 读写 + beforeunload batch + MutationObserver 同步等 workaround。
> 范围：`useTheme` / `useLayoutState` / `useWindowState`。**`useEditorTabs` 不在本阶段**，与 6.5 的 useTabStore 合并执行。

## 进度勾选

- [x] 7.1 useTheme → useThemeStore（删 MutationObserver）—— 已完成 commit `e20bc0d`
- [~] 7.2 useLayoutState → useLayoutStore —— **不做**（persist 不适合，见下文分析）
- [~] 7.3 useWindowState → useWindowStateStore —— **不做**（persist 根本不适合，见下文分析）
- [x] 7.4 删除 `frontend/src/hooks/useTheme.ts` 壳（已删除；useLayoutState.ts / useWindowState.ts 保留）

---

## persist 适用性分析（决定 7.2/7.3 不做的依据）

persist 中间件的核心特性：**每次 `set()` 自动同步写 localStorage**。适合的场景是「多组件共享状态 + 每次变化即持久化 + restore 无副作用」。逐个 hook 对照：

| 适用条件 | 7.1 useTheme | 7.2 useLayoutState | 7.3 useWindowState |
|---|---|---|---|
| 多组件共享状态 | ✓ 7 个消费方 | ✗ 仅 1 个（WorkspaceView） | ✗ 仅 1 个（WorkspaceView） |
| 每次变化即持久化 | ✓ 无防抖 | ✗ 有 300ms 防抖 | ✗ beforeunload 才写 |
| restore 无副作用 | ✓ 纯读 localStorage | ✓ 纯读 | ✗ 异步调 Wails API（WindowSetSize/Position/ToggleMaximise） |
| 无运行时状态 | ✓ | ✓ | ✗ isMaximised 是运行时状态 |

**结论**：
- **7.1 适合 persist**：7 个消费方（跨组件同步需求，删 MutationObserver hack）+ 无防抖 + 纯读 restore。
- **7.2 不适合 persist**：只 1 个消费方（无跨组件同步需求，store 价值不大）+ 300ms 防抖与 persist 每次 set 写冲突。保留原 hook 更合理。
- **7.3 根本不适合 persist**：beforeunload 保存模式 + Wails API 异步 restore + isMaximised 运行时状态，三个都不匹配。强行迁移会引入复杂度且无收益。

不同场景用不同方案是正常的工程判断。强行用 persist 统一不适合 persist 的场景（7.3）才是问题。

---

## 4 个 hook 现状

| Hook | localStorage key | 存什么 | 写时机 | 跨组件共享 | 迁移难度 |
|---|---|---|---|---|---|
| `useEditorTabs` | `goink_tabs_all` | `Record<novelId, TabMeta[]>` | beforeunload batch | 否（仅 ContentPanel） | **高**（与 6.5 合并，不在本阶段） |
| `useLayoutState` | `goink_sidepanel_width` / `goink_chatpanel_width` | 两个 number | 300ms 防抖 | 否 | **低**（但 persist 不适合，保留原样） |
| `useWindowState` | `goink_window_*`（5 字段） | 窗口几何 | beforeunload | 否 | **中**（与 Wails API 耦合，persist 不适合，保留原样） |
| `useTheme` | `theme`（旧）→ `goink-theme`（新） | `"light" \| "dark"` | 立即写 | **是**（7 个组件 + graphColors） | **低-中**（删 MutationObserver hack） |

---

## 7.1 useTheme → useThemeStore ✅ 已完成

**commit**: `e20bc0d` `refactor(theme): migrate useTheme to useThemeStore with persist`

**已完成改动**：
- 新建 `frontend/src/stores/useThemeStore.ts`（54 行，简化版 persist store）
- `useTheme.ts` 改为 3 行 re-export 壳（7.4 删除）
- 7 个消费方 import + 调用改 `useThemeStore`：WorkspaceView / InitView / ArcListView / StoryArcGraph / GitCommitView / ContentPanel / graphColors
- 2 个测试 mock 改指向 `@/stores/useThemeStore`

**行为变更**（已确认接受）：
- 首次启动不再跟随系统主题，默认 light（原 `localStorage.getItem("theme") === null` fallback 删除）
- persist key 从 `theme` 改为 `goink-theme`，老用户首次升级丢失一次主题设置
- MutationObserver 跨组件同步 hack 删除（store 单一数据源天然同步）
- matchMedia effect 删除（不再跟随系统，无需监听）

**为什么 7.1 适合 persist**：7 个消费方（跨组件同步需求）+ 无防抖（每次 set 写不冲突）+ 纯读 restore（无副作用）。删 MutationObserver 是实质性工程收益。

---

## 7.2 useLayoutState → useLayoutStore ❌ 不做

**决策**：保留原 `useLayoutState.ts` 不动。

**不做的原因**：
1. **只 1 个消费方**（WorkspaceView）—— 无跨组件同步需求，store 的"单一数据源"价值体现不出来
2. **300ms 防抖与 persist 冲突** —— persist 每次 set 自动写 localStorage，无法防抖。删防抖则拖拽时写频率上升（实际影响可忽略，但失去防抖优化）；保留防抖则不能用 persist
3. **主要收益是删模板代码**（loadNumber + 防抖 useEffect + useCallback setter，约 30 行）—— 但防抖不是 workaround，是正常优化，删它没有"清理 hack"的价值
4. **如果强行迁移**：要么删防抖（行为微变），要么用 store + 手动写 localStorage（代码量不减）—— 两个选项都不理想

**保留原样的影响**：功能无影响。代码风格上 7.1 用 persist、7.2 用手写 localStorage，但这两种模式适合各自场景，不是"不统一"。

---

## 7.3 useWindowState → useWindowStateStore ❌ 不做

**决策**：保留原 `useWindowState.ts` 不动。

**不做的原因**：
1. **beforeunload 保存模式** —— 原 hook 在窗口关闭时才写 localStorage（`window.addEventListener("beforeunload", save)`）。persist 每次 set 都写，模式完全冲突
2. **restore 涉及 Wails API 异步调用** —— mount 时读 localStorage 调 `WindowSetSize/Position/ToggleMaximise` 恢复窗口。persist 的 `onRehydrateStorage` 不适合放异步副作用
3. **isMaximised 是运行时状态** —— 不应持久化到 store state（但原代码确实存了 `goink_window_maximised` key）
4. **5 个 localStorage key + 屏幕边界 clamp 逻辑** —— 迁移复杂度高，收益最低

**保留原样的影响**：功能无影响。useWindowState 的 beforeunload + Wails API 模式是特殊场景，手写 localStorage 是最合适的方案。

---

## 7.4 删除旧 hook 文件 ✅ 已完成

**前置条件**：7.1 完成（7.2/7.3 不做，useLayoutState.ts / useWindowState.ts 保留）。

**已完成改动**：
- `useTheme.ts` re-export 壳已删除（所有消费方已指向 @/stores/useThemeStore）
- `useLayoutState.ts` / `useWindowState.ts` 保留不动（7.2/7.3 不做）

---

## 阶段 7 完成标准（调整后）

- [x] useTheme 迁到 zustand store + persist（7.1 完成）
- [x] MutationObserver 跨组件同步 hack 删除
- [~] useLayoutState / useWindowState 保留原样（persist 不适合，工程判断不迁移）
- [x] useTheme 相关测试全绿，测试 mock 改指向 store
- [x] 手测主题切换通过

## 不在本阶段范围

- **`useEditorTabs`**：与阶段 6.5 的 useTabStore 合并执行。6.5 启动时一并设计 persist（partialize 只持久化 `Record<novelId, TabMeta[]>`，不持久化 `tabs`/`activeTabId`/`idSeq`）。
- **`useLayoutState` / `useWindowState`**：persist 不适合（无跨组件需求 + 防抖/beforeunload 冲突 + Wails API 耦合），保留原样。
- **i18n 语言设置**：`frontend/src/i18n/index.ts` 由 i18next 自管理，不动。

## 风险红线

- **`useTheme` 的 system fallback 行为**：已确认接受行为变更 —— 首次启动不再跟随系统主题，默认 light。老用户升级丢失一次主题设置（persist key 从 `theme` 改为 `goink-theme`）。
- **`useLayoutState` 的 clamp 边界**：180-480 / 280-800，保留原 hook 的 clamp 逻辑不动。
- **`useWindowState` 的 restore 顺序**：保留原 hook 的 beforeunload + Wails API restore 逻辑不动。
- **persist key 命名**：7.1 使用 `goink-theme`（kebab-case）。7.2/7.3 不迁移，保留原 `goink_xxx` snake_case key。
- **测试 mock**：7.1 的 `vi.mock("@/hooks/useTheme")` 已改为 `vi.mock("@/stores/useThemeStore")`。7.2/7.3 保留原 mock 不动。
