# 记忆与设定系统设计

> **v2 暂定版本**（2026-07-26）
> 相比 v1 的核心架构改动：按"归属层级"重新划分 system 消息容器，引入 `UserProfile` 概念，取代原 `NovelProfile`。
> - v1: `NovelProfile = 设定 + 偏好`（按稳定性划分），`NovelState = 元数据 + goink.md`
> - v2: `UserProfile = 偏好 + memory`（按归属划分，用户级），`NovelState = 元数据 + goink.md + 设定`（小说级）
>
> memory 取消 `workflow` 类型（稳定工作流偏好归 preference），memory 模型 `NovelID` 改为必填（取消全局 memory 概念，记忆是小说级项目记忆）。

## 背景

当前 Goink 的 agent 在跨 session 协作上存在明显短板：新 session 完全不继承旧 session 的任何内容（[chat.go loadOrCreateSession](file:///home/nianhe/projects/goink/app/chat.go) 不读旧 session）。用户每次开新对话，agent 都"失忆"。同时用户提出的"世界观/设定系统"在当前代码里不存在，相关内容散落在 `novel.description`、`goink.md`、`preference_items`、`locations` 各处。

本设计引入两个独立但互补的机制：
- **长期记忆系统**：跨 session 的协作记忆（agent 与用户之间发生的事情）
- **设定系统**：小说虚构世界内部的、相对稳定的事实（世界观、力量体系、角色固定属性）

同时将现有的**偏好系统**改造为与设定统一的 upsert 模式。

## 现状分析

| 机制 | 现状 | 持久化 | 注入方式 |
|---|---|---|---|
| 偏好 | 已有，3 个工具（get/create/update）+ 通用 delete_record | `preference_items` 表 | 工具按需 `get_preferences`，不进 system prompt |
| goink.md | 已有 | git 文件 `novels/{id}/goink.md` | 每轮自动注入 NovelState + agent edit 维护 |
| 压缩 summary | 已有 | `messages` 表（role=user, system-reminder 包裹） | session 内，不跨 session |
| 设定系统 | **已落地**（v1.3.0 第一阶段，[internal/setting](file:///home/nianhe/projects/goink/internal/setting)） | `setting_items` 表 | NovelProfile 注入（v1 实现，v2 待迁移到 NovelState） |
| 长期记忆 | **不存在** | — | — |
| `Session.Summary` 字段 | 已定义未启用 | [types.go:18](file:///home/nianhe/projects/goink/internal/session/types.go) | — |

关键事实：
- 上下文窗口 200k，压缩阈值 80%（160k），**大部分 session 聊不到上限就结束**——压缩触发不是常态。
- NovelState 当前只读 novel 表 + goink.md（[novel_state.go](file:///home/nianhe/projects/goink/internal/agentcfg/novel_state.go)），偏好/角色/时间线等都不自动注入。
- 压缩时 NovelState 自动重建（[compress.go:92](file:///home/nianhe/projects/goink/internal/agent/compress.go) 调同一函数），扩展 NovelState 后设定/记忆在压缩时也会自动重建，无需额外处理。
- 通用删除工具 `delete_record`（[delete_tools.go](file:///home/nianhe/projects/goink/internal/mcp_tools/delete_tools.go)）9 张表共用，参数 `table` enum + `id`，走审批 + 硬删除。

## 概念边界

### 五个机制各司其职（认知科学对应）

业界 LLM agent 普遍按认知科学（Tulving 1972, *Episodic and Semantic Memory*；后续 ENGRAM arxiv:2511.12960；Claude Code Memory 等）把 agent 记忆分 3-4 类。Goink 的 5 个机制与之**完全对应**：

| goink 机制 | 认知科学对应 | 性质 | 时效 | 谁写 | 管什么 | 例子 |
|---|---|---|---|---|---|---|
| 偏好 Preference | **Procedural Memory**（程序性） | 创作规则（meta-fiction） | 长期显式 | 用户/agent | "怎么写" | 用短句、不写血腥 |
| 设定 Setting | **Semantic Memory**（语义） | 小说世界事实（in-fiction） | 长期显式 | 用户/agent | "世界是什么" | 修仙等级、主角武器 |
| 长期记忆 Memory | **Episodic Memory**（情景） | agent 与用户的协作过程 | 长期隐式 | 后端提取 / agent 主动 | "发生过什么" | 协作事件、纠正、反馈 |
| goink.md | **Working Memory**（工作） | 小说状态快照 | 中期动态 | agent 维护 | "现在到哪了" | 进展、悬念 |
| 压缩 summary | （session 内压缩） | 工作断点 | 短期 | 后端压缩 | "当前在干什么" | 断点、待办 |

**四者正交，不重复**。记忆按认知科学严格分类，边界清晰。

### 关键边界

- **偏好 vs 设定**：偏好是创作层面规则（procedural），设定是小说世界内事实（semantic）。分开。
- **设定 vs goink.md**：设定是静态底层规则，goink.md 是动态状态快照。不冲突。
- **记忆 vs 偏好/设定**：记忆记录的是 **agent 和用户之间发生的协作事件**（方案选择、反馈、纠正），**不是小说内容或创作规则**。例如"用户让 agent 起名，agent 提了 3 个方案，用户选了第二个"是记忆；"主角的剑名为断水"是设定；"用短句"是偏好。
- **长期记忆 vs goink.md**：长期记忆不复制 goink.md 的"小说进展"内容，只记协作事实。
- **长期记忆 vs 压缩 summary**：压缩 summary 是 session 内工作内存，长期记忆是跨 session 归档。分开存储、分开注入。

### memory 类型边界（v2 调整）

memory 取消 `workflow` 类型。原 v1 把工作流偏好（"先看大纲再写"）归 memory 是不合理的——稳定的工作流偏好本质是 procedural memory，应该归 preference。未稳定的工作流倾向则不记录（避免噪音）。

v2 memory 类型 4 类：

| 类型 | 含义 | 例子 |
|---|---|---|
| `collaboration_event` | 协作事件 | 用户选了某方案、否定了某方向 |
| `user_state` | 用户当前状态 | "今天写少点"、"最近累" |
| `correction` | 纠正记录 | agent 犯错被用户纠正 |
| `feedback` | 反馈 | 用户对某次协作满意/不满意的原因 |

---

## 架构总览（v2）

按"归属层级"划分 system 消息容器，引入 `UserProfile` 取代 `NovelProfile`：

```
UserProfile = 偏好（procedural，含全局 + 小说级） + memory（episodic，按 novelID 过滤）
             ↑ 用户级容器，类比 trae 的 user_profile.md

NovelState  = 元数据 + goink.md（working） + 设定（semantic）
             ↑ 小说级容器，类比 trae 的 project_memory.md
```

### 注入顺序

```
[AgentIdentity]           ← system1 提示词
[AlwaysSkills]            ← 常驻技能
[SkillCatalog]            ← 技能目录
[UserProfile]             ← system（用户级，按 novelID 过滤）
  ├─ 偏好（全部常驻，带 id，含全局 + 小说专属，软上限 4k）
  └─ 长期记忆 top-k（按 novelID 过滤，软上限 3k）
[NovelState]              ← system（小说级，每轮注入 + 压缩时重建）
  ├─ 小说基础信息（已有）
  ├─ 设定（全部常驻，带 id，软上限 8k）
  └─ 故事状态文档（goink.md，已有）
[历史 messages]           ← 含压缩 summary
[本轮 user]
```

**只有两条业务 system 消息**：UserProfile + NovelState。不拆分内部子消息（接受 KV cache 效率下降换取结构简单）。

### KV cache 影响（接受）

- UserProfile 跨轮稳定（偏好稳定 + memory 中频更新）→ cache 命中率较高
- NovelState 含 goink.md（高频更新）+ 设定（稳定）→ 整条 cache 命中率较低
- 接受 NovelState cache 效率下降换取结构简单（用户明确选择"不拆消息"）

### 归属逻辑

| 数据 | 归属 | 理由 |
|---|---|---|
| 全局偏好 | UserProfile | 用户级创作习惯，跨小说复用 |
| 小说级偏好 | UserProfile | 本质是用户偏好，只是声明"这本书适用"（按 novelID 过滤注入） |
| 设定 | NovelState | 小说级 in-fiction 事实，跨小说不复用 |
| memory | UserProfile | 协作记忆，按 novelID 过滤注入（数据本身小说级，容器用户级） |
| goink.md | NovelState | 小说级动态状态 |
| 元数据 | NovelState | 小说基础信息 |

**说明**：memory 数据本身是小说级（绑定 NovelID），但放在 UserProfile 容器里。注入时按 novelID 过滤（全局 memory 取消 + 当前小说 memory），类比 trae 的 user_profile.md 跨项目但写入时按项目隔离。

---

## 长期记忆系统（第二阶段）

### 数据模型

新增 `agent_memory` 表：

```go
type AgentMemory struct {
    ID              int64
    Content         string    // 原子记忆条目，1-2 句
    Importance      int       // 1-5，agent 自评或后端提取时 LLM 评定
    Type            string    // collaboration_event / user_state / correction / feedback
    NovelID         int64     // 必填，小说级项目记忆（v2 取消全局概念）
    SourceSessionID int64     // 来源 session
    CreatedAt       time.Time
    LastAccessed    time.Time // 注入时更新，用于淘汰
    Version         int       // 合并/更新时递增
}
```

v2 相比 v1 的改动：
- `NovelID` 从 `*int64`（可空，nil = 全局）改为 `int64`（必填）——取消全局 memory 概念
- `Type` enum 去掉 `workflow`——稳定工作流偏好归 preference

记忆类型说明（4 类）：
- `collaboration_event`：协作事件（用户选了某方案、否定了某方向）
- `user_state`：用户状态（"今天写少点""最近喜欢快节奏"）
- `correction`：纠正记录（agent 犯错被用户纠正）
- `feedback`：反馈（用户对某次协作满意/不满意的原因）

注册到 [migrate.go](file:///home/nianhe/projects/goink/internal/migrate/migrate.go) AutoMigrate 列表。

### 写入机制 A：被动（推荐为主）

**核心原则：基于全量 messages 提取，不删 tool、不基于 summary**。

原因：
- 删 tool 会丢失"agent 调了什么工具"的协作上下文（工具行为是协作事件的一部分）
- 基于 summary 提取会二次失真（summary 已是有损压缩）
- 业界"删 tool"策略（Claude Code 占位符替换、Microsoft 保留最近 N 组、LangChain offload 到文件）都是用于**压缩上下文**，不是提取记忆

**入口 1：压缩时顺便提取（零额外成本）**

修改 [compress.go](file:///home/nianhe/projects/goink/internal/agent/compress.go) 的 `compressionPrompt`，让 LLM 在输出 5 段 summary 的同时，额外输出一个 JSON 块：

```
<memories>
[
  {"content": "用户选择第二个起名方案，否定了前两个", "importance": 4, "type": "collaboration_event"},
  {"content": "用户反馈上次对话太长，希望精简", "importance": 3, "type": "feedback"}
]
</memories>
```

后端解析后写入 `agent_memory` 表。input = 全量 messages（含 tool），记忆基于全量，不失真。复用压缩的 LLM 调用，不额外加载上下文，KV cache 自然复用（同一 conversation 内）。

**入口 2：session 结束时提取（best effort）**

- 触发：session 超时（N 分钟无活动）或开新 session 时检查上一个 session 的 `memory_extracted` 标记，未提取则补提取
- KV cache：**无法复用**（跨 conversation，服务商 cache 不跨 conversation 共享）——接受这个现实，best effort
- input = **全量 messages（含 tool）**，不删 tool、不基于 summary。让 LLM 从全量上下文理解协作事件
- 提取请求复用原 session 的 system prompt（前缀匹配，最大化 cache 命中机会，虽然跨 conversation 大概率失效）
- token 成本：一次独立调用，200k 窗口下未触发压缩的 session 全量能装下

**`sessions` 表加字段**：

```go
type Session struct {
    // ... 现有字段
    MemoryExtracted bool      // 是否已提取长期记忆
    LastActivity    time.Time // 最后活动时间，用于超时判定
}
```

**超时检测**：Wails 后端起 goroutine 定时扫描（或每次 user message 后重置定时器）。

### 写入机制 B：主动（备选 / 补充）

**工具**：`save_memory(content, importance, type)`

- agent 实时调用，写入 `agent_memory` 表
- agent 自评 importance（1-5）
- system prompt 严格约束调用时机（见下）

**system prompt 约束**（控制负担，避免滥用，关键：记忆是协作过程，不是小说内容）：

```
【长期记忆维护】
记忆记录的是你和用户之间的协作过程，不是小说内容或创作规则。
调用 save_memory 的时机：
- 用户对你的协作方式给出反馈（"你这样写更好""下次先问我要不要继续"）
- 发生值得记住的协作事件（用户选了某方案、否定了某方向）
- 用户表达当前状态（"今天写少点""最近累"）

不要记录：
- 小说设定 → 用 upsert_setting
- 创作偏好（含工作流偏好）→ 用 upsert_preference
- 小说进展 → 用 goink.md
- 当前 session 待办 → 压缩 summary 处理
```

### 两种机制对比

| | 被动 | 主动 |
|---|---|---|
| 时机 | 压缩时 / session 结束 | agent 实时 |
| agent 负担 | 零 | 一次工具调用 + 决策负担 |
| KV cache | 入口 1 复用，入口 2 无法复用 | 自然复用（对话内） |
| 效果 | 后端全局审视，可能提取 agent 没意识到的 | 依赖 agent 判断，可能漏记/错记 |
| 实时性 | 延迟（压缩/session 结束才提取） | 实时 |
| 复杂度 | 高（时机判定、超时 goroutine） | 低（一个工具） |

**推荐**：被动为主（入口 1 零成本必做）+ 主动为辅（应对用户明确要求记忆的场景）。两者不冲突，可同时实现。也可先做被动，主动作为 phase 2。

### 注入策略

新 session 创建时，从 `agent_memory` 表检索，注入 UserProfile：

- **当前小说的 memory**（v2 取消全局 memory 概念，只查 `novel_id = ?`）
- 排序：`importance DESC, last_accessed DESC`
- 截断：token 预算 **3k**（约 30-40 条）
- 注入位置：UserProfile 之内，作为独立块 `【长期记忆】`
- 注入时更新 `LastAccessed`

**不做向量检索**——长期记忆的真正使用场景是"agent 写作时自然带出之前的协作共识"，不是"用户问起才回忆"。向量检索是看起来美好的陷阱（只有"你还记得 xx 吗"才有用）。按重要度排序 + 截断即可。

**不做 search_memory 工具**——工具越多 agent 压力越大，低频工具会干扰 agent 决策。记忆的可达性 = top-k 注入。接受"top-k 外的记忆暂时无法触达"的限制。如果未来确实需要，再加。

### 淘汰机制

记忆库达到上限时（如 500 条），淘汰：
- `importance` 低的优先
- 同 important 下 `LastAccessed` 久未访问的优先
- 可配置上限，避免无限膨胀

### 去重

**先不做**，直接 append + 重要度/时间淘汰自然收敛。理由：
- 早期记忆条目不多，重复不严重
- 去重逻辑（向量初筛 + LLM 判断）复杂度高，且 LLM 判断本身有成本和不确定
- 先跑起来看效果，重复严重再加去重

---

## 设定系统（upsert 模式）— 第一阶段

### 数据模型

新增 `setting_items` 表，纯 value 条目（不用 KV 结构）：

```go
type SettingItem struct {
    ID         int64
    NovelID    int64     // 必填，小说专属（v2 取消全局设定概念，is_global 字段移除）
    Category   string    // 世界观 / 力量体系 / 角色 / 地理 / 历史 / 物品（自由文本）
    Content    string    // 自由文本，不强行结构化内部
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

注册到 [migrate.go](file:///home/nianhe/projects/goink/internal/migrate/migrate.go) AutoMigrate 列表。

**v2 改动**：取消 `IsGlobal` 字段。设定是 in-fiction 事实，本质就是小说级，跨小说复用不合理（参见[概念边界](#概念边界)）。

**为什么不引入 KV 结构**（如"武器：xxx、血脉：xxx"）：设定的本质是混合的——角色需要结构化字段（武器/血脉），世界观/力量体系需要长文描述，硬塞 KV 会破坏表达。LLM 时代自然语言描述对 agent 注入更自然（读起来连贯），LLM 能从 Content 里精准提取信息，不需要 KV 辅助检索。token 效率上纯 value 更紧凑（避免每条带 key 的开销）。如需视觉结构化，前端用 Markdown 渲染 Content 内的列表/表格即可（如"主角林凡\n- 武器：断水剑\n- 血脉：龙族"），无需后端引入 KV 字段。

**为什么纯 value 而非 KV（补充理由）**：
- id = DB 主键，唯一性保证（KV 的 key 冲突维护麻烦）
- category + content 灵活，能装异构设定（力量体系是结构化的、世界观是散文式的）
- 和偏好模式一致，便于统一工具

### 注入策略

**全部常驻**注入 NovelState（[novel_state.go](file:///home/nianhe/projects/goink/internal/agentcfg/novel_state.go) 扩展），**带 id**：

```
【设定】
[#1 | 世界观] 修仙世界，灵气复苏...
[#2 | 力量体系] 练气→筑基→金丹→元婴...
[#3 | 角色] 主角林凡，剑修，武器断水剑
```

带 id 的目的：agent 要更新某条设定时，从注入里拿到 id，调 upsert 时传 id。

**软上限 8k token**：设定总量超 8k token 时，前端提示用户精简/拆分，避免膨胀挤占上下文（200k 窗口下 8k 占比 4%，可接受）。

**压缩时自动重建**：NovelState 扩展后，[compress.go:92](file:///home/nianhe/projects/goink/internal/agent/compress.go) 压缩时调同一函数，设定会自动重新注入，和现有 system 消息重建一致，无需额外处理。

### 工具：upsert_setting（唯一写工具）

```
upsert_setting(id?, category, content)
- 传 id → 更新该 id 的 content（和 category）
- 不传 id → 创建新条目
```

- agent 看到要更新"力量体系"时，从注入里看到 `#2`，调 `upsert_setting(id=2, content="...")`
- 要新增"地理"设定时，调 `upsert_setting(category="地理", content="...")` 不传 id

**不给 get 工具**：NovelState 每轮注入（system 消息不会被压缩掉），agent 随时能看到全量设定。省一个工具。

**删除走通用 `delete_record`**：在 [delete_tools.go](file:///home/nianhe/projects/goink/internal/mcp_tools/delete_tools.go) 的 `table` enum 加 `setting`，新增 `deleteSetting` 方法。复用现有审批 + 硬删除机制。

### 与现有表的关系

- **locations 表**：管空间关系图谱。设定里的"地理"管文字描述。互补不冲突。
- **characters 表**：管关系图谱 + 基础属性。设定管"主角武器、血脉"这种固定事实。可互补。

不扩展现有表，独立 `setting_items` 表更灵活、不侵入。

---

## 偏好系统改造（upsert 模式）— 第一阶段

### 现状

[novel_tools.go](file:///home/nianhe/projects/goink/internal/mcp_tools/novel_tools.go) 里偏好有 3 个工具：`get_preferences` / `create_preference` / `update_preference`。删除走通用 `delete_record`（table=preference）。不进 system prompt，agent 主动调 `get_preferences` 才能读到。

### 改造为 upsert 模式

**注入**：开局全量注入 UserProfile（v2 改动：从 NovelState 移到 UserProfile），**带 id**，含全局 + 小说专属：

```
【偏好】
[#1 | 全局 | 对话风格] 用短句，避免长段落
[#2 | 全局 | 内容限制] 不写血腥
[#3 | 本书 | 叙事节奏] 快节奏，多对话
```

**工具**：只给 `upsert_preference`（替代 get/create/update 3 个）

```
upsert_preference(id?, category, content, is_global?)
- 传 id → 更新该 id 的 content（和 category、is_global）
- 不传 id → 创建新条目
```

**不给 get / create / update 工具**：全量注入已覆盖读，upsert 覆盖写。

**删除继续走 `delete_record`**（table=preference，已有，不变）。

**工具数量**：偏好原 3 个 → 1 个 upsert_preference。

### token 影响

偏好从"工具按需"变成"全量常驻"，增加常驻 token。设软上限 4k token。

**总常驻预算**：设定 8k + 偏好 4k + 长期记忆 3k = **15k**（占 200k 窗口 7.5%，合理）。

### 改造影响面

- 移除 `get_preferences` / `create_preference` / `update_preference` 工具（[novel_tools.go](file:///home/nianhe/projects/goink/internal/mcp_tools/novel_tools.go)）
- 新增 `upsert_preference` 工具
- **UserProfile 新建**（v2 改动）：注入偏好（带 id），取代 NovelProfile 的偏好注入职责
- 前端 API 适配（[app/preference.go](file:///home/nianhe/projects/goink/app/preference.go) 现有 CRUD 保留）
- system prompt 里【创作偏好维护】章节调整（引导 agent 用 upsert_preference），并迁移到 UserProfile 上下文
- 工具白名单更新（[identity.go](file:///home/nianhe/projects/goink/internal/agentcfg/identity.go)）：移除旧 preference 工具，加 upsert_preference

---

## 与现有系统的边界

| 数据 | 写入 | 读取 |
|---|---|---|
| 偏好 | 用户/agent via `upsert_preference` | **全量常驻** UserProfile（带 id，按 novelID 过滤全局+小说专属） |
| 设定 | 用户/agent via `upsert_setting` | **全量常驻** NovelState（带 id） |
| 长期记忆 | 被动提取 / `save_memory` | **top-k 常驻** UserProfile（按 novelID 过滤） |
| goink.md | agent via `edit` | 每轮自动注入 NovelState |
| 压缩 summary | 后端压缩 | session 内，不跨 session |

**v2 注入顺序**（UserProfile + NovelState 两条业务消息）：

```
[AgentIdentity]           ← system
[AlwaysSkills]            ← system
[SkillCatalog]            ← system
[UserProfile]             ← system（用户级，跨轮较稳定，按 novelID 过滤）
  ├─ 偏好（全部常驻，带 id，含全局+小说专属，软上限 4k）
  └─ 长期记忆 top-k（按 novelID 过滤，3k 预算）
[NovelState]              ← system（小说级，每轮注入 + 压缩时重建）
  ├─ 小说基础信息（已有）
  ├─ 设定（全部常驻，带 id，软上限 8k）
  └─ 故事状态文档（goink.md，已有）
[历史 messages]           ← 含压缩 summary
[本轮 user]
```

**压缩时重建**：[compress.go:92](file:///home/nianhe/projects/goink/internal/agent/compress.go) 调 `agentcfg.UserProfile(a.db, opts.NovelID)` + `agentcfg.NovelState(a.db, opts.NovelID)`，和每轮注入用同一函数。扩展后设定/偏好/记忆在压缩时自动重新注入，无需额外处理。

---

## 工具清单汇总

改造后的工具清单（统一 upsert 模式）：

| 工具 | 用途 | 阶段 | 备注 |
|---|---|---|---|
| `upsert_setting(id?, category, content)` | 设定写/更新 | 第一阶段 | 传 id 更新，不传创建 |
| `upsert_preference(id?, category, content, is_global?)` | 偏好写/更新 | 第一阶段 | 替代原 3 个工具（get/create/update） |
| `delete_record(table, id)` | 通用删除 | 已有 | table enum 加 `setting`；偏好删除已支持（table=preference） |
| `save_memory(content, importance, type)` | 主动写长期记忆 | 第二阶段 | phase 2 可选，type enum 取消 workflow |

移除的工具（第一阶段）：
- `get_preferences` / `create_preference` / `update_preference`（被 upsert_preference 替代）

不创建的工具：
- `get_settings` / `create_setting` / `update_setting` / `delete_setting`（直接用 upsert_setting + delete_record）
- `search_memory`（不做，避免低频工具干扰 agent）

---

## 迁移点

### 第一阶段（设定 + 偏好改造 + UserProfile 引入）

1. **新增表**：`setting_items`，注册到 [migrate.go](file:///home/nianhe/projects/goink/internal/migrate/migrate.go) ✅ 已落地
2. **新建 setting 模块**：`internal/setting/types.go`（模型）、`internal/setting/store.go`（CRUD）✅ 已落地
3. **新建 MCP 工具**：`internal/mcp_tools/setting_tools.go`（`upsert_setting`）✅ 已落地
4. **delete_record 扩展**：[delete_tools.go](file:///home/nianhe/projects/goink/internal/mcp_tools/delete_tools.go) 的 `table` enum 加 `setting` ✅ 已落地
5. **工具注册**：[registry.go](file:///home/nianhe/projects/goink/internal/mcp_tools/registry.go) 注册 `upsert_setting` ✅ 已落地
6. **偏好工具改造**：[novel_tools.go](file:///home/nianhe/projects/goink/internal/mcp_tools/novel_tools.go) 移除 `get_preferences` / `create_preference` / `update_preference`，新增 `upsert_preference` ✅ 已落地
7. **设定 is_global 砍掉**（v2 新增）：[setting/types.go](file:///home/nianhe/projects/goink/internal/setting/types.go) 删 `IsGlobal` 字段；[setting/store.go](file:///home/nianhe/projects/goink/internal/setting/store.go) 删 `ListGlobalSettings`，`ListSettings` 改为只查 `novel_id = ?`；[app/setting.go](file:///home/nianhe/projects/goink/app/setting.go) 删 is_global 联动和 `SettingResult.Global`；[setting_tools.go](file:///home/nianhe/projects/goink/internal/mcp_tools/setting_tools.go) 删 `IsGlobal` 参数；[novel_profile.go](file:///home/nianhe/projects/goink/internal/agentcfg/novel_profile.go) 加载设定改 `WHERE novel_id = ?`；[app/novel.go](file:///home/nianhe/projects/goink/app/novel.go) DeleteNovel 改为只按 novel_id 删 setting ⏳ 待做（本轮下一任务）
8. **新建 UserProfile**（v2 新增）：[internal/agentcfg/user_profile.go](file:///home/nianhe/projects/goink/internal/agentcfg/user_profile.go) 新建，注入偏好 + memory（memory 阶段二才接入）⏳ 待做
9. **NovelState 扩展设定**：[novel_state.go](file:///home/nianhe/projects/goink/internal/agentcfg/novel_state.go) 注入设定（带 id）⏳ 待做
10. **NovelProfile 退役**（v2 新增）：把设定注入从 NovelProfile 移到 NovelState，偏好注入从 NovelProfile 移到 UserProfile；NovelProfile 文件可保留为过渡，或直接删除 ⏳ 待做
11. **system prompt 调整**：[identity.go](file:///home/nianhe/projects/goink/internal/agentcfg/identity.go) 调整【创作偏好维护】章节引导用 upsert，迁移到 UserProfile 上下文；新增【设定维护】章节，留在 NovelState 上下文 ⏳ 待做
12. **工具白名单更新**：[identity.go](file:///home/nianhe/projects/goink/internal/agentcfg/identity.go) MainAgent 加 `upsert_setting` / `upsert_preference`，移除旧 preference 工具；Review/Memory Agent 相应调整 ⏳ 待做
13. **App 层适配**：[app/preference.go](file:///home/nianhe/projects/goink/app/preference.go) 保留前端 CRUD API；[app/setting.go](file:///home/nianhe/projects/goink/app/setting.go) 删 is_global 相关字段（呼应步骤 7） ⏳ 待做

### 第二阶段（长期记忆）

14. **新增表**：`agent_memory`，注册到 migrate.go
15. **sessions 表加字段**：`memory_extracted`、`last_activity`
16. **compressionPrompt 扩展**：[compress.go](file:///home/nianhe/projects/goink/internal/agent/compress.go) 增加 `<memories>` JSON 块输出 + 解析写入。input = 全量 messages（含 tool）
17. **UserProfile 扩展**：注入长期记忆 top-k（3k 预算，按 novelID 过滤）
18. **新建 memory 模块**：`internal/memory/store.go`
19. **MCP 工具**：[memory_tools.go](file:///home/nianhe/projects/goink/internal/mcp_tools/memory_tools.go) 加 `save_memory`（主动方案，可选，type enum 不含 workflow）
20. **system prompt 扩展**：[identity.go](file:///home/nianhe/projects/goink/internal/agentcfg/identity.go) 加【长期记忆维护】章节，归属 UserProfile 上下文
21. **超时 goroutine**：后端启动时起定时扫描，检测 session 超时触发提取（入口 2）

---

## 未决问题

1. **被动入口 2 的超时阈值**：10 分钟 / 30 分钟 / 用户主动结束？需实测服务商 KV cache TTL 后定。
2. **去重机制**：先不做（append + 重要度淘汰自然收敛），跑起来看重复严重程度再加。
3. **记忆库上限**：暂定 500 条，需观察实际产出速率调整。
4. **主动方案是否实现**：先做被动，主动（save_memory）作为 phase 2 备选。
5. **偏好改造的影响面**：前端、现有调用点需梳理。`get_preferences` 被移除后，依赖它的地方（如 Review/Memory Agent 白名单）需调整。
6. **软上限机制**：设定 8k / 偏好 4k / 记忆 3k 超预算时的处理——前端提示 vs 后端截断 vs 标记核心。需定。
7. **`Session.Summary` 字段**：已定义未启用。可考虑在被动入口 2 提取时顺便写入，作为 session 级 summary 持久化。
8. **memory 真的归属 UserProfile 还是 NovelState**（v2 暂定 UserProfile）：memory 数据本身是小说级（绑定 NovelID），但容器归属 UserProfile。后续实操中如果发现 memory 跟偏好混在一条消息里导致 cache 频繁失效严重，可考虑把 memory 拆到 NovelState。本版本按"UserProfile = memory + 全部偏好"暂定。
9. **NovelProfile 是否完全退役**（v2 新增）：v1 实现的 NovelProfile 可保留作为过渡，或直接删除由 UserProfile + NovelState 取代。本轮按"完全退役"规划。
