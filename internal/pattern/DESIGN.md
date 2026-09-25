# Pattern Extraction Design

## 问题起源

风格素材库（style_sample）解决了「模仿一段文字的写作风格」的问题，核心原则是「片段优于全书」——风格是局部信号（句长、用词、标点），三五段代表性段落足以分析。

但用户还有一个不同的需求：**从一整本书中提取叙事套路/模式**。比如导入一本末世文，提取出「囤货 → 降临 → 生存 → 破局 → 收尾」的阶段模板，后续写同类型书时直接套用。

风格提取和模式提取的本质区别：

| | 风格提取 | 模式提取 |
|---|---|---|
| 关注点 | 怎么写（句长、用词、节奏） | 写什么（情节结构、爽点分布、角色功能） |
| 信号范围 | 局部（三五段足够） | 全局（必须看全书结构） |
| 对压缩的态度 | 压缩是毁灭性的（丢失表面肌理） | 压缩是增益的（抽象掉细节才能看到骨架） |
| 输入 | 选中的素材片段 | 已导入的整本 Novel |
| 产物 | 仿写 Skill | 套路模板 Skill |

## 核心思路

**LLM 自主划分阶段边界，递归压缩至可处理量级**。不是后端按固定章节数硬切，而是让 LLM 输出结构化的 Chunk（name + start/end chapter），后端只做 token 预算管理和循环控制。

后端角色：拼 batch、拆 chunks、估 token、决定继续压还是进最终轮。
LLM 角色：判断语义边界、命名阶段、合并相邻阶段、最终提炼套路。

## 完整 Pipeline

### Token 预算函数

所有分批和阈值判断使用同一个函数，基于模型上下文窗口动态计算，不硬编码：

```go
func batchBudget(contextWindow int) int {
    if contextWindow >= 200_000 {
        return 100_000
    }
    return int(float64(contextWindow) * 0.7)
}
```

- 上下文 ≥ 200K（如 DeepSeek V4 的 1M）：预算 100K，保守充裕
- 小于 200K（如 32K/128K 模型）：取 70%，留 30% 给系统提示词和输出
- Step 2 分批发送和最终轮跳出阈值共用此函数

### Pre-step：章节标题分析

所有章节标题 → 一次便宜 LLM 调用 → 输出疑似阶段边界列表。

后端不可能靠标题相似度算准边界——「物资耗尽」和「丧尸围城」编辑距离大但同属一个阶段，「修炼突破」和「突破瓶颈」相似但可能分属两个阶段。只有 LLM 能从标题的递进关系中感知边界。

```
输入：第1章 平静的日常 / 第2章 异常信号 / ... / 第100章 新的开始
输出：
  - 疑似边界：第12章↔第13章（日常→末世前兆）
  - 疑似边界：第47章↔第48章（铺垫→全面爆发）
  - ...
```

产物是一组 `(start_chapter_id, end_chapter_id, hint)` 三元组，作为 Step 1 的参照。提示词同时展示阅读章节号与稳定 ID：章节号供 LLM 理解顺序，ID 供其返回可持久引用的端点。Step 1 的 LLM 优先参考这些边界，但不强制——实际边界仍由内容决定。

### Step 1（固定）：章节摘要生成

**输入**：每批按 `batchBudget(ctxWindow)` 预算动态切分章节，附带 Pre-step 的疑似边界标注

**增量策略——以 batch 内单章粒度混合输入**：

每批构建时，逐章查 `chapters.summary`，区分三种情况：

| 该章状态 | 输入 | 输出要求 |
|---|---|---|
| 无摘要 | 全文，标注 `[需提取]` | LLM 需生成摘要 |
| 已有摘要 | 摘要原文，标注 `[上下文]` | 仅作参考，不重新生成 |
| 整批全有摘要 | — | 整批跳过 |

```
输入示例：
第1章[上下文]：末世前三天，主角开始囤积物资...
第2章[上下文]：继续收购，发现异常信号...
第3章[需提取]：[全文]
第4章[上下文]：...
第5章[需提取]：[全文]

提示词：
  - 标注[上下文]的章节摘要仅作叙事参考，不需要重新输出
  - 标注[需提取]的章节需生成摘要
  - 通过调用工具 `output_chapter_summaries` 输出结果
```

后端构造 batch 时已知道缺失章节 ID，收到 JSON 后校验是否全部覆盖。已有摘要不动，缺失的入库。章节号仅在 prompt 中辅助 LLM 理解阅读顺序；摘要回传和入库均按稳定 ID 定位。比「全批重出」省输入 token（已摘要的只占 ~500 token 而非 ~5K 正文），比「跳单章」保上下文完整性。

