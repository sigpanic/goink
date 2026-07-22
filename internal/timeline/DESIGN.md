# Timeline 设计文档

## 概述

Timeline 是 AI 写作的剧情追踪系统，用于管理伏笔回收和用户创作意图。由两张表组成：`chapter_plans`（章节规划）和 `time_entries`（伏笔 + 用户指令）。对 AI 透明——`get_timeline` 一个工具返回全部，内部查两张表。

## 与 Python 版本的差异

### time_entries 表（Python 26 字段 → Go 13 字段）

**保留的优化：**

1. **category 约束枚举**（`foreshadowing` | `user_directive`）：系统级分类驱动不同查询和注入策略，此处需精确行为区分，不做自由文本。

2. **target_chapter 必填作为排序键，不作为过滤条件**：Python 用它做 `WHERE target_chapter <= current + 3` 过滤，不准确就丢数据。Go 只用于 `ORDER BY`，不准确不影响可见性。Review agent 每章后校准。

3. **混合注入替代距离过滤**：近期窗口全量 + 全局索引一行一条。Python 用 target_chapter ±3 做硬过滤配合 LIMIT 15，导致远期高重要度条目可能被近期低重要度挤出，永远不可见。

4. **无 evolved_from_id 指针链**：Python 因为枚举 `relationship_type` 导致一对角色需要多行并行记录，每行各自演化，才被迫引入自引用链表。Go 时间线从设计上不需要演化链——追加不可变，(source, target) 配对 + 时间戳隐式排序。

**砍掉的字段及理由：**

| 字段 | 理由 |
|------|------|
| `extra_metadata` | content 和 detail_json 已覆盖扩展需求 |
| `related_entry_ids` | Python 写入但从未被查询遍历 |
| `tags` | 自由标签可放入 detail_json |
| `arc_id` | Python 从未在任何查询或业务逻辑中使用，等 story_arc 实现后再加 |
| `sequence` | Python 从未被排序或过滤使用 |
| `version` | 编辑历史审计，非 AI 写作核心需求 |
| `last_editor` | source 字段已区分 ai/user |
| `original_ai_output` | Python 保存但从不回读 |
| `resolved_at` | 可通过 resolved_chapter 推导 |
| `time_horizon` | target_chapter 已表达时间远近，无需额外字段 |

### 表拆分（Python 1 张 → Go 2 张）

Python 将 chapter_plan、foreshadowing、user_directive、plot_node 四种 category 混入同一张 `timeline_entries` 表。Go 拆分为：

- **chapter_plans**：章节规划，每章必消费，AI 写到哪消费到哪，滚动更新
- **time_entries**：伏笔 + 用户意图，埋和收可能跨几百章，需要长期追踪

拆分理由：规划和伏笔的消费方式根本不同——规划是"每章确定性输入"，伏笔是"按需提醒"。混在一起导致注入策略不得不为四种 category 写四套查询逻辑（见 Python `get_context_for_generation` 的 5 步拼装）。

### category 精简（Python 4 值 → Go 2 值）

| Python | Go | 说明 |
|--------|-----|------|
| `foreshadowing` | `foreshadowing` | 保留 |
| `user_directive` | `user_directive` | 保留 |
| `chapter_plan` | → `chapter_plans` 表 | 独立成表 |
| `plot_node` | 移除 | StoryArc 承担"关键情节里程碑"职责 |

plot_node 在 Python 中仅有两条创建路径：AI 手动 MCP 工具调用，以及角色关系变更的副作用。无自动提取、无专属查询、不参与统计、Layer 3 不注入——语义模糊且系统冷遇。Go 由 StoryArc 承担其"跨章节里程碑"的设计意图。

### status 变化

Python 6 状态（`pending` / `active` / `completed` / `resolved` / `abandoned` / `deferred`）→ Go 4 状态（`pending` / `active` / `resolved` / `abandoned`）。

- `completed` 和 `resolved` 语义重叠——Python 内部甚至自动将非 foreshadowing 的 `resolved` 强制转为 `completed`（`service.update_entry` 第 158-163 行）
- `deferred` 从未在上下文查询或 MCP 工具中使用

## 核心设计决策

### 1. target_chapter 只排不滤

