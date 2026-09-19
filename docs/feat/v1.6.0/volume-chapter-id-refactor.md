# 分卷 + 章节 id 化改造方案

> 本文档只描述"做什么"，不写具体代码实现。
> 一切以代码为准，文档仅供参考；实现时如发现文档与代码有差异，向用户汇报。

## 一、目标

1. **章节删除**：支持任意位置章节删除（含中间章节），文件系统稳定不重命名
2. **分卷功能**：长篇小说按卷分组组织，支持卷 CRUD 与跨卷移动
3. **交叉引用稳定**：所有章节引用统一改用 `chapter_id`（chapters.id），删除/移动章节时引用永不错位
4. **AI 暴露兼容**：AI 直接用 id 寻文件零转译，list_chapters 同时返回 id + chapter_number，AI 用 id 操作、用 num 理解

## 二、与历史决策的关系

历史脉络（参考 [v1.2.0 字段改名方案](../v1.2.0/chapter-field-rename.md)）：

| 阶段 | 决策 | 字段名 | 实际语义 |
|---|---|---|---|
| Python 版 | / | `source_chapter_id` 等 | chapters.id 真外键 |
| Go 版迁移时（v1.2.0 之前） | 改用 num 语义 | 沿用 `source_chapter_id` | 章节号（字段名与语义不一致） |
| v1.2.0 | 纯字段改名 | `source_chapter` 等 | 章节号（语义未变，字段名匹配语义） |
| **本方案** | 逆转 Go 版迁移时的 id→num 决策 | 改回 `_id` 后缀 | chapters.id 真外键 |

**关键澄清**：
- v1.2.0 是纯字段改名（_id → _chapter_number），语义未变；本方案不否定 v1.2.0 的改名
- 本方案逆转的是**Go 版迁移时把 id 语义改成 num 语义**的决策（v1.2.0 之前就发生了）
- **AI 暴露层改用 id**：逆转 v1.2.0 的"AI 用 num"决策——AI 直接用 `chapters/{id}.md` 寻文件零转译；list_chapters 同时返回 id + chapter_number，AI 用 id 操作、用 num 理解"第几章"
- 本方案改造的是**DB 内部存储与交叉引用 + AI 暴露层**：DB 用 chapter_id 外键删除/移动零成本，AI 用 id 直寻文件零转译

**结论**：本方案做三件事——
1. DB 内部存储从 num 改回 id（逆转 Go 版迁移时的 id→num 决策）
2. AI 暴露层从 num 改 id（逆转 v1.2.0 的 num 暴露决策，list 同时返回 num 给 AI 理解）
3. 新增分卷功能（volume 表 + volume_id + 章节管理独立 tab）

## 三、当前架构现状（待改造点）

| 层 | 现状 | 改造点 |
|---|---|---|
| 文件系统 | `chapters/{chapter_number:03d}.md` | 改用 `chapters/id_{id}.md`（id_ 前缀隔离旧 num 命名空间，rename 判断结构性可靠） |
| DB chapter 表 | 含 `chapter_number int` 字段，(novel_id, chapter_number) 唯一索引 | 移除 chapter_number，新增 volume_id |
| DB 交叉引用表 | timeline/arc_node/reader/character_relations 用章节号 int | 全改 chapter_id int64 外键 |
| DB writing_log | chapter_number int | 改 chapter_id int64（删除后允许孤儿） |
| git.ChapterPath | `ChapterPath(num int) string` | `ChapterPath(id int64) string` |
| rw_tools 路径解析 | `parseChapterNum` 直接拿 num 拼 path | 改为 `parseChapterID` 直寻文件零转译；新建走 `chapters/new.md` 占位 |
| delete_record 工具 | 不支持 chapter 表 | **不扩展**，删章节仅前端（app.DeleteChapter） |
| 分卷 | 零实现，仅 import/txt.go 有"第X卷"正则 | 新增 volume 表 + chapter.volume_id + CRUD |
| volume_id | 全仓 0 命中 | 全新引入 |

## 四、新方案总览

| 项 | 设计 |
|---|---|
| 文件名 | `chapters/{id}.md` / `outlines/{id}.md` |
| DB chapter 表 | 移除 chapter_number；新增 `volume_id *int64` + `sort_order int`；保留 `id, novel_id, title, summary, word_count, created_at, updated_at` |
| 排序依据 | 默认 `volume_id ASC NULLS FIRST, sort_order ASC`（sort_order 是内部排序键，新建末尾 MAX+1，插入中间时批量 +1 后续） |
| 章节号 | 不存 DB，列表查询时按 `(volume_id, sort_order)` 排序位次实时生成 1,2,3... |
| 交叉引用 | timeline/arc_node/reader 全改 `chapter_id int64` 外键 |
| writing_log | chapter_number 改 chapter_id；删除章节后允许孤儿引用（不阻塞删除） |
| character_relations | chapter_number 改 chapter_id |
| AI 路径 | `chapters/{id}.md`，AI 直接用 id，rw_tools 零转译 |
| 删除流程 | 仅前端入口；删文件 + 删 DB 记录 + 检测 timeline/arc_node/reader 引用（有则拒绝）；`delete_record` mcp_tool 不扩展支持 chapter 表 |
| 跨卷移动 | 仅改 chapter.volume_id，零成本 |
| 分卷 | volume 表：`id/novel_id/name/sort_order/created_at/updated_at` |
| AI 卷感知 | list_chapters 返回 `id + volume_name + 实时章节号 + 标题`；AI 新建章节时传 volume_name（rw_tools 经 chapters/new.md 反查 volume_id），可读写卷纲（volumes/{id}.md） |

## 五、DB Schema 变更

### 5.1 chapter 表

- **移除** `chapter_number` 字段及其唯一索引
- **新增** `volume_id *int64`（可空外键，NULL 表示未分卷）
- **新增** `sort_order int`（内部排序键，用户/AI 不可见；新建末尾 MAX+1，插入中间时批量 +1 后续）
- 保留：id, novel_id, title, summary, word_count, created_at, updated_at

**sort_order 与 chapter_number 的区别**：
- chapter_number 是业务编号，用户/AI 都看，必须连续
- sort_order 是内部排序键，仅 ORDER BY 用，谁都不看
- "第 N 章"实时按 `(volume_id, sort_order)` 排序后位次生成，不存 DB

