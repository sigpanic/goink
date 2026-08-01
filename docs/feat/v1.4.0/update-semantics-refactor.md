# v1.4.0 — 全面重构 Update 语义：彻底废除伪 PATCH

## 背景与动机

### 现状：伪 PATCH 四不像

Goink 所有 `Update*Input` 结构体字段用值类型 + `omitempty` tag，后端用 `if input.Field != ""` 判断是否更新。注释写"只更新非零值字段"，自我安慰是 PATCH 语义，实际是个四不像：

```go
// 现状（伪 PATCH）
type UpdateNovelInput struct {
    Title       string `json:"title,omitempty"`
    Description string `json:"description,omitempty"`
    Genre       string `json:"genre,omitempty"`
}

func (a *App) UpdateNovel(novelID int64, input UpdateNovelInput) (*novel.Novel, error) {
    // ...
    if input.Title != "" { n.Title = input.Title }       // 空字符串 = 不更新
    if input.Description != "" { n.Description = input.Description }
    if input.Genre != "" { n.Genre = input.Genre }
    // ...
}
```

### 伪 PATCH 的三个根本缺陷

**缺陷 1：无法主动清空字段**

`omitempty` 让零值（`""`/`0`/`false`）在 JSON 序列化时被跳过。用户在编辑表单里把字段清空保存，后端 `if input.Field != ""` 跳过，字段保持原值。用户的清空意图被静默忽略。

| 用户意图 | 前端传值 | 后端行为 | 结果 |
|---|---|---|---|
| 不改字段 | 原值（非空） | `if != ""` 进分支 | 更新（幂等） |
| 改成新值 | 新值（非空） | `if != ""` 进分支 | 更新 ✓ |
| **主动清空** | `""` | `if != ""` 跳过 | **保持原值** ❌ |

**缺陷 2：前端全量传时毫无意义**

前端编辑表单预填原值，保存时全量传所有字段。后端 `if != ""` 判断成为多余——前端不会漏传（表单预填了），判断零值反而阻止用户清空。

`PatchAndSave` 工具（[internal/storage/patch.go](../../../internal/storage/patch.go)）基于 `json.Marshal` + `omitempty` 跳过零值字段，同样是伪 PATCH。前端全量传时，所有字段都有值，`json.Marshal` 序列化所有字段，`json.Unmarshal` 覆盖所有字段 → 退化成 PUT。`PatchAndSave` 的 PATCH 语义完全失效。

**缺陷 3：无法区分"不传"和"传零值"**

值类型 + `omitempty` 无法区分三种状态：

| 前端意图 | 真正 PATCH 应传 | 伪 PATCH 实际 | 结果 |
|---|---|---|---|
| 不改字段 | 不传（undefined） | 原值或空字符串 | 原值更新（幂等），空值被忽略 |
| 改成新值 | 传新值 | 传新值 | 正确 ✓ |
| 主动清空 | 传空字符串 `""` | 传空字符串 `""` | **被忽略，无法清空** ❌ |

### 前端漏传风险（伪 PATCH 在掩盖 bug）

子 agent 调查发现，5 处 `Update*` 调用存在漏传字段，伪 PATCH 的 `if != ""` 在兜底保护：

| 风险 | 调用 | 漏传字段 | 后果（若改 PUT 不修前端） |
|---|---|---|---|
| 极高 | `UpdateArcNode` 快速状态切换 | title/description/target_chapter/actual_chapter | 只传 status，节点被清空 |
| 高 | `UpdateCharacter` | personality | AI 写入的角色性格 JSON 丢失 |
| 高 | `UpdateLocation` | detail_json | AI 写入的位置详情 JSON 丢失 |
| 中 | `UpdateStoryArc` | status/reactivate_at | 故事线状态丢失 |
| 中 | `UpdateArcNode` 编辑 | actual_chapter/status | 节点实际章节丢失 |

漏传的字段大多是 **AI 写入、前端只读、UI 无编辑入口**的字段。当前伪 PATCH 在保护这些漏传场景——后端忽略空值，所以数据没丢。但这是**错误的兜底**：应该修前端漏传，而不是后端假装看不见。

## 设计目标

