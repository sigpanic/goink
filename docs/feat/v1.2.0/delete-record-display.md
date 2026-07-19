# v1.2.0 — delete_record 展示链路修复

## 背景与动机

v1.2.0 在 `73916ee feat: i18n chat domain components` 中把 `ToolCallCard.tsx` 的 7 段内联硬编码中文
替换为 `t()` 调用并传占位符参数，但同 commit 在 `zh-CN.json` / `en.json` 里新增的 `confirmDelete*`
key **漏写了占位符**，导致审批弹窗只显示 `"确认删除 角色关系"` 这类纯文本，
看不到具体的 source / target / relation / title / name。

与此同时，后端 `display.go` 对 `delete_record` 的展示处理一直停留在固定字符串 `"删除记录"`，
既不区分删除的实体类型（角色 / 地点 / 关系 / 弧节点 ...），也不在完成态透传 `result.Data.deleted`，
因此 executing / completed 阶段都只能看到笼统的"删除记录"。

本次修复需要解决三个递进的问题：

1. 审批弹窗缺占位符（i18n 回归 bug）
2. executing / completed 都是笼统 "删除记录"，看不出删的是什么类型
3. 完成态完全不显示具体内容，审批通过后只剩 "删除记录"

## 根因定位

### 回归 commit
- hash: `73916ee769e7282898e0618141b95f16b77657cd`
- date: 2026-07-02
- message: `feat: i18n chat domain components`
- 改动范围: 13 个 chat 组件 + 2 个 locale json
- 后端 `display.go` 不在该 commit 改动列表内，所以 displayText 一直是固定字符串

### 调用方 vs locale 的占位符错位

`frontend/src/components/chat/ToolCallCard.tsx:68-85` 的 `ApprovalBody` 调用方传参：

| i18n key | 调用方传参 | locale 实际占位符 |
|---|---|---|
| `confirmDeleteCharacterRelation` | `{ source, target, relation }` | 无 |
| `confirmDeleteLocationRelation` | `{ locationA, locationB, relation }` | 无 |
| `confirmDeleteArcNode` | `{ title, storyArc }` | 无 |
| `confirmDeleteReaderEntry` | `{ id, entryType, plantedChapter }` | 无 |
| `confirmDeletePreference` | `{ category, id }` | 无 |
| `confirmDeleteTimelineEntry` | `{ title }` | 无 |
| `confirmDeleteGeneric` | `{ label, title }` | `{{label}}` 一个 |

react-i18next 在 `compatibilityJSON: v4` 模式下，调用方多传的参数会被静默丢弃，
所以审批弹窗只显示静态文案，回归由此产生。

### 后端两处白名单

`internal/agent/agent.go:302-309` 的 `result.Data` 合并白名单、
`internal/agent/display.go:243-245` 的 `toolDisplays.result` 字段白名单，
均硬编码为 `{ "web_search", "web_fetch" }`，`delete_record` 不在其中。
导致 delete_record 返回的 `result.Data.deleted`（含 id / name / title / type / source / target / relation 等）
**完全传不到前端**。

### 前端完成态渲染逻辑

`ToolCallCard.tsx:164-188` 完成态渲染只读取 `status + activityKind + displayText`，
不读取 `toolName / approvalType / approvalPayload`，所以即便后端补齐 `result` 字段，
前端也需要新增分支才能在完成态展示具体内容。

## 设计目标

- **优化 1（必做 · 回归 bug）**：补齐 i18n locale 占位符，让审批弹窗恢复完整内容
- **优化 2（推荐）**：`buildDisplay` 给 `delete_record` 加 table 分支，让 executing/completed 区分类型
- **优化 3（可选 · 方案 A）**：完成态透传 + 渲染 `result.deleted`，让历史回看 / auto 模式能看到具体内容

三件事**互不依赖**，可独立实施；优化 2 与优化 3 是互补关系（类型 vs 具体实体）。

## 优化 1：i18n 占位符修复

### 涉及文件
- `frontend/src/i18n/locales/zh-CN.json`（行 155-161）
- `frontend/src/i18n/locales/en.json`（行 159-165）

### zh-CN.json 目标值

```json
"confirmDeleteCharacterRelation": "确认删除 角色关系「{{source}}」→「{{target}}」（{{relation}}）？",
"confirmDeleteLocationRelation": "确认删除 地点关系「{{locationA}}」↔「{{locationB}}」（{{relation}}）？",
"confirmDeleteArcNode": "确认删除 弧节点「{{title}}」（{{storyArc}}）？",
"confirmDeleteReaderEntry": "确认删除 读者视角条目 #{{id}}（{{entryType}}，第{{plantedChapter}}章）？",
"confirmDeletePreference": "确认删除 偏好项 [{{category}}]（#{{id}}）？",
"confirmDeleteTimelineEntry": "确认删除 时间线条目「{{title}}」？",
"confirmDeleteGeneric": "确认删除 {{label}}「{{title}}」？",
```

