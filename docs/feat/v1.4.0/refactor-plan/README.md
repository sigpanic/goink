# 前端架构改造 · 实行计划

> 配套文档：[../frontend-architecture-refactor.md](../frontend-architecture-refactor.md)（调查稿/设计文档）
> 本文是「怎么一步步做」的执行手册，设计文档是「为什么这么做」的论证。

## 核心目标

引入 **Zustand（UI 状态）+ TanStack Query（数据获取）** 双库，把 `WorkspaceView`（786 行/34 useState）从巨石改造成纯壳，每个领域走「query + store + mutation + dialogs」四件套。

## 总原则

1. **一步 = 一个 commit = 一次独立审计**。每步只改一处，能被 pre-commit hook 验证（build/lint/test），能被你 review。
2. **一次只走一步**。走完一步等你明确说 commit 才提交，明确说继续才走下一步。
3. **每步可回退**。改坏了 `git revert` 单个 commit 即可，不会牵连。
4. **不动代码不写文档以外的东西**。本计划只规划动作，执行时按步骤改对应文件。
5. **步骤文档是参考，行动前调研代码**。执行步骤是参考而非权威，动手前务必核对当前代码（行号/逻辑可能已漂移），有出入与用户汇报商讨，不要自行决定就开始写代码。
6. **禁止功能/实现降级**。重构不得静默简化、跳过边界 case 或丢弃特性；遇到降级压力（实现复杂、风险高、卡壳）必须与用户商讨，不得自行决定降级。
7. **迁移过程 UI/UX 完全不变**。重构不得借机重写 UI 导致视觉大变样或丢失细微交互（hover/focus/快捷键/过渡动画/边界态提示等）。本计划只搬代码、不改呈现。若某步发现不得不变动 UI/UX（如旧实现依赖被破坏），必须立刻汇报用户商讨，不得自行决定变更。手测点以「与重构前完全一致」为验收标准。
8. **禁止丢失错误处理 / 降级错误展示**。重构严禁以任何方式弱化或丢弃错误处理：
   - **不得丢失错误副作用**：手动 fetch 迁移到 `useQuery` / `useMutation` 时，必须把原 `catch` 块的副作用（`toastError` / `console.error` / 上报等）搬到 `useEffect` 监听 `query.error`（v5 query 无 `onError` 回调；mutation 仍可用 `onError`）。query 把错误吞进 `error` 字段后不会自动触发副作用，漏挂 useEffect 即等于静默丢错误。
   - **不得改变错误展示方式**：原 `toast` + UI 文本双重提示的，迁移后必须双重提示都在；原带具体 `err.message` 的，迁移后必须仍显示具体消息，不得改成只显示固定文案。
   - **数据加载失败时 UI 正常渲染**：失败状态仅作用在「数据显示处」（如 list 区 / graph 容器区），header / 控件按钮 / 浮窗 / 图例等周边 UI 必须正常渲染。严禁整屏只渲染一个「加载失败」文本，导致周边交互入口丢失。
   - **降级压力测试**：每步 query/mutation 化完成后，必须自检「错误路径是否完整保留原行为」——toast 是否仍弹、UI 是否仍显示、错误消息是否仍具体。发现降级立即修，禁止「等下个步骤再补」。
9. **错误提示原则（toast + UI）**。任何抛到前端的错误必须以合理方式让用户看到原因，不得静默：
   - **不静默**：错误必须至少有一种用户可见的反馈（toast 或 UI 显示），不允许只 `console.error` 把错误埋在 dev 工具里。
   - **不重复**：同一错误事件只触发一次 toast，避免重复弹窗骚扰。query/mutation 场景靠 `useEffect` 监听 `error` 引用变化触发（TanStack Query error 引用稳定，fetch 失败后不变，useEffect 只触发一次）；其他场景靠 `try/catch` 单次触发。
   - **灵活配合**：toast + UI 文本双重、只 toast、只 UI 文本都行，按场景选：
     - 数据列表加载失败：UI 显示「加载失败」文本 + toast 显示具体 `err.message`（双重提示，列表区不挤占）
     - 命令操作失败（CRUD/approve/reject 等）：toast 显示 `xxxFailed: err.message`（操作本身无固定 UI 区，toast 即可）
     - 表单提交失败：表单下方 UI 显示错误 + toast 提示（双重）
   - **必须带具体 err.message**：toast/UI 错误文案必须是 `<i18n key>: <err.message>` 格式，不允许只显示固定 i18n 文案不带具体原因（用户无法定位问题）。
   - **降级压力测试**：每步迁移后必须自检——错误路径是否仍能让用户看到具体原因？toast 是否只触发一次？UI 是否仍正常渲染周边控件？
