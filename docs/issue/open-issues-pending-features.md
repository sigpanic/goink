# Open Issues 承诺功能落地核查

> 核查时间：2026-08-13
> 核查范围：sigpanic/goink 公开仓库全部 17 个 open issue
> 核查方法：在 feat/goink-wt worktree 用只读 git 命令（`git log master`、`git show master:path`、`git grep master`）核查 master 分支代码实际状态，对照 issue 中 owner 的承诺
> 核查人：AI agent

## 一、核查结论总览

| 状态 | 数量 | 说明 |
|---|---|---|
| 已实现（master 已落地） | 2 项 | #37 合并 system 消息、#31 搜索联动定位 |
| 部分实现 | 1 项 | #30 SSE 原始内容 debug 日志（采样+异常输出，非每 chunk debug） |
| 仍未实现 | 7 项 | #36、#33（大纲编辑/ZIP 导出/实体导出）、#27（分卷/章节删除）、#26 |
| 无需跟进 | 7 项 | 已修复（#29/#28/#21/#17/#8）、非代码问题（#34）、闲聊（#12/#23）、部分修复待确认（#24/#32） |

## 二、已实现功能（master 已落地，feat 分支需 rebase 拉取或已有扩展）

### 1. [#37](https://github.com/sigpanic/goink/issues/37) 合并多条 system 消息（llama.cpp 适配）

- **承诺时间**：2026-08-12
- **承诺内容**：在 LLM 出站请求层做归一化，合并多条 system 消息为单条置首；建议增加配置开关 `merge_multiple_system_messages`
- **master 实现状态**：已实现（commit `d3145f9`）
  - `internal/llm/stream.go:198` `buildPayload` 调用 `mergeSystemMessages`
  - `internal/llm/stream.go:273-305` 函数定义：收集所有 `role=system` 非空 string 内容，用空行拼成单条置首
  - `internal/llm/stream_test.go:147-303` 9 个测试用例覆盖
- **遗留**：**未实现配置开关**，当前为无条件出站归一化（`merge_multiple_system` 关键词全仓 0 命中）。issue 建议的"对原生支持多条 system 的后端可关闭合并"未做
- **分支差异**：**feat/goink-wt 分支缺失此功能**（未 rebase master），`mergeSystemMessages` 在 feat 全仓 0 命中。需 `git rebase master` 拉取

### 2. [#31](https://github.com/sigpanic/goink/issues/31) 搜索点击联动定位

- **承诺时间**：2026-08-03
- **承诺内容**：随前端架构重构一起优化搜索体验，统一处理各菜单的联动定位
- **master 实现状态**：已实现
  - `frontend/src/components/search/SearchPanel.tsx:24-31,163,171` 暴露 `onNavigateEntity(panelId,entityId)` / `onNavigateChapter(filePath,title,chapterNum,matchPos,matchLen)`，点击结果触发
  - `sidebar/SidePanel.tsx:40-47,131-132` 接线
  - `views/WorkspaceView.tsx` master 用 `useState` 维护 character/location/reader/preference/setting 5 类实体 focusId，prop 下传
  - `CharacterGraph.tsx`、`LocationGraph.tsx`、`reader/ReaderView.tsx`、`style/StyleView.tsx` 用 focusId find + 聚焦节点
  - `content/ContentPanel.tsx:305,310` + `ContentPanel.css` 用 `search-context-highlight`/`search-keyword-highlight` 高亮章节正文匹配位置
- **分支差异**：feat/goink-wt 是 master 的**重构 + 扩展版**
  - feat 重构为 zustand `stores/useFocusStore.ts` + `hooks/useFocusWithNonce.ts`
  - 各 List 组件订阅 `focusEntity`，各 View 用 `useFocusWithNonce(panelId)`
  - **扩展到更多实体**：新增 storyarc、timeline、novel-setting 的 focus 订阅
  - SearchPanel 的 `onNavigateEntity/onNavigateChapter` 接口两分支一致

## 三、部分实现

### 3. [#30](https://github.com/sigpanic/goink/issues/30) SSE 原始内容 debug 日志