### 5.2 volume 表（新增）

字段：`id, novel_id, name, sort_order, created_at, updated_at`

约束：`(novel_id, sort_order)` 唯一索引，`(novel_id, name)` 唯一索引

卷纲存文件系统（不入库）：`volumes/{volume_id}.md`，类似现有 `chapters/{id}.md`、`outlines/{id}.md`、`goink.md` 的文件模型。卷纲内容：创作主题、目标章节范围、节奏、关键角色/伏笔等。AI 可通过 rw_tools 读写卷纲（路径正则扩展支持 `volumes/\d+\.md`）。

### 5.3 time_entries 表（GORM 表名，结构体 TimelineEntry）

| 旧字段 | 新字段 | 类型变化 |
|---|---|---|
| `target_chapter int` | `target_chapter_id int64` | int → int64 nullable |
| `source_chapter int` | `source_chapter_id int64` | int → int64 nullable |
| `resolved_chapter int` | `resolved_chapter_id int64` | int → int64 nullable |

### 5.4 arc_nodes 表（GORM 表名，结构体 ArcNode）

| 旧字段 | 新字段 | 类型变化 |
|---|---|---|
| `target_chapter int` | `target_chapter_id int64` | int → int64 nullable |
| `actual_chapter int` | `actual_chapter_id int64` | int → int64 nullable（标记完成时填入的实际发生章节） |

### 5.5 reader_perspectives 表（GORM 表名，结构体 ReaderPerspective）

| 旧字段 | 新字段 | 类型变化 |
|---|---|---|
| `planted_chapter int` | `planted_chapter_id int64` | int → int64 |
| `revealed_chapter int` | `revealed_chapter_id int64` | int → int64 nullable |

### 5.6 writing_log 表

| 旧字段 | 新字段 | 类型变化 |
|---|---|---|
| `chapter_number int` | `chapter_id int64` | int → int64 nullable |

删除章节后 chapter_id 变孤儿，但**不阻塞删除**，历史日志仍可按日期统计。

### 5.7 character_relations 表

| 旧字段 | 新字段 | 类型变化 |
|---|---|---|
| `chapter_number int` | `chapter_id int64` | int → int64 nullable |

### 5.8 migrate_state 表（新增，迁移状态管理）

字段：`migration string, step string, status string, started_at timestamp, finished_at timestamp`

- `migration`：迁移标识，一次大迁移一组行，如 `"v1.6.0-chapter-id-refactor"`，未来新迁移加新组即可复用此表
- `step`：迁移内执行步骤标识，`(migration, step)` 唯一，一行 = 一个步骤（如 `"1-crossref-data"`）
- `status`：`"running"` / `"done"` / `"failed"`，单步骤状态
- `started_at` / `finished_at`：时间戳，便于排查

migrate.Run 流程（注册表驱动，框架见 internal/migrate/step.go，v1.6.0 步骤实现见 internal/migrate/v160 子包）：
- 先 AutoMigrate 建 migrate_state 表（backup 的组判断依赖它存在）
- 只对 `Destructive: true` 的迁移备份：查本迁移组（`migration = "v1.6.0-..."`）是否全部 done：全 done 则跳过备份；否则备份（按迁移名目录，临时目录原子 rename）。纯增量迁移（仅加列）不备份
- 框架遍历注册表步骤：已 done 跳过；未 done 置 running → 执行（步骤内部幂等）→ done / failed
- 新库短路：chapters 表不存在 → 所有 step 直接 INSERT done

**新用户处理**：新库短路直接全 done，不跑任何 migrate 步骤。

**为什么不用 HasColumn 做 flag**：migrate 步骤7删了 chapter_number 列后，如果中途中断（如步骤6文件 rename 未完成），重启后 HasColumn(chapter, chapter_number) 不存在会误判"已迁移"，导致文件 rename 永远不完成。migrate_state 表独立追踪整体状态，跟 schema 列状态解耦，更可靠。

**为什么不用 golang-migrate 等标准库**：与本项目 GORM AutoMigrate 模式冲突（两套 migrate 系统打架），且 SQL-first 难表达复杂逻辑迁移（num→id 反查+文件 rename）。自建 migrate_state 表本质是 golang-migrate schema_migrations 表的轻量灵活版，跟现有 migrate.go 模式一致。

## 六、文件系统变更

| 项 | 旧 | 新 |
|---|---|---|
| 章节正文路径 | `chapters/{chapter_number:03d}.md` | `chapters/id_{id}.md` |
| 章节大纲路径 | `outlines/{chapter_number:03d}.md` | `outlines/id_{id}.md` |
| `git.ChapterPath` 函数签名 | `ChapterPath(num int) string` | `ChapterPath(id int64) string` |
| `git.OutlinePath` 函数签名 | `OutlinePath(num int) string` | `OutlinePath(id int64) string` |
| rw_tools 路径正则 | `chapters/\d{3,6}\.md` | `chapters/\d+\.md` + `chapters/new.md`（占位新建） |
| rw_tools 解析函数 | `parseChapterNum(p string) int` | `parseChapterID(p string) int64` + `isNewChapterPath(p string) bool` |

调用方需同步更新：
- `internal/chapter/store.go` 的 FilePath 计算
- `internal/mcp_tools/rw_tools.go` 的所有路径处理
- `app/chapter.go` 的 CreateChapter 写文件
- `app/novel.go` 的 export 逻辑
- `internal/rag/` 的 chapter_id 寻文件

## 七、migrate 流程

### 7.1 触发时机

启动时自动跑，沿用现有 `migrate.Run` 模式。

### 7.2 步骤顺序

