package mcp_tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"novel/internal/chapter"
	"novel/internal/git"
	"novel/internal/rag"
	"novel/internal/skill"
	"novel/internal/text"
	"novel/internal/writing"
)

// ── edit ──────────────────────────────────────────────────

// EditArgs 是 edit 工具的参数。
type EditArgs struct {
	Path       string `json:"path" jsonschema:"required,description=要编辑的文件路径。章节文件格式为 chapters/001.md（三位数字），大纲为 outlines/001.md，故事状态为 goink.md" validate:"required"`
	ChangeType string `json:"change_type" jsonschema:"required,enum=full_replace,enum=search_replace,enum=line_range_replace,description=编辑方式。full_replace：全文替换；search_replace：查找并替换指定文本；line_range_replace：替换指定行范围" validate:"required,oneof=full_replace search_replace line_range_replace"`
	SearchText string `json:"search_text" jsonschema:"description=要查找的原文片段（search_replace 时必填）。请从文件中精确复制" validate:"omitempty"`
	NewContent string `json:"new_content" jsonschema:"description=新内容。full_replace 时为完整全文；search_replace 时为替换后的文本；line_range_replace 时为插入的新行" validate:"omitempty"`
	ReplaceAll bool   `json:"replace_all" jsonschema:"description=是否替换所有匹配项。默认 false（仅替换第一个匹配）" validate:"omitempty"`
	StartLine  int    `json:"start_line" jsonschema:"description=起始行号 1-based 含此行（line_range_replace 时必填），必须 <= end_line" validate:"omitempty,min=1"`
	EndLine    int    `json:"end_line" jsonschema:"description=结束行号 1-based 含此行（line_range_replace 时必填）" validate:"omitempty,min=1"`
	Reason     string `json:"reason" jsonschema:"description=修改原因，供人类审阅" validate:"omitempty"`
	Title      string `json:"title" jsonschema:"description=章节标题。新建章节时必填；对已有章节传入时将覆盖原标题（仅 chapters/xxx.md 路径生效）" validate:"omitempty"`
}

// EditTool 编辑文件（章节或故事状态），支持全文替换、查找替换、行范围替换。
// 修改在内存中完成后生成 git diff 提交审批，通过后写入文件。
type EditTool struct{}

func (t *EditTool) Name() string           { return "edit" }
func (t *EditTool) Description() string    { return editDescription }
func (t *EditTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *EditTool) JSONSchema() json.RawMessage { return SchemaOf(EditArgs{}) }
func (t *EditTool) ExposeToLLM() bool           { return true }
func (t *EditTool) NewArgs() any                { return &EditArgs{} }

