# 章节相关字段命名纠错方案

## 一、背景

章节删除功能经评估后**暂不做**（理由：写到 N 章后突然删中间章的场景极少，副作用大，"清空内容"是更安全的替代方案）。本方案只做字段命名纠错。

起因：调研章节删除时发现 timeline 表 `source_chapter_id` / `resolved_chapter_id` 字段名以 `_id` 结尾、类型 `int64`，但 Go 版实际存储的是**章节号**（chapter_number，int），不是 chapters.id。进一步排查发现共 **4 个字段** 存在同样的命名错位，且系统提示词有一条通用规则在反向误导。

---

## 二、问题根源：Python → Go 迁移时语义变更但字段名未同步

### 2.1 Python 版这两个字段真的是 chapters.id 外键

**`python-master/backend/timeline/models.py:80-82`**:

```python
source_chapter_id: Mapped[int | None] = mapped_column(ForeignKey("chapters.id", ondelete="SET NULL"))
resolved_chapter_id: Mapped[int | None] = mapped_column(ForeignKey("chapters.id", ondelete="SET NULL"))
```

Python 版：
- 字段类型 `ForeignKey("chapters.id")` —— **真正的外键到 chapters.id**
- `ondelete="SET NULL"` —— 级联约束
- `relationship(foreign_keys=[source_chapter_id])` —— ORM 关系定义

**所以这不是命名笔误，是 Go 版迁移时的设计变更**：

| 版本 | 字段名 | 实际存什么 | 字段名是否对 |
|------|--------|-----------|------------|
| Python | `source_chapter_id` | chapters.id（真外键）| ✅ 对 |
| Go | `source_chapter_id` | 章节号（语义变了）| ❌ 错（字段名没跟着改）|

Go 版迁移时决定用章节号而不是 chapters.id（章节号更稳定、更直观，chapters.id 是实现细节），但**字段名沿用了 Python 版的 `_id` 后缀**，导致命名与语义不一致。

### 2.2 Go 版实际存章节号的铁证

1. **`frontend/src/components/timeline/TimelineView.tsx:105`**
   ```ts
   setWindowCenter(entry.target_chapter || entry.source_chapter_id || 1)
   ```
   `windowCenter` 是章节窗口中心位置，用 `target_chapter`（明确章节号）和 `source_chapter_id` 用 `||` 串联做备选——只有两者都是章节号语义才成立。

2. **`frontend/src/components/timeline/TimelineView.tsx:243`**
   ```ts
   resolved_chapter_id: form.status === 'resolved' ? form.resolved_chapter_id || form.target_chapter : 0,
   ```
   `resolved_chapter_id` 的兜底值直接用 `form.target_chapter`（明确章节号），证明两者同语义空间。

3. **`frontend/src/components/timeline/TimelineView.tsx:218`** — 创建时 `source_chapter_id: 0`，传 0 是章节号"未指定"语义，chapters.id 应传 null。

4. **`internal/mcp_tools/timeline_tools.go:120`** — jsonschema description 是"在哪章创建/埋下的"，LLM 按字面传章节号。

5. **`internal/timeline/types.go:89-91`** — 注释明确写"在哪章创建/埋下的"、"在哪章回收"。

6. **`internal/search/service_test.go:541`** — `TargetChapter: 5, SourceChapterID: 1`，1 和 5 都是章节号。

7. **`internal/mcp_tools/rw_tools.go:210`** — `writing.WritingLog{ChapterID: int64(chapNum)}`，chapNum 来自 parseChapterNum 是章节号，被强转 int64 存入 ChapterID 字段。

### 2.3 系统提示词的反向误导

**`internal/agentcfg/identity.go:320`**:
> `部分工具会返回格式化信息，内嵌了xx_id 为数据库id，可以用来操作该条目`

按这条规则，LLM 看到 `source_chapter_id` / `resolved_chapter_id` / `chapter_id` 这些 `_id` 后缀字段，会理直气壮地认为是 chapters.id，而不是章节号。这是字段错位 + 通用规则共同导致的 LLM 困惑。

### 2.4 当年纠结的源头