- **承诺时间**：2026-08-03
- **承诺内容**：增加 SSE 原始内容 debug 日志，便于排查"空 body"还是"仅 reasoning"
- **master 实现状态**：部分实现（commit `79fd71d`）
  - `internal/llm/stream.go:319` `sseDiagnostics` 结构
  - `stream.go:327-334` 采集前 10 行 + 后 10 行原始 SSE 行样本（环形缓冲）
  - `stream.go:353` JSON 解析失败时 `logger.Warn("SSE JSON parse failed",...)`
  - `stream.go:542-548` 空响应时 `logger.Warn("empty sse response","diag",diag)` 输出含原始行样本 + HTTP 快照的结构化诊断
- **未达成的部分**：不是每 chunk 的原始 debug 日志，属**采样 + 异常时输出**。日常正常流的原始 chunk 不记录
- **分支差异**：feat 与 master 一致

## 四、仍未实现（master 和 feat 都没做）

### 4. [#36](https://github.com/sigpanic/goink/issues/36) 工具连续失败直接中断对话

- **承诺时间**：2026-08-10
- **承诺内容**：Goink 侧加强打断机制，工具连续失败时直接中断对话，不再靠提醒
- **现状**：未达到承诺
  - `internal/agent/agent.go:343-351` 仍只 `appendMsg` 一条 `<system-reminder>已被禁用</system-reminder>` 提醒 LLM，**未真正禁用工具，也未中断对话**
  - `failCnt[name]` 只在 `!Success && ErrKind=="system"` 时计数（agent.go:343），**参数错误（ErrKind=""）不计入**——正是 issue #36 报告的场景未覆盖
  - `failCnt[name]==3` 时只 appendMsg 提醒，循环继续
  - `interrupted=true`（agent.go:231）仅用于 ctx 取消（用户手动中止），与工具失败无关
- **gap**：① 未实现"直接中断对话" ② 未覆盖参数错误类失败 ③ 未真正禁用工具（只是文字提醒）

### 5. [#33](https://github.com/sigpanic/goink/issues/33) 章节大纲手动编辑入口

- **承诺时间**：2026-08-04
- **承诺内容**：后续会加手动编辑入口（目前只能通过对话让 AI 生成大纲）
- **现状**：未实现
  - `frontend/src/components/content/OutlineViewer.tsx` 只有只读渲染（`<Markdown content={content} />`），无 onChange/onSave
  - `ContentPanel.tsx:827` 用 OutlineViewer 渲染 outline tab
  - 全仓无 `OutlineEditor`/`editOutline`/`saveOutline`/`onOutlineChange`
- **临时方案**（issue 中已告知用户）：直接编辑文件系统 `Goink/novels/{id}/outlines/NNN.md`

### 6. [#33](https://github.com/sigpanic/goink/issues/33) 导出为 ZIP（每章一个 .md 文件）

- **承诺时间**：2026-08-04
- **承诺内容**：后续会加"导出为 ZIP（每章一个 .md 文件）"的格式分支，已加入待办
- **现状**：未实现
  - `internal/export/export.go:26-36` `ExportNovel` switch 只支持 `epub`/`markdown`/`txt`
  - `markdown.go:11-51` 导出为**单个 .md**（书名 + 目录 + 全部章节拼接），非每章一文件打包
  - `archive/zip` 仅出现在 `epub.go`（EPUB 内部打包）和 `export_test.go`（测 EPUB），无 ZIP 导出分支

### 7. [#33](https://github.com/sigpanic/goink/issues/33) 角色 / 故事线 / 时间线导出（Markdown）

- **承诺时间**：2026-08-04（owner 询问格式，用户回复"Markdown 更加友好"，owner 暗示会做但未明确承诺时间）
- **现状**：未实现
  - 全仓 `exportCharacter`/`exportStory`/`exportTimeline`/`导出角色` 等关键词 0 命中
  - `internal/export/` 只处理 `novel + chapters`，无任何实体导出

### 8. [#27](https://github.com/sigpanic/goink/issues/27) 分卷功能

- **承诺时间**：2026-07-29
- **承诺内容**：计划在再下一个版本增加，支持按卷分组、设置每卷包含的章节范围
- **现状**：未实现
  - 仅 `internal/import/txt.go:22,26,39` 有卷标记正则（`第X卷`/`卷N`），用于 txt 导入时**切分章节**的启发式
  - `import_test.go:313`/`txt_test.go:242-276` 测试上述导入切分
  - 无 volume 实体/模型/DB 字段/管理 UI（`VolumeID`/`volumeId` 全仓 0 命中）

