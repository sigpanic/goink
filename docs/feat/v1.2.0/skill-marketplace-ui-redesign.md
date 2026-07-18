# v1.2.0 — Skill 市场前端 UI 设计

## 背景

后端已完成 Skill 市场相关 API（`ListRemoteSkills` / `GetRemoteSkillContent` / `InstallRemoteSkill`，均返回 `*apperr.Result[T]`），仓库指向 `sigpanic/goink-skills`。前端需要新增市场入口、实现浏览/详情/安装交互，并对安装时的同名覆盖给出明确确认。

参考已有模块交互：
- `PatternExtractView` 的 `idle | extracting | preview` phase 切换 + 顶部工具栏布局
- `StyleView` 的详情弹窗 + 底部按钮区
- `StyleSampleCard` 的玻璃态卡片样式
- `SkillPreview` 的 frontmatter 表格 + mode 彩色 tag

## 整体设计

市场入口放在 `SkillList.tsx` 顶部按钮区，以 Store 图标按钮形式出现，点击后打开 `SkillMarketplace` 全屏遮罩弹窗。

弹窗尺寸要明显大于普通的详情弹窗（参考 `StyleView` 的 `w-[900px] h-[88vh]` 是详情级尺寸）。市场是浏览 + 详情 + 覆盖对比的多用途容器，且卡片网格需要足够横向空间展示，因此不论仓库里 skill 数量多少，弹窗都应保持大尺寸：宽度撑到 `w-[min(1600px,96vw)]`、高度撑到 `h-[92vh] max-h-[94vh]`。即使 skill 很少也不缩小，避免内容少时弹窗过小、内容多时尺寸跳变的视觉抖动。

弹窗内部不使用多个 modal 嵌套，而是采用 **phase 状态切换**：浏览态、详情态、覆盖确认态共用同一个弹窗容器，仅切换顶部工具栏和内容区。这与 `PatternExtractView` 的 preview 切换逻辑一致，避免层级混乱。

### phase 状态机

```
browse              市场列表，玻璃态卡片网格 + 分页
  ↓ 点击卡片
detail              单个 skill 详情，frontmatter 表格 + markdown
  ↓ 点安装且目标层有同名
confirm_overwrite   覆盖确认，左右两栏对比本地 vs 远程
```

三态都通过顶部按钮在状态间切换，不弹新窗。安装只从 detail 态触发，覆盖确认只从 detail 态触发。

## 各 phase 设计

### browse 态

顶部左侧：Store 图标 + 市场标题
顶部右侧：搜索框（防抖 300ms）+ 刷新按钮

内容区：玻璃态卡片网格，使用 `grid-cols-[repeat(auto-fill,minmax(280px,1fr))]` 自适应列数。卡片整体可点击，点击进入 detail 态。

卡片采用与 `StyleSampleCard` 一致的玻璃态样式：
- 未安装：`bg-card/80 backdrop-blur-2xl border border-white/15`，hover 时 `hover:border-primary/20 hover:shadow-lg hover:-translate-y-0.5`
- 已安装：浅蓝色玻璃 `bg-sky-50/60 backdrop-blur-2xl border border-sky-200/50 opacity-75`，hover 时 opacity 回到 100

卡片内容自上而下：skill 名（粗体）+ 版本号（淡色右对齐）、description 完整显示（不截断）、category + mode 彩色 tag + author。已安装的卡片右下角常驻"已安装 v{version}"小标签。

mode 的彩色 tag 复用 `SkillPreview` 的颜色约定：manual→`bg-tag-blue`、always→`bg-tag-green`、auto→`bg-tag-amber`。

底部：分页栏。每页默认 20 条，可选 10/20/50。显示"共 N 条 · 第 X/Y 页"，提供页码导航和 pageSize 下拉。注意这是真分页（后端 `PageParams.Normalize` 归一化），不是无限滚动。

错误条：网络/限流/未找到/拒绝/其他错误时，在卡片网格上方显示 destructive 风格的错误条，按 `err_code` 分类展示文案。network/rate_limited/other 类错误额外显示"重试"按钮。网络错误时再额外显示一行可点击的仓库链接提示（`BrowserOpenURL` 打开 `https://github.com/sigpanic/goink-skills`），告知用户可手动访问仓库或换网络环境。

### detail 态

顶部左侧：返回按钮（ArrowLeft 图标）+ skill 名 + v版本号
顶部右侧：安装到用户层 + 安装到小说层 两个主操作按钮

novel 层按钮在 `!novelId` 时 disabled。已安装时按钮文字改为"重新安装到..."。loading 时按钮显示 spinner + "安装中..."。

内容区参考 `PatternExtractView` preview 态的 `flex-1 min-h-0 overflow-y-auto px-6 py-4 space-y-3`：

上区是 frontmatter 表格，使用 `border bg-muted/20 w-full text-sm rounded-lg overflow-hidden`。左列固定窄宽 `w-20`、灰字标签；右列是字段值。字段顺序：name（粗体）、description、category、mode（彩色 tag + 国际化文案）、author、version。表格仅在对应字段存在时渲染该行。

下区是 markdown 正文，用 `rounded-lg border bg-muted/10 p-4` 包裹 `<Markdown content={body} />`。

进入 detail 态时调 `GetRemoteSkillContent(name)` 拉取远程内容，用 `splitFrontmatter` 拆成 frontmatter 和 body。加载中显示 spinner + 文案，加载失败显示错误提示。

### confirm_overwrite 态

顶部左侧：返回 detail 按钮 + 覆盖确认标题
顶部右侧：取消（切回 detail）+ 确认覆盖（destructive 风格）