| 步骤 | 操作 | 幂等检查 |
|---|---|---|
| 1. 加 chapter_id 列 | timeline/arc_node/reader/writing_log/character_relations 五张表加 `chapter_id int64` 列 | `HasColumn` 已存在则跳过 |
| 2. 数据重写 | 按 `(novel_id, chapter_number)` 反查 `chapter.id`，写入新 `chapter_id` 列；**反查失败**（章节号无对应 chapter 记录，如数据损坏或用户手改过 DB）则 `chapter_id` 写 NULL 并记录告警日志，不阻塞迁移 | `WHERE chapter_id IS NULL` 仅重写未完成的行 |
| 3. 建 volume 表 | AutoMigrate 新表 | GORM AutoMigrate 幂等 |
| 4. 加 volume_id + sort_order 列 | chapter 表加 `volume_id *int64` + `sort_order int`；sort_order 初始化 = chapter_number（保留原顺序） | `HasColumn` 已存在则跳过；sort_order 为 NULL 时初始化 |
| 5. 建 volumes/ 目录 | 每个 novel 仓库创建空 `volumes/` 目录 + `.gitkeep` | 目录存在则跳过 |
| 6. 文件逐个 mv（见 7.3） | 遍历所有 novel 仓库 rename 文件 `chapters/{num:03d}.md → chapters/id_{id}.md`、`outlines/{num:03d}.md → outlines/id_{id}.md` | 目标存在/源不存在则跳过 |
| 7. 删旧字段 | 删 chapter_number 列及交叉引用表的旧 num 列 | `HasColumn` 不存在则跳过 |

### 7.3 文件系统迁移（每 novel 独立处理）

跨 DB+FS 无法用单一事务保证，采用**逐个 mv + commit**实现幂等可重入：

**阶段 A：逐个 rename**（os.Rename，非 git mv）
- 对每个 chapter 记录，按 id 逐个 rename：
  - 若 `chapters/id_{id}.md` 已存在 → 跳过（已迁移）
  - 若 `chapters/{num:03d}.md` 不存在 → 跳过（源已 mv 走，已迁移）
  - 否则 `os.Rename(chapters/{num:03d}.md, chapters/id_{id}.md)`
- 大纲同理（`outlines/{num:03d}.md → outlines/id_{id}.md`）
- id_ 前缀将 id 命名空间与旧 num 纯数字命名空间完全隔离：任何 num 值不可能撞名，目标存在即本文件已完成迁移，判断结构性可靠
- 跨文件系统时（os.Rename 返回 EXDEV）退化成 cp+rm（同文件系统内通常一次 rename 完成）

**为什么用普通 mv 不用 git mv**：
- os.Rename 同文件系统内是原子 syscall，要么成功要么失败，无"一半"状态
- git mv 内部是 mv + git rm + git add 三步，非原子，中断后 stage 状态混乱（untracked + deleted 混杂），可重入判断复杂
- 普通 mv 不涉及 git stage，commit 阶段统一 `git add -A` 处理

**阶段 B：git 提交**（按 novel 仓库分别 commit）
- `git add -A` + `git commit -m "migrate: rename chapter files to id"`
- git status 干净则跳过

### 7.4 中断恢复

每个步骤设计为可重入，靠 migrate_state 表（见 5.8）按 `(migration, step)` 记录状态做 flag：
- 框架遍历注册表：已 done 的 step 跳过；未 done 的置 running → 执行（单步骤幂等）→ done / failed
- 单步骤幂等检查（step 内部自检自身完成条件）：
  - DB 列已存在 → 跳过
  - DB 数据重写条件 `WHERE chapter_id IS NULL` → 已重写的不重复
  - 文件目标 `chapters/id_{id}.md` 已存在 → 跳过 rename
  - 文件源 `chapters/{num:03d}.md` 不存在 → 跳过 rename
  - git 干净 → 跳过 commit
- 全部 step done → backup 组判断"全 done"→ 后续启动跳过备份

重启 `migrate.Run` 自动从未 done 的 step 继续，单步骤幂等保证不重复执行已完成的子操作。

### 7.5 注意事项

- migrate 期间应用启动会变慢（遍历所有 novel 仓库 os.Rename + git commit），日志输出进度
- 单个 novel 仓库失败不影响其他 novel，错误记录后继续
- DB 操作建议用 `db.Transaction()` 包，文件系统操作单独幂等处理

### 7.6 自动备份（migrate 前置）

migrate 触发时第一步先备份，避免破坏性变更失败后无法恢复：

- 备份根目录：`filepath.Join(platform.DataDir(), "backups", timestamp)`，其中 `platform.DataDir()` 平台相关（Windows exe 目录可写时为 exe 目录，不可写时为 `%LOCALAPPDATA%\Goink`；其他平台 `~/Goink/`；可被 `GOINK_DATA_DIR` 覆盖）
- 备份内容：
  - `cp novel-agent.db → backups/{timestamp}/novel-agent.db`
  - `cp -r novels/ → backups/{timestamp}/novels/`
- 备份幂等：同名 timestamp 目录已存在则跳过（不重复备份）
- 备份失败处理：日志告警但**不阻塞 migrate**（备份失败不卡住启动）；紧急恢复时用户手动从 `backups/` 恢复
- 备份保留策略：保留最近 3 份，更老的自动清理（避免磁盘膨胀）

## 八、AI 暴露层（id 直寻，零转译）

### 8.1 路径格式

AI 看到的 path 是 `chapters/{id}.md`（不补零，id 语义）。AI 从 list_chapters 拿到 id 后直接用 `chapters/{id}.md` 寻文件，rw_tools 零转译直寻。

### 8.2 rw_tools 内部流程（零转译）

**读取/编辑已有章节时**：
1. `parseChapterID(path)` 解析出 id
2. 用 `git.ChapterPath(id)` 拼 `chapters/{id}.md` 寻真实文件
3. 读写文件
4. chapter 记录必须已存在，不存在则报错"章节不存在，请先创建"

**新建章节时**（path=`chapters/new.md` 占位）：
1. AI 传 `chapters/new.md` + full_replace + content + title（+ 可选 volume_name）
2. rw_tools 识别 path == "chapters/new.md" → 新建模式
3. 建 chapter 记录（id 自增，sort_order = 该卷 MAX+1，volume_id 由 volume_name 反查或 NULL，title）
4. 写文件 `chapters/{id}.md`
5. 响应返回真实 path `chapters/{id}.md` + id + chapter_number + volume_name
6. AI 后续操作用返回的真实 path

**edit 工具 description 约定**：新建章节时 path 传 `chapters/new.md`，工具会返回真实 path，后续操作用返回的 path。

### 8.3 列表工具

