# commit 路线（细化版）

> 配套 [volume-chapter-id-refactor.md](./volume-chapter-id-refactor.md) 的实施步骤细化。只写做什么，不写怎么做。一切以代码为准。

## 路线修正（2026-09）

**保留小粒度 commit**（每个 commit 做一件事、可独立 review），**放弃的只是「每个 commit 必须可编译」这个约束**。

原方案要求每个 commit 可独立编译，导致底层与上层互相等待：底层签名要 id、上层此刻只有 num → 底层先改则编译不过，上层先改则要先能拿到 id。为绕开死结只能硬造中间态（双字段共存、num→id 反查桥接、TODO 标注），同一处改两遍。

而原 PR1 本身就要求「必须一起合并，中间状态编译失败」——**既然中途必然不可编译，维护「每 commit 可编译」的假象没有意义**。

**现方案**：自底向上分层平推，小粒度 commit 保留，「可编译」列仅作标注。中间层提交用 `--no-verify` 跳过 hook。

## 字段清单（确认完整）

| 表 | 字段 | 类型 |
|---|---|---|
| chapters | ChapterNumber | int |
| time_entries | TargetChapter/SourceChapter/ResolvedChapter | int |
| arc_nodes | TargetChapter/ActualChapter | int |
| reader_perspectives | PlantedChapter/RevealedChapter | int |
| writing_log | ChapterNumber | int |
| character_relations | ChapterNumber | int |
| vec_novel_{id}（虚拟表，非 GORM） | chapter_number | integer |

GORM model 层 6 张表 11 个字段已确认无遗漏。vec_novel_{id} 虚拟表（RAG 向量索引）含 chapter_number 列，需单独处理。

## 分层顺序

| 层 | 主题 | 为什么在这个位置 |
|---|---|---|
| **L0** | `internal/volume` store 基建 | 最前：migrate 步骤与 rw_tools 的 new.md 通道都依赖卷基建 |
| **L2** | chapter.Store 按 id / sort_order | L0 之后：sort_order 分配已由 store 接管 |
| **L3** | rag + search | 依赖 L2 的按 id 查询 |
| **L4** | 其他内部包（export / pattern / agent） | 依赖 L2 |
| **L5** | mcp_tools | 依赖 L2-L4 |
| **L6** | app 层 | 依赖 L0 的 volume store |
| **L7** | 前端 | 依赖 L6 的 API |
| **L1b** | 收尾：删旧 SQL 列 | **最后**：删 SQL 列受迁移步骤顺序约束——1.7 必须在 1.5 反查、1.6 文件 rename 之后（它们要读 num 列）。`chapter.ChapterNumber` 随 L2 一次性移除 |

**关于「解绑旧列约束」（原 L1a，已撤销）**：曾计划早期先去掉 `not null` / `uniqueIndex` 让代码停止写 num，已撤销：

- **不需要**：中间不运行应用，没有任何 INSERT 发生，not-null 约束在重构期不构成约束
- **SQLite 不支持 `ALTER COLUMN`**：单独去 `NOT NULL` 需重建整表，纯属白折腾
- **不需要「双写兼容」**：不存在中间发行版（用户不会跑到半成品），代码无需「有 id 用 id、没 id 回退 num」的兼容逻辑

**双字段为什么存在**：不是设计需要，是「代码还没改到」的产物。id 列已由 1.5 反查填好但暂无代码读它；num 列仍被 L3-L6 的代码读写。两者共存只因代码改造（L0/L2-L6）未完成。

## commit 路线

「可编译」列标注该 commit 后代码能否通过 `go build`；层内相邻的 ❌ 可合并提交。

### L0 基建 — `internal/volume` store

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 0.1 | `feat(volume): add store with CRUD` | 新建 `internal/volume/store.go`：Create / Update / Delete / GetByID / ListByNovel / Reorder；方法以 `*gorm.DB` 为参数（调用方传 db 或 tx），不用 receiver 持连接风格——`SetMaxOpenConns(1)` 下事务内经外部连接查询会死锁 | ✅ |
| 0.2 | `feat(volume): add chapter sort_order allocation` | 把 `sort_order` 分配算法从 `mcp_tools/rw_tools.go` 的 `createChapterRecord` 下沉：按目标分组（指定卷或未分卷）的 max+1 追加 | ✅ |
| 0.3 | `refactor(rw_tools): call volume store` | rw_tools 的 `resolveVolume` / `lastVolume` / `createChapterRecord` 改为调用 volume store，删除自实现 | ✅ |