**Prompt 要点**：
- 提取本章的关键事件（1-2句话）
- 主角的主动行为 vs 被动遭遇
- 情绪走向（从 X 到 Y）
- 新出场角色及其功能（盟友/敌人/工具人）
- 与前后章的逻辑关系

**启发式边界对齐分批**：

Step 1 分批时，不按 token 预算硬切，而是对齐 Pre-step 产出的疑似边界。后端用已有的 `llm.CountMessageTokens` 计算每章正文 token，累加到接近预算上限时：

```
batch_tokens = 0
for each chapter:
    batch_tokens += 本章 token
    if batch_tokens >= 预算:
        前瞻接下来 N 章（如 10 章），找最近的疑似边界标记
          ├─ 找到边界且加上那几章不超预算上限 → 延伸到边界处切
          └─ 未找到或会超上限 → 在当前章处切
```

预算可设为 ~100K，上限 ~130K。1M 上下文窗口下这点弹性完全可接受。代价为零——token 计算用的是已有的 `llm.CountMessageTokens`，边界信息来自 Pre-step，两者都是现成的。

这确保每批送给 LLM 的内容在语义上尽量完整，避免一刀切在转折点中间。

### Step 2（循环）：Chunk 递归压缩

**核心数据结构——Chunk**：

```json
{
  "name": "末世前物资囤积",
  "start_chapter_id": 42,
  "end_chapter_id": 84,
  "content": "主角发现末世即将来临，开始大量囤积物资。从最初的单人采购逐步发展为组织化运作，期间遭遇数次小规模冲突和资源争夺..."
}
```

- `name`：阶段名称，用于最终 Skill 的结构展示和人类阅读
- `start_chapter_id` / `end_chapter_id`：稳定章节定位，确定性字段，追溯链不可丢失；后端用本次加载的阅读顺序映射判断范围先后和排序
- `content`：该阶段的实质性摘要，递归轮 LLM 依此判断合并与否；最终轮依此生成套路模板

每轮流程：

**第 1 轮（始终执行）**：单章摘要 → 按 ~100K token 分批 → LLM 输出第一批 chunks。无论摘要总量是否已低于阈值，此轮不可跳过——LLM 需要专注划边界，再进入最终提炼。

**后续轮（按需）**：

分批采用**流式累加**方式，逐 chunk 累加 token，超预算即发，一次遍历完成：

```go
batch := []Chunk{}
batchTokens := 0

for _, ch := range chunks {
    t := llm.CountMessageTokens(ch.Name + ch.Content)
    if batchTokens+t > 100_000 && len(batch) > 0 {
        发送 batch → LLM → 收集新 chunks
        batch = []Chunk{ch}
        batchTokens = t
    } else {
        batch = append(batch, ch)
        batchTokens += t
    }
}
发送最后一批
```

每批发送后收集 LLM 产出的新 chunks。所有批次完成后，汇总所有新 chunks：

```
估算汇总 chunks 的总 token（同样的流式累加）
if 总量 ≤ 阈值（~80K token）
    → 跳出循环，进入 Step 3
else
    → 继续下一轮
```

**合并判断完全由 LLM 决定**：
- 相邻 chunk `[1-47: 物资囤积] ↔ [48-95: 末世前夕]` 语义衔接紧密 → 输出 `[1-95: 末世前铺垫]`
- 交接点有明确转折 → 保留原边界
- 一个 chunk 内部出现微妙转向 → 可以拆成两个

**第 1 轮输入是单章摘要而非 chunks**：Step 1 产出的是单章摘要列表。第 1 轮将它们按 ~100K 分批，每批 LLM 输出第一批 chunks（如 `[1-12: 物资囤积][13-38: 末世前夕]...[39-50: 降临初期]`），边界完全由 LLM 根据内容划，不按固定章节数硬切。

**递归终止条件**：所有 chunks 总 token ≤ ~80K。此时一次调用可以带上所有 chunks 进入最终轮。

**章节定位始终可追溯**：每个 chunk 的 start/end chapter 是确定性字段，无论压缩多少轮，最终弧段都能精确对应到原文章节范围。

### Step 3（最终轮）：套路 Skill 生成

**输入**：所有 chunks（总 token ≤ ~80K）
**输出**：完整套路 Skill（YAML frontmatter + markdown）

**Prompt 要点**：
- 给套路起一个贴切名称（如「末世生存流」「重生复仇流」）
- 基于 chunks 的章节范围标注，输出阶段分解（每个阶段的章节范围 + 特征描述）
- 爽点节奏（高潮分布、压制-释放模式）
- 角色功能模板（主角的功能、配角的类型分布、反派出场节奏）
- 抽象出可复用的叙事规律（不是描述这本书，而是通用模板）

