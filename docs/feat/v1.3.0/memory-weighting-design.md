# 记忆权重与衰减方案

> **配套文档**：[memory-system-design.md](./memory-system-design.md)
>
> 本文档只讨论"记忆如何排序/衰减/复盘"，不重复 memory 的数据模型、写入机制、注入策略等整体设计。
> 落地时仍按 memory-system-design.md 的步骤 14-21 顺序，本方案是对其中"注入策略"和"淘汰机制"的细化。

## 背景与问题

[memory-system-design.md 注入策略](./memory-system-design.md#注入策略) 原方案：

> 排序：`importance DESC, last_accessed DESC`，截断 3k token。

**问题**：纯 importance 排序会让"高重要但久远无用"的记忆永远占据 top-k，"低重要但当前有用"的记忆没机会注入。importance 是创建时定的，不会反映"这条记忆现在还有没有用"。

## 核心思路：拆开两个维度

把"内在价值"和"当前有用性"拆成正交的两个字段，各司其职：

| 字段 | 含义 | 谁更新 | 变化频率 |
|---|---|---|---|
| `Importance`（1-5） | 内在价值（这条记忆本身多重要） | LLM 提取时评定，**之后固定不变** | 创建时定一次 |
| `LastAccessed` | 最近被判定为"有用"的时刻 | **复盘时 AI 判定有用则更新为 now** | 每次复盘可能更新 |

**关键**：`Importance` 不再随时间/使用变化，它是记忆的内在属性。`LastAccessed` 才反映"当前有用性"，由复盘驱动。

## 评分与排序

不用纯 `importance` 排序，改用复合分：

```
score = importance * recency_factor(last_accessed)
```

`recency_factor` 是时间衰减函数，`Δt = now - last_accessed`：

| 函数形式 | 公式 | 特点 |
|---|---|---|
| 指数衰减 | `exp(-Δt / τ)` | 平滑衰减，永不到 0，近期权重高 |
| 线性衰减 | `max(0, 1 - Δt / T)` | 有归零点 T，超过 T 的记忆 score=0 |
| 阶梯 | 0-3d=1.0, 3-7d=0.8, 7-14d=0.6, 14-30d=0.4, >30d=0.2 | 实现简单，边界突跳 |

**推荐指数衰减**：平滑、无突跳、符合艾宾浩斯遗忘曲线的直觉（用户提到"参考遗忘曲线仿生"）。τ 待实测，初定 7 天或 14 天。

**排序**：`score DESC`，截断 3k token。

### 效果对比

| 记忆类型 | importance | last_accessed | recency | score | 行为 |
|---|---|---|---|---|---|
| 高重要 + 最近有用 | 5 | 0d | 1.0 | 5.0 | 注入 ✅ |
| 高重要 + 久未激活 | 5 | 30d | 低 | 中 | 沉底 ✅ 解决痛点 |
| 低重要 + 最近有用 | 2 | 0d | 1.0 | 2.0 | 有机会注入 ✅ |
| 低重要 + 久未用 | 2 | 30d | 低 | 最低 | 自然淘汰 ✅ |

## 复盘机制（与提取合并，不单独做）

**关键决策**：复盘不单独做，跟记忆提取合并。两个时机，两条路径，提取方式不同：

| 时机 | 路径 | 提取方式 | cache 约束 |
|---|---|---|---|
| 压缩时（入口 1） | 路径 A | **response_format: json_object**（不动 tools） | 前缀必须完全不变，保 cache 命中 |
| session 结束（入口 2） | 路径 B | **toolcall**（清洗 messages + submit_memories 工具） | 独立调用，无 cache 可复用，可清洗 |

### 为什么两条路径提取方式不同

**核心约束：压缩调用的 KV cache 不能 miss。**

1M 上下文模型，session 聊到 500K tokens，压缩时若前缀变化（改 tools 顺序/内容/名字、改 system、改历史消息顺序）→ 整个 500K 前缀 cache miss → 这次压缩请求要把 500K 全部重算，价格直接翻倍。session 累积的所有 cache 红利在压缩这一次调用里清零。

**tools 参数是 cacheable 的稳定前缀**（业界调查确认：OpenAI / Anthropic / Google 均把 tools 算入 cache 前缀，顺序/内容/名字任何变化导致 miss）。因此：

- **路径 A（压缩时）**：绝对不能在 tools 里加 `submit_memories`——一动 tools 整个前缀 miss。只能用 **response_format: json_object**（独立于 tools 的生成参数，不影响 cache 前缀），在追加的 compressionPrompt 里要求 LLM 以 JSON 输出 summary + memory，tools 参数保持与 chat 调用完全一致。
- **路径 B（session 结束）**：是独立调用，无 cache 可复用，清洗 messages 后走 toolcall 反而更可靠（schema 约束，解析稳定）。

### 复盘流程（两路径共用）

1. session 开头注入 top-k 记忆（带 `id`，未来放入 NovelProfile system 段）
2. session 进行
3. 压缩 / session 结束时，提取 AI 收到：
   - 注入的 top-k 记忆（带 `id` + 当前 `importance`）
   - 对话历史（路径 A 全量；路径 B 清洗后）
4. 提取 AI 输出结构化结果：
   - **新记忆**：从 session 内容提取，每条带 `importance`
   - **注入记忆的 0/1 激活标记**：每条注入 id 在本 session 是否有用/相关
5. 后端：
   - 激活=1 的记忆：`LastAccessed = now`
   - 激活=0 的记忆：不动（让时间衰减函数自然拉低它的 score）

### 路径 A：response_format: json_object 模式（压缩时）

**前缀保命中原则**：
- tools 参数：与 chat 调用完全一致，**不动**（加/删/改顺序/改名字都会破坏 cache）
- response_format：设为 `{"type": "json_object"}`，独立于 tools 的生成参数，**不影响 cache 前缀**（cache 前缀 = system + messages + tools，response_format 是解码参数不在缓存范围）
- system 消息：**历史快照，不变，能命中**（压缩调用用 `opts.Messages`，是数据库查出的历史快照；`BuildSystemMessages` 重建发生在 summary 生成之后的 `persistCompression` 阶段，不在压缩调用时）
- 历史 messages：顺序内容不变，能命中
- compressionPrompt：作为 user 消息追加到末尾，只有这一段是未命中（不可避免）

**compressionPrompt 改造**：在现有压缩提示词基础上，增加 memory 输出要求，并要求 LLM 以 JSON 对象输出。schema 在 compressionPrompt 文本中描述（`json_object` 只保证 JSON 语法合法，不约束 schema；`json_schema` 能约束 schema 但国产模型/中转站兼容性差，故不用）。

LLM 一次输出一个 JSON 对象，含三件事：
1. `summary`：原有压缩摘要正文
2. `memories.reactivated`：id 列表（判定有用的注入记忆）
3. `memories.new`：新 memory 数组（每条含 content / importance / type）

输出格式：

```json
{
  "summary": "压缩摘要正文...",
  "memories": {
    "new": [
      {"content": "...", "importance": 4, "type": "collaboration_event"}
    ],
    "reactivated": [12, 45]
  }
}
```

后端解析：`json.Unmarshal` 整个输出，分别取 `summary` 和 `memories`。**降级容错**：若 LLM 未输出 `memories` 字段或解析失败，跳过 memory 提取，只取 `summary` 做压缩（不阻塞压缩主流程）。

**为什么用 response_format: json_object 而非纯文本 JSON 块**：
- `json_object` 强制 LLM 输出合法 JSON 语法，避免 `<memories>` 标签包裹方式下标签缺失/多余文本导致解析失败
- 独立于 tools 参数，不影响 cache 前缀命中
- 比 `json_schema` 兼容性好（部分国产模型/中转站不支持 `json_schema` 的 strict 模式）

**为什么不用 toolcall**：toolcall 要求在 tools 参数里定义 `submit_memories`，这会改变 tools 前缀，破坏 cache 命中（见上节"为什么两条路径提取方式不同"）。

### 路径 B：清洗 + toolcall 模式（session 结束）

**独立调用，无 cache 约束**。清洗 messages 后带 1 个 `submit_memories` 工具走 toolcall，schema 约束输出更可靠。

**messages 清洗规则**（策略1：保留配对，清空 args + 占位 tool 结果）：

业界调查结论（OpenAI / Anthropic / Google / DeepSeek / 智谱 / Kimi / 通义 / 百川 / 零一 / 阶跃 10 家）：所有支持 tool calling 的 provider 都强制要求 `assistant.tool_calls` 必须有对应 `role=tool` 消息，缺一个 400。因此清洗时**必须保留配对关系**，只裁剪内容。

| 消息类型 | 清洗规则 | 理由 |
|---|---|---|
| assistant 带 tool_calls | 保留 tool_calls 结构和 id/name，`arguments` 改 `"{}"`；`content` 保留原样（LLM 文本回复有记忆价值）；`reasoning_content`（DeepSeek）/ thinking block（Anthropic）原样保留 | 保留配对，协议合规 |
| role=tool 消息 | 保留 `tool_call_id` 和 `name`，`content` 改占位符 `"[tool result omitted]"` | 保留配对，省 token |
| system 消息 | **只保留 NovelProfile 这 1 条**（未来 memory 放入 NovelProfile，含注入的 top-k 记忆），删 Identity/Always/Catalog/State | 保留 memory 供 LLM 评估，其余 sys 与记忆提取无关 |
| user/assistant 纯文本 | 保留不动 | 对话内容是记忆提取的核心 |

**为什么不用策略2（彻底删配对）**：策略2 删除配对后可能出现"user + 连续 assistant 无 toolcall"，Gemini 严格交替协议可能 400；Anthropic signed thinking 删除 tool_calls 后签名断裂；DeepSeek reasoning_content 与 tool_calls 配对断裂。策略1 零风险。

**为什么保留 NovelProfile 这 1 条 sys**：旧 memory 通过 system 消息注入，若全删 sys，LLM 不知道注入过哪些旧 memory，无法输出 `reactivated` 评估。未来 memory 放入 NovelProfile 段，保留这一条即可让 LLM 看到旧 memory 做评估。

**submit_memories 工具 schema**（路径 B 用）：

```json
{
  "type": "function",
  "function": {
    "name": "submit_memories",
    "description": "提交本 session 提取的新记忆和注入记忆的激活标记",
    "parameters": {
      "type": "object",
      "properties": {
        "new": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "content": {"type": "string"},
              "importance": {"type": "integer", "minimum": 1, "maximum": 5},
              "type": {"type": "string"}
            },
            "required": ["content", "importance"]
          }
        },
        "reactivated": {
          "type": "array",
          "items": {"type": "integer"},
          "description": "被判定有用的注入记忆 id 列表"
        }
      },
      "required": ["new", "reactivated"]
    }
  }
}
```

**参考实现**：[internal/pattern/extract.go](file:///home/nianhe/projects/goink/internal/pattern/extract.go) 的 `callTool` —— skill 模块的套路提取已用此模式。通用化后调 `llm.CallTool`（见 [toolcall-refactor-design.md](./toolcall-refactor-design.md)）。

**输出示例**（LLM 调用 `submit_memories` 的 arguments）：

```json
{
  "new": [
    {"content": "用户选择第二个起名方案，否定了前两个", "importance": 4, "type": "collaboration_event"},
    {"content": "用户提到后续想写主角回乡的情节", "importance": 3, "type": "creative_intent"}
  ],
  "reactivated": [12, 45, 78]
}
```

**为什么 toolcall 比 JSON 文本块好（路径 B）**：
- LLM 对 toolcall 的结构化输出更可靠（schema 约束，字段缺失/类型错会被供应商拦截）
- 不需要正则/标签解析 `<memories>` 块，避免 LLM 输出格式偏差导致解析失败
- 与项目现有模式一致（pattern/extract.go），复用 `EventToolCallEnd` 处理逻辑
- thinking mode 兼容（不用 tool_choice，靠提示词引导）

**路径 A 为什么不能用 toolcall**：见上节"为什么两条路径提取方式不同"——tools 是 cache 前缀，加 `submit_memories` 会破坏 cache 命中，代价不可接受。

### 为什么是 0/1 判定而非让 AI 输出新权重

| 方案 | 优点 | 缺点 |
|---|---|---|
| **0/1 激活**（本方案） | 简单、稳定、好解析、AI 负担低 | 粒度粗，不能区分"很有用"和"略有 |
| AI 输出新 importance | 粒度细 | AI 评分不稳定，可能乱打分；importance 被反复改会失去"内在价值"语义 |

选 0/1：把"内在价值"（importance，稳定）和"当前有用性"（last_accessed，动态）完全解耦，AI 只判定后者，前者不动。

## 不做机械 -1 衰减

考虑过"每次引用 -1 importance"的方案，**否决**，原因：

- 反直觉：经常被用的记忆反而快速衰减成低重要，与目标相反
- "引用"定义模糊：注入算引用？agent 实际用了算引用？前者会让所有 top-k 快速衰减，后者需要 agent 主动反馈增加负担
- 用"复盘 0/1 激活 + 时间衰减函数"已能解决"高重要久远无用占据 top-k"的痛点，不需要机械衰减

## 字段调整对数据模型的影响

memory-system-design.md 的 `AgentMemory` 模型字段不变，只是语义细化：

```go
type AgentMemory struct {
    ID              int64
    Content         string
    Importance      int       // 1-5，创建时评定，之后不变（本方案明确）
    Type            string    // 弱化为可选元信息，不用于注入过滤
    NovelID         int64
    SourceSessionID int64
    CreatedAt       time.Time
    LastAccessed    time.Time // 语义：最近被复盘判定为"有用"的时刻（不再是"最近注入时刻"）
    Version         int
}
```

**LastAccessed 语义变化**：
- 原方案：注入时更新（每轮注入都刷新）→ 反映"最近被注入"
- 本方案：**只在复盘判定有用时更新** → 反映"最近被判定有用"

**注入时不再更新 LastAccessed**——注入 ≠ 有用。只有复盘 0/1 判定为 1 才更新。

## Type 字段弱化（呼应"分类不靠谱"担忧）

`Type` 字段（collaboration_event / user_state / correction / feedback / creative_intent）：

- **不用于注入过滤**：注入按 score 排序，不按 type 筛选
- **不强制 enum**：LLM 提取时不确定就不填，允许自由文本
- **仅作元信息**：供 agent 读注入时理解记忆性质，不影响排序

这样分类错了也不影响注入效果，降低 LLM 提取失败率。

## 落地步骤（嵌入 memory-system-design.md 第二阶段）

本方案不新增独立步骤，而是细化原有步骤：

| 原步骤 | 本方案细化 |
|---|---|
| 步骤 16（compressionPrompt 扩展） | **路径 A（压缩时）**：compressionPrompt 追加 memory 输出要求 + JSON 输出指令，设 `response_format: json_object`，不动 tools 保 cache；后端 `json.Unmarshal` 整个输出，取 `summary` + `memories`，降级容错（无 `memories` 字段则只做压缩）。**路径 B（session 结束）**：清洗 messages（策略1：清空 args + 占位 tool 结果 + 保留 NovelProfile sys），带 `submit_memories` 工具走 toolcall，调 `llm.CallTool`，从 `EventToolCallEnd.ArgumentsJSON` 解析 `new` + `reactivated` |
| 步骤 17（NovelProfile 注入） | 排序改 `score = importance * recency_factor(last_accessed) DESC`；注入时**不更新** LastAccessed；memory 注入放入 NovelProfile 段（路径 B 清洗时保留这 1 条 sys 供 LLM 评估旧 memory） |
| 步骤 18（memory 模块） | store 加 `ReactivateMemories(ids []int64)` 方法，批量更新 LastAccessed = now |
| 步骤 21（超时 goroutine） | session 结束补提取走路径 B：清洗 messages + toolcall，输出 `new` + `reactivated` |

## 未决问题

1. **时间常数 τ**：指数衰减的 τ 取 7 天 / 14 天 / 30 天？影响衰减速度，需实测。
2. **recency_factor 函数形式**：指数 / 线性 / 阶梯？推荐指数，但需实测对比。
3. **top-k 外的记忆怎么激活**：它们没被注入，没机会被复盘——可能需要定期全量重评，或接受"top-k 外的记忆自然沉底"（依靠重要性提升后自然回到 top-k）。
4. **复盘时机**：压缩时（路径 A）必做（文本 JSON 块，保 cache）；session 结束（路径 B）是否做复盘？独立调用无 cache 约束，走 toolcall。两条路径提取方式不同的原因见"为什么两条路径提取方式不同"节。
5. **creative_intent 类型**：是否正式加入 Type enum？呼应"隐式创意留档"扩展，记录用户表达但未落地的后续思路。
6. **初值**：新建记忆时 `LastAccessed = CreatedAt`，首次复盘才有机会更新。
