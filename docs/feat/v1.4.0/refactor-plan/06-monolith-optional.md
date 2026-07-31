# 阶段 6：拆巨石组件 / 去 imperativeHandle（可选，痛点驱动）

> 前置条件：[阶段 5](./05-misc-modules.md) 完成 + 对应模块的 P1 测试扩展就位。
> 本阶段**痛点驱动**：等真正痛了再动，不强行做。风险高，必须有测试兜底。
> 不建议在 P1 测试未覆盖这些组件前启动本阶段。

## 进度勾选

- [ ] 6.1 扩 P1 测试覆盖 ChatPanel / ArcListView / TimelineView / ContentPanel
- [ ] 6.2 拆 ChatPanel
- [ ] 6.3 拆 ArcListView
- [ ] 6.4 拆 TimelineView
- [ ] 6.5 去 ContentPanel imperativeHandle

---

## 6.1 扩 P1 测试

**目标**：为巨石拆分兜底。拆分前必须有行为测试，否则不敢动。

**改动文件**：新建对应测试文件

**怎么做**：按 [00-conventions.md §2](./00-conventions.md#2-测试原则) 测试原则，为每个巨石组件补行为测试（用户输入→可观察输出）。ChatPanel 重点测：事件队列、turn 重建、session 切换、审批 UI、diff tab 触发。ContentPanel 重点测：tab 开关、文件保存、highlight。

**验证**：`npm run test` 通过。

**风险**：零（只加测试）。

**commit**：`test(chat): add behavior tests for ChatPanel` 等

---

## 6.2 拆 ChatPanel（1532 行）

**目标**：拆成 `<ChatMessageList>` + `<ChatInput>` + `<ApprovalPanel>` + `<DiffTab>` 等，EventQueue/重排定时器抽 hook。

**改动文件**：`frontend/src/components/chat/ChatPanel.tsx` 拆分

**怎么做**：拆分设计应单独成文（参考本文档体例）。核心：EventQueue 抽成 `useEventQueue` hook；session 管理、drag resize、审批 UI 各自独立组件。流式数据逻辑保持。

**验证**：build + lint + test（6.1 测试必须仍绿）。

**风险**：高。1532 行 + 27 个 useEffect/useCallback + 事件驱动，最复杂的拆分。

**手测点**：聊天收发、流式、审批 approve/reject、diff tab、session 切换、drag resize。

**commit**：拆分后多个 `refactor(chat): extract Xxx component`

---

## 6.3 拆 ArcListView（1129 行）

**目标**：拆成 `<ArcList>` + `<ArcEditor>` + `<ArcGraph>` 三件套。

**改动文件**：`frontend/src/components/storyarc/ArcListView.tsx` 拆分

**怎么做**：List + 编辑表单 + 图三合一拆开。数据层在阶段 4.3 已走 query，本步只拆 UI。

**验证**：build + lint + test。

**风险**：中-高。

**手测点**：arc CRUD、node CRUD、图操作。

**commit**：`refactor(storyarc): split ArcListView into list/editor/graph`

---

## 6.4 拆 TimelineView（934 行）

**目标**：同 ArcListView，拆成 `<TimelineList>` + `<TimelineEditor>` 三件套。

**改动文件**：`frontend/src/components/timeline/TimelineView.tsx` 拆分

**验证**：build + lint + test。

**风险**：中-高。

**commit**：`refactor(timeline): split TimelineView into list/editor`

---

## 6.5 去 ContentPanel imperativeHandle

**目标**：把 `ContentPanelHandle` 的 8 个命令式方法改成声明式 tab store，同时干掉残留的命令式协调。

**改动文件**：新建 `frontend/src/stores/useTabStore.ts`；改 `frontend/src/components/content/ContentPanel.tsx`、`frontend/src/views/WorkspaceView.tsx`、`frontend/src/components/chat/ChatPanel.tsx`

**怎么做**：
- 建 tab store：`tabs`、`activeTabId`、`openFile`、`openDiffTab`、`closeAllTabs`、`openFileWithHighlight` 等。
- ContentPanel 从 `forwardRef + useImperativeHandle` 改成订阅 tab store 自行渲染。
- 父组件（WorkspaceView/ChatPanel）改成 dispatch store action，不再 `contentRef.current?.xxx()`。
- 删 `ContentPanelHandle` 类型 + `contentRef`。

**验证**：build + lint + test（1.5/1.6/1.7 测试必须仍绿）。

**风险**：高。审批关键路径，三方协议（WorkspaceView + ChatPanel + ContentPanel）。必须有 6.1 测试兜底。

**手测点**：审批流 approve/reject 后 diff tab 正常；搜索章节高亮；切小说 tabs 清空；skill 打开。

**commit**：`refactor(content): replace imperativeHandle with declarative tab store`

---

## 阶段 6 完成标准

- 巨石组件拆分（如做了）
- imperativeHandle 去除（如做了）
- 所有测试全绿
- 手测审批流 + 切小说 + 搜索高亮全通过

---

## 整体改造完成

回到 [README.md](./README.md) 确认所有阶段勾选完成。
