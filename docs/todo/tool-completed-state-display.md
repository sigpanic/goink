# 给大部分 MCP 工具加完成态具体内容显示

## 背景

v1.2.0 已经给 `delete_record` 实现了完成态具体内容显示（详见
[`docs/feat/v1.2.0/delete-record-display.md`](../feat/v1.2.0/delete-record-display.md)），
模式是"共享 ToolCallCard + 内嵌 `.tool-result-body` 分支"。

但其他 create / update 类工具目前**只有 DisplayText**，完成态显示
"创建新角色" / "更新角色设定" 这种笼统文本，用户回看历史时看不到具体创建/更新了什么。

**典型场景**：auto 模式下 create_character 直接从 executing 跳到 completed，
用户根本没看到具体角色名，只在历史里看到"创建新角色 + ✓完成"，不知道是哪个角色。

## 目标

把 delete_record 的"完成态具体内容"模式推广到大部分 create / update 类工具，
让用户回看历史时能看到"已创建 角色「林晚」"、"已更新 地点「书房」"等。

## 现状盘点

按"用户回看价值"分级：

### 强烈推荐（用户回看价值高）

| 工具 | 当前 DisplayText | 建议完成态文案 |
|---|---|---|
| `create_character` | 创建新角色 | 已创建 角色「{name}」 |
| `create_location` | 创建新地点 | 已创建 地点「{name}」 |
| `create_timeline_entry` | 记录追踪条目 | 已记录 时间线条目「{title}」 |
| `create_story_arc` | 创建故事弧线 | 已创建 故事弧「{name}」 |
| `create_arc_node` | 创建弧线节点 | 已创建 弧节点「{title}」（{story_arc}） |
| `create_reader_perspective_entry` | 添加读者视角 | 已记录 读者视角条目 #{id}（{entry_type}，第{planted_chapter}章） |
| `create_preference` | 创建创作偏好 | 已记录 偏好项 [{category}] |
| `create_new_chapter` | 创建新章节 | 已创建 第{chapter_number}章 |

理由：这些是"创建"操作，结果是不可逆的"新增"，用户回看时最想知道"具体是哪个实体"。

### 可选（更新类，回看价值中等）

| 工具 | 当前 DisplayText | 建议完成态文案 |
|---|---|---|
| `update_character` | 更新角色设定 | 已更新 角色「{name}」 |
| `update_location` | 更新地点设定 | 已更新 地点「{name}」 |
| `update_timeline_entry` | 更新追踪条目 | 已更新 时间线条目「{title}」 |
| `update_story_arc` | 更新故事弧线 | 已更新 故事弧「{name}」 |
| `update_arc_node` | 更新弧线节点 | 已更新 弧节点「{title}」 |
| `update_reader_perspective_entry` | 更新读者视角 | 已更新 读者视角条目 #{id} |
| `update_preference` | 更新创作偏好 | 已更新 偏好项 [{category}] |
| `update_chapter_plan` | 更新章节计划 | 已更新 章节计划 第{n}章 |

理由：更新操作回看价值低于创建（用户已经知道这个实体存在了），但仍然比纯 DisplayText 信息量大。

### 不推荐（DisplayText 已够或重复）

| 工具 | 不加原因 |
|---|---|
| `edit_chapter` / `edit` / `read` | buildDisplay 已经查 DB 生成"编辑/查看 第N章 标题"，DisplayText 已具体 |
| `update_creative_profile` | 全局唯一规则，不存在多实例，DisplayText "设置创作规则" 已够 |
| `update_character_relationship` | 关系更新走 approval，ApprovalBody 已显示完整内容，完成态再加重复 |
| `create_location_relation` / `update_location_relation` | 同上 |
| `lint_chapter` | 检查类工具，结果通过 error/result 体现，不需要"已检查 第N章" |
| `get_*` 类工具 | 查询类操作，DisplayText "查看 xxx" 已够，回看价值低 |

## 设计方案

完全复用 delete_record 的模式，**不引入新机制**：

### 后端改动

#### 1. 扩展 `resultDataMergeTools` / `resultFieldTools` 白名单

`internal/agent/display.go` 中把上述"强烈推荐"+"可选"的工具加入两个白名单：

```go
var resultDataMergeTools = map[string]bool{
    "web_search":                    true,
    "web_fetch":                     true,
    "delete_record":                 true,
    "create_character":              true,
    "create_location":               true,
    "create_timeline_entry":         true,
    "create_story_arc":              true,
    "create_arc_node":               true,
    "create_reader_perspective_entry": true,
    "create_preference":             true,
    "create_new_chapter":           true,
    "update_character":              true,
    "update_location":               true,
    // ... 其他 update 工具
}
```