**`internal/timeline/types.go:87`** 注释自相矛盾：
```go
TargetChapter int `...` // 预计回收章节号，主排序键，必填。不用于过滤，不准确不影响可见性，这个需要提醒llm完成的时候留下准确的id
```
前半句"章节号"，后半句"留下准确的id"，同一句话两个概念混用。git 历史显示是 commit `a3c1ef9` 当时引入的笔误，本意应是"留下准确的章节号"。

### 2.5 DESIGN.md 表述沿袭自 Python 版

**`internal/timeline/DESIGN.md:152`**:
```
| Chapter | `source_chapter_id` / `resolved_chapter_id` 引用章节 |
```
这个表述"引用章节"是从 Python 版沿袭的——Python 版是真外键引用，Go 版改成章节号引用但文档没更新。

---

## 三、问题字段清单

| 字段 | 类型 | 实际语义 | 字段名是否错 | DB 列名 |
|------|------|---------|------------|--------|
| `timeline.TimelineEntry.SourceChapterID` | int64 | 章节号（在哪章埋下）| ✅ 错 | `source_chapter_id` |
| `timeline.TimelineEntry.ResolvedChapterID` | int64 | 章节号（在哪章回收）| ✅ 错 | `resolved_chapter_id` |
| `writing.WritingLog.ChapterID` | int64 | 章节号 | ✅ 错 | `chapter_id` |
| `character.CharacterRelation.ChapterID` | int64 | 章节号（定为章节号）| ✅ 错 | `chapter_id` |

### 3.1 其他章节字段都是对的

| 字段 | 类型 | description | 语义 |
|------|------|------------|------|
| `current_chapter` | int | "当前章节号" | num ✓ |
| `target_chapter` | int | "预计回收章节号" | num ✓ |
| `actual_chapter` | int | "实际发生的章节号" | num ✓ |
| `planted_chapter` | int | "种下的章节号" | num ✓ |
| `revealed_chapter` | int | "实际揭露或回收的章节号" | num ✓ |
| `chapter_numbers` | []int | "限定章节号范围" | num ✓ |

规律：**`int` 类型 + `_chapter` 后缀 = 章节号（对的）**；**`int64` 类型 + `_id` 后缀 = 字段名错位（实际也是章节号）**。

### 3.2 对比其他实体

| 实体 | 引用方式 | 证据 |
|------|---------|------|
| 角色 | `character_id` (int64) | description "角色ID" |
| 地点 | `location_id` (int64) | description "地点ID" |
| 弧线 | `arc_id` (int64) | description "弧线ID" |
| 偏好 | `preference_id` (int64) | description "偏好条目ID" |

**其他实体统一用 id（自增主键），只有章节是用 num（业务编号）**。章节是异类，没有遵循"_id 后缀 = 数据库 id"的统一约定——这是 Python 版沿袭下来的设计差异，本方案通过改名把章节字段统一到章节号语义。

---

## 四、改名方案

### 4.1 字段映射

**两类命名策略**：

- **timeline** 用 `SourceChapter` / `ResolvedChapter` —— 因为它们是"关系型字段"（哪一章埋下/回收），与现有的 `TargetChapter` 同类，保持 `_chapter` 后缀风格一致
- **writing_log / character_relation** 用 `ChapterNumber` —— 因为它们是"直接引用 chapters 表的字段"，与 chapters 表的 `ChapterNumber` 字段名对齐，表达"引用 chapters.chapter_number"

| 旧名 | 新名 | 新 DB 列名 | 新 JSON key |
|------|------|-----------|-------------|
| `timeline.SourceChapterID` | `SourceChapter` | `source_chapter` | `source_chapter` |
| `timeline.ResolvedChapterID` | `ResolvedChapter` | `resolved_chapter` | `resolved_chapter` |
| `writing.WritingLog.ChapterID` | `ChapterNumber` | `chapter_number` | `chapter_number` |
| `character.CharacterRelation.ChapterID` | `ChapterNumber` | `chapter_number` | `chapter_number` |

### 4.2 改动文件清单