func (t *EditTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*EditArgs)

	// 内置 skill 只读
	if strings.HasPrefix(a.Path, "/builtin/skills/") {
		return &ToolResult{Success: false, Error: "内置 skill 为只读，不可编辑"}, nil
	}

	// 1. 校验路径格式
	if !validPath(a.Path) {
		return &ToolResult{Success: false, Error: "无效文件路径，支持 chapters/001.md ~ chapters/999999.md、outlines/001.md ~ outlines/999999.md、goink.md、skills/<name>.md、~/.goink/skills/<name>.md"}, nil
	}

	// 2. 读取当前文件
	var fileExists bool
	current, err := git.ReadFile(tc.NovelID, a.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if a.ChangeType == "full_replace" {
				current = ""
			} else {
				return &ToolResult{Success: false, Error: "文件不存在: " + a.Path}, nil
			}
		} else if errors.Is(err, git.ErrPathEscape) {
			return &ToolResult{Success: false, Error: "路径非法: " + a.Path}, nil
		} else {
			return nil, fmt.Errorf("read file %s: %w", a.Path, err)
		}
	} else {
		fileExists = true
	}

	// 3. 根据 change_type 生成新内容
	proposed, err := applyChange(a, current)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("编辑操作失败: %s", err.Error())}, nil
	}

	if proposed == current {
		return &ToolResult{
			Success: true,
			Data:    map[string]any{"path": a.Path, "message": "内容未变化，跳过"},
		}, nil
	}

	// 4. 校验 skill 格式（在审批前，格式不对直接返回 LLM 修正）
	if isSkillPath(a.Path) {
		if _, err := skill.ParseBytes([]byte(proposed), ""); err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("skill 格式错误: %s", err.Error())}, nil
		}
	}

	// 5. 审批（阻塞等待用户确认）
	var approvalFeedback string
	if tc.Approver != nil {
		payload := map[string]any{
			"original":    current,
			"modified":    proposed,
			"path":        a.Path,
			"change_type": a.ChangeType,
			"reason":      a.Reason,
		}
		if tc.EmitApproval != nil {
			tc.EmitApproval(tc.ToolID, "file_edit", payload)
		}
		approval, err := tc.Approver.RequestApproval(ctx, tc.ToolID, payload)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return &ToolResult{Success: false, Error: "操作被中断"}, nil
			}
			return nil, fmt.Errorf("approval: %w", err)
		}
		if !approval.Approved {
			info := "你的修改被用户拒绝"
			if approval.Feedback != "" {
				info += "。用户反馈：" + approval.Feedback
			}
			return &ToolResult{
				Success: false,
				Error:   "审批未通过",
				Data: map[string]any{
					"path":        a.Path,
					"change_type": a.ChangeType,
					"approved":    false,
				},
				Inject: []InjectMessage{{Role: "user", Content: info}},
			}, nil
		}
		approvalFeedback = approval.Feedback
	}

	// 6. 章节/大纲 DB 记录维护（新建写入标题，已有章节可覆盖标题）
	if isChapterPath(a.Path) || isOutlinePath(a.Path) {
		chapNum := parseChapterNum(a.Path)
		if chapNum == 0 {
			chapNum = parseOutlineNum(a.Path)
		}
		if chapNum > 0 {
			if !fileExists {
				title := a.Title
				if title == "" {
					title = fmt.Sprintf("第%d章", chapNum)
				}
				ch := chapter.Chapter{
					NovelID:       tc.NovelID,
					ChapterNumber: chapNum,
					Title:         title,
				}
				if err := tc.DB.WithContext(ctx).Where("novel_id = ? AND chapter_number = ?", tc.NovelID, chapNum).FirstOrCreate(&ch).Error; err != nil {
					return nil, fmt.Errorf("auto-create chapter record: %w", err)
				}
			} else if a.Title != "" && isChapterPath(a.Path) {
				if err := tc.DB.WithContext(ctx).
					Model(&chapter.Chapter{}).
					Where("novel_id = ? AND chapter_number = ?", tc.NovelID, chapNum).
					Update("title", a.Title).Error; err != nil {
					return nil, fmt.Errorf("update chapter title: %w", err)
				}
			}
		}
	}

	// 7. 写入前重读对比，阻止并发冲突
	if fresh, err := git.ReadFile(tc.NovelID, a.Path); err == nil && fresh != current {
		return &ToolResult{Success: false, Error: "文件已被修改，请重新读取最新内容后重试"}, nil
	}

	// 8. 写入文件
	if err := git.WriteFile(tc.NovelID, a.Path, proposed); err != nil {
		if errors.Is(err, git.ErrPathEscape) {
			return &ToolResult{Success: false, Error: "路径非法: " + a.Path}, nil
		}
		return nil, fmt.Errorf("write file: %w", err)
	}

	wails.EventsEmit(ctx, "file:changed", map[string]any{
		"novel_id": tc.NovelID,
		"path":     a.Path,
	})

	// 异步刷新章节向量 + 更新字数
	if isChapterPath(a.Path) {
		chapNum := parseChapterNum(a.Path)
		rag.SubmitRefresh(tc.NovelID, chapNum, proposed)
		if tc.SearchService != nil {
			tc.SearchService.UpdateCachedChapter(tc.NovelID, chapNum, proposed)
		}
		stats := text.ComputeStats(proposed)

		// 记录字数变化
		var oldWC int
		tc.DB.WithContext(ctx).
			Model(&chapter.Chapter{}).
			Select("COALESCE(word_count, 0)").
			Where("novel_id = ? AND chapter_number = ?", tc.NovelID, chapNum).
			Scan(&oldWC)
		if delta := stats.WordCount - oldWC; delta != 0 {
			tc.DB.WithContext(ctx).Create(&writing.WritingLog{
				Date:          time.Now().Format("2006-01-02"),
				NovelID:       tc.NovelID,
				ChapterNumber: chapNum,
				WordDelta:     delta,
			})
		}

		tc.DB.WithContext(ctx).
			Model(&chapter.Chapter{}).
			Where("novel_id = ? AND chapter_number = ?", tc.NovelID, chapNum).
			Update("word_count", stats.WordCount)

	}

	// 9. inject 维护提醒（章节全量替换且 >500 字时）
	var injects []InjectMessage
	if approvalFeedback != "" {
		injects = append(injects, InjectMessage{Role: "user", Content: "用户通过了审批并反馈：" + approvalFeedback})
	}
	if a.ChangeType == "full_replace" && isChapterPath(a.Path) && len([]rune(proposed)) > 500 {
		chapNum := parseChapterNum(a.Path)
		injects = append(injects, InjectMessage{
			Role:    "user",
			Content: fmt.Sprintf("你刚刚完成了第%d章的全量替换。请执行以下维护操作：\n1. 检查并更新角色设定（性格变化、新能力、身份转变等）\n2. 更新故事时间线（伏笔回收、新伏笔记录、章节计划推进）\n3. 更新读者认知（新悬念、已回收悬念）\n4. 更新故事弧线节点进度\n完成后向用户汇报修改摘要。", chapNum),
		})
	}

	data := map[string]any{
		"path":        a.Path,
		"change_type": a.ChangeType,
		"approved":    true,
	}
	if a.ChangeType == "line_range_replace" {
		afterEnd := a.StartLine + strings.Count(a.NewContent, "\n")
		data["before"] = linePreview(current, a.StartLine, a.EndLine)
		data["after"] = linePreview(proposed, a.StartLine, afterEnd)
	}
	return &ToolResult{
		Success: true,
		Data:    data,
		Inject:  injects,
	}, nil
}