### 9. [#27](https://github.com/sigpanic/goink/issues/27) 章节删除

- **承诺时间**：2026-07-29（owner 询问场景，2026-08-06 用户已回复"中间章节合并/结构调整删除"，**owner 未回应**）
- **现状**：未实现，且 owner 未对用户场景回复做回应
  - `app/chapter.go:19,28,33,38` 只有 `GetChapters`/`GetMaxChapterNumber`/`UpdateChapterTitle`/`CreateChapter`，**无 `DeleteChapter`**
  - `internal/rag/vector_store.go:211` 的 `DeleteChapterChunks` 仅清向量，非删章节
  - 前端 ChapterList 无删除入口

### 10. [#26](https://github.com/sigpanic/goink/issues/26) 多模型适配国外主流服务商

- **承诺时间**：2026-07-19（"1.3.0 版本推出"），2026-08-01 更新（"仍在计划中，会尽快推进"）
- **现状**：未实现
  - `internal/llm/providers.go:5-243` `Builtin` 仅 7 家国内 provider：`deepseek`/`doubao`/`qwen`/`zhipu`/`minimax`/`mimo`/`moonshot`
  - `anthropic`/`claude`/`gemini`/`openrouter`/`groq`/`together`/`mistral` 在 providers.go/config.go/前端 settings 全仓 0 命中
  - `internal/llm/web_search.go:27` 用 DeepSeek 的 Anthropic 兼容端点做联网搜索，非通用 provider 预设

## 五、已承诺且已实现（对照参考）

- [#27](https://github.com/sigpanic/goink/issues/27) AI 对话历史记录删除 — 已实现
  - `frontend/src/components/chat/DeleteSessionDialog.tsx`
  - `frontend/src/components/chat/useDeleteSession.ts`
  - `app/chat_api.go` 后端绑定
- [#17](https://github.com/sigpanic/goink/issues/17) 风格素材库批量导入 — 已实现（v1.1.0）
- [#8](https://github.com/sigpanic/goink/issues/8) AI 模型/书籍管理 — 已实现（#10）

## 六、无需跟进

| issue | 原因 |
|---|---|
| #29 | v1.3.1 已修复（模型 ID 含 `/` 的截断 bug） |
| #28 | v1.3.0 已修复（macOS 签名校验失败） |
| #21 | v1.1.0 已修复（macOS 26 创建作品失败） |
| #34 | 非代码问题（KIMI 账户 TPD 配额超限） |
| #12 / #23 | 闲聊/非功能反馈 |
| #24 | URL 处理已部分修复，GLM 端点已说明 |
| #32 | 章节格式识别已修复；模型配置展示问题需用户确认是否复现 |

## 七、分支差异与建议

### feat/goink-wt 与 master 的功能差距

| 功能 | master | feat/goink-wt | 建议 |
|---|---|---|---|
| 合并 system 消息（#37） | 已实现 | **缺失** | `git rebase master` 拉取（需用户授权） |
| 搜索联动定位（#31） | 已实现（useState） | 已实现（zustand 重构 + 扩展实体） | feat 版本更优，合并回 master 时以 feat 为准 |
| SSE 诊断（#30） | 部分实现 | 一致 | 无差异 |
| 其余 7 项 | 未实现 | 未实现 | 无差异 |

### 后续行动建议（按工作量与影响排序）

1. **rebase 拉取 #37**：feat 分支落后 master，rebase 即可拿到合并 system 消息功能，零开发成本
2. **#36 中断对话机制**：改动集中在 `agent.go` 失败计数逻辑，影响所有用户的工具调用循环体验，收益高
3. **#33 ZIP 导出 + 实体导出**：导出模块独立，改动隔离，可一并做
4. **#27 章节删除**：涉及交叉引用清理（角色出场章/时间线/伏笔/故事弧），工作量大，需先设计
5. **#26 国外 provider 适配**：provider 层独立，但需逐家测试，工作量大
6. **#33 大纲手动编辑入口**：前端独立改动，工作量小
7. **#27 分卷功能**：涉及 DB schema + 章节 model + UI，工作量最大，建议最后做