### L2 chapter.Store — 按 id / sort_order

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 2.1 | `refactor(chapter): list queries order by sort_order` | `ListByNovel` / `ListAllByNovel` / `SearchByNovel` 按卷、`volumes.sort_order`、`chapters.sort_order` 升序，未分卷最后；`GetRecent` 取该顺序末尾 N 章并倒序返回 | ✅ |
| 2.2 | `refactor(chapter): create records through store` | 新增 `chapter.Store.Create`：在同一事务中处理目标分组的 `sort_order` 分配与记录创建；rw_tools 直接调用，删除 `createChapterRecord`，并在未指定卷时选择最后一卷；不再为新记录分配旧 `chapter_number` | ✅ |
| 2.3 | `refactor(chapter): remove legacy number API` | 新增 `GetReadingNumberByID`，按当前阅读序实时算章号；`CountByNovel` 返回总章节数（即最大展示章节号）；`GetByNovelAndNumber` → `GetByID`；删 `GetLatestNumber`；`UpdateTitle` 改按 id；`chapter.Chapter` 删除 `ChapterNumber`。其他领域调用方留待各自迁移，因此本提交预期不可编译 | 🟡 |
| 2.4 | `refactor(chapter): store methods take *gorm.DB` | 方法风格整改为 `*gorm.DB` 参数（同 L0 约束，`SetMaxOpenConns(1)` 下事务安全） | ❌ |

### L3 rag + search

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 3.1 | `refactor(rag): chunk_id keyed by chapter id` | `splitter.go` 的 chunk_id 去掉内嵌章节号（`"%d_summary"` / `"%d_brief"` / `"%d_%d"` → id-based）；改完需重建向量 | ❌ |
| 3.2 | `refactor(rag): SubmitRefresh takes chapter id` | `SubmitRefresh` 改按 chapter_id 提交；删 `refresh_queue` 的 num→id 反查桥接；`RefreshTask` 去 `ChapterNumber` | ❌ |
| 3.3 | `refactor(search): ChapterNum becomes ChapterID` | `search.Service` 字段 `ChapterNum` → `ChapterID`；展示按 id 反查实时章节号 + 卷名拼"卷一·第10章" | ❌ |

### L4 其他内部包

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 4.1 | `refactor(export): chapter number computed at render` | epub / txt / markdown 的章节号来源改 id + 实时算 | ❌ |
| 4.2 | `refactor(pattern): chapter fields use id` | `pattern` 的 extract / prompts / types 章节字段改 id | ❌ |
| 4.3 | `refactor(agent): display computes chapter number` | `agent/display.go` 章节号展示改实时算 | ❌ |

### L5 mcp_tools

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 5.1 | `refactor(mcp_tools): cross-ref tools use chapter_id` | timeline / storyarc / reader / character_relations 工具的章节字段改 `*_chapter_id`，AI 直接传 id 不转译 | ❌ |
| 5.2 | `feat(mcp_tools): get_chapter_list returns volume and live number` | `get_chapter_list` 返回 id + 实时 chapter_number + volume_name + title | ❌ |
| 5.3 | `feat(rw_tools): support volume outline paths` | 卷纲路径 `volumes/{id}.md` 支持（正则 + 读写分支） | ❌ |
| 5.4 | `refactor(mcp_tools): memory/delete tools use chapter_id` | memory_tools 章节过滤改 id；delete_tools 同步 | ❌ |

### L6 app 层

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 6.1 | `feat(volume): app-layer CRUD` | app: CreateVolume / UpdateVolume / DeleteVolume / GetVolumes / ReorderVolumes（删卷前检查关联章节）；wails 绑定自动生成 | ❌ |
| 6.2 | `feat(chapter): app-layer delete/insert/move` | app: DeleteChapter（交叉引用检测拒绝 + 删文件 + 删记录 + RAG 清理）/ InsertChapter / MoveChapterToVolume；wails 绑定自动生成 | ❌ |
| 6.3 | `refactor(chapter): CreateChapter allocates sort_order` | `CreateChapter` 改走 volume store 分配 sort_order（当前用 `GetLatestNumber`）；`UpdateChapterTitle` 改按 id；novel export / content.go 同步 | ❌ |

