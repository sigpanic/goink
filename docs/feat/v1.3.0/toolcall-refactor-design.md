# ToolCall 提取层重构设计

> **调查稿**（2026-07-27）
> 目标：将 `pattern` 包私有的 `callTool` 通用化为 `llm` 包公开函数，供 `pattern` / `style` / `memory` 三个模块共用，统一"LLM → 结构化 JSON 输出"的提取范式。

## 背景与动机

Goink 里有三个模块都需要"让 LLM 输出结构化数据"：

| 模块 | 当前方式 | 输出形态 | 现状问题 |
|---|---|---|---|
| `pattern`（模式提取） | toolcall | `BoundaryHintsOutput` / `ChunksOutput` / `SkillOutput` 等 | `callTool` 是私有方法，绑死在 `Extractor` 上，复用不了 |
| `style`（风格提取） | 纯文本 markdown | `skill.Skill`（经 `skill.ParseBytes` 解析） | AI 输出不稳，frontmatter 格式漂移会导致解析失败 |
| `memory`（长期记忆提取，规划中） | 路径 B 用 toolcall，路径 A 用文本 JSON 块 | 路径 B：`submit_memories` 工具 schema（见 memory-weighting-design.md） | 路径 A 不纳入（保 cache）；路径 B 尚未实现，需要 toolcall 基础设施 |

`pattern.callTool` 已经把 toolcall 的流式接收、重试、状态回调、错误聚合都趟通了，逻辑成熟。问题是它以 `Extractor` 方法的形式存在，签名里带 `ExtractPatternInput`，`style` 和 `memory` 用不上。三个模块各自再写一遍 toolcall 流处理 = 重复代码 + 行为漂移风险。

本设计把 `callTool` 抽到 `llm` 包，做成通用函数，三个模块共用。

## 现状调查

### pattern.callTool 实现要点