### en.json 目标值

```json
"confirmDeleteCharacterRelation": "Confirm delete character relation \"{{source}}\" → \"{{target}}\" ({{relation}})?",
"confirmDeleteLocationRelation": "Confirm delete location relation \"{{locationA}}\" ↔ \"{{locationB}}\" ({{relation}})?",
"confirmDeleteArcNode": "Confirm delete arc node \"{{title}}\" ({{storyArc}})?",
"confirmDeleteReaderEntry": "Confirm delete reader entry #{{id}} ({{entryType}}, ch.{{plantedChapter}})?",
"confirmDeletePreference": "Confirm delete preference [{{category}}] (#{{id}})?",
"confirmDeleteTimelineEntry": "Confirm delete timeline entry \"{{title}}\"?",
"confirmDeleteGeneric": "Confirm delete {{label}} \"{{title}}\"?",
```

### 规则约束（来自 project_memory）

- 这 7 个 key 无 pluralization 需求，按普通 key 处理（不拆 `_one/_other`）
- zh-CN 不使用 `_one/_other` 后缀
- en 即使要加 pluralization 也只有 `_one/_other` 一对，这里不需要

### 效果
审批弹窗恢复为 `确认删除 角色关系「林晚」→「苏白」（夫妻）？` 等完整内容。

## 优化 2：displayText 按 table 细化

### 涉及文件
- `internal/agent/display.go`（在 `buildDisplay` 函数 `:111-187` 内新增分支）

### 实现思路

参考 `chapterTools` 的"基于 args 生成个性化文本"模式（`display.go:137-174`），
但**不查 DB**——因为 `args.table` 已经足够区分类型，且 `buildDisplay` 阶段拿不到 name（name 在 `result.Data.deleted` 里）。

在 `buildDisplay` 中 `chapterTools` 分支之前/之后新增 `delete_record` 分支：

```go
if name == "delete_record" {
    if table, ok := args["table"].(string); ok {
        switch table {
        case "character":
            baseText = "删除角色"
        case "character_relation":
            baseText = "删除角色关系"
        case "location":
            baseText = "删除地点"
        case "location_relation":
            baseText = "删除地点关系"
        case "timeline_entry":
            baseText = "删除时间线条目"
        case "story_arc":
            baseText = "删除故事弧"
        case "arc_node":
            baseText = "删除弧节点"
        case "reader_perspective_entry":
            baseText = "删除读者视角条目"
        case "preference":
            baseText = "删除偏好项"
        }
    }
}
```

### 复用现有逻辑

- `:177-180` 的 "正在" 前缀逻辑（PhaseExecuting / PhaseSelected 自动加前缀）继续生效
- `:48` 的 `toolDisplayNames["delete_record"] = "删除记录"` 保留作为兜底
  （当 `args.table` 缺失或值未知时仍能显示）

### 效果对比

| 阶段 | 修复前 | 修复后 |
|---|---|---|
| executing | 正在删除记录 | 正在删除角色 / 正在删除地点关系 ... |
| completed | 删除记录 | 删除角色 / 删除地点关系 ... |
| failed | 删除记录 | 删除角色 / 删除地点关系 ... |

## 优化 3：完成态显示具体内容（方案 A）

### 涉及文件
- 后端：`internal/agent/agent.go:302-309`
- 后端：`internal/agent/display.go:243-245`
- 前端：`frontend/src/components/chat/ToolCallCard.tsx:164-188`
- 前端类型：`frontend/src/lib/wailsjs/go/models.ts` 相关 toolDisplays 类型（如需）

### 后端改动 1：agent.go 合并白名单

把硬编码的 `(name == "web_search" || name == "web_fetch")` 改成集合判断：

```go
resultDataMergeTools := map[string]bool{
    "web_search":    true,
    "web_fetch":      true,
    "delete_record": true,
}
if resultDataMergeTools[name] && result.Success && result.Data != nil {
    if metadata == nil {
        metadata = make(map[string]any)
    }
    for k, v := range result.Data {
        metadata[k] = v
    }
}
```

### 后端改动 2：display.go toolDisplays result 字段

同样把 `:243-245` 的硬编码改为集合判断，把 `delete_record` 加进去：

```go
resultFieldTools := map[string]bool{
    "web_search":    true,
    "web_fetch":      true,
    "delete_record": true,
}
if resultFieldTools[to.name] && to.result != nil && to.result.Success && to.result.Data != nil {
    entry["result"] = to.result.Data
}
```

### 前端改动：完成态新增 delete_record 渲染分支