#### timeline 字段
- `internal/timeline/types.go:89,91` — Go 字段名 `SourceChapterID`→`SourceChapter`、`ResolvedChapterID`→`ResolvedChapter`；gorm column + json tag 同步改；类型 int64→int；注释同步
- `internal/timeline/types.go:87` — 顺手修注释笔误"留下准确的id"→"留下准确的章节号"
- `internal/timeline/DESIGN.md:33,152` — 文档同步（"引用章节"→"引用章节号"）
- `internal/mcp_tools/timeline_tools.go:120,170,207,217,230` — Go 字段名 + json tag + jsonschema description 同步改
- `app/timeline_view.go:65,82,107` — Go 字段名 + json tag 同步改
- `internal/agentcfg/identity.go:256` — 系统提示词字段名 `resolved_chapter_id`→`resolved_chapter`
- `frontend/src/components/timeline/TimelineView.tsx:46,49,105,182,218,243` — 字段名同步改
- `frontend/src/lib/wailsjs/go/models.ts` — wails generate 自动重新生成，不手动改

#### writing_log 字段
- `internal/writing/types.go:11` — Go 字段名 `ChapterID`→`ChapterNumber`；gorm column `chapter_id`→`chapter_number`；json tag 同步；类型 int64→int
- `internal/writing/store.go:30,34` — 函数签名 `chapterID int64`→`chapterNumber int` + 查询字段名
- `internal/writing/store_test.go` — 测试字段名同步
- `internal/mcp_tools/rw_tools.go:210` — `ChapterID: int64(chapNum)`→`ChapterNumber: chapNum`（顺手去掉 int64 强转）

#### character_relation 字段
- `internal/character/types.go:54` — Go 字段名 `ChapterID`→`ChapterNumber`；gorm column `chapter_id`→`chapter_number`；json tag 同步；类型 int64→int；注释从"此关系在哪个章节确立/变更"保留
- `internal/mcp_tools/character_tools.go:282,392` — Go 字段名 + json tag + jsonschema description（description 从"此关系确立/变化的章节ID"改为"此关系确立/变化的章节号"）
- `internal/character/store.go` — 如有引用则同步
- `dev_test/patch_test/main.go:31,122,130` — 测试字段名同步

#### 系统提示词与注释
- `internal/agentcfg/identity.go:320` — 通用规则补一句章节特例说明（见 4.3）

#### DB 迁移
- `internal/migrate/migrate.go` — 新增 4 个 `ALTER TABLE ... RENAME COLUMN` 迁移：
  ```sql
  ALTER TABLE time_entries RENAME COLUMN source_chapter_id TO source_chapter;
  ALTER TABLE time_entries RENAME COLUMN resolved_chapter_id TO resolved_chapter;
  ALTER TABLE writing_log RENAME COLUMN chapter_id TO chapter_number;
  ALTER TABLE character_relations RENAME COLUMN chapter_id TO chapter_number;
  ```
  注意：迁移前需查 `PRAGMA table_info` 判断列名是否已存在，避免重复执行报错（SQLite 不支持 IF EXISTS）。

### 4.3 系统提示词补充

**`internal/agentcfg/identity.go:320`** 通用规则补充章节特例：

```
部分工具会返回格式化信息，内嵌了xx_id 为数据库id，可以用来操作该条目。
注意：章节相关字段例外——章节用"章节号"（业务编号 1,2,3...）引用，
不是 chapters.id。字段名以 _chapter / _chapter_number 结尾的都是章节号。
```

### 4.4 前端字段同步

- `TimelineView.tsx` 所有 `source_chapter_id` / `resolved_chapter_id` 引用改为 `source_chapter` / `resolved_chapter`
- wails generate 后 `models.ts` 类型自动更新
- 前端如有其他引用（如 `useApp.ts` 类型定义）同步改

---

## 五、数据迁移

### 5.1 字段类型

- `int64` → `int`：SQLite 是动态类型，存储一致，**不需要数据转换**
- 列名 rename：SQLite 3.25+ 支持 `ALTER TABLE ... RENAME COLUMN`，4 行 SQL 即可
- **不破坏旧数据**：rename 后旧列名消失，所有行的新列名保留原值