// linePreview 返回指定行范围的前后上下文预览，带行号。区间 1-based 闭区间。
func linePreview(content string, start, end int) string {
	lines := strings.Split(content, "\n")
	ctxStart := start - 1
	if ctxStart < 0 {
		ctxStart = 0
	}
	ctxEnd := end
	if ctxEnd > len(lines) {
		ctxEnd = len(lines)
	}

	// 前后各多取一行上下文
	preStart := ctxStart - 1
	if preStart < 0 {
		preStart = 0
	}
	postEnd := ctxEnd + 1
	if postEnd > len(lines) {
		postEnd = len(lines)
	}

	var b strings.Builder
	for i := preStart; i < postEnd; i++ {
		if i == ctxStart {
			b.WriteString("─── 改动区间 ───\n")
		}
		fmt.Fprintf(&b, "%d|%s\n", i+1, lines[i])
		if i == ctxEnd-1 {
			b.WriteString("─── 改动结束 ───\n")
		}
	}
	return b.String()
}

// ── 编辑操作 ──────────────────────────────────────────────

func applyChange(a *EditArgs, current string) (string, error) {
	switch a.ChangeType {
	case "full_replace":
		return a.NewContent, nil

	case "search_replace":
		if a.SearchText == "" {
			return "", fmt.Errorf("search_replace 模式需要提供 search_text")
		}
		result, found, hint := searchReplace(current, a.SearchText, a.NewContent, a.ReplaceAll)
		if !found {
			if hint != "" {
				return "", fmt.Errorf("%s", hint)
			}
			return "", fmt.Errorf("未找到匹配文本，请用精确文本重试")
		}
		return result, nil

	case "line_range_replace":
		if a.StartLine <= 0 || a.EndLine <= 0 {
			return "", fmt.Errorf("line_range_replace 模式需要提供 start_line 和 end_line")
		}
		if a.StartLine > a.EndLine {
			return "", fmt.Errorf("start_line 不能大于 end_line")
		}
		return lineRangeReplace(current, a.StartLine, a.EndLine, a.NewContent)

	default:
		return "", fmt.Errorf("未知的 change_type: %s", a.ChangeType)
	}
}