- **彻底废除伪 PATCH**：不再用值类型 + `omitempty` + `if != ""` 的四不像模式
- **两层独立语义**：App 层（前端）用 PUT，MCP 工具层（LLM）用 PATCH，互不影响
- **前端简单省事**：全量传，不用 diff 判断哪些字段变了
- **LLM 省 token**：只传变化字段，不为未改字段浪费 token
- **前端漏传可检测**：TS 类型系统强制全量传，编译时拦截
- **AI 写入字段安全**：input 结构体不含 AI 字段，PUT 覆盖不会丢 AI 数据

## 方案：两层独立语义

### 架构

```
┌─────────┐  PUT 全量传       ┌─────────┐
│ 前端    │ ───────────────→ │ App 层  │ db.Save 全量覆盖
│ (表单)  │                  │         │
└─────────┘                  └─────────┘

┌─────────┐  RawArgs PATCH    ┌─────────┐
│ LLM     │ ───────────────→ │ MCP 工具│ First + json.Unmarshal(RawArgs) + Save
│ (agent) │  只传变化字段     │ 层      │（LLM 原始 JSON 直接覆盖 entity）
└─────────┘                  └─────────┘
```

两层用**不同的结构体**，互不影响：
- App 层：`UpdatePreferenceInput`、`UpdateNovelInput` 等
- MCP 工具层：`UpsertPreferenceItem`、`UpsertSettingItem` 等

### App 层（前端）→ PUT 语义

**后端**：
- input 字段用值类型，**不用 omitempty**
- input 只含用户可编辑字段（AI 字段不在 input 里，PUT 覆盖不会丢 AI 数据）
- 后端直接赋值 + `db.Save`（走 GORM 回调）
- 不用 `PatchAndSave`，不用 `if != ""` 判断

```go
// UpdatePreferenceInput 采用 PUT 语义：前端全量传，后端全量覆盖。
type UpdatePreferenceInput struct {
    Category string `json:"category"`
    Content  string `json:"content"`
    IsGlobal bool   `json:"is_global"`
}

func (a *App) UpdatePreference(novelID int64, id int64, input UpdatePreferenceInput) (*novel.PreferenceItem, error) {
    var item novel.PreferenceItem
    if err := a.novel.DB.WithContext(a.ctx).First(&item, id).Error; err != nil {
        return nil, fmt.Errorf("update preference: %w", err)
    }
    // 归属校验
    if !item.IsGlobal && item.NovelID != novelID {
        return nil, fmt.Errorf("update preference: 无权修改其他小说的偏好")
    }
    // PUT 全量覆盖
    item.Category = input.Category
    item.Content = input.Content
    item.IsGlobal = input.IsGlobal
    if input.IsGlobal {
        item.NovelID = 0
    } else {
        item.NovelID = novelID
    }
    if err := a.novel.DB.WithContext(a.ctx).Save(&item).Error; err != nil {
        return nil, fmt.Errorf("update preference: %w", err)
    }
    return &item, nil
}
```

**前端**：
- 编辑表单预填原值，保存时全量传所有字段
- 不用 diff 判断哪些字段变了

**TS 类型强制全量传**：
去掉 `omitempty` 后，Wails 生成的 TS 类型从 `field?: type`（可选）变 `field: type`（必填）。前端传对象字面量时少传字段会 TS 编译错误。

```ts
// 改造前（omitempty，可选）
class UpdatePreferenceInput {
    category?: string;    // 可选，漏传不报错
    content?: string;
    is_global?: boolean;
}

// 改造后（无 omitempty，必填）
class UpdatePreferenceInput {
    category: string;     // 必填，漏传 TS 编译错误
    content: string;
    is_global: boolean;
}
```

> **注意**：TS 必填校验是半吊子——对象字面量能拦，`new UpdatePreferenceInput()` 构造拦不住（因为 `constructor(source: any = {})` 用 `any` 接收）。但比 omitempty（完全不校验）强，前端实际都是传对象字面量。

### MCP 工具层（LLM）→ json.Unmarshal RawArgs PATCH

**后端机制**（项目所有 MCP 更新工具的统一模式）：