`ToolCallCard.tsx:164-188` 的统一分支里，在 `tool-label` span 渲染完后，
新增条件分支：当 `toolName === 'delete_record' && entry.result?.deleted` 时，
复用 `ApprovalBody` 的同款渲染逻辑（建议抽成共享组件 `<DeletedEntityBody deleted={...} />`）。

效果示例：
- 已删除 角色「林晚」
- 已删除 角色关系「林晚」→「苏白」（夫妻）
- 已删除 地点「书房」
- 已删除 时间线条目「林晚收到匿名信」

### 数据契约确认

`delete_record` 成功后返回 `ToolResult.Data = { "deleted": meta }`，`meta` 字段：

| table | meta 字段 |
|---|---|
| character | `id`, `name`, `type="character"` |
| character_relation | `id`, `source`, `target`, `relation`, `type="character_relation"` |
| location | `id`, `name`, `type="location"` |
| location_relation | `id`, `location_a`, `location_b`, `relation`, `type="location_relation"` |
| timeline_entry | `id`, `title`, `type="timeline_entry"` |
| story_arc | `id`, `name`, `type="story_arc"` |
| arc_node | `id`, `title`, `story_arc`, `type="arc_node"` |
| reader_perspective_entry | `id`, `entry_type`, `planted_chapter`, `type="reader_perspective_entry"` |
| preference | `id`, `category`, `type="preference"` |

前端可以复用 `ApprovalBody` 中已有的 `typeLabels` 映射和 i18n key（可考虑新增
`chat.deletedCharacterRelation` 等完成态专用 key，或直接复用 `confirmDelete*`
去掉"确认"前缀的语义）。

## 实施顺序与提交策略

按用户要求分两阶段：

### 阶段 1（commit-1）
1. 写本文档（`docs/feat/v1.2.0/delete-record-display.md`）
2. 实施优化 1：改 `zh-CN.json` + `en.json` 7 个 key
3. `npm run build` + `eslint` + `go build` 验证通过
4. commit message: `fix(i18n): restore confirmDelete placeholders lost in chat i18n migration`

### 阶段 2（commit-2，等用户 review 后再决定）
1. 实施优化 2：改 `internal/agent/display.go` 新增 `delete_record` table 分支
2. 实施优化 3：
   - 改 `internal/agent/agent.go:302-309` 白名单
   - 改 `internal/agent/display.go:243-245` 白名单
   - 改 `frontend/src/components/chat/ToolCallCard.tsx:164-188` 完成态分支
3. `go build` + `go test` + `npm run build` + `eslint` 验证通过
4. commit message: `feat(display): show delete_record entity details in completed state`

## 验证要点

### 优化 1 验证
- 审批弹窗对 7 种删除类型都显示完整占位符内容（不出现 `{source}` 字面量）
- en 切换后显示英文版完整占位符
- `confirmDeleteGeneric` 的 `{{title}}` 占位符生效（修复前只渲染 `{{label}}`）

### 优化 2 验证
- LLM 触发删除角色时，executing 显示"正在删除角色"，completed 显示"删除角色"
- 9 种 table 值都能映射到正确的中文名
- `args.table` 缺失时兜底为"删除记录"（不崩）

### 优化 3 验证
- auto 模式（跳过审批）完成删除后，ToolCallCard 完成态显示"已删除 角色 林晚"
- 手动审批通过后，完成态同样显示"已删除 ..."
- 回看历史消息时，已完成的 delete_record 仍能看到具体内容
- 9 种 table 类型对应的字段都能正确渲染（不出现 `undefined`）

## 风险与边界

### 优化 1
- 风险极低，纯文案改动
- 不破坏 i18next compatibilityJSON v4 兼容性

### 优化 2
- 风险低，新加分支不影响其他工具的 displayText 生成
- 兜底保留"删除记录"，`args.table` 未知值时不会显示空

### 优化 3
- 后端白名单扩展属于"加法"，不影响 web_search/web_fetch 原有行为
- 前端完成态新增分支属于"加法"，不影响其他工具的完成态渲染
- 需注意 `result.deleted` 字段命名一致性（后端 `delete_tools.go` 返回的就是 `deleted` key）
- 前端 TS 类型可能需要扩展 `ToolDisplay` 接口的 `result` 字段类型

## 涉及文件清单

- `/home/nianhe/projects/todo/frontend/src/i18n/locales/zh-CN.json`
- `/home/nianhe/projects/todo/frontend/src/i18n/locales/en.json`
- `/home/nianhe/projects/todo/frontend/src/components/chat/ToolCallCard.tsx`
- `/home/nianhe/projects/todo/internal/agent/agent.go`
- `/home/nianhe/projects/todo/internal/agent/display.go`
- `/home/nianhe/projects/todo/internal/mcp_tools/delete_tools.go`（只读参考）