10. **领域无跨组件 UI 状态时不建 store**。store 为跨组件共享状态 / 避免 props 透传而设；组件内部自用状态（editMode / form / viewTab / search 等）留组件内，不进 store。如 character 领域 editMode/form/viewTab 留 CharacterListView 组件内，仅因删除合并（主区 CharacterListView + 侧边栏 CharacterList 共用唯一 ConfirmDialog）引入跨组件 deletingCharacterId，建最小 useCharacterStore 只放该字段。

## 步骤总览

| 阶段 | 文档 | 步骤范围 | 风险 | 前置条件 |
|---|---|---|---|---|
| 1 基建 | [01-foundation.md](./01-foundation.md) | 1.1 装依赖 → 1.7 P1 测试 | 零~低 | 无 |
| 2 拆 WorkspaceView | [02-workspaceview.md](./02-workspaceview.md) | 2.1 PanelId 类型 → 2.8 useFocusStore | 低~中 | 1.7 测试就位 |
| 3 小说领域模板 | [03-novel-template.md](./03-novel-template.md) | 3.1 useNovels → 3.9 4 处消费方迁移 | 中 | 阶段 2 完成 |
| 4 7 实体批量 | [04-entities-batch.md](./04-entities-batch.md) | character → novel-setting，删 refreshNonce | 中 | 阶段 3 验证手感 |
| 4a 错误 toast 中间件 | [04a-query-error-toast.md](./04a-query-error-toast.md) | QueryCache.subscribe 全局中间件，修 character 重复 toast + novel 静默失败 | 低~中 | 4.1.1 完成（character 已 query 化） |
| 4b 搜索补全 | [04b-search-preference-setting-reader.md](./04b-search-preference-setting-reader.md) | preference/setting/reader 接入搜索 | 低~中 | 阶段 4 完成（正交于阶段 5，可并行）|
| 5 其他模块 | [05-misc-modules.md](./05-misc-modules.md) | chat/content/skill/git/search | 中 | 阶段 4 完成 |
| 6 拆巨石（可选） | [06-monolith-optional.md](./06-monolith-optional.md) | ChatPanel/ArcListView/去 imperativeHandle | 高 | 痛点驱动，先扩测试 |
| 7 localStorage 迁 persist | [07-localstorage-persist.md](./07-localstorage-persist.md) | useTheme/useLayoutState/useWindowState → store + persist | 低~中 | 阶段 5 完成；正交于阶段 6，可并行 |

通用规范（queryKey、测试原则、commit 风格、目录约定）见 [00-conventions.md](./00-conventions.md)。

## 进度追踪

每完成一步，在本表对应行打勾（commit 后由你或我更新）：

- [ ] 1.1 装 zustand + @tanstack/react-query
- [ ] 1.2 App.tsx 接 QueryClientProvider
- [ ] 1.3 建 src/lib/queryKeys.ts
- [ ] 1.4 P1 测试 · 面板切换
- [ ] 1.5 P1 测试 · 搜索导航
- [ ] 1.6 P1 测试 · 审批桥接
- [ ] 1.7 P1 测试 · switchNovel 重置

（后续阶段的勾选清单在各阶段文档头部）

## 风险红线

- **P3 数据层迁移是最大风险**（扩散面大），必须分批，每批一个模块独立验证。
- **useApp.ts 在阶段 5 之前保留不动**（它有修 bug 留下的 useMemo，删早了会重现丢事件 bug，详见设计文档「useApp 章节历史真相」）。
- **refreshNonce 机制在阶段 4 之前保留**，最后一个领域迁移完才整体删除。
- **chat 流式数据永远不走 query 缓存**，保持本地 state。
- **useEditorTabs 的 persist 迁移与阶段 6.5 useTabStore 合并执行**，不在阶段 7 单独迁，避免两次改同一组 tab 状态逻辑。
- **flushSync 在搜索章节跳转路径保留**（3.8 调研结论）：ContentPanel 条件渲染，从非 chapters 面板搜索章节时未挂载，flushSync 确保挂载后才调 `contentRef.current?.openFileWithHighlight`。完整删除需 ContentPanel 改为始终挂载或搜索跳转改声明式，属痛点驱动，非必做。