**产物格式**：标准 Skill 文件，存入 `skills/` 目录。

**可选增强**：最终轮 LLM 在串 chunks 时如果发现信息缺失（如某段衔接不清晰），可主动调工具查对应章节的原文或摘要。V1 可不做。

## 走一遍完整案例

以 1500 章末世文为例：

**Pre-step**：1500 条标题 → 1 次调用 → 30 个疑似边界

**Step 1**：1500 章正文 → ~75 批（每批 20 章，~100K token）→ 1500 条摘要入库（每条 ~500 token）

**Step 2 第 1 轮**：1500 条摘要，总量 ~750K token，远超 80K 阈值
  → 分批：每批 ~100K token（约 200 条摘要），共 8 批
  → 每批 LLM 输出 ~30-50 个初始 chunks
  → 合并后约 300 个 chunks，总量 ~150K token

**Step 2 第 2 轮**：300 个 chunks，150K > 80K
  → 分批：每批 ~100K token，共 2 批
  → 每批 LLM 输出 ~15-20 个合并 chunks
  → 合并后约 35 个 chunks，总量 ~40K token（≤ 80K，终止）

**Step 3**：35 个 chunks（40K token）→ 1 次调用 → 套路 Skill

**API 调用统计**：
| 阶段 | 调用次数 | 总输入 token | 总输出 token |
|---|---|---|---|
| Pre-step | 1 | ~20K | ~2K |
| Step 1 | ~75 | ~7.5M | ~750K |
| Step 2 第 1 轮 | ~8 | ~800K | ~120K |
| Step 2 第 2 轮 | ~2 | ~200K | ~20K |
| Step 3 | 1 | ~45K | ~5K |
| **合计** | ~87 | **~8.6M** | **~897K** |

**成本**（DeepSeek V4 Flash 闲时）：输入 ¥8.57 + 输出 ¥1.79 ≈ **¥10**。峰时 ¥20。

## 结构化输出方案

### 为什么用 Tool Calling 而非 response_format

`response_format` 的 `json_schema` + `strict` 各家支持参差不齐：

| 供应商 | json_schema | json_object |
|---|---|---|
| DeepSeek | ❌ | ✅ |
| Kimi | ✅ (k2.5+) | — |
| MiniMax | ✅ | ❌ |
| Qwen | ✅ | ✅ |
| GLM | ✅ | ✅ |
| Mimo | ✅ | ✅ |

Tool Calling (function calling) 每家都完整支持，且 LLM 对 tool_arguments 的 JSON 输出可靠性高于 prompt 约束的 json_object。

### 复用现有基础设施

三步全部踩在已有代码上：

1. **Schema 生成**：定义 Go struct + jsonschema tag → `mcp_tools.SchemaOf()` → OpenAI tool definition。与现有 30+ MCP tool 同一条生成路径。

```go
type ChapterSummaries struct {
    Summaries []ChapterSummaryItem `json:"summaries"`
}
type ChapterSummaryItem struct {
    ChapterID int64  `json:"chapter_id" jsonschema:"required"`
    Summary   string `json:"summary" jsonschema:"required"`
}
// tools = []map[string]any{{"type": "function", "function": {..., "parameters": SchemaOf(ChapterSummaries{})}}}
```

2. **流解析**：`ChatStream` 已有 `EventToolCallStart/End` 事件，`tool_arguments` 现成可消费。只需在 `CallOptions` 加 `ToolChoice` 字段。

3. **校验**：`Registry` 用的 `go-playground/validator` 同样校验 LLM 输出——反序列化 → Validate → 字段完整性检查。

### 各 Step 的 Tool 定义

**Step 1**：`output_chapter_summaries` —— 强制调用，LLM 输出逐章摘要数组。

**Step 2**：`output_chunks` —— 强制调用，LLM 输出 chunk 数组（name + start/end + content）。

**Step 3**：无 tool —— 最终轮直接输出 markdown Skill，用普通流式文本即可。

## 与现有架构的关系

### 复用导入基础设施

EPUB/TXT/MD 导入 → 自动切章 → 创建 Novel + Chapter。导入完成后 Novel 已有完整章节列表，模式提取直接遍历。

### 复用 Skill 系统

模式提取的产物就是 Skill，无需新存储。`skills/{套路名}.md`，和仿写 Skill 一起出现在技能列表中。

### 复用 CancelManager

模式提取是长任务（可能几分钟），需要可取消。和聊天、风格提取共用 `CancelManager`，前缀 `pattern:`。

### 不引入新抽象层

