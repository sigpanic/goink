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

## 步骤总览

| 阶段 | 文档 | 步骤范围 | 风险 | 前置条件 |
|---|---|---|---|---|
| 1 基建 | [01-foundation.md](./01-foundation.md) | 1.1 装依赖 → 1.7 P1 测试 | 零~低 | 无 |
| 2 拆 WorkspaceView | [02-workspaceview.md](./02-workspaceview.md) | 2.1 PanelId 类型 → 2.8 useFocusStore | 低~中 | 1.7 测试就位 |
| 3 小说领域模板 | [03-novel-template.md](./03-novel-template.md) | 3.1 useNovels → 3.9 4 处消费方迁移 | 中 | 阶段 2 完成 |
| 4 7 实体批量 | [04-entities-batch.md](./04-entities-batch.md) | character → novel-setting，删 refreshNonce | 中 | 阶段 3 验证手感 |
| 4b 搜索补全 | [04b-search-preference-setting-reader.md](./04b-search-preference-setting-reader.md) | preference/setting/reader 接入搜索 | 低~中 | 阶段 4 完成（正交于阶段 5，可并行）|
| 5 其他模块 | [05-misc-modules.md](./05-misc-modules.md) | chat/content/skill/git/search | 中 | 阶段 4 完成 |
| 6 拆巨石（可选） | [06-monolith-optional.md](./06-monolith-optional.md) | ChatPanel/ArcListView/去 imperativeHandle | 高 | 痛点驱动，先扩测试 |

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