`resultFieldTools` 同步扩展。

#### 2. 各工具 `result.Data` 字段对齐

确认每个工具 success 时返回的 `result.Data` 包含前端需要的字段（name/title/id 等）。
当前 `create_*` 工具可能只返回 `{"id": xxx}`，需要在 `mcp_tools/` 下各工具实现里补返回字段：

```go
// 示例：character_tools.go 中 CreateCharacter 成功分支
return mcp_tools.ToolResult{
    Success: true,
    Data: map[string]any{
        "created": map[string]any{  // ← 新增 created 包裹
            "id":   ch.ID,
            "name": ch.Name,
            "type": "character",
        },
    },
}
```

**注意**：delete_record 用 `deleted` 包裹，create_* 用 `created` 包裹，update_* 用 `updated` 包裹。
这样前端可以根据 key 区分渲染分支。

### 前端改动

#### 3. i18n key 扩展

`zh-CN.json` / `en.json` 的 chat 域新增 `createdEntity*` / `updatedEntity*` 系列 key
（参考已有的 `deletedEntity*` 模式）：

```json
"createdEntityCharacter": "已创建 角色「{{name}}」",
"createdEntityLocation": "已创建 地点「{{name}}」",
"createdEntityTimelineEntry": "已记录 时间线条目「{{title}}」",
"createdEntityStoryArc": "已创建 故事弧「{{name}}」",
"createdEntityArcNode": "已创建 弧节点「{{title}}」（{{storyArc}}）",
"createdEntityReaderEntry": "已记录 读者视角条目 #{{id}}（{{entryType}}，第{{plantedChapter}}章）",
"createdEntityPreference": "已记录 偏好项 [{{category}}]",
"createdEntityChapter": "已创建 第{{chapterNumber}}章",
// ...
"updatedEntityCharacter": "已更新 角色「{{name}}」",
// ...
```

#### 4. ToolCallCard.tsx 新增 CreatedEntityBody / UpdatedEntityBody 共享组件

参考本轮 `DeletedEntityBody` 的实现：

```tsx
function CreatedEntityBody({ created }: { created: Record<string, unknown> }) {
  const { t } = useTranslation()
  const typeLabels = getTypeLabels(t)
  // ...按 created.type 分支渲染
}

function UpdatedEntityBody({ updated }: { updated: Record<string, unknown> }) {
  // 同上
}
```

主组件完成态分支扩展：

```tsx
const entityBody = isCompleted && result ? (
  toolName === 'delete_record' && result.deleted
    ? <DeletedEntityBody deleted={result.deleted as Record<string, unknown>} />
  : toolName === 'create_character' && result.created
    ? <CreatedEntityBody created={result.created as Record<string, unknown>} />
  // ... 其他 create / update 工具
  : null
) : null
```

可以进一步用映射表简化（避免 if 链）：

```tsx
const ENTITY_BODY_RENDERERS: Record<string, (data: Record<string, unknown>) => ReactNode> = {
  delete_record: (d) => <DeletedEntityBody deleted={d.deleted as Record<string, unknown>} />,
  create_character: (d) => <CreatedEntityBody created={d.created as Record<string, unknown>} />,
  // ...
}
```

### 实施步骤（建议分批）

#### 批次 1：create_* 工具（强烈推荐 8 个）

1. 后端 `mcp_tools/` 下 8 个 create 工具的 success 分支补 `result.Data.created` 包裹
2. 后端白名单扩展 8 个工具
3. 前端 i18n 加 8 个 `createdEntity*` key
4. 前端 ToolCallCard 加 `CreatedEntityBody` 共享组件 + 完成态分支
5. 验证：跑 `go build` + `npm run build` + 各场景手测

#### 批次 2：update_* 工具（可选 8 个）

同上模式，但用 `updated` 包裹 + `updatedEntity*` i18n key。

#### 批次 3：抽象共享组件（可选重构）

如果批次 1+2 完成后发现 `DeletedEntityBody` / `CreatedEntityBody` / `UpdatedEntityBody`
代码重复太多，可以抽一个 `<EntityBody mode="deleted|created|updated" data={...} />`
统一组件，按 mode 切换 i18n key 前缀。

## 涉及文件清单