`list_chapters` 返回给 AI 的字段（**id + num 同时给**）：
- `id`：chapter.id，AI 用于操作 path 和交叉引用
- `chapter_number`：实时计算的章节号（按排序位次），AI 用于理解"第几章"
- `volume_name`：所属卷名（未分卷时为空）
- `title`、`summary`、`word_count` 等元数据

显示格式建议：`卷一·第3章·初入江湖`，AI 内部用 id=12 操作

### 8.4 写入工具（timeline/arc_node/reader/character_relation）

AI 调用 `create_timeline_entry(target_chapter_id=12)` 时**直接传 id=12**，工具内不做 num→id 转译，直接存 `target_chapter_id` 列。

AI 从 list_chapters 看到 `{id:12, chapter_number:10, volume_name:"卷一"}` → 想在第10章埋伏笔 → 传 `target_chapter_id=12`。

### 8.5 读取/搜索

`internal/search/service.go` 的 `ChapterNum` 字段改为 `ChapterID`（int64），DB 取出 `target_chapter_id` 直接填入；展示给 AI 时按 chapter_id 反查实时章节号 + 卷名拼成"卷一·第10章"显示。

### 8.6 AI 拿到的快照 num 可能失效问题

用户改了结构（删章、跨卷移动、重排）后，AI 上下文里之前的"第 N 章"可能失效。id 方案下 AI 用 id 操作不受影响（id 稳定），但 AI 理解"第 N 章"时可能错位。缓解：
- 每次结构操作后通过 InjectMessage 告知 AI"第 N 章已删除/移动到卷二"
- AI 拿不准时主动调 list_chapters 刷新

## 九、删除流程

### 9.1 删除入口

- **app 层**：新增 `DeleteChapter(chapterID int64) error`，前端章节管理 tab 调用
- **mcp_tools**：**不扩展** `delete_record` 支持 chapter 表——遵循"结构操作仅前端"原则，AI 不能删章节，只能提示用户在前端删
- **AI 删章节需求**：AI 想删章节时，通过 InjectMessage 或回复告知用户"建议删除第 N 章（id=X），请到章节管理 tab 操作"

### 9.2 删除步骤

1. 接收 `chapter_id` 参数
2. 查 chapter 记录确认存在且属于当前 novel
3. 检测交叉引用：
   - `timeline_entry` 的 `target_chapter_id` / `source_chapter_id` / `resolved_chapter_id`
   - `arc_node.target_chapter_id`
   - `reader_perspective_entry.planted_chapter_id` / `revealed_chapter_id`
   - `character_relations.chapter_id`
   - 任一存在引用 → **拒绝删除**，返回引用清单（沿用 [delete_tools.go](../../../internal/mcp_tools/delete_tools.go) 对 character 关联的处理模式）
   - `writing_log.chapter_id` 有引用 → **不阻塞删除**（历史日志）
4. 删文件 `chapters/{id}.md`、`outlines/{id}.md`
5. 删 DB chapter 记录
6. writing_log 的 `chapter_id` 变孤儿（指向已删除章节），查询时显示"已删除章节"
7. RAG：调用 `DeleteChapterChunks(novelID, chapterID)`（[vector_store.go](../../../internal/rag/vector_store.go) 现有方法签名需改）
8. InjectMessage 给 AI："第 N 章已删除。原引用此章的 timeline/arc_node/reader 记录已提示用户清理"

### 9.3 章节号实时性

- 删除中间章节后，后续章节的实时章节号自动重排（删除第 3 章，原第 4 章变第 3 章）
- AI 上下文中的"第 5 章"删除后会指向不同章节 → 通过 InjectMessage 提示 AI
- 交叉引用因用 chapter_id 稳定，不受章节号重排影响

## 十、分卷功能

### 10.1 volume 表

字段：`id, novel_id, name, sort_order, created_at, updated_at`

约束：
- `(novel_id, sort_order)` 唯一索引
- `(novel_id, name)` 唯一索引
- novel 删除时级联删除其所有 volume

### 10.2 CRUD 接口

app 层：
- `CreateVolume(novelID int64, name string) (*Volume, error)`
- `UpdateVolume(volumeID int64, name string) error`
- `DeleteVolume(volumeID int64) error` — 删前检查是否有关联章节，有则拒绝
- `GetVolumes(novelID int64) ([]Volume, error)`
- `ReorderVolumes(novelID int64, volumeIDs []int64) error` — 批量更新 sort_order

AI 通道（经 rw_tools，非 mcp_tool）：
- AI 新建章节走 `chapters/new.md` 占位（见第十一节），可传 `volume_name` 让 rw_tools 反查 `volume_id`
- AI 不感知卷 CRUD（不提供 create/update/delete volume 工具，结构操作仅前端）
- AI 可通过 rw_tools 读写卷纲文件 `volumes/{id}.md`

### 10.3 章节排序

`list_chapters` 排序：`volume_id ASC NULLS FIRST, sort_order ASC`

NULLS FIRST 表示未分卷的章节排在最前。

### 10.4 跨卷移动

- app 层：`MoveChapterToVolume(chapterID, volumeID int64) error`（volumeID=0 表示移出卷，置 NULL）
- 仅改 `chapter.volume_id`，不动 sort_order 等其他字段
- 不影响章节号（实时算）
- 移动到新卷末尾：可选调整 sort_order = 目标卷 MAX(sort_order) + 1

### 10.5 章节插入（新功能）

- app 层：`InsertChapter(novelID, afterChapterID int64, volumeID *int64, title string) (*Chapter, error)`
- 事务里：
  1. 取 afterChapter 的 sort_order = N
  2. `UPDATE chapter SET sort_order = sort_order + 1 WHERE novel_id=? AND volume_id=? AND sort_order > N`
  3. `INSERT` 新章 sort_order = N + 1
- **仅前端**：AI 不调用 insert_chapter 工具（遵循"结构操作仅前端"原则）；前端章节管理 tab 提供插入入口，AI 想插入章节时通过回复告知用户去前端操作

## 十一、新建章节策略（chapters/new.md 占位）

### 11.1 矛盾本质

