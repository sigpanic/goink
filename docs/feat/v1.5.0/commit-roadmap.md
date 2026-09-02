# commit 路线（细化版）

> 配套 [volume-chapter-id-refactor.md](./volume-chapter-id-refactor.md) 的实施步骤细化。只写做什么，不写怎么做。一切以代码为准。

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

## PR1：破坏性核心（必须一起合并）

替代原 commit #1，拆成 7 个细粒度 commit：

| # | Commit message | 做什么 |
|---|---|---|
| 1.1 | `feat(migrate): add migrate_state + volume tables` | 新建 migrate_state 表（step+status+时间戳）+ volume 表（id/novel_id/name/sort_order+时间戳）；加 MigrateState/Volume model 到 AutoMigrate 列表 |
| 1.2 | `feat(chapter): add volume_id + sort_order + cross-ref chapter_id columns` | chapter 表加 volume_id *int64 + sort_order int；5 张交叉引用表（time_entries/arc_nodes/reader_perspectives/writing_log/character_relations）各加 chapter_id 列；**保留旧字段共存**（迁移期双字段） |
| 1.3 | `feat(rag): migrate vec table chapter_number to chapter_id` | vec_novel_{id} 虚拟表 chapter_number 列改为 chapter_id；新建 vec 表用 chapter_id 列；旧 chunk 数据反查 chapter.id 迁移（失败写 0 + 日志） |
| 1.4 | `feat(migrate): auto-backup before migration` | migrate 跑前自动备份 novel-agent.db + novels/ 到 platform.DataDir()/backups/{timestamp}/；保留最近 3 份；备份失败不阻塞 migrate |
| 1.5 | `refactor(migrate): rewrite cross-ref data num->id` | 5 张交叉引用表 + vec 表数据重写：按 (novel_id, 旧 num 列) 反查 chapters.id 写入新 chapter_id 列；反查失败写 NULL + 告警日志；WHERE chapter_id IS NULL 幂等 |
| 1.6 | `refactor(migrate): init sort_order + create volumes/ + rename files` | chapter.sort_order 初始化 = chapter_number（保留原顺序）；每个 novel 仓库建 volumes/ 目录 + .gitkeep；文件逐个 os.Rename chapters/{num:03d}.md → chapters/{id}.md + outlines/{num:03d}.md → outlines/{id}.md；EXDEV 退化 cp+rm；按 novel 各自 git commit |
| 1.7 | `refactor(migrate): drop legacy chapter_number columns + set done` | 删 chapter.chapter_number 列 + 5 张交叉引用表旧 num 列；写 migrate_state status="done"；新用户 DB 初始化后 INSERT 所有已知 step 为 done |

### PR1 内部依赖

- 1.1 → 1.2 → 1.3（建表后才能加列）
- 1.4 可独立（备份逻辑）
- 1.5 依赖 1.2（加列后才能重写）
- 1.6 依赖 1.2（sort_order 加列后才能初始化）
- 1.7 依赖 1.5/1.6（数据迁移+文件迁移完成后才能删旧列）

## PR1 后续：代码层适配（原 commit #2-#4）

| # | Commit message | 做什么 |
|---|---|---|
| 2 | `refactor(git): ChapterPath/OutlinePath use id` | ChapterPath/OutlinePath 签名 num int → id int64；加 VolumePath(volumeID int64)；更新所有调用方（chapter/store、app/chapter、app/novel export、rag/vector_store） |
| 3.1 | `refactor(rw_tools): parseChapterID + new.md path` | path 正则改 chapters/\d+\.md + chapters/new.md；parseChapterNum→parseChapterID + isNewChapterPath；删除 upsert 逻辑；新建走 chapters/new.md 占位→建记录拿 id 写真实文件返回 path；edit 工具 description 加 new.md 约定 |
| 3.2 | `refactor(rw_tools): list output + volume outline path` | list_chapters 返回 id+chapter_number+volume_name+title；卷纲路径 volumes/\d+\.md 支持 |
| 4.1 | `refactor(mcp_tools): timeline/storyarc/reader use chapter_id` | timeline/storyarc/reader 工具的章节字段改 chapter_id；AI 调用直接传 id 不转译 |
| 4.2 | `refactor(mcp_tools): character_relations + writing_log use chapter_id` | character + writing_log 字段同步改 chapter_id |
| 4.3 | `refactor(search): ChapterNum→ChapterID + runtime num` | search service 字段 ChapterNum→ChapterID；查询结果实时算 num 显示 |

## PR2：增量功能

| # | Commit message | 做什么 |
|---|---|---|
| 5.1 | `feat(volume): app-layer volume CRUD + reorder` | app: CreateVolume/UpdateVolume/DeleteVolume/GetVolumes/ReorderVolumes；wails 绑定自动生成 |
| 5.2 | `feat(chapter): app-layer move/insert/delete` | app: MoveChapterToVolume/InsertChapter/DeleteChapter（含交叉引用检测拒绝）；wails 绑定自动生成 |
| 6.1 | `feat(frontend): chapter management tab skeleton` | 新增独立 tab"章节管理"：panel.ts+ActivityBar+WorkspaceView 分支+ChapterManagementView 主骨架；现有 ChapterList 保留；i18n key |
| 6.2 | `feat(frontend): volume management panel` | 卷管理面板：CRUD UI + 排序；按卷分组渲染 |
| 6.3 | `feat(frontend): chapter operations UI` | 章节 [⋮] 菜单：删除/插入/移动；拖拽跨卷移动 |
| 7.1 | `test(migrate): idempotency + cross-ref + delete detection` | go test 覆盖 migrate 幂等性、交叉引用 chapter_id、DeleteChapter 引用检测 |
| 7.2 | `test(volume + rw_tools): CRUD + new.md creation` | go test 覆盖 volume CRUD、rw_tools new.md 创建 |
| 7.3 | `docs: manual verification checklist` | 手动验证清单 |

## PR 拆分

| PR | 含 commit | 风险 | 说明 |
|---|---|---|---|
| PR1 破坏性核心 | 1.1-1.7 + 2 + 3.1/3.2 + 4.1/4.2/4.3 | 高 | 一次性破坏性变更，合并后应用启动自动跑 migrate，无回滚；1.1-4.3 必须一起合并（中间状态编译失败） |
| PR2 增量功能 | 5.1/5.2 + 6.1/6.2/6.3 + 7.1/7.2/7.3 | 低 | 纯新增，不触发迁移；可独立合并 |

## 不做的事

- 不写一次性脚本迁移 LLM 历史调用记录（用户确认可接受）
- 不重排章节号（删除留"实时连续"效果，章节号不存 DB）
- 不修改 system 消息字段命名（仅改 DB 与工具层）
- 不实时 InjectMessage 提醒 num 变化（AI 拿不准时主动调 list_chapters 刷新；可选 chat 时检查一致性，低优先级后续做）

## 待确认

1. 章节号实时计算的语义：删除第 3 章后第 4 章变第 3 章，AI 上下文里的"第 5 章"指向变化，仅靠 AI 主动 list 刷新是否足够？
2. migrate 自动备份保留份数：保留最近 3 份是否合适？