### L7 前端

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 7.1 | `feat(frontend): chapter management tab skeleton` | 新增独立 tab"章节管理"：panel.ts + ActivityBar + WorkspaceView 分支 + ChapterManagementView 主骨架；现有 ChapterList 保留；i18n key | ✅ |
| 7.2 | `feat(frontend): volume management panel` | 卷管理面板：CRUD UI + 排序；按卷分组渲染 | ✅ |
| 7.3 | `feat(frontend): chapter operations UI` | 章节 [⋮] 菜单：删除 / 插入 / 移动；拖拽跨卷移动 | ✅ |

### L1b 收尾 — 删旧字段

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 1.1 | `refactor(migrate): guard legacy num columns` | 1.5/1.6 补 num 列的 `HasColumn` 守卫（防 migrate_state 状态丢失后查不存在的列 → 启动失败） | ✅ |
| 1.2 | `refactor(migrate): drop legacy num columns` | 启用 migrate 1.7——先 DROP INDEX（`uk_novel_chapter` + writing_log 章节号索引）再 DROP COLUMN，删 chapter + 5 张交叉引用表旧 num 列；交叉引用 model 字段在对应领域改完后移除。`chapter.ChapterNumber` 已在 L2 移除 | ❌ |

**删列与删字段是两个独立约束，别绑在一起**：

| 动作 | 唯一约束 |
|---|---|
| **删 model 字段** | 代码引用要改完，或明确将其作为后续领域的编译迁移清单。中间不运行应用，所以 not-null、INSERT 全不构成约束——**任何时候都能删** |
| **删 SQL 列** | 迁移代码还要不要读它。1.5 反查读 num 列 + `chapters.chapter_number`；1.6 文件 rename 读 `chapters.chapter_number`（拼 `{num:03d}.md` 源路径）。1.7 必须在 1.5/1.6 之后 |

迁移读 num 走 raw SQL + 匿名结构体（`crossref.go` / `rename.go`），**不依赖 model**——这也是两者独立的证据。

放同一个 commit 只是因为它们服务于同一个终态，不是因为互相绑定。

## 注意事项

**中间层提交用 `git commit --no-verify` 跳过 hook**（hook 会跑 go build/test/lint，中间态必然失败）。进入下一层前确认本层已自洽。

**数据迁移部分仍必须幂等可重跑**（1.4/1.5/1.6/1.7 的 step 内部自检），这是运行时破坏性操作，与代码 id 化是两回事。

**迁移反查与 model 解耦**：`crossref.go` / `rename.go` 读 `chapter_number` 用的是 raw SQL + 局部匿名结构体，不依赖 `chapter.Chapter`。因此 L2.3 删 model 字段**不会**让迁移步骤编译失败。

## 已完成

| 内容 | commit |
|---|---|
| migrate 框架（engine + registry + Step 接口） | `a1e66eb` |
| migrate 1.5 交叉引用 num→id 数据重写 | `a1e66eb` |
| migrate 1.4 自动备份 | `f8b7c6b` |
| migrate 1.6 sort_order 初始化 + 文件 rename | `d90888b` `c4dec20` |
| git：ChapterPath / OutlinePath / VolumePath 按 id | `cc43909` |
| rag：vec 表列改 chapter_id（DROP 重建） | `162ccee` |
| model：volume / migrate_state 表；chapter 加 volume_id + sort_order；5 张交叉引用表加 `*_id` 列 | 各 commit |
| rw_tools：id 化 + new.md 通道 | `3f5c6b3` |

## 不做的事

- 不写一次性脚本迁移 LLM 历史调用记录（用户确认可接受）
- 不重排章节号（删除留"实时连续"效果，章节号不存 DB）
- 不修改 system 消息字段命名（仅改 DB 与工具层）
- 不实时 InjectMessage 提醒 num 变化（AI 拿不准时主动调 list_chapters 刷新；可选 chat 时检查一致性，低优先级后续做）
- 不扩展 `delete_record` 支持 chapter 表（结构操作仅前端）

## 待确认

1. 章节号实时计算的语义：删除第 3 章后第 4 章变第 3 章，AI 上下文里的"第 5 章"指向变化，仅靠 AI 主动 list 刷新是否足够？
2. migrate 自动备份保留份数：保留最近 3 份是否合适？
3. 1.3 删旧字段后，历史 LLM 工具调用记录里的 num 字段回看会显示异常——已确认可接受（v1.2.0 有先例），但需确认前端历史记录展示是否受影响。