// searchReplace 在 content 中查找 searchText 并替换为 newContent。
// replaceAll=false 时仅替换第一个匹配。返回修改后的内容、是否找到匹配、以及失败时的模糊匹配提示。
func searchReplace(content, searchText, newContent string, replaceAll bool) (result string, found bool, hint string) {
	searchText = strings.TrimRight(searchText, "\n")

	// 层 1：精确匹配
	if idx := strings.Index(content, searchText); idx >= 0 {
		n := 1
		if replaceAll {
			n = -1
		}
		return strings.Replace(content, searchText, newContent, n), true, ""
	}

	// 层 2：TrimSpace 后精确匹配
	trimmedSearch := strings.TrimSpace(searchText)
	if trimmedSearch != searchText {
		if idx := strings.Index(content, trimmedSearch); idx >= 0 {
			n := 1
			if replaceAll {
				n = -1
			}
			return strings.Replace(content, trimmedSearch, newContent, n), true, ""
		}
	}

	// 层 3：标点归一化后匹配（引号变体统一再比）
	// 只要两边有一方被归一化改变就重新比对
	normSearch := normalizePunctuation(searchText)
	normContent := normalizePunctuation(content)
	if normSearch != searchText || normContent != content {
		// 归一化后字符宽度可能不同（弯引号 3 字节 → ASCII 1 字节），必须按 rune 对齐
		normCRunes := []rune(normContent)
		normSRunes := []rune(normSearch)
		if pos := runeIndex(normCRunes, normSRunes); pos >= 0 {
			origRunes := []rune(content)
			original := string(origRunes[pos : pos+len(normSRunes)])
			n := 1
			if replaceAll {
				n = -1
			}
			return strings.Replace(content, original, newContent, n), true, ""
		}
	}

	// 层 4：模糊匹配反馈（不替换，只告诉 LLM 正确文本长什么样）
	hint = fuzzyHint(searchText, content)
	return "", false, hint
}

// normalizePunctuation 将中文标点变体统一映射为 ASCII 等效字符。
// 仅在查找匹配时使用，不修改文件内容。
var quoteReplace = strings.NewReplacer(
	"“", `"`, "”", `"`, // " " 弯引号
	"「", `"`, "」", `"`, // 「 」 直角引号
	"『", `"`, "』", `"`, // 『 』 双直角引号
	"＂", `"`, // ＂ 全角引号
	"‘", `'`, "’", `'`, // ' ' 弯单引号
	"＇", `'`, // ＇ 全角单引号
)

func normalizePunctuation(s string) string {
	return quoteReplace.Replace(s)
}