内容区顶部一行 amber 色提示条，文案形如"如果继续，将使用远程内容替换本地 {用户层/小说层} 的 skill"，其中 {target} 根据 `installTarget` 国际化。

下方左右两栏对比，每栏一个 `border rounded-lg overflow-hidden` 容器，顶部 header 显示"本地当前内容" / "远程新内容"，下方滚动区分别展示 splitFrontmatter 后的精简 frontmatter 表格（`text-xs`）和 markdown body。

## 安装流程设计

detail 态点安装按钮时，不能直接调 `InstallRemoteSkill`，因为目标层可能已有同名 skill。需要先探测：

**GetContent 试探方案**：调用现有 `app.GetContent(novelID, path)` 探测目标层是否有同名 skill。
- `pathForSource('user', name)` = `~/.goink/skills/{name}.md`
- `pathForSource('novel', name)` = `skills/{name}.md`

`GetContent` 对这两类路径走 `git.ReadFile` 直接读文件（不走 `skill.Get` 优先级查找），返回值约定：文件存在返回内容字符串，文件不存在返回空串（不报错）。

所以：
- GetContent 返回非空 → 目标层有同名 → 切 confirm_overwrite 态，本地栏展示返回的内容，远程栏调 `GetRemoteSkillContent` 拿到展示
- GetContent 返回空 → 目标层无同名 → 直接调 `InstallRemoteSkill` 安装

确认覆盖时直接调 `InstallRemoteSkill`，后端会覆盖目标层文件。

安装成功后切回 browse 态，刷新本地 skill 索引（重新调 ListSkills）和市场列表，并触发 `onInstalled` 回调让 `SkillList` 也刷新。

## 已安装状态判断

`SkillList` 用 `ListSkills` 拿到的是按优先级去重后的元数据（`skill.Store.ListMeta` 实现：novel > user > builtin，同名只返回最高优先级）。前端无法通过这个 API 准确判断"目标层是否有同名 skill"，所以安装流程用上面的 GetContent 试探。

但 browse 态卡片只需要判断"该 skill 是否已在任意层安装过"（用于卡片视觉区分），不需要精确到目标层。所以用 ListSkills 构建 `Set<name>` 索引即可（不关心 source），同时构建 `Map<name, version>` 用于卡片右下角的"已安装 v{version}"标签（同名取最大版本）。

## 关闭与重置

弹窗 open 变 false 时重置所有状态：phase 回 browse、selectedSkill 清空、remoteContent / localContent / remoteContentForConfirm 清空、installTarget 回 user、query / debouncedQuery 清空、page 回 1、error / contentError 清空。

遮罩点击行为按 phase 区分：browse 态点遮罩可关闭（防误操作但允许快速退出）；detail / confirm_overwrite 态点遮罩不关闭（避免误退出丢失浏览上下文）。✕ 按钮在所有态都可关闭。

## 文件改动

| 文件 | 改动 |
|---|---|
| `frontend/src/components/skill/SkillMarketplace.tsx` | 新建。主组件，约 700-800 行 |
| `frontend/src/components/skill/SkillList.tsx` | 顶部按钮区加 Store 图标按钮（市场入口），加 `marketplaceOpen` state，加 `handleMarketplaceInstalled` 回调（刷新本地列表），末尾渲染 `<SkillMarketplace>` |
| `frontend/src/hooks/useApp.ts` | 导出 `ListRemoteSkills` / `GetRemoteSkillContent` / `InstallRemoteSkill` 三个方法 + `apperr` / `storage` / `remote` 三个类型 |
| `frontend/src/i18n/locales/zh-CN.json` | `skill` 命名空间下新增 `marketplace` 子命名空间 |
| `frontend/src/i18n/locales/en.json` | 同上，注意 `pagination` 按 i18next v4 复数约定拆 `_one` / `_other` |

## i18n key 设计

`skill.marketplace.*` 子命名空间覆盖：标题/关闭/搜索/刷新/加载/空态/仓库链接、分页相关、已安装标签、安装按钮（含重新安装/loading 态）、返回/取消、frontmatter 字段名、mode 文案（智能/命令式/常驻）、内容加载/错误、novel 必选提示、安装成功/失败、覆盖确认相关（标题/警告/本地内容/远程内容/确认按钮）、5 类 GitHub API 错误 + 通用错误 + 重试。

zh-CN 用 `pagination` 单 key；en 按 v4 复数约定拆 `pagination_one` / `pagination_other`。

错误文案要点：
- network：明确提示 api.github.com 国内基本可直连，建议检查网络重试、手动访问仓库、或换能连 GitHub 的环境
- rate_limited：提示未认证 60 次/小时限制
- not_found / forbidden / other：相应提示

## 关键技术点

- Wails API 返回类型：
  - `ListRemoteSkills` / `GetRemoteSkillContent` / `InstallRemoteSkill` 返回 `apperr.Result[T]`（含 `data` / `err_code` / `err_msg`），前端需检查 `err_code`
  - `GetContent` 直接返回 `Promise<string>`（非 Result 包装）
  - `ListSkills` 直接返回 `Promise<skill.SkillMeta[]>`（非 Result 包装）
- 防抖搜索：query 变化 300ms 后同步到 debouncedQuery 并重置 page 为 1
- 分页参数：page 从 1 开始，size 默认 20
- Props：`{ open, onOpenChange, novelId, onInstalled? }`
- 复用现有工具：`splitFrontmatter`（`@/components/content/types`）、`Markdown`、`toastError`、`BrowserOpenURL`
- `SkillMarketplace.tsx` 用 Write 新建；其他 4 个文件用 Edit 精确修改，不覆盖整文件

## 校验

完成后跑：`go build ./...` + `go test ./...` + `cd frontend && npm run build` + `eslint` + `i18n:check`。