- 输入是已有的 Novel（不建新实体）
- 输出是已有的 Skill（不建新实体）
- 章节摘要存入已有的 `chapters.summary`（不建新字段）
- Chunks 仅在内存中流转，不持久化（重跑从摘要开始即可）

## API 设计

### ExtractPattern

```
ExtractPattern(ExtractPatternInput) → ExtractPatternResult
```

**输入**：
- `novel_id`：源 Novel ID
- `provider_name`：LLM provider
- `model_id`：模型 ID
- `reasoning_effort`：推理等级（可选）
- `chapter_ids`：指定章节范围（可选，默认全书）

**输出**：
- `name`：套路名称
- `description`：一句话描述
- `raw_content`：完整 Skill 正文
- `file_path`：保存路径（`skills/{name}.md`）
- `trace`：本次提取的可视化轨迹（章节数、上下文窗口、边界、摘要、每轮 chunks、批次 token 估算）

**流程**：
1. Pre-step：加载所有章节标题 → 分析疑似边界
2. Step 1：分批压缩章节正文 → 逐章摘要入库
3. Step 2：循环压缩 chunks 直到 token ≤ 阈值
4. Step 3：所有 chunks → 套路 Skill
5. 每步检查 ctx.Done()，用户取消时优雅终止

### pattern:progress

```
EventsEmit("pattern:progress", ExtractPatternProgress)
```

模式提取是长任务，前端不应只显示一个 spinner。后端在关键节点推送事件：

| stage | 含义 | 典型可视化 |
|---|---|---|
| `loaded` | 已加载章节，计算上下文窗口和 batch 预算 | 总章节数、预算信息 |
| `boundaries` | Pre-step 边界分析完成 | 章节轴上的疑似阶段边界 |
| `summaries` | Step 1 摘要批次完成或复用缓存 | 批次进度、摘要列表 |
| `chunks` | Step 2 某轮某批 chunk 生成/压缩完成 | 轮次节点、阶段块图 |
| `finalizing` | Step 3 进入最终 Skill 生成 | 最终输入 chunks |
| `done` | Skill 生成并解析成功 | 完成态和保存入口 |

事件字段：
- `novel_id`：用于过滤当前作品事件
- `stage/message/percent/current/total`：基础进度
- `round/batch_index/batch_total/tokens`：批次和轮次信息
- `boundaries/summaries/chunks`：可视化所需的结构化中间产物

### CancelExtractPattern

```
CancelExtractPattern(novelID int64)
```

### GetExtractPatternProgress（可选，V2）

```
GetExtractPatternProgress(novelID int64) → ExtractPatternProgress
```

当前实现优先使用 `pattern:progress` 事件推送；如果未来需要跨窗口恢复、任务后台运行或历史任务回看，再补持久化进度和轮询 API。

## Frontend

### 入口

建议在 WorkspaceView 的 Novel 工具栏加「提取套路」按钮。模式提取的输入是一本书，天然属于 Novel 上下文。

### 交互流程

1. 用户点击「提取套路」→ 弹出配置面板（章节范围 + 模型选择 + 开始按钮）
2. 提取中 → 进度提示（Pre-step / Step 1 第X批 / Step 2 第X轮 / 最终提炼），按钮变红色取消
3. 完成后 → 预览生成的 Skill（复用现有 Skill 预览：frontmatter 表格 + markdown 渲染）
4. 用户确认 → 保存到 `skills/` 目录

## 边界情况

| 场景 | 处理 |
|---|---|
| Novel 无章节 | 提示「请先导入小说或创建章节」 |
| 章节数 < 5 | 提示「章节过少，模式提取需要足够的内容量」 |
| 某章内容为空 | 跳过空章，不参与压缩，摘要标注「无内容」 |
| 某章无标题 | 后端标注「第X章（无标题）」 |
| 某章已有摘要 | 跳过该章的 Step 1 压缩，复用已有摘要 |
| 用户取消 | ctx.Done() 检查，已完成的摘要不丢失（已入库） |
| LLM 输出非预期 Chunk 结构 | 重试该批 1 次，仍失败则报错 |
| 某批压缩失败 | 重试该批，最多 2 次；仍失败则跳过并在日志标注 |

## 不做的

1. **多书交叉提炼**：V1 只做单书。多书需要对齐不同书的结构，复杂度过高。
2. **持久化 Chunks**：Chunks 不存盘。摘要已入库，重跑从 Step 2 开始即可。
3. **Step 3 工具调用**：最终轮 LLM 暂不给工具查原文。V1 保持简单。
4. **自动识别套路类型**：不做分类模型，LLM 在提炼时自然会归类。
5. **对比模式**：不做两本书的套路差异对比。