### 5.2 迁移脚本位置

在 `internal/migrate/migrate.go` 现有 AutoMigrate 之后追加 rename 迁移。需要判断列名是否已存在避免重复执行（SQLite 不支持 IF EXISTS，需要先查 `PRAGMA table_info`）。

### 5.3 兼容性风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| LLM 历史工具调用记录字段名不一致 | messages 表存的旧 args JSON 含旧字段名，回看历史时读不到新字段显示 0 | 只影响历史回看展示，不影响新调用；用户确认历史调用本来就看不了这些字段，可接受 |
| MCP 工具 schema 字段名变化 | LLM 新调用会用新字段名 | 系统提示词补充章节特例说明（4.3）|
| 前端旧字段引用未改全 | 编译失败 | wails generate 后 TypeScript 编译会报错，按报错改 |
| DB 迁移失败 | 列已存在 / 列不存在 | 迁移前查 PRAGMA table_info 判断 |

---

## 六、实施步骤

### 步骤 1：DB 迁移
1. `internal/migrate/migrate.go` 加 4 个 rename column 迁移（带存在性检查）
2. 验证：旧数据库迁移后列名正确，新数据库直接建新列名

### 步骤 2：timeline 字段改名
1. `internal/timeline/types.go` 改字段名 + gorm tag + 注释（顺手修 :87 笔误）
2. `internal/timeline/DESIGN.md` 同步
3. `internal/mcp_tools/timeline_tools.go` 改字段名 + json tag + jsonschema
4. `app/timeline_view.go` 改字段名 + json tag
5. `internal/agentcfg/identity.go:256` 改提示词字段名
6. `internal/agentcfg/identity.go:320` 补章节特例说明

### 步骤 3：writing_log 字段改名
1. `internal/writing/types.go` 改字段名 + gorm tag
2. `internal/writing/store.go` 改函数签名
3. `internal/writing/store_test.go` 改测试
4. `internal/mcp_tools/rw_tools.go:210` 改字段名 + 去掉 int64 强转

### 步骤 4：character_relation 字段改名
1. `internal/character/types.go` 改字段名 + gorm tag + 注释
2. `internal/mcp_tools/character_tools.go` 改字段名 + json tag + jsonschema description
3. `internal/character/store.go` 如有引用同步
4. `dev_test/patch_test/main.go` 改测试

### 步骤 5：前端同步
1. `frontend/src/components/timeline/TimelineView.tsx` 改字段名
2. `wails generate module` 重新生成绑定
3. 按编译报错改其他前端引用

### 步骤 6：验证
- `go build ./...` 通过
- `go test ./internal/timeline/... ./internal/migrate/... ./internal/writing/... ./internal/character/...` 通过
- `npm run build` 通过
- `eslint` 通过
- `theme:check` / `i18n:check` 通过
- 手动启动应用：timeline CRUD 正常、writing log 正常、character relation 正常

---

## 七、不做的事

- **不做章节删除功能**：经评估不值得（详见背景）
- **不改 `timeline.TargetChapter` / `storyarc.TargetChapter` 等已正确的字段**：它们命名对，不动
- **不改 `novel_tools.go` get_chapter_list 返回的 `id` 字段**：那是真 chapters.id，含义清晰
- **不回到 Python 版的 chapters.id 语义**：Go 版用章节号是有意为之（章节号更稳定），只改字段名不改语义
- **不做右键菜单 / 详情面板等 UI 改造**：与字段改名无关
- **不重排章节号**：与字段改名无关
- **不写一次性脚本迁移 LLM 历史调用记录**：用户确认历史调用本来就看不了这些字段

---

## 八、待确认问题

1. **`character_relations` 表结构**：改名列前需确认表结构，避免与其他列冲突（执行步骤 1 前先查 `PRAGMA table_info(character_relations)`）
2. **`writing_log` 表结构**：同上，确认无其他 `chapter_number` 列冲突
3. **迁移脚本兼容性**：是否需要支持"已经手动改过列名"的数据库？还是假设所有用户都在旧列名状态？（默认假设所有用户都在旧列名状态，迁移带存在性检查即可）