// runeIndex 在 rune 切片 a 中查找子切片 b，返回首位置，未找到返回 -1。
func runeIndex(a, b []rune) int {
	for i := 0; i <= len(a)-len(b); i++ {
		match := true
		for j, r := range b {
			if a[i+j] != r {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// fuzzyHint 在 content 中找到与 searchText 最相似的段落，返回格式化的提示信息。
// 失败时调用，帮助 LLM 根据实际内容修正 search_text 后重试。
func fuzzyHint(searchText, content string) string {
	searchLines := strings.Split(strings.TrimSpace(searchText), "\n")
	contentLines := strings.Split(content, "\n")
	if len(searchLines) == 0 || len(contentLines) == 0 {
		return ""
	}

	w := len(searchLines)
	bestScore := 0.0
	bestStart := 0
	bestW := w

	// 滑动窗口逐段比较
	for i := 0; i <= len(contentLines)-w; i++ {
		candidate := strings.Join(contentLines[i:i+w], "\n")
		score := partialRatio(searchText, candidate)
		if score > bestScore {
			bestScore = score
			bestStart = i
		}
	}
	// 也尝试窗口 ±2 行
	for _, delta := range []int{2, -2, 1, -1} {
		ws := w + delta
		if ws <= 0 || ws > len(contentLines) {
			continue
		}
		for i := 0; i <= len(contentLines)-ws; i++ {
			candidate := strings.Join(contentLines[i:i+ws], "\n")
			score := partialRatio(searchText, candidate)
			if score > bestScore {
				bestScore = score
				bestStart = i
				bestW = ws
			}
		}
	}

	if bestScore < 0.4 {
		return "未找到任何相似内容，请用精确文本或 line_range_replace 重试。"
	}

	contextStart := bestStart
	if contextStart > 2 {
		contextStart -= 2
	}
	contextEnd := bestStart + bestW
	if contextEnd+2 < len(contentLines) {
		contextEnd += 2
	} else {
		contextEnd = len(contentLines)
	}
	// 取匹配行 + 前后各 2 行上下文
	contextLines := contentLines[contextStart:contextEnd]
	if len(contextLines) > 8 {
		contextLines = contextLines[:8]
	}
	nearby := strings.Join(contextLines, "\n")

	return fmt.Sprintf(
		"未找到精确匹配。以下为模糊匹配到的最相似片段（相似度 %.0f%%，第 %d-%d 行附近），仅供参考——请自行判断是否就是你想要修改的位置：\n%s\n如果确认就是此处，可直接用 line_range_replace(start_line=%d, end_line=%d) 修改，或根据实际内容修正 search_text 后重新调用 search_replace。",
		bestScore*100, bestStart+1, bestStart+bestW, nearby, bestStart+1, bestStart+bestW,
	)
}

// partialRatio 计算两段文本的字符级相似度（0.0~1.0）。
// 使用滑动窗口在较长的文本中找与较短文本最匹配的片段。
func partialRatio(a, b string) float64 {
	short, long := a, b
	if len(a) > len(b) {
		short, long = b, a
	}
	if len(short) == 0 {
		if len(long) == 0 {
			return 1.0
		}
		return 0
	}
	shortRunes := []rune(short)
	longRunes := []rune(long)
	if len(longRunes) < len(shortRunes) {
		longRunes, shortRunes = shortRunes, longRunes
	}

	best := 0.0
	for i := 0; i <= len(longRunes)-len(shortRunes); i++ {
		matches := 0
		for j, sr := range shortRunes {
			if sr == longRunes[i+j] {
				matches++
			}
		}
		score := float64(matches) / float64(len(shortRunes))
		if score > best {
			best = score
		}
	}
	return best
}

// lineRangeReplace 替换 [startLine, endLine] 区间（1-based，含两端）。
func lineRangeReplace(content string, startLine, endLine int, newContent string) (string, error) {
	lines := strings.Split(content, "\n")
	if startLine < 1 || endLine > len(lines) || startLine > endLine {
		return "", fmt.Errorf("行号超出范围: start=%d end=%d 总行数=%d", startLine, endLine, len(lines))
	}

	var result []string
	result = append(result, lines[:startLine-1]...)
	if newContent != "" {
		result = append(result, strings.Split(newContent, "\n")...)
	}
	result = append(result, lines[endLine:]...)
	return strings.Join(result, "\n"), nil
}

// ── 路径校验 ──────────────────────────────────────────────

var pathRe = regexp.MustCompile(`^(chapters/\d{3,6}\.md|goink\.md|outlines/\d{3,6}\.md|skills/[^/]+\.md|~/.goink/skills/[^/]+\.md)$`)

func validPath(p string) bool {
	return pathRe.MatchString(p)
}

func isChapterPath(p string) bool {
	return strings.HasPrefix(p, "chapters/")
}

func parseChapterNum(p string) int {
	var n int
	_, _ = fmt.Sscanf(p, "chapters/%d.md", &n)
	return n
}

func isOutlinePath(p string) bool {
	return strings.HasPrefix(p, "outlines/")
}

func isSkillPath(p string) bool {
	return strings.HasPrefix(p, "skills/") || strings.HasPrefix(p, "~/.goink/skills/")
}

func parseOutlineNum(p string) int {
	var n int
	_, _ = fmt.Sscanf(p, "outlines/%d.md", &n)
	return n
}

// ── 工具描述 ──────────────────────────────────────────────

const editDescription = `编辑小说文件（章节正文或大纲或故事状态 goink.md 或技能文件）。支持三种编辑模式：

1. **full_replace** — 全文替换整个文件。new_content 为完整的替换后内容。
2. **search_replace** — 查找并替换指定文本。search_text 为要查找的原文片段（请从文件中精确复制），new_content 为替换后的文本。replace_all=false（默认）仅替换第一个匹配项，replace_all=true 替换所有匹配。如果连续两次 search_replace 因"未找到匹配"失败，直接用 line_range_replace 代替——不要在同一种模式上反复重试。
3. **line_range_replace** — 替换指定行范围。start_line 和 end_line 为 1-based 行号（含两端），new_content 为插入的新内容。

路径格式：
- chapters/001.md ~ chapters/999999.md（三位数字补齐的章节文件）
- outlines/001.md ~ outlines/999999.md（章节大纲文件）
- goink.md（故事状态文档）
- skills/<name>.md（小说级技能）
- ~/.goink/skills/<name>.md（用户级技能）
只用上述路径。

title 参数：新建章节/大纲时传入标题；对已有章节传入非空 title 将覆盖原标题（不传或传空字符串则保留原标题）。

所有修改会先生成 git diff 提交用户审批，审批通过后才写入文件。被拒绝时返回用户反馈，可根据反馈修正后重试。`

// ── read ──────────────────────────────────────────────────

// ReadArgs 是 read 工具的参数。
type ReadArgs struct {
	Path         string `json:"path" jsonschema:"required,description=要读取的文件路径。章节文件格式为 chapters/001.md（三位数字），大纲为 outlines/001.md，故事状态为 goink.md" validate:"required"`
	IncludeLines *bool  `json:"include_lines" jsonschema:"default=true,description=是否包含行号前缀（如 123|）。默认 true，用于精确引用和行范围编辑。传 false 获取纯文本"`
	StartLine    int    `json:"start_line" jsonschema:"default=1,description=起始行号 1-based 含此行，必须 <= end_line" validate:"omitempty,min=1"`
	EndLine      int    `json:"end_line" jsonschema:"default=2000,description=结束行号 1-based 含此行，超出自动截到文末；设为 0 使用默认值 2000" validate:"omitempty,min=0"`
}

// ReadTool 读取文件内容（章节正文或故事状态 goink.md）。
// 默认含行号前缀（123|），LLM 传 include_lines=false 获取纯文本。
// start_line/end_line 支持行范围读取，用于翻页和精确引用。
type ReadTool struct{}

func (t *ReadTool) Name() string           { return "read" }
func (t *ReadTool) Description() string    { return readDescription }
func (t *ReadTool) Category() ToolCategory { return CategoryNovelManagement }

func (t *ReadTool) JSONSchema() json.RawMessage { return SchemaOf(ReadArgs{}) }
func (t *ReadTool) ExposeToLLM() bool           { return true }
func (t *ReadTool) NewArgs() any                { return &ReadArgs{} }

func (t *ReadTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*ReadArgs)

	// builtin skill 走 store 内存
	if strings.HasPrefix(a.Path, "/builtin/skills/") {
		return t.readBuiltinSkill(a, tc)
	}

	if !validPath(a.Path) {
		return &ToolResult{Success: false, Error: "无效文件路径，支持 chapters/001.md ~ chapters/999999.md、outlines/001.md ~ outlines/999999.md、goink.md、skills/<name>.md、~/.goink/skills/<name>.md、/builtin/skills/<name>.md（只读）"}, nil
	}

	content, err := git.ReadFile(tc.NovelID, a.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &ToolResult{Success: false, Error: "文件不存在: " + a.Path}, nil
		}
		if errors.Is(err, git.ErrPathEscape) {
			return &ToolResult{Success: false, Error: "路径非法: " + a.Path}, nil
		}
		return nil, fmt.Errorf("read file %s: %w", a.Path, err)
	}

	start := a.StartLine
	if start == 0 {
		start = 1
	}
	end := a.EndLine
	if end == 0 {
		end = 2000
	}

	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	if start > end {
		return &ToolResult{Success: false, Error: fmt.Sprintf("start_line(%d) 不能大于 end_line(%d)", start, end)}, nil
	}
	if start > totalLines {
		return &ToolResult{Success: false, Error: fmt.Sprintf("起始行 %d 超出文件总行数 %d", start, totalLines)}, nil
	}
	if end > totalLines {
		end = totalLines
	}

	selected := lines[start-1 : end]

	includeLines := a.IncludeLines == nil || *a.IncludeLines

	var output string
	if includeLines {
		var sb strings.Builder
		for i, line := range selected {
			fmt.Fprintf(&sb, "%d|%s\n", start+i, line)
		}
		output = strings.TrimRight(sb.String(), "\n")
	} else {
		output = strings.Join(selected, "\n")
	}

	display := a.Path
	if isChapterPath(a.Path) {
		display = fmt.Sprintf("第%d章", parseChapterNum(a.Path))
	} else if isOutlinePath(a.Path) {
		display = fmt.Sprintf("第%d章大纲", parseOutlineNum(a.Path))
	}

	data := map[string]any{
		"path":        a.Path,
		"display":     display,
		"content":     output,
		"total_lines": totalLines,
		"start_line":  start,
		"end_line":    end,
	}
	if end < totalLines {
		data["truncated"] = true
	}

	return &ToolResult{Success: true, Data: data}, nil
}

// readBuiltinSkill 从 store 内存读取内置 skill，全量返回。
func (t *ReadTool) readBuiltinSkill(a *ReadArgs, tc ToolContext) (*ToolResult, error) {
	if tc.SkillStore == nil {
		return &ToolResult{Success: false, Error: "skill store 未初始化"}, nil
	}

	name := strings.TrimSuffix(strings.TrimPrefix(a.Path, "/builtin/skills/"), ".md")
	if name == "" {
		return &ToolResult{Success: false, Error: "无效的 skill 路径: " + a.Path}, nil
	}

	sk, ok := tc.SkillStore.Get(tc.NovelID, name)
	if !ok {
		return &ToolResult{Success: false, Error: fmt.Sprintf("内置 skill %q 不存在", name)}, nil
	}

	return &ToolResult{Success: true, Data: map[string]any{
		"path":    a.Path,
		"display": fmt.Sprintf("技能: %s", sk.Name),
		"content": sk.RawContent,
	}}, nil
}

// ── 工具描述 ──────────────────────────────────────────────

const readDescription = `读取小说文件或技能文件。

路径格式（与 edit 工具一致）：
- chapters/001.md ~ chapters/999999.md（章节文件）
- outlines/001.md ~ outlines/999999.md（章节大纲）
- goink.md（故事状态文档）
- skills/<name>.md（小说级技能）
- ~/.goink/skills/<name>.md（用户级技能）
- /builtin/skills/<name>.md（内置技能，只读）
特性：
- 默认添加行号前缀（123|），方便后续 edit 工具进行 line_range_replace 和 search_replace
- start_line 和 end_line 支持行范围读取：默认读前 2000 行，可通过调整参数翻页或精确引用
- 返回 total_lines 表示全文行数，用于判断是否被截断
- include_lines=false 返回纯文本（不含行号）`

// ── 注册 ──────────────────────────────────────────────────

// RegisterRWTools 注册读写工具。
func RegisterRWTools(r *Registry) {
	r.Register(&ReadTool{})
	r.Register(&EditTool{})
}