核心矛盾：`target_chapter` 是 LLM 对不确定未来的数字估算，天生不准。Python 拿它做过滤（`<= current + 3`），不准就丢数据。

Go 方案：只用于 `ORDER BY target_chapter ASC, importance DESC`，不作为 WHERE 条件。索引全量列出（100 条截断兜底），不准确不影响可见性。Review agent 每章后校准，准确性随时间自然提升。

### 2. 混合注入

第 N 章写作时系统自动拼入（非 MCP 调用，AI 无感知）：

**chapter_plans（固定 3 行，全量注入，不需要截断）：**
```
next → 全量注入（完整 content，1 行）
near → 全量注入（完整 content，1 行）
far  → 全量注入（完整 content，1 行）
```
表级保证 (novel_id, scope) 唯一，每个小说固定 3 行。AI 修改时 UPSERT 覆盖。

**time_entries：**
```
近期窗口全量（target_chapter ∈ [N-3, N+5]，未解决）：
  完整 title + content + detail_json，按 target_chapter ASC
  约 0-8 条

全部索引（所有未解决条目）：
  id | category | title | target_chapter | importance | source_chapter
  按 target_chapter ASC, importance DESC，100 条截断
  每行一条，全量加载约 500-800 token
```

**系统提示词强制要求：**

> 索引中如存在与当前章节相关的条目，必须在写作前调用 `get_timeline(id=X)` 获取完整信息，不得凭索引标题猜测内容。已标记为 resolved/abandoned 的条目无需再关注。

### 3. 为什么时间线不用自由文本

偏好系统和角色关系使用了自由文本（category/reaction_type），因为内容本质是 LLM 生产的自然语言，系统不需要理解语义。

但时间线的 category 和 status 驱动不同的查询策略和上下文格式化逻辑——系统必须区分"这个是伏笔"、"这个是用户指令"来执行不同的注入行为。这是系统级分类，约束枚举是正确的选择。

### 4. Review Agent

每章写完后自动触发（非用户手动），职责：

1. 故事超出预期 → 调整 target_chapter
2. 伏笔已被自然回收但未标记 → 标记 resolved
3. 与当前剧情冲突 → 上报主 agent 决定处理

## 排序规则

**chapter_plans**：固定 3 行，按 scope 顺序排列（next → near → far），注入时固定格式

**time_entries 索引段**：`target_chapter ASC, importance DESC`

## MCP 工具（3 个，和 Python 一致）

| 工具 | 功能 | 内部路由 |
|------|------|---------|
| `get_timeline` | 统一返回 chapter_plans + time_entries | 查 chapter_plans（3 行）+ time_entries（排序） |
| `add_timeline_entry` | 批量创建 1-6 条，事务写入 | chapter_plan? → UPSERT (novel_id, scope)；其他 → INSERT time_entries |
| `update_timeline_entry` | 更新单条 | chapter_plan? → UPSERT；time_entry? → 部分字段更新 |

## AI 写作完整流程

```
第 N 章写作前 — LLM自己获取plans 和 timeline的信息：
  ├─ chapter_plans（next + near + far，全量）
  └─ time_entries（近期窗口全量 + 全局索引）

第 N 章写作中 — AI 按需查询：
  └─ get_timeline(id=X) 获取索引中某个伏笔的完整信息

第 N 章写完后 — AI 主动维护状态：
  ├─ add_timeline_entry  埋新伏笔 / 更新章节计划
  └─ update_timeline_entry  回收伏笔 / 调整 target_chapter

第 N 章写完后 — Review Agent 自动触发：
  ├─ 校准 target_chapter
  ├─ 标记已自然回收的伏笔
  └─ 发现冲突上报主 agent
```

## 与其他模块的关系

| 模块 | 关系 |
|------|------|
| Novel | `novel_id` FK，级联删除 |
| Chapter | `source_chapter` / `resolved_chapter` 引用章节号 |
| Character | timeline可以记录角色名称（写入工具描述/系统提示词） 告诉llm如果这个节点需要用到xx角色，就把角色名写进去，后续可以搜出来角色 |
| StoryArc | 未来的弧线里程碑通过 TimelineEntry 追踪（预留 detail_json 中的 arc 引用） |