### 批次 1（create_* 工具）
- `internal/mcp_tools/character_tools.go`（CreateCharacter 补 created 字段）
- `internal/mcp_tools/location_tools.go`（CreateLocation 补 created 字段）
- `internal/mcp_tools/timeline_tools.go`（CreateTimelineEntry 补 created 字段）
- `internal/mcp_tools/storyarc_tools.go`（CreateStoryArc / CreateArcNode 补 created 字段）
- `internal/mcp_tools/reader_tools.go`（CreateReaderPerspectiveEntry 补 created 字段）
- `internal/mcp_tools/preference_tools.go`（CreatePreference 补 created 字段）
- `internal/mcp_tools/chapter_tools.go`（CreateNewChapter 补 created 字段）
- `internal/agent/display.go`（扩展两个白名单）
- `frontend/src/i18n/locales/zh-CN.json` + `en.json`（加 8 个 i18n key）
- `frontend/src/components/chat/ToolCallCard.tsx`（加 CreatedEntityBody + 完成态分支）

### 批次 2（update_* 工具）
- 同上对应文件，update 工具补 `updated` 字段 + `updatedEntity*` i18n key

## 优先级与收益评估

| 批次 | 改动量 | 收益 | 推荐度 |
|---|---|---|---|
| 批次 1（create_*） | 后端 ~8 处 + 前端 1 处 + i18n 16 行 | 高（auto 模式下用户能看到创建了什么） | ⭐⭐⭐ 推荐做 |
| 批次 2（update_*） | 后端 ~8 处 + 前端 1 处 + i18n 16 行 | 中（更新操作回看价值低于创建） | ⭐⭐ 可选 |
| 批次 3（抽象重构） | 前端 1 处 | 低（代码整洁度，不影响功能） | ⭐ 视情况 |

## 注意事项

### 1. result.Data 字段契约稳定性

前端各 EntityBody 读取的字段（如 `created.name` / `created.title` / `created.type`）
必须与后端 `result.Data.created` 的字段名严格对齐。建议每个工具在文档中明确声明返回字段，
或在 `mcp_tools/base.go` 中定义 `Created`/`Updated` 类型常量。

### 2. auto 模式 vs manual 模式

- auto 模式：executing → completed，无审批态，用户只看到完成态具体内容
- manual 模式：executing → awaiting_approval → completed，审批态已显示具体内容，完成态再加是冗余但有用（回看历史时）

因此**完成态具体内容显示在 auto 模式下价值最高**。如果实现成本高，可考虑先做 auto 模式专用。

### 3. 性能考虑

`result.Data` 合并到 `event.metadata` 后通过 SSE 推给前端，每个 create / update 工具
都会多传几 KB 数据。对于 create_character 这种小数据量工具，影响可忽略；
但如果未来要给 `web_fetch` 之类大 result 加白名单（已有），要注意 SSE 包大小限制。

### 4. 历史回看路径

`buildToolDisplay` 的 `resultFieldTools` 白名单同步扩展即可，前端 `rebuildTurns`
已支持读取 `td.result` 字段。

### 5. 不要破坏 delete_record 现有行为

本轮 delete_record 已经实现完成态显示，扩展时不要改 `DeletedEntityBody` 的字段读取逻辑。
如果发现字段缺失需要兜底，应该统一在 `DeletedEntityBody` / `CreatedEntityBody` /
`UpdatedEntityBody` 中处理（参考 stage 2 验证报告的 ⚠9a 项）。

## 与 v1.2.0 delete_record 修复的关系

- v1.2.0 的 delete_record 修复是**回归 bug 修复 + 完成态显示 PoC**
- 本 todo 是把 PoC 模式推广到大部分 create / update 工具
- 实施时可以参考 `internal/agent/display.go` 中 `resultDataMergeTools` / `resultFieldTools`
  白名单的扩展方式，以及 `frontend/src/components/chat/ToolCallCard.tsx` 中
  `DeletedEntityBody` 的实现模式

## 后续可能的扩展

### A. 给 edit_chapter 加完成态

`edit_chapter` 目前走 chapterTools 模式（buildDisplay 查 DB 生成"编辑 第N章 标题"），
完成态已经显示具体章节号。但如果想显示"已编辑 第N章 标题" + diff 摘要（如"+128 -15 行"），
可以走类似模式：把 `edit_chapter` 加入白名单，result.Data 输出 diff 统计。

### B. 给 run_subagent 加完成态

`run_subagent` 的 DisplayText 已经按 agent_type 细化（"探索故事记忆"/"审核章节内容"），
但完成态可以显示"已审核 第3章"或"已检索 N 条记忆"。需要 run_subagent 工具实现中
返回统计信息到 result.Data。