位置：[internal/pattern/extract.go:506-566](file:///home/nianhe/projects/goink/internal/pattern/extract.go)

```go
func (e *Extractor) callTool(
    ctx context.Context,
    input ExtractPatternInput,   // ← 只用 ProviderName/ModelID/ReasoningEffort
    toolName string,
    schema any,                  // ← mcp_tools.SchemaOf(v) 的输入 struct
    messages []map[string]any,
    attempts int,                // ← 重试次数
    maxTokens int,               // ← 0 表示走 ModelInfo 默认
    onStatus func(LLMStatus),    // ← 状态回调，可 nil
) (json.RawMessage, error)
```

关键逻辑：
1. 用 `mcp_tools.SchemaOf(schema)` 生成工具 JSON Schema，组装单工具 `tools` 数组
2. `callOptions(input)` 从 `ExtractPatternInput.ReasoningEffort` 构造 `llm.CallOptions`
3. 循环 `attempts` 次，每次间隔 30s（`time.After`，ctx 取消则退出）
4. 调 `e.LLMClient.ChatStream`，遍历事件：
   - `EventError` → 记 `lastErr`
   - `EventThinking` → `onStatus(LLMThinking)`（只推一次，`sentThinking` 去重）
   - `EventToolCallStart` → `onStatus(LLMGenerating)`
   - `EventToolCallEnd` → 匹配 `toolName` 且 `ArgumentsJSON` 非空则直接返回
5. 流结束未拿到工具调用 → `lastErr` 兜底为 "LLM 未调用工具 xxx"
6. 全部重试失败 → `errors.Join(allErrs...)`

调用点 5 处（均在 [internal/pattern/extract.go](file:///home/nianhe/projects/goink/internal/pattern/extract.go)）：

| 调用点 | toolName | schema | attempts | maxTokens | onStatus |
|---|---|---|---|---|---|
| `preAnalyzeBoundaries` | `output_boundary_hints` | `BoundaryHintsOutput{}` | 1 | 0 | 有 |
| `ensureSummaries` | `output_chapter_summaries` | `ChapterSummariesOutput{}` | 2 | 0 | nil |
| `initialChunks` | `output_chunks` | `ChunksOutput{}` | 2 | 0 | nil |
| `compressChunks` | `output_chunks` | `ChunksOutput{}` | 2 | 0 | nil |
| `finalSkill` | `output_skill` | `SkillOutput{}` | 2 | 32000 | 有 |

### style.Extract 现状

位置：[internal/style/extract.go:14-60](file:///home/nianhe/projects/goink/internal/style/extract.go)

```go
events := llmClient.ChatStream(ctx, providerName, buildExtractMessages(combined.String()), nil, modelID, opts)
var fullText strings.Builder
for evt := range events {
    if evt.Type == llm.EventError { return nil, ... }
    if evt.Type == llm.EventContent { fullText.WriteString(evt.Data) }
}
raw := fullText.String()
sk, err := skill.ParseBytes([]byte(raw), "ai")
```

差异点：
- `tools` 传 `nil`（纯文本输出）
- 只收 `EventContent`，拼成完整 markdown
- 用 `skill.ParseBytes` 解析 frontmatter + 正文
- **无重试**、**无 onStatus**、**无 maxTokens 控制**
- 提示词里硬约束输出格式（`---\nname: ...`），但 LLM 偶发漏 frontmatter 或多包一层 ```markdown 围栏就会解析失败

### llm 包基础设施

- [ChatStream](file:///home/nianhe/projects/goink/internal/llm/stream.go)：`(ctx, providerName, messages, tools, model, opts) <-chan StreamEvent`，已是通用接口
- `CallOptions`：`Temperature` / `MaxTokens` / `ReasoningEffort` / `ThinkingEnabled` / `ToolChoice`
- `StreamEvent`：`EventToolCallEnd` 时 `Delta.ArgumentsJSON` 是 `json.RawMessage`，可直接反序列化
- [mcp_tools.SchemaOf](file:///home/nianhe/projects/goink/internal/mcp_tools/base.go)：`any → json.RawMessage`，用 `jsonschema` tag 生成，已通用

`llm` 包已具备所有底层能力，缺的只是一个"把流式 toolcall 收尾 + 重试 + 错误聚合"包起来的便捷函数。

### memory 提取的需求

按 [memory-weighting-design.md](file:///home/nianhe/projects/goink/docs/feat/v1.3.0/memory-weighting-design.md)，memory 提取分两条路径，**只有路径 B 纳入 toolcall 重构**：

| 路径 | 时机 | 提取方式 | 是否纳入 toolcall 重构 | 理由 |
|---|---|---|---|---|
| 路径 A | 压缩时 | 文本 JSON 块（`<memories>` 块） | **不纳入** | 压缩调用必须保 KV cache 命中，tools 是 cacheable 前缀，加 `submit_memories` 会破坏命中，代价不可接受（1M 模型 500K 压缩 miss 则价格翻倍） |
| 路径 B | session 结束 | 清洗 messages + `submit_memories` toolcall | **纳入** | 独立调用无 cache 约束，清洗后走 toolcall 更可靠（schema 约束） |

路径 B 的 `submit_memories` schema 含 `new`（数组）和 `reactivated`（数组）两个字段，清洗规则（策略1：保留配对，清空 args + 占位 tool 结果 + 保留 NovelProfile sys）见 memory-weighting-design.md。路径 B 调 `llm.CallTool`，是本重构的直接消费者。

路径 A 在 `agent` 包的压缩调用内自行解析文本 JSON 块，不经过 `llm.CallTool`。

### pattern / style 的 cache 说明

`pattern` 和 `style` 走 toolcall **无 cache 命中问题**：两者都是独立 LLM 调用（模式提取 / 风格提取），不复用任何前缀 cache，tools 参数可自由定义。只有路径 A（压缩时提取记忆）受 cache 约束不能用 toolcall。

## 通用化方案

### 新增：llm.CallTool

把 `pattern.callTool` 搬到 `llm` 包，去掉对 `ExtractPatternInput` 的依赖：

```go
// 在 internal/llm/ 新建 calltool.go
package llm

// CallToolStatus 描述一次 toolcall 调用过程中的状态，用于回调推送。
type CallToolStatus string

const (
    CallToolThinking   CallToolStatus = "thinking"   // 模型推理中
    CallToolGenerating CallToolStatus = "generating" // 模型输出工具参数中
)

// CallToolOptions 是 CallTool 的可选参数。
type CallToolOptions struct {
    ReasoningEffort string          // 空则不设置
    MaxTokens       int             // 0 表示走 ModelInfo 默认
    Attempts        int             // 重试次数，≤1 表示只试一次
    OnStatus        func(CallToolStatus) // 状态回调，可 nil
}

// CallTool 请求 LLM 调用指定工具并返回结构化 JSON。
//
// 部分供应商的 thinking mode 不兼容 tool_choice，因此只提供 tools，
// 由提示词约束模型调用目标工具。
//
// tools 数组由调用方组装（通常只含 1 个工具）；本函数负责流式接收、
// 重试、错误聚合。onStatus 在 LLM 流事件时被调用，nil 时跳过。
//
// 返回值是 EventToolCallEnd 的 ArgumentsJSON（json.RawMessage），
// 调用方按目标类型 json.Unmarshal。
func (c *Client) CallTool(
    ctx context.Context,
    providerName, modelID string,
    messages []map[string]any,
    tools []map[string]any,
    toolName string,         // 期望调用的工具名，用于匹配 EventToolCallEnd
    opts *CallToolOptions,
) (json.RawMessage, error)
```

实现内容（从 `pattern.callTool` 平移）：
1. 从 `opts` 构造 `CallOptions`（`ReasoningEffort` / `MaxTokens`）
2. 循环 `opts.Attempts` 次（≤1 视为 1），间隔 30s，ctx 取消则退出
3. 调 `c.ChatStream`，遍历事件：
   - `EventError` → 记 `lastErr`
   - `EventThinking` → `opts.OnStatus(CallToolThinking)`（去重一次）
   - `EventToolCallStart` → `opts.OnStatus(CallToolGenerating)`
   - `EventToolCallEnd` → 匹配 `toolName` 且 `ArgumentsJSON` 非空则返回
4. 流结束未拿到 → `lastErr` 兜底 "LLM 未调用工具 xxx"
5. 全部失败 → `errors.Join(allErrs...)`

**为什么 tools 数组由调用方组装**：`mcp_tools.SchemaOf` 在 `mcp_tools` 包，`llm` 包不应反向依赖 `mcp_tools`（依赖方向：`mcp_tools` → `llm`）。调用方自己 `SchemaOf` + 组装 `tools`，`llm.CallTool` 只负责流式接收。这样 `llm` 包保持底层纯净。

### onStatus 回调的处理

用户关切点：`onStatus` 各模块各自实现，抽成参数后不需要的模块传 nil 是否可行。

结论：**可行，且现有 `pattern.callTool` 已有 nil 检查**（`if onStatus != nil`）。通用化后：
- `pattern`：5 个调用点里 2 个有 onStatus（boundaries / finalSkill），3 个传 nil（summaries / initialChunks / compressChunks）——保持原样
- `style`：传 nil（风格提取不需要前端状态推送，或后续按需补）
- `memory`：session 结束提取传 nil（后台任务，无前端）；压缩路径走 `agent` 包自己的流，不经过 `CallTool`

`CallToolStatus` 定义在 `llm` 包，`pattern.LLMStatus` 可改为 `type LLMStatus = llm.CallToolStatus` 别名（或直接替换，`pattern` 内部改用 `llm.CallToolStatus`），消除类型重复。

### 调用点改造

#### pattern 包

`pattern.callTool` 删除，5 个调用点改为：

```go
// 改造前
raw, err := e.callTool(ctx, input, outputBoundariesTool, BoundaryHintsOutput{},
    boundaryMessages(chapters), 1, 0, onStatus)

// 改造后
tools := []map[string]any{{
    "type": "function",
    "function": map[string]any{
        "name":       outputBoundariesTool,
        "description": "返回模式提取的结构化输出。",
        "parameters": mcp_tools.SchemaOf(BoundaryHintsOutput{}),
    },
}}
raw, err := e.LLMClient.CallTool(ctx, input.ProviderName, input.ModelID,
    boundaryMessages(chapters), tools, outputBoundariesTool,
    &llm.CallToolOptions{
        Attempts:        1,
        OnStatus:        onStatus,
    })
```

**简化点**：可以在 `pattern` 包内留一个薄封装 `e.callTool(toolName, schema, messages, attempts, maxTokens, onStatus)`，内部调 `llm.CallTool`，组装 `tools` 数组（因为 `pattern` 5 个调用点的 `tools` 组装逻辑完全一样：单工具 + 固定 description）。这样 5 个调用点几乎不用改，只改 `callTool` 方法体。

建议走薄封装方案，改动最小：

```go
// pattern 包内保留，但改为调 llm.CallTool
func (e *Extractor) callTool(
    ctx context.Context, input ExtractPatternInput,
    toolName string, schema any, messages []map[string]any,
    attempts int, maxTokens int, onStatus func(LLMStatus),
) (json.RawMessage, error) {
    tools := []map[string]any{{
        "type": "function",
        "function": map[string]any{
            "name":        toolName,
            "description": "返回模式提取的结构化输出。",
            "parameters":  mcp_tools.SchemaOf(schema),
        },
    }}
    opts := &llm.CallToolOptions{
        Attempts: attempts,
        OnStatus: func(s llm.CallToolStatus) {
            if onStatus != nil {
                onStatus(LLMStatus(s)) // LLMStatus 是别名时直接转
            }
        },
    }
    if maxTokens > 0 {
        opts.MaxTokens = maxTokens
    }
    if input.ReasoningEffort != "" {
        opts.ReasoningEffort = input.ReasoningEffort
    }
    return e.LLMClient.CallTool(ctx, input.ProviderName, input.ModelID,
        messages, tools, toolName, opts)
}
```

5 个调用点零改动，只删 `pattern` 自己的流式/重试/错误聚合逻辑（约 40 行）。

#### style 包

`style.Extract` 升级为 toolcall：

```go
// 定义输出 schema
type StyleOutput struct {
    Name        string `json:"name" jsonschema:"required,description=风格名称"`
    Description string `json:"description" jsonschema:"required,description=一句话描述"`
    Content     string `json:"content" jsonschema:"required,description=技能正文 markdown，不含 frontmatter"`
}

// 调用
tools := []map[string]any{{
    "type": "function",
    "function": map[string]any{
        "name":       "output_style",
        "description": "返回风格提取的结构化输出。",
        "parameters": mcp_tools.SchemaOf(StyleOutput{}),
    },
}}
raw, err := llmClient.CallTool(ctx, providerName, modelID,
    buildExtractMessages(combined.String()), tools, "output_style",
    &llm.CallToolOptions{Attempts: 2})
// json.Unmarshal(raw, &out) → buildSkillMarkdown
```

收益：消除 frontmatter 解析失败风险，`skill.ParseBytes` 可不再用于 style 路径（`buildSkillMarkdown` 已有，pattern 也在用，可抽到 `skill` 包共用）。

#### memory 包（未来）

session 结束提取路径直接调 `llm.CallTool`，`tools` 用 `submit_memories` schema，`OnStatus` 传 nil。

## skill 产出层抽象评估

用户问："style 和 pattern 更进一步，还是都产生 skill 理论上也可以抽象，代码量大吗，是不是不太值得。"

调查结论：**只统一 callTool 层，不统一 skill 产出层**。理由：

| 维度 | pattern.finalSkill | style.Extract |
|---|---|---|
| 输入 | 章节叙事阶段块（`[]Chunk`） | 原文样本 + 统计信息 |
| 提示词 | `finalSkillMessages`（套路模板语境） | `buildExtractMessages`（风格分析语境） |
| 输出 schema | `SkillOutput{Name, Description, Content}` | 升级后同构 |
| 后处理 | `buildSkillMarkdown` 拼 frontmatter | 同 `buildSkillMarkdown` |
| 调用上下文 | 多轮 pipeline 的最后一步，带 trace | 单次调用 |

**可共用部分**只有 `buildSkillMarkdown`（拼 frontmatter）——这个建议抽到 `skill` 包（如 `skill.BuildMarkdown(name, desc, content)`），`pattern` 和 `style` 都调它。代码量约 15 行，收益清晰。

**不可共用部分**：提示词构造、输入组装、调用时机，差异大，强行抽象会引入"通用接口 + 各自适配"的过度设计。`pattern.finalSkill` 是 pipeline 终点，`style.Extract` 是独立入口，二者职责不同，保持独立函数更直白。

**callTool 层共用**才是真正消除重复的部分（流式接收 + 重试 + 错误聚合，约 40 行 × 3 模块 = 120 行重复）。skill 产出层强行抽象只省 15 行，不值得。

## 工作量评估

| 任务 | 改动文件 | 行数估算 | 风险 |
|---|---|---|---|
| 新增 `llm/calltool.go` | 1 | +60（含类型 + 函数） | 低，逻辑从 pattern 平移 |
| `pattern.callTool` 改为薄封装 | 1 | -40 +25 | 低，5 个调用点零改动 |
| `pattern.LLMStatus` 改别名 | 1 | ±3 | 低 |
| `style.Extract` 升级 toolcall | 1 | -15 +25 | 中，需新增 `StyleOutput` schema + 调提示词 |
| `skill.BuildMarkdown` 抽出 | 1 | +15 -10 | 低 |
| `style` 提示词调整（去掉 frontmatter 硬约束） | 1 | ±20 | 中，需测试输出质量 |
| 测试调整 | 2-3 | ±50 | 中 |

**总计**：约 +120 / -65 净增 55 行，核心改动 4 个文件。属小型重构，1-2 个 commit 可完成。

**建议拆分**：
1. commit 1：`llm.CallTool` + `pattern` 切换（纯重构，行为不变，可独立验证）
2. commit 2：`style` 升级 toolcall + `skill.BuildMarkdown` 抽出（行为变化，需测试输出质量）

## 是否值得做

值得。理由：
1. `memory` 模块即将开发，toolcall 是刚需，不抽象就要再抄一遍
2. `style` 当前纯文本解析不稳，toolcall 是已知解法，抽象后顺手升级
3. `pattern.callTool` 逻辑成熟，平移风险低，5 个调用点零改动
4. 代码量小（净增 55 行），收益（3 模块共用 + 消除 frontmatter 解析风险）明确

不建议做的部分：skill 产出层强行统一（只省 15 行，引入过度设计）。

## 执行顺序建议

1. **先落地 `llm.CallTool`**：新建 `internal/llm/calltool.go`，从 `pattern.callTool` 平移逻辑，加 `CallToolStatus` / `CallToolOptions` 类型
2. **`pattern` 切换**：`callTool` 方法体改为调 `llm.CallTool`，`LLMStatus` 改别名，5 个调用点不动
3. **验证 `pattern`**：跑 `go build ./...` + `go test ./internal/pattern/`，确认行为不变
4. **`skill.BuildMarkdown` 抽出**：从 `pattern.buildSkillMarkdown` 搬到 `skill` 包，`pattern` / `style` 共用
5. **`style` 升级 toolcall**：新增 `StyleOutput` schema，改 `Extract` 调 `llm.CallTool`，提示词去掉 frontmatter 硬约束
6. **验证 `style`**：跑 `go test ./internal/style/`，人工抽测输出质量

每步独立可回滚，第 1-3 步是纯重构（行为不变），第 4-6 步是行为升级（需测试）。

## 未决问题

1. **`CallToolOptions.Attempts` 语义**：当前 `pattern` 传 1 表示"只试一次不重试"，传 2 表示"最多试 2 次"。通用化后是否统一为 `Attempts` 表示"最大尝试次数"（≥1）？建议是，文档里写清楚。
2. **`OnStatus` 去重**：`pattern.callTool` 里 `EventThinking` 只推一次（`sentThinking` 去重），`EventToolCallStart` 每次都推。通用化后保持原行为还是改为"每个状态只推一次"？建议保持原行为（thinking 去重、generating 不去重），因为 `EventToolCallStart` 可能多次（虽然单工具通常一次）。
3. **`style` 升级后 `skill.ParseBytes` 是否还用**：`style` 路径不再需要，但 `pattern.finalSkill` 升级后也不需要（用 `buildSkillMarkdown`）。`skill.ParseBytes` 仍用于解析用户手写 / 旧版 skill 文件，保留。

这些是文档层面的小决策，实现时按建议默认值走即可，不阻塞。
