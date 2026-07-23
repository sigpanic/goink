# 模式提取鲁棒性重构

## Context

模式提取（套路提取）实测暴露三类问题：

1. **Step3 最终 skill 生成脆弱**：pipeline 前三步（边界 / 摘要 / 压缩）都用 function calling 工具约束输出，唯独 Step3 `finalSkill` 改用纯自由文本流式生成（`ChatStream(..., nil, ...)`），再用 `skill.ParseBytes` 严校验。思考类模型在正文前吐前言、或 frontmatter 格式偏差，都会让 `splitFrontmatter` 解析不到 → `name` 为空 → 报「缺少 name 字段」硬失败，整任务作废，无重试无兜底。
2. **失败后前端页面消失**：`PatternExtractView` catch 块直接 `setPhase("idle")` + `setRunningTask(null)`，进度视图卸载，前面推送的进度事件全部不可见。
3. **书籍列表不刷新**：`PatternExtractView` 只在挂载时拉一次 `GetNovels()`，导入新书后列表不更新，PopSelect 显示「无可用作品」且选不回新书（重启后正常）。

目标：Step3 改工具调用消除 name 缺失；前端重构为「选择页 / 会话页」两页面导航，会话页常驻、失败不跳回；书籍下拉打开时实时重拉。

---

## 一、后端：Step3 改工具调用

### `internal/pattern/types.go`
新增结构体（与 `BoundaryHintsOutput` 等同款 `jsonschema` tag）：
```go
type SkillOutput struct {
    Name        string `json:"name" jsonschema:"required,description=技能名称（中文模式名）"`
    Description string `json:"description" jsonschema:"required,description=一句话描述何时使用此叙事模式"`
    Content     string `json:"content" jsonschema:"required,description=技能正文 markdown，含 ## 套路概览/阶段拆解/爽点节奏/角色功能模板/可复用叙事规律/使用注意 等章节，不含 frontmatter"`
}
```

### `internal/pattern/prompts.go`
改写 `finalSkillSystemPrompt`：从「输出完整 markdown 含 frontmatter」改为「通过调用 `output_skill` 工具返回 name / description / content，content 为不含 frontmatter 的正文 markdown」。章节大纲要求（套路概览/阶段拆解/...）保留。

### `internal/pattern/extract.go`
- 改 `finalSkill` 方法：从 `ChatStream(..., nil, ...)` 自由流式，改走 `callTool(ctx, input, "output_skill", SkillOutput{}, finalSkillMessages(chunks), 2, onStatus)`（`attempts=2`，与摘要/压缩一致；`onStatus` 推送 thinking/generating）。
- 解析工具返回的 `SkillOutput`，新增辅助 `buildSkillMarkdown(name, description, content)` 组装完整 markdown：固定 frontmatter（`category: 套路模板` / `mode: auto` / `author: ai` / `version: 1`）+ content。
- 返回组装后的 raw。`Extract` 里 `ParseBytes(raw, "ai")` 校验保留（此时 name 必有，基本必过；content 空兜底报错）。
- 进度事件 `StageFinalizing` 的 thinking/generating 由 `callTool` 的 onStatus 继续推送，前端状态指示不变；唯一损失是正文不再逐字流式。

---

## 二、前端：两页面导航 + session 内部子状态

顶层用 `view: "select" | "session"` 切换两个独立页面，**只由用户点击驱动**（开始提取 / 返回）；promise 成败只改 session 页内部子状态，不参与顶层切换。

### `frontend/src/components/pattern/PatternExtractView.tsx`（重构为容器）
- 顶层 state：`view: "select" | "session"` + 启动参数（novelId / chapterIds / modelKey / taskId / title / chapterCount）。
- `view="select"`：渲染选择页（书籍 / 章节范围 / 模型 + 开始提取按钮）。
- 点「开始提取」→ 组装参数 + `createPatternTaskID()` → `view="session"`，参数传入 session 页。
- session 页 `onExit` 回调 → `view="select"` + 清 session 状态。

### 新增 `frontend/src/components/pattern/PatternSessionView.tsx`（提取会话页）
- Props：启动参数 + `onExit` 回调。
- 内部子状态：`status: "running" | "done" | "failed"`、`result`、`error`。
- `usePatternProgress(taskId)` 拿 progress/events，session 期间不 reset，failed 也不清。
- `useEffect` 启动即调 `app.ExtractPattern(...)`：resolve → `status="done"` + result；reject → `status="failed"` + error（保留 events）。
- 渲染：
  - `running`：`PatternProgressView`
  - `done`：结果预览（搬原 preview：frontmatter 表格 + Markdown 正文）+ 保存 / 重做 / 返回按钮
  - `failed`：错误条 + `PatternProgressView`（保留进度历史）+ 重试 / 返回按钮
- 「重试」：用原参数重新调 `ExtractPattern`，status 回 running。
- 「返回」：调 `onExit`。
- `handleSave` 从原 `PatternExtractView` 搬入。

### bug1：书籍下拉 onOpen 重拉（在选择页内）
书籍 `PopSelect` 挂 `onOpen`，打开时调 `app.GetNovels()` 刷新 `novels` state。每次打开即最新列表，导入新书立即可见，选不回问题同步消失。

---

## 三、复用点（不新造轮子）
- `callTool`（[extract.go:513](file:///home/nianhe/projects/goink/internal/pattern/extract.go#L513)）：工具调用 + 重试 + onStatus，Step3 直接复用。
- `mcp_tools.SchemaOf`（[base.go:308](file:///home/nianhe/projects/goink/internal/mcp_tools/base.go#L308)）：`SkillOutput` 带 tag 即生成 schema。
- `skill.ParseBytes`（[parse.go:43](file:///home/nianhe/projects/goink/internal/skill/parse.go#L43)）：校验不变。
- `usePatternProgress`（[usePatternProgress.ts](file:///home/nianhe/projects/goink/frontend/src/hooks/usePatternProgress.ts)）：events 累积 + 不自动 reset，session 页直接复用。
- `PatternProgressView`：running / failed 态都复用。

---

## 四、验证
1. 后端：`go build ./...`、`go test ./internal/pattern/...`
2. 前端：`cd frontend && npm run build`、`npm run lint`
3. 端到端（`make dev`）：
   - 导入新书 → 打开模式提取 → 点开书籍下拉 → 能看到新书（bug1）
   - 选 5+ 章开始提取 → 进入 session 页看进度（running）
   - 正常完成 → session 页显示结果预览，可保存 / 重做 / 返回（done）
   - 制造失败（断网 / 切无效模型）→ session 页显示错误 + 进度历史，不跳回选择页，可重试 / 返回（failed）
   - 确认最终 skill 的 name 不再缺失（Step3 工具调用）
4. pre-commit hook 自动跑 go build/test/lint + frontend build/lint/test，无需手动验证。