```go
// 1. First 加载完整 entity（含所有字段）
db.First(&entity, id)

// 2. json.Unmarshal LLM 原始 JSON 到 entity
//    LLM 只传变化字段 → tc.RawArgs 只含这些字段 → Unmarshal 只覆盖这些字段
//    没传的字段保持 First 加载的原值
json.Unmarshal(tc.RawArgs, &entity)

// 3. Save 保存（走 GORM 回调）
db.Save(&entity)
```

**这是真 PATCH**：
- LLM 传什么覆盖什么，没传的保持原值
- 不用手写 `if != ""` 判断每个字段
- 依赖 LLM 天然只传变化字段（省 token）
- 用 `tc.RawArgs`（LLM 原始 JSON），不用 args 结构体（args 经过 Go 反序列化后丢失"字段是否传"的信息）

**项目内所有 MCP 更新工具都用此模式**：
- [character_tools.go:259](../../../internal/mcp_tools/character_tools.go#L259) `json.Unmarshal(tc.RawArgs, &ch)`
- [location_tools.go:329](../../../internal/mcp_tools/location_tools.go#L329) `json.Unmarshal(tc.RawArgs, &loc)`
- [timeline_tools.go:244](../../../internal/mcp_tools/timeline_tools.go#L244) `json.Unmarshal(tc.RawArgs, &entry)`
- [storyarc_tools.go:258](../../../internal/mcp_tools/storyarc_tools.go#L258) `json.Unmarshal(tc.RawArgs, &arc)`
- [reader_perspective_tools.go:259](../../../internal/mcp_tools/reader_perspective_tools.go#L259) `json.Unmarshal(tc.RawArgs, &entry)`

**`upsert_preference` 是特例，刻意不用 RawArgs 模式**：

preference 工具从原本的 `json.Unmarshal(tc.RawArgs)` 模式**刻意重构**为 upsert + if 判断模式，因为需要 RawArgs 模式无法支持的能力：
1. **upsert 语义**（ID 不传=新建，传=更新）—— RawArgs 模式只能更新，不能创建
2. **batch 原子事务**（1-5 个，失败回滚）—— RawArgs 模式是单条操作
3. **is_global 切换时调整 NovelID**（业务逻辑）—— `json.Unmarshal` 不会处理 NovelID 联动
4. **归属校验**（不能改其他小说的偏好）—— RawArgs 模式没有校验环节

所以 `upsert_preference` 用 `if item.Category != ""` + `if item.IsGlobal != nil` 手动判断，是**刻意的设计选择**，不是偏离惯例。

### 为什么不统一用一种语义

| 调用方 | PUT 的问题 | PATCH 的问题 |
|---|---|---|
| 前端 | 无（表单全量传，简单） | 前端要 diff 判断哪些字段变了，啰嗦 |
| LLM | LLM 全量传所有字段，浪费 token | 无（LLM 天然只传变化字段） |

两层需求矛盾，统一用一种必然有一方受损。拆开两层各自最优。

## 改造范围

### App 层 Update 方法（PUT 改造）

| 方法 | input 字段 | AI 写入字段 | PUT 改造要点 |
|---|---|---|---|
| `UpdatePreference` | category/content/is_global | 无 | 无（已改造完成） |
| `UpdateNovel` | title/description/genre | 无 | 无 |
| `UpdateCharacter` | name/description/personality/abilities | personality（**当前在 input 里**） | **input 移除 personality**（AI 字段不该给前端传）+ 修前端漏传 |
| `UpdateLocation` | name/location_type/description/parent_location_id/tags/clear_parent | detail_json（不在 input ✓） | 修前端漏传，detail_json 安全 |
| `UpdateTimelineEntry` | title/content/detail_json/target_chapter/importance/status/resolved_chapter | detail_json（**当前在 input 里**） | **input 移除 detail_json** + 修前端漏传 |
| `UpdateChapterPlan` | scope/content | 无 | 无 |
| `UpdateStoryArc` | name/description/arc_type/importance/status/reactivate_at | 无 | 修前端漏传 status/reactivate_at |
| `UpdateArcNode` | title/description/target_chapter/actual_chapter/status | 无 | 修前端漏传 actual_chapter/status + 快速状态切换需特殊处理 |
| `UpdateReaderPerspective` | type/content/planted_chapter/related_truth/revealed_chapter | 无 | 无 |
| `UpdateStyleSample` | id/name/content/tags/is_global/novel_id | 无 | 无（已是 PUT 语义） |

### 前端漏传修复（PUT 改造的前提）

PUT 全量覆盖要求前端表单包含 input 的所有字段。当前 5 处漏传必须先修：

| 文件 | 漏传字段 | 修复方式 |
|---|---|---|
| `CharacterListView.tsx` | personality | openEdit 预填 personality，或 input 去掉 personality（AI 字段不该在 input 里） |
| `LocationListView.tsx` | detail_json | input 去掉 detail_json（AI 字段，前端不该传） |
| `ArcListView.tsx`（编辑故事线） | status/reactivate_at | openEditArc 预填 status/reactivate_at |
| `ArcListView.tsx`（编辑节点） | actual_chapter/status | openEditNode 预填 actual_chapter/status |
| `ArcListView.tsx`（快速状态切换） | 只传 status | 改成读取原节点 → 改 status → 全量传，或单独提供 `UpdateArcNodeStatus` API |

### check_omitempty CI 脚本

[scripts/check_omitempty](../../../scripts/check_omitempty) 是 PatchAndSave 时代的产物，检查 `Update*Input` 字段是否有 `omitempty`。PUT 改造后字段不用 omitempty，这个脚本会报错。

**处理方式**：
- 脚本支持 `//nolint:omitempty` 注释跳过检查
- 但 check_omitempty **未接入 CI**（pre-commit hook 和 GitHub Actions 都不跑），是孤立脚本
- v1.4.0 改造后可考虑删除该脚本，或保留给 MCP 工具层（PATCH 语义的 input 仍用 omitempty）

## 实施步骤

### 第 1 步：UpdatePreference（已完成，首个示范）

- App 层 `UpdatePreference` 改 PUT
- 前端 `PreferenceView` 编辑表单加归属切换 UI
- 修复 is_global 切换时 NovelID 处理
- 加归属校验（不能改其他小说的偏好）

### 第 2 步：修前端漏传（PUT 改造前提）

- 修 5 处 `openEdit` 漏填字段
- 修 `UpdateArcNode` 快速状态切换（改成全量传或单独 API）
- AI 写入字段（personality/detail_json）从 input 移除（前端不该传 AI 字段）

### 第 3 步：App 层全量改 PUT

按风险从低到高：
1. `UpdateNovel`（3 字段，无 AI 字段，无漏传）
2. `UpdateChapterPlan`（2 字段，无 AI 字段，无漏传）
3. `UpdateReaderPerspective`（5 字段，无 AI 字段，无漏传）
4. `UpdateTimelineEntry`（7 字段，无 AI 字段，无漏传）
5. `UpdateStoryArc`（修漏传后改 PUT）
6. `UpdateArcNode`（修漏传 + 快速状态切换后改 PUT）
7. `UpdateCharacter`（input 去掉 personality 后改 PUT）
8. `UpdateLocation`（input 去掉 detail_json 后改 PUT，注意 ClearParent 特殊处理）
9. `UpdateStyleSample`（已是 PUT 语义，去掉 omitempty 即可）

每个方法改造：
- input 去掉 omitempty
- 后端删 `if != ""` 判断，直接赋值 + `db.Save`
- 重新生成 Wails 绑定
- 验证 TS 类型变必填
- 前端验证 build + lint

### 第 4 步：MCP 工具层保持 json.Unmarshal RawArgs PATCH（preference 例外）

MCP 工具层大部分工具（character/location/timeline/storyarc/reader_perspective）用项目统一的 PATCH 模式：
- `db.First(&entity, id)` 加载完整 entity
- `json.Unmarshal(tc.RawArgs, &entity)` 用 LLM 原始 JSON 覆盖
- `db.Save(&entity)` 保存

**`upsert_preference` 是例外**（刻意设计）：
- 用 upsert + if 判断模式（不用 RawArgs）
- 因为需要 upsert 语义/batch 事务/is_global 切换 NovelID/归属校验
- 详见上文"特例"说明

### 第 5 步：清理 check_omitempty

- 评估是否保留 check_omitempty 脚本
- 如果保留，限制只检查 MCP 工具层的 input（App 层已改 PUT）
- 如果删除，确认没有其他依赖

## 风险评估

### 风险 1：前端漏传导致数据清空

**场景**：PUT 全量覆盖，前端漏传某字段，后端用零值覆盖。

**缓解**：
- TS 必填类型编译时拦截（对象字面量少传字段报错）
- 改造前先修 5 处漏传（第 2 步）
- AI 写入字段从 input 移除（前端不该传 AI 字段）

**残留风险**：`new UpdatePreferenceInput()` 构造不校验（TS 半吊子）。但前端实际都是传对象字面量，风险低。

### 风险 2：input 含 AI 字段时会被覆盖（input 设计问题，非 PUT 风险）

**澄清**：PUT 只覆盖 input 里的字段，其他字段从 `First` 加载保留原值。所以"AI 数据丢失"不是 PUT 语义的风险，而是 **input 结构体设计**的问题。

```go
// PUT 的行为：
db.First(&item, id)           // 1. 加载完整 entity（含 AI 字段）
item.Category = input.Category // 2. 只覆盖 input 里的字段
item.Content = input.Content   //    input 没有的字段（如 personality）保留原值
db.Save(&item)                 // 3. 保存整个 entity
```

**input 含 AI 字段时才有风险**：
- 当前 `UpdateCharacterInput` 含 `personality` 字段（AI 写入）
- 前端回传 personality 旧值 → 覆盖 AI 后台更新的新值
- 这是 input 设计错误——AI 字段不该在 input 里

**解决**：AI 写入字段（personality/detail_json）从 input 移除。input 只含用户可编辑字段，AI 字段不在 input 里 → PUT 天生安全（AI 字段从 First 加载保留）。

**残留风险**：无（input 不含 AI 字段时，PUT 不碰 AI 字段）。

### 风险 3：并发覆盖（Lost Update）

**场景**：A 加载偏好 → B 加载同一偏好 → A 改了保存 → B 没改但回传原值保存 → A 的修改被覆盖。

**缓解**：Goink 是单用户桌面应用，并发几乎不存在。可忽略。

### 风险 4：db.Save 全量覆盖的副作用

**场景**：`db.Save` 保存所有字段，包括 ID/CreatedAt 等系统字段。

**缓解**：
- item 从数据库加载，ID/CreatedAt 有值，Save 时保留原值
- GORM Save 用 ID 作为 WHERE 条件，不会创建新行
- input 不含 ID/CreatedAt 等系统字段，后端不会误改

## 已完成（v1.4.0 首批）

- [x] `UpdatePreference` 改 PUT（[app/novel.go](../../../app/novel.go)）
- [x] 前端 `PreferenceView` 编辑表单加归属切换 UI（[PreferenceView.tsx](../../../frontend/src/components/preference/PreferenceView.tsx)）
- [x] 修复 `CreatePreference` is_global bug（NovelID 残留）
- [x] 修复 `UpdatePreference` is_global 切换 bug（NovelID 不调整）
- [x] 加归属校验（不能改其他小说的偏好）
- [x] TS 类型变必填（去掉 omitempty）

## 待办

### 前端（融入 v1.4.0 前端架构重构，领域推进时顺手做）

- [ ] 各领域 useUpdateXxx mutation 的 payload 全量回传 input 所有字段（见 [refactor-plan/00-conventions.md §6](./refactor-plan/00-conventions.md)）
- [ ] `UpdateArcNode` 快速状态切换：从 query 缓存读完整节点 → 改 status → 全量回传，或单独提供 `UpdateArcNodeStatus` API

### 后端（单独重构）

- [ ] 从前端 input 移除前端不可编辑字段（`UpdateCharacterInput.Personality` / `UpdateLocationInput.DetailJSON` 等 AI 字段）
- [ ] input 全必填 + 去 `omitempty` + `db.Save` 全量覆盖（按第 3 步顺序）
- [ ] 评估 check_omitempty 脚本去留

### 说明

- 前端全量回传在后端 input 变化前后都能工作：移除 AI 字段前透传 query 缓存最新值，移除后 TS 类型变必填前端自然适配。
- 前后端解耦，前端规范统一是「全量回传 input 字段」，不依赖后端重构进度。
