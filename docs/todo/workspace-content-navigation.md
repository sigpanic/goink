# WorkspaceView 内容导航桥接拆分

## 现状与目标

`WorkspaceView` 同时处理布局、面板路由和 SidePanel 到 `ContentPanel` 的打开文件命令。章节、故事状态、卷纲、技能和搜索章节的回调均依赖 `contentRef`；继续增加内容类型会让主 View 承担更多路径和标题细节。

本次只把内容导航回调抽到 `frontend/src/views/workspace/useContentNavigation.ts`。这是 Workspace 层的组件桥接，放在 `views/workspace/`，而不是内容领域 Store：当前 `ContentPanel` 仍通过 `ContentPanelHandle` 打开文件，尚未实施统一容器 tab 模型或去 imperativeHandle 的重构。

## 行为边界

- 章节与卷纲使用后端返回的路径；章节号只用于展示标题。故事状态仍使用固定 `goink.md`，技能的新建入口沿用现有 `skills/{name}.md` 规则。
- 选择章节、故事状态或卷纲时，先更新 `useEditorStore.tabTarget`，再调用 `contentRef.current?.openFile`。技能入口继续更新 `activeSkillName` 并保留原有只读与初始编辑模式。
- 搜索章节跳转保留 `flushSync(() => setActivePanel("chapters"))`，确保条件渲染的 `ContentPanel` 已挂载后再调用 ref；有高亮位置时使用 `openFileWithHighlight`，否则使用 `openFile`。
- 面板切换、实体搜索、小说操作和审批回调留在 `WorkspaceView`；本次不改变审批与 diff tab 的调用顺序。

## 实施与验证

先单独 review 并修复卷纲前端提交发现的问题：关闭普通标签不应取消尚未执行的自动保存；卷纲重开时的标题刷新只作用于卷纲。该修正与导航拆分分成两个提交，避免功能修复混入纯重构。

导航拆分涉及新 hook、`WorkspaceView.tsx` 和 `WorkspaceView.test.tsx`。验证侧栏打开章节、故事状态、卷纲和技能，及从非章节面板搜索跳转的挂载顺序；运行前端 build、lint、test。主要风险是把 `flushSync` 移到 hook 时改变 ref 的挂载时序，或漏传技能的只读/编辑模式。
