# 阶段 1：基建（零功能风险）

> 前置条件：无。本阶段只装库、接 Provider、写测试，不改任何业务行为。
> 完成后：双库就位、安全网就位，新旧模式可并存。

## 进度勾选

- [ ] 1.1 装依赖
- [ ] 1.2 App.tsx 接 QueryClientProvider
- [ ] 1.3 建 src/lib/queryKeys.ts
- [ ] 1.4 P1 测试 · 面板切换
- [ ] 1.5 P1 测试 · 搜索导航
- [ ] 1.6 P1 测试 · 审批桥接
- [ ] 1.7 P1 测试 · switchNovel 重置

---

## 1.1 装 zustand + @tanstack/react-query 依赖

**目标**：把双库加进 `package.json`，为后续所有阶段铺路。不动业务代码。

**改动文件**：`frontend/package.json`、`frontend/package-lock.json`

**怎么做**：在 `frontend/` 下用 `npm install --save` 装三个包：`zustand`、`@tanstack/react-query`、`@tanstack/react-query-devtools`。devtools 是开发调试用，体积小，建议装。

**验证**：`npm run build` 通过；`package.json` 的 dependencies 出现这三个包。

**风险**：零。未 import 任何代码。

**手测点**：无（应用行为完全不变）。

**commit**：`build(frontend): add zustand and @tanstack/react-query deps`

---

## 1.2 App.tsx 接 QueryClientProvider

**目标**：在应用根挂 QueryClient，让 `useQuery` 全局可用。

**改动文件**：新建 `frontend/src/lib/queryClient.ts`；改 `frontend/src/App.tsx`

**怎么做**：
- 新建 `queryClient.ts`，导出一个 `QueryClient` 实例（模块级单例，无需 useState）。默认配置：`staleTime` 设 30s（防切面板短时间重复 fetch）、`refetchOnWindowFocus: false`（桌面应用切窗不应触发重 fetch）、`retry: 1`。
- 改 `App.tsx`，在 `TooltipProvider` 外层包 `QueryClientProvider`，`client` 用上一步的实例。

**验证**：`npm run build` 通过；`wails dev` 启动应用正常显示（此时还没用 useQuery，行为不变）。

**风险**：零。Provider 包裹不影响现有组件。

**手测点**：应用能正常启动、切面板、开小说。

**commit**：`feat(frontend): wrap app root with QueryClientProvider`

---

## 1.3 建 src/lib/queryKeys.ts

**目标**：集中 queryKey 常量，避免后续步骤乱写字符串导致缓存不共享。

**改动文件**：新建 `frontend/src/lib/queryKeys.ts`