新方案 path 用 id，但 id 是后端自增的，AI **不可能预知**。现状 rw_tools 靠"AI 传 chapters/{num}.md + upsert 记录"机制创建章节，num 同时是 path 和 DB 字段。id 方案下此机制失效——AI 无法在 path 里写一个还不存在的 id。

### 11.2 选定方案：chapters/new.md 占位

AI 新建章节时传 `chapters/new.md`（占位 path），rw_tools 后端建记录拿真实 id 写真实文件，响应返回真实 path：

```
AI: edit(path="chapters/new.md", change_type="full_replace", new_content="...", title="初入江湖", volume_name="卷一")
rw_tools 内部:
  1. 识别 path == "chapters/new.md" → 新建模式
  2. 反查 volume_name → volume_id（找不到则 NULL）
  3. 建 chapter 记录（id 自增, sort_order = 该卷 MAX+1, volume_id, title）
  4. 写文件 chapters/{id}.md
  5. 响应返回 {path: "chapters/{id}.md", id, chapter_number, volume_name}
AI 后续: edit(path="chapters/{id}.md", ...)  // 用响应返回的真实 path
```

### 11.3 rw_tools upsert 逻辑删除

现状 [rw_tools.go:142-176](../../../internal/mcp_tools/rw_tools.go#L142-L176) 的"查 chapter_number 不存在则 Create"逻辑**整段删除**。读写时 chapter 记录必须已存在，不存在则报错"章节不存在，请先用 chapters/new.md 创建"。唯一例外是 path == "chapters/new.md" 走新建分支。

### 11.4 edit 工具 description 约定

edit 工具 description 加一条：新建章节时 path 传 `chapters/new.md`，工具会返回真实 path `chapters/{id}.md`，后续操作用返回的 path。

### 11.5 前端新建章节（独立通道）

前端章节管理 tab 的"新建章节"按钮走 app.CreateChapter（不走 rw_tools），直接建记录拿 id 写空文件，用户填标题/选卷。与 AI 走 chapters/new.md 是两条独立通道，互不影响。

## 十二、前端 UI 改造点（独立 tab 章节管理）

### 12.1 独立 tab 设计

参考 [CharacterListView](../../../frontend/src/components/character/CharacterListView.tsx) 全宽独立 View 模式（非 SidePanel 内的 *List 组件），新增独立 tab"章节管理"，**中间区域独立**，不嵌在现有 SidePanel 或 ContentPanel 内。

| 改动位置 | 做什么 |
|---|---|
| [frontend/src/types/panel.ts](../../../frontend/src/types/panel.ts) | `PanelId` 加 `"chapter-management"` |
| [frontend/src/components/shell/ActivityBar.tsx](../../../frontend/src/components/shell/ActivityBar.tsx) | activities[] 加一项（图标 + labelKey） |
| [frontend/src/views/WorkspaceView.tsx](../../../frontend/src/views/WorkspaceView.tsx) | 主区域加分支 `activePanel === "chapter-management" ? <ChapterManagementView novelId={...} /> : ...` |
| 新建 `frontend/src/components/chapter-management/` | ChapterManagementView 主组件 + 卷管理面板 + 拖拽逻辑 |
| 后端 [app/chapter.go](../../../app/chapter.go) + [chapter/store.go](../../../internal/chapter/store.go) | 加 DeleteChapter/InsertChapter/MoveChapterToVolume + volume CRUD（供前端调用） |
| i18n locales（`frontend/src/i18n/locales/`） | 加 `shell.chapterManagement` 等 key |

### 12.2 章节管理 tab 布局

```
┌─ ActivityBar ─┬─ 章节管理主区域（独立，全宽） ─────────────────┐
│  ...          │  ┌─ 卷管理面板 ────────────────────────────┐  │
│  [章节管理]   │  │ 卷一  卷二  卷三  [+ 新建卷]  [排序]    │  │
│  ...          │  └────────────────────────────────────────┘  │
│               │  ┌─ 章节列表（按卷分组） ─────────────────┐  │
│               │  │ 卷一                                    │  │
│               │  │   第1章  初入江湖  3200字  [⋮]          │  │
│               │  │   第2章  山中遇险  2800字  [⋮]          │  │
│               │  │ 卷二                                    │  │
│               │  │   第3章  进城     4100字  [⋮]          │  │
│               │  │ 未分卷                                  │  │
│               │  │   第7章  临时草稿 800字   [⋮]          │  │
│               │  └────────────────────────────────────────┘  │
└───────────────┴──────────────────────────────────────────────┘
```

### 12.3 章节列表分组渲染

- 按 volume 分组：未分卷组（NULLS FIRST）+ 各卷组（按 sort_order）
- 章节号实时显示（按卷内位次，1-based）
- 每行 hover 显示操作菜单 [⋮]：删除、插入到此章前/后、移动到卷

### 12.4 章节删除入口

- 章节行 [⋮] 菜单 → "删除"
- 弹窗确认 + 显示交叉引用清单（timeline/arc_node/reader/character_relations 的引用）
- 有引用则禁用删除按钮并提示"请先清理以下引用：..."
- 确认删除后调 app.DeleteChapter(chapterID)，后端检测引用 + 删文件 + 删记录 + RAG 清理

### 12.5 章节插入入口

- 章节行 [⋮] 菜单 → "在此章前插入" / "在此章后插入"
- 弹窗填标题 + 选卷
- 调 app.InsertChapter(afterChapterID, volumeID, title)

### 12.6 卷管理面板

- 顶部卷管理面板：卷标签列表（卷一/卷二/...）+ 新建卷按钮 + 排序按钮
- 卷 CRUD：点击卷标签编辑/重命名/删除
- 删卷前检查是否有关联章节，有则拒绝（提示先移动章节）
- 拖拽卷标签调整顺序

### 12.7 跨卷移动

- 拖拽章节行到目标卷标签 → 调 app.MoveChapterToVolume(chapterID, volumeID)
- 移动后章节号实时重排（实时计算）

### 12.8 现有 ChapterList（SidePanel 内）保留

现有 [ChapterList.tsx](../../../frontend/src/components/chapter/ChapterList.tsx)（SidePanel 内的章节列表）保留不动，仍用于写作时快速切换章节进 ContentPanel 编辑器。章节管理 tab 是独立的"结构管理"入口，与写作流的章节切换是两个通道。

## 十三、风险与回滚

### 13.1 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| migrate 跨 DB+FS 无原子事务 | 中断后状态不一致 | migrate_state 表 running/done flag + 逐个 mv 幂等(7.3) + 单步骤幂等检查(7.4) + 重启自动恢复 |
| 现有用户数据量大时 migrate 启动慢 | 启动等待时间长 | 同步等待但日志输出进度；migrate_state done 后启动秒过 |
| AI 上下文"第 N 章"删除后指向变化 | AI 引用漂移 | 删除响应 InjectMessage 显式提示 |
| LLM 历史工具调用记录字段名变化 | 历史回看显示异常 | 用户确认可接受（v1.2.0 已有先例） |
| chapter_number 列删除后旧版本应用无法启动 | 降级失败 | 一次性破坏性变更，不支持降级 |

### 13.2 回滚

- 一次性破坏性变更，不支持自动回滚
- 自动备份：migrate 前自动备份到 `platform.DataDir()/backups/{timestamp}/`（含 novel-agent.db + novels/ 全量拷贝，详见 7.6）
- 紧急回滚：手动从 `backups/` 恢复 DB 与 novels/，代码层靠 git revert

## 十四、不做的事

- 不写一次性脚本迁移 LLM 历史调用记录（用户确认可接受）
- 不重排章节号（删除留"实时连续"效果，章节号不存 DB）
- 不修改 system 消息字段命名（仅改 DB 与工具层）

## 十五、实施步骤（自底向上平推）

> 本节已于实施中修正。原「细粒度 commit + PR1/PR2 拆分」方案被放弃，改为**自底向上分层平推**。

### 15.1 为什么放弃细粒度 commit

原方案要求每个 commit 可独立编译，导致底层与上层互相等待：

- 底层函数签名要 id，但上层此刻只有 num（从 path 解析）→ 底层先改则编译不过
- 上层先改，则要先把 num 解析成 id → 需要底层已支持 id

为绕开死结，中间态被硬造出来：双字段共存、num→id 反查桥接、TODO 标注何时删除。代价是同一处改两遍，且过渡代码本身要额外维护。

而 PR1 本身就要求「#1-#4 必须一起合并，中间状态编译失败」——**既然中途必然不可编译，维护「每个 commit 可编译」的假象没有意义**。

改为自底向上平推：一层改完直接改下一层，中途任意时刻编译不过都接受，最后统一恢复可编译。

### 15.2 hook 处理

pre-commit hook 会跑 `go build`/`go test`/`golangci-lint`，中间层提交必然失败。两种处理：

- 中间层提交用 `git commit --no-verify` 临时跳过
- 或把同一层的改动合并成少数几个大 commit，只在层边界提交

**注意**：`--no-verify` 只对中间层使用；进入下一层前应确认本层已自洽（哪怕上层还没跟上）。

### 15.3 分层顺序与当前进度

自底向上，下层不依赖上层：

| 层 | 内容 | 状态 |
|---|---|---|
| **L0 基建** | `internal/volume` store：CRUD + `sort_order` 分配算法。⚠️ 方法必须以 `*gorm.DB` 为参数（调用方传 db 或 tx）——严禁 receiver 持连接风格，本应用 `SetMaxOpenConns(1)`，事务内经外部连接查询会因连接池耗尽确定性死锁 | ❌ 未做（`internal/volume` 只有 types.go，sort_order 算法暂躺在 `mcp_tools`） |
| **L2 chapter.Store** | 全部按 id / sort_order：`ListByNovel`/`ListAllByNovel`/`SearchByNovel`/`GetRecent` 改 `ORDER BY volume_id ASC NULLS FIRST, sort_order ASC`；`GetByNovelAndNumber`→`GetByID`；删 `GetLatestNumber`（改由 sort_order 分配）；`UpdateTitle` 改按 id；方法风格整改为 `*gorm.DB` 参数 | ❌ 6 个方法全按 chapter_number |
| **L3 rag + search** | rag：`SubmitRefresh` 改按 chapter_id 提交，删 num→id 反查桥接，**chunk_id 去掉内嵌章节号**（`"%d_summary"` 等 → id-based）后重建向量；search：字段 `ChapterNum`→`ChapterID`，展示按 id 反查实时章节号 + 卷名 | 🟡 vec 列已切，桥接与 chunk_id 未改；search 未改 |
| **L4 其他内部包** | export（epub/txt/markdown）、pattern（extract/prompts/types）、agent/display | ❌ 全用 num |
| **L5 mcp_tools** | 交叉引用工具（timeline/storyarc/reader/character_relations）章节字段改 `*_chapter_id`；`get_chapter_list` 返回 id + 实时 chapter_number + volume_name + title；rw_tools 支持卷纲 `volumes/{id}.md`；memory_tools 章节过滤改 id；delete_tools 同步 | 🟡 rw_tools 已 id 化，其余未改 |
| **L6 app 层** | `DeleteChapter`/`InsertChapter`/`MoveChapterToVolume`（含交叉引用检测拒绝）；volume CRUD（Create/Update/Delete/Get/Reorder）；`CreateChapter` 改走 volume store 分配 sort_order；`UpdateChapterTitle` 改按 id；novel export、content.go 同步 | ❌ 未做 |
| **L7 前端** | 章节管理 tab（见第十二节） | ❌ 未做 |
| **L1b 收尾** | model 删旧 num 字段 + migrate 1.7 DROP 列（**先 DROP INDEX 再 DROP COLUMN**）；1.5/1.6 补 num 列的 `HasColumn` 守卫 | ❌ |

**为什么 L0 必须在最前**：migrate 步骤（建 volume 表、初始化 sort_order）与 rw_tools 的 new.md 通道都依赖卷基建。原方案把它排在 PR2，导致 L5 自实现一份（`createChapterRecord` 里那套跨卷 sort_order 锚点算法），L6 还得搬家。

**为什么 L1b 必须在最后**：删 SQL 列受**迁移步骤顺序**约束——1.7 必须在 1.5 反查、1.6 文件 rename 之后（它们要读 num 列）。删 model 字段则任何时候都行，只看代码引用是否改完（否则编译不过）。

**关于「解绑旧列约束」（原 L1a，已撤销）**：曾计划早期先去掉 `not null` / `uniqueIndex` 约束让代码停止写 num，已撤销：

- **不需要**：中间不运行应用，没有任何 INSERT 发生，not-null 约束在重构期不构成约束
- **SQLite 不支持 `ALTER COLUMN`**：单独去 `NOT NULL` 需重建整表（create → copy → drop → rename），纯属白折腾
- **不需要「双写兼容」**：不存在中间发行版（用户不会跑到半成品），代码无需「有 id 用 id、没 id 回退 num」的兼容逻辑

**双字段为什么存在**：不是设计需要，是「代码还没改到」的产物。id 列已由 1.5 反查填好但暂无代码读它；num 列仍被 L2-L6 的代码读写。两者共存只因代码改造（L0/L2-L6）未完成。

**迁移反查与 model 解耦**：`crossref.go` / `rename.go` 读 `chapter_number` 用的是 raw SQL + 局部匿名结构体，不依赖 `chapter.Chapter`。因此删 model 字段与删 SQL 列是**两个独立约束**——前者只看编译，后者只看迁移步骤顺序。

**坑：1.5/1.6 缺 num 列守卫**：`crossref.go` 只守了 idCol 存在，未守 numCol；`rename.go` 直接 `SELECT chapter_number`。若 1.7 已删列而 migrate_state 状态丢失（手动清表 / 异常恢复），这两步会因查不存在的列而失败 → 应用启动失败。触发条件窄但守卫成本极低。

### 15.4 细粒度 commit 路线

保留小粒度 commit（每个 commit 做一件事、可独立 review）。**放弃的只是「每个 commit 必须可编译」这个约束** —— 中间层提交用 `--no-verify` 跳过 hook。

「可编译」列标注该 commit 后代码能否通过 `go build`；层内相邻的 ❌ 可合并提交，层边界是 review 的自然分界。

#### L0 基建 — `internal/volume` store

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 0.1 | `feat(volume): add store with CRUD` | 新建 `internal/volume/store.go`：Create / Update / Delete / GetByID / ListByNovel / Reorder；方法以 `*gorm.DB` 为参数（调用方传 db 或 tx），不用 receiver 持连接风格 | ✅ |
| 0.2 | `feat(volume): add chapter sort_order allocation` | 把 `sort_order` 分配算法从 `mcp_tools/rw_tools.go` 的 `createChapterRecord` 下沉：卷内 max+1 / 空卷取前卷 max+1 / 退化全局 max+1 / 腾位批量 +1 | ✅ |
| 0.3 | `refactor(rw_tools): call volume store` | rw_tools 的 `resolveVolume` / `lastVolume` / `createChapterRecord` 改为调用 volume store，删除自实现 | ✅ |

#### L2 chapter.Store — 按 id / sort_order

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 2.1 | `refactor(chapter): list queries order by sort_order` | `ListByNovel` / `ListAllByNovel` / `SearchByNovel` / `GetRecent` 排序改 `volume_id ASC NULLS FIRST, sort_order ASC` | ❌ |
| 2.2 | `refactor(chapter): GetByID replaces GetByNovelAndNumber` | `GetByNovelAndNumber` → `GetByID`；删 `GetLatestNumber`（sort_order 分配已接管）；`UpdateTitle` 改按 id | ❌ |
| 2.3 | `refactor(chapter): store methods take *gorm.DB` | 方法风格整改为 `*gorm.DB` 参数（同 L0 约束，`SetMaxOpenConns(1)` 下事务安全） | ❌ |

#### L3 rag + search

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 3.1 | `refactor(rag): chunk_id keyed by chapter id` | `splitter.go` 的 chunk_id 去掉内嵌章节号（`"%d_summary"` / `"%d_brief"` / `"%d_%d"` → id-based） | ❌ |
| 3.2 | `refactor(rag): SubmitRefresh takes chapter id` | `SubmitRefresh` 改按 chapter_id 提交；删 `refresh_queue` 的 num→id 反查桥接；`RefreshTask` 去 `ChapterNumber` | ❌ |
| 3.3 | `refactor(search): ChapterNum becomes ChapterID` | `search.Service` 字段 `ChapterNum` → `ChapterID`；展示按 id 反查实时章节号 + 卷名拼"卷一·第10章" | ❌ |

#### L4 其他内部包

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 4.1 | `refactor(export): chapter number computed at render` | epub / txt / markdown 的章节号来源改 id + 实时算 | ❌ |
| 4.2 | `refactor(pattern): chapter fields use id` | `pattern` 的 extract / prompts / types 章节字段改 id | ❌ |
| 4.3 | `refactor(agent): display computes chapter number` | `agent/display.go` 章节号展示改实时算 | ❌ |

#### L5 mcp_tools

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 5.1 | `refactor(mcp_tools): cross-ref tools use chapter_id` | timeline / storyarc / reader / character_relations 工具的章节字段改 `*_chapter_id`，AI 直接传 id 不转译 | ❌ |
| 5.2 | `feat(mcp_tools): get_chapter_list returns volume and live number` | `get_chapter_list` 返回 id + 实时 chapter_number + volume_name + title | ❌ |
| 5.3 | `feat(rw_tools): support volume outline paths` | 卷纲路径 `volumes/{id}.md` 支持（正则 + 读写分支） | ❌ |
| 5.4 | `refactor(mcp_tools): memory/delete tools use chapter_id` | memory_tools 章节过滤改 id；delete_tools 同步 | ❌ |

#### L6 app 层

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 6.1 | `feat(volume): app-layer CRUD` | app: CreateVolume / UpdateVolume / DeleteVolume / GetVolumes / ReorderVolumes（删卷前检查关联章节）；wails 绑定自动生成 | ❌ |
| 6.2 | `feat(chapter): app-layer delete/insert/move` | app: DeleteChapter（交叉引用检测拒绝 + 删文件 + 删记录 + RAG 清理）/ InsertChapter / MoveChapterToVolume；wails 绑定自动生成 | ❌ |
| 6.3 | `refactor(chapter): CreateChapter allocates sort_order` | `CreateChapter` 改走 volume store 分配 sort_order（当前用 `GetLatestNumber`）；`UpdateChapterTitle` 改按 id；novel export / content.go 同步 | ❌ |

#### L7 前端

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 7.1 | `feat(frontend): chapter management tab skeleton` | 新增独立 tab"章节管理"：panel.ts + ActivityBar + WorkspaceView 分支 + ChapterManagementView 主骨架；现有 ChapterList 保留；i18n key | ✅ |
| 7.2 | `feat(frontend): volume management panel` | 卷管理面板：CRUD UI + 排序；按卷分组渲染 | ✅ |
| 7.3 | `feat(frontend): chapter operations UI` | 章节 [⋮] 菜单：删除 / 插入 / 移动；拖拽跨卷移动 | ✅ |

#### L1b 收尾 — 删旧字段

| # | Commit message | 做什么 | 可编译 |
|---|---|---|---|
| 1.1 | `refactor(migrate): guard legacy num columns` | 1.5/1.6 补 num 列的 `HasColumn` 守卫（防 migrate_state 状态丢失后查不存在的列 → 启动失败） | ✅ |
| 1.2 | `refactor(chapter): drop legacy num columns and fields` | ① 启用 migrate 1.7——先 DROP INDEX（`uk_novel_chapter` + writing_log 章节号索引）再 DROP COLUMN，删 chapter + 5 张交叉引用表旧 num 列；② model 移除 `chapter.ChapterNumber` 与 5 张交叉引用表旧 num 字段，清理全部引用（当前 272 处 / 43 文件） | ✅ |

**删列与删字段是两个独立约束，别绑在一起**：

| 动作 | 唯一约束 |
|---|---|
| **删 model 字段** | 代码引用要改完（否则编译不过）。中间不运行应用，所以 not-null、INSERT 全不构成约束——**任何时候都能删** |
| **删 SQL 列** | 迁移代码还要不要读它。1.5 反查读 num 列 + `chapters.chapter_number`；1.6 文件 rename 读 `chapters.chapter_number`（拼 `{num:03d}.md` 源路径）。1.7 必须在 1.5/1.6 之后 |

迁移读 num 走 raw SQL + 匿名结构体（`crossref.go` / `rename.go`），**不依赖 model**——这也是两者独立的证据。

放同一个 commit 只是因为它们服务于同一个终态，不是因为互相绑定。

### 15.5 与 migrate 的关系

migrate 的**代码**（framework + v160 steps）已随细粒度路线完成大半（1.4/1.5/1.6 已落地），只有 1.7 待启用。剩余工作是**代码层 id 化**（L0-L7），与 migrate 步骤并行不冲突：

- migrate 负责把**存量数据**从 num 迁到 id（已基本完成）
- L0-L7 负责让**代码**只认 id（进行中）

两者必须同时生效才能跑起来 —— 这是原 PR1 要求「一起合并」的根本原因。

## 十六、已决与待确认

### 已决（本方案确定）

1. **新建章节策略**（第十一节）：chapters/new.md 占位方案，rw_tools 删除 upsert 逻辑
2. **AI 暴露层**（第八节）：AI 直接用 id（chapters/{id}.md），list_chapters 返回 id+chapter_number+volume_name+title，零转译
3. **交叉引用迁移反查失败**（7.2 步骤2）：写 NULL + 日志，不阻塞迁移
4. **自动备份**（7.6）：migrate 前自动备份到 `platform.DataDir()/backups/{timestamp}/`
5. **delete_record 不扩展**（第九节）：AI 不能删章节，删章节仅前端
6. **结构操作仅前端**：不新增 insert/move/delete chapter mcp_tool
7. **实施顺序**（第十五节）：**自底向上分层平推**（L0 基建 → L1a 解绑 → L2 chapter.Store → L3 rag+search → L4 其他内部包 → L5 mcp_tools → L6 app → L7 前端 → **L1b 删旧字段**），允许中途编译失败，中间层提交用 `--no-verify` 跳过 hook。删列（L1b）必须在最后：删列不可逆，任何一层没改完就删列，运行时会读到不存在的列。原「7 个细粒度 commit + PR1/PR2 拆分」方案已放弃（原因见 15.1）
8. **migrate 执行模式**：启动时同步等待（应用启动时跑 migrate，跑完进主界面；大数据量启动慢但简单）
9. **migrate_state 表**（5.8）：自建迁移状态表（migration+step 两级），按步骤 running/done/failed 记录，不用 HasColumn 做 flag（避免中途中断误判）；不引入 golang-migrate（与 GORM AutoMigrate 冲突）
10. **文件迁移**（7.3）：逐个 os.Rename + git commit 两阶段，不用 cp+rm 三阶段，不用 git mv（非原子）；EXDEV 时退化 cp+rm
11. **AI 不自动开卷**：AI 写到卷末时提示用户"建议开新卷"，由用户在前端创建卷；AI 不自主开卷（卷边界是用户掌控的大动作）
12. **实时计算 num**：不 DB 维护 num 字段，list 时按 (volume_id, sort_order) 排序位次实时生成；DB 维护 num 等于回退 chapter_number 老方案，失去 id 方案价值
13. **get 函数签名**：前端 get/delete 函数保持现状 (T, error)，不走 PageResult（章节列表一次拿全部，不需要分页），不走 errcode（错误类型单一，delete 引用冲突走 ToolResult.Data 结构化返回）
14. **迁移代码组织**：migrate 包只留框架（MigrateState + backup + Step 接口 + runSteps）；v1.6.0 迁移步骤实现全部放 `internal/migrate/v160` 子包（registry.go + 各 step 文件）；Step 为接口避免 migrate → v160 循环依赖；vec 表重写归 commit 1.3（DROP 重建 + RebuildAll），1.5 只重写 5 张 GORM 表；`engine.Migration` 带 `Destructive` 字段声明是否破坏性（backup 只对 Destructive 迁移执行）+ `Description` 字段简述迁移目的（打印进日志便于排查）

### 待确认

1. **章节号实时计算的语义**：删除第 3 章后第 4 章变第 3 章，AI 上下文里的"第 5 章"指向变化，仅靠 InjectMessage 提示是否足够？
2. **migrate 自动备份保留份数**：保留最近 3 份是否合适？