**怎么做**：按 [00-conventions.md §1.2](./00-conventions.md#12-key-清单) 的清单建常量文件。每个实体导出一组 factory（如 `novelKeys.all`、`novelKeys.detail(id)`、`characterKeys.list(novelId)`），禁止裸写字符串数组。本步把已知实体（novel/chapter/character/location/storyarc/timeline/reader/preference/novel-setting/style-sample/skill/session）的 keys 全建好，后续阶段直接用。

**验证**：`npm run build` + `npm run lint` 通过。本步无 useQuery 消费，纯常量。

**风险**：零。

**手测点**：无。

**commit**：`feat(frontend): add queryKey constants module`

---

## 1.4 P1 测试 · WorkspaceView 面板切换

**目标**：为后续 P0/P2/P3 重构兜底，建立第一条行为测试。测「点 ActivityBar 某项 → 主区渲染对应组件」，不测内部状态。

**改动文件**：新建 `frontend/src/views/WorkspaceView.test.tsx`

**怎么做**：
- WorkspaceView 直接依赖的 hook/wails 函数都要 mock。清单（执行时按报错补）：`@/hooks/useApp`、`@/hooks/useTheme`、`@/hooks/useLayoutState`、`@/hooks/useWindowState`、`@/hooks/useImportNovel`、`@/lib/wailsjs/runtime/runtime`（WindowMinimise/WindowToggleMaximise/Quit）、`@/lib/wailsjs/go/app/App` 的 `CheckUpdate`。
- useApp 只 mock 该测试用到的函数（参考 `CharacterList.test.tsx` 的模式，不全量 mock 100+）。i18n 用测试环境默认（返回 key 字符串）。
- 子组件策略：先不 mock 子组件（测真实渲染树）；若渲染太重或依赖未 mock 项导致失败，再逐个 mock 边界组件。
- 用例覆盖：`initialNovelId=0` 默认渲染 novels/书架；`initialNovelId≠0` 渲染 chapters/ContentPanel；点 characters 面板渲染 CharacterListView 内容。

**验证**：`npm run test` 通过，新增用例全绿。

**风险**：低。可能遇到子组件渲染依赖未 mock 的项，按报错补 mock。

**手测点**：无（纯测试）。

**commit**：`test(workspace): add panel switching behavior tests`

---

## 1.5 P1 测试 · 搜索导航

**目标**：覆盖 `handleSearchNavigateEntity`（WorkspaceView.tsx L269-300）和 `handleSearchNavigateChapter`（含 `flushSync`，L302-320）。

**改动文件**：`frontend/src/views/WorkspaceView.test.tsx`（追加用例）

**怎么做**：
- entity 跳转：触发 `onSearchNavigateEntity`，断言切到对应面板且对应 View 收到 focusId（通过 View 内可观察内容验证，不读 state）。
- chapter 跳转：mock `contentRef.current.openFileWithHighlight`，断言被调用且参数含 matchPos/matchLen。
- contentRef mock 方案：mock `ContentPanel` 组件本身用 `forwardRef` 暴露 mock 方法，或 `vi.spyOn` 拦截。执行时选可行的。

**验证**：`npm run test` 通过。

**风险**：中。flushSync 路径 + contentRef mock 可能棘手。若 mock 不通可降级为「只测面板切换，不测高亮」，后续阶段补。

**手测点**：无。

**commit**：`test(workspace): add search navigation behavior tests`

---

## 1.6 P1 测试 · 审批桥接

**目标**：覆盖 `handleApprove`/`handleReject`（L211-219）调用 `app.ApproveTool` + `contentRef.handleDiffApprove/Reject`。

**改动文件**：`frontend/src/views/WorkspaceView.test.tsx`（追加用例）

**怎么做**：mock `app.ApproveTool` 和 `contentRef.current` 的两个方法；模拟 ChatPanel 触发 `onApprove(toolId, feedback)`/`onReject`；断言 `ApproveTool` 被以正确参数调用（approve 传 true，reject 传 false）且对应 `handleDiffApprove/Reject` 被调用。contentRef mock 方案同 1.5。

**验证**：`npm run test` 通过。

**风险**：中。依赖 contentRef mock。

**手测点**：无。

**commit**：`test(workspace): add approval bridge behavior tests`

---

## 1.7 P1 测试 · switchNovel 状态重置

**目标**：覆盖 4 处 switchNovel 重复逻辑（`handleImportedNovel` L176-188 / `handleSelectNovel` L322-334 / `handleCreateNovel` L336-355 / `handleCreateNovelFromDialog` L357-379），确保切小说后 tabs/activeContent/gitFile 都重置。这一步是阶段 3 switchNovel 迁移到 store 的安全网，**必须先有**。

**改动文件**：`frontend/src/views/WorkspaceView.test.tsx`（追加用例）

**怎么做**：mock `contentRef.current.closeAllTabs` 和 `app.SetActiveNovel`；分别触发选择小说/导入小说/创建小说；断言 `closeAllTabs` 被调用、`SetActiveNovel` 被以正确 id 调用、状态重置齐全（通过可观察行为验证）。

**验证**：`npm run test` 通过。

**风险**：低。

**手测点**：无。

**commit**：`test(workspace): add switchNovel state reset behavior tests`

---

## 阶段 1 完成标准

- zustand + @tanstack/react-query 装好
- QueryClientProvider 在 App.tsx 生效
- queryKeys.ts 就位
- WorkspaceView.test.tsx 覆盖面板切换/搜索导航/审批/switchNovel 四组行为
- `npm run build && npm run lint && npm run test` 全绿
- `wails dev` 手测应用正常

完成后进入 [02-workspaceview.md](./02-workspaceview.md)。
