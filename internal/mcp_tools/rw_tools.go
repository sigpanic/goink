package mcp_tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/rag"
	"github.com/sigpanic/goink/internal/skill"
	"github.com/sigpanic/goink/internal/text"
	"github.com/sigpanic/goink/internal/volume"
	"github.com/sigpanic/goink/internal/writing"
)

// ── edit ──────────────────────────────────────────────────

// EditArgs 是 edit 工具的参数。
type EditArgs struct {
	Path       string `json:"path" jsonschema:"required,description=要编辑的文件路径。章节正文：chapters/id_{章节id}.md（主格式）或 chapters/{卷id}/id_{章节id}.md（等价容错写法）；章节大纲：outlines/id_{章节id}.md 或 outlines/{卷id}/id_{章节id}.md；新建章节：chapters/{卷id}/new.md 或 chapters/new.md（不带卷时默认分进最后一卷，整本书未建卷则创建未分卷章节）；新建大纲：outlines/{卷id}/new.md 或 outlines/new.md（规则同章节，只能 full_replace）；故事状态 goink.md；小说级技能 skills/<name>.md；用户级技能 ~/.goink/skills/<name>.md" validate:"required"`
	ChangeType string `json:"change_type" jsonschema:"required,enum=full_replace,enum=search_replace,enum=line_range_replace,description=编辑方式。full_replace：全文替换；search_replace：查找并替换指定文本；line_range_replace：替换指定行范围" validate:"required,oneof=full_replace search_replace line_range_replace"`
	SearchText string `json:"search_text" jsonschema:"description=要查找的原文片段（search_replace 时必填）。请从文件中精确复制" validate:"omitempty"`
	NewContent string `json:"new_content" jsonschema:"description=新内容。full_replace 时为完整全文（必填，传空会报错；若要清空整个文件请改用 line_range_replace(1, total_lines, \"\")，total_lines 从 read 返回获取）；search_replace 时为替换后的文本（传空则删除匹配到的文本）；line_range_replace 时为替换该行范围的新内容（传空则删除该范围行）" validate:"omitempty"`
	ReplaceAll bool   `json:"replace_all" jsonschema:"description=是否替换所有匹配项。默认 false（仅替换第一个匹配）" validate:"omitempty"`
	StartLine  int    `json:"start_line" jsonschema:"description=起始行号 1-based 含此行（line_range_replace 时必填），必须 <= end_line" validate:"omitempty,min=1"`
	EndLine    int    `json:"end_line" jsonschema:"description=结束行号 1-based 含此行（line_range_replace 时必填）" validate:"omitempty,min=1"`
	Reason     string `json:"reason" jsonschema:"required,description=必填。本次修改的原因/意图，供作者审批参考" validate:"required"`
	Title      string `json:"title" jsonschema:"description=章节标题。new.md 新建时可选，缺省为“第N章”（N 为分配的章节号）；对已有章节/大纲传入时覆盖原标题（chapters/ 与 outlines/ 路径均生效）" validate:"omitempty"`
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

	// 章节/大纲路径走章节流程；其余（goink.md、技能文件）走通用文件流程
	if ref, isOutline, ok := parseRWPath(a.Path); ok {
		return t.editChapterLike(ctx, a, tc, ref, isOutline)
	}
	if !validPath(a.Path) {
		return &ToolResult{Success: false, Error: invalidPathHint}, nil
	}
	return t.editPlainFile(ctx, a, tc)
}

// invalidPathHint 是 edit/read 共用的非法路径提示。
const invalidPathHint = "无效文件路径。章节正文 chapters/id_{id}.md、大纲 outlines/id_{id}.md、新建 chapters/{卷ID}/new.md 或 chapters/new.md、outlines/{卷ID}/new.md 或 outlines/new.md、goink.md、skills/<name>.md、~/.goink/skills/<name>.md"

// readFileForEdit 读取待编辑文件的当前内容。
// 文件不存在时 full_replace 视为从空文件创建，其余模式返回 os.ErrNotExist。
func readFileForEdit(novelID int64, path, changeType string) (string, error) {
	content, err := git.ReadFile(novelID, path)
	if err == nil {
		return content, nil
	}
	if errors.Is(err, os.ErrNotExist) && changeType == "full_replace" {
		return "", nil
	}
	return "", err
}

// fileReadError 将读取文件的已知业务错误转换为 ToolResult。
// handled=false 表示非业务错误，交由上层作为系统错误返回。
func fileReadError(path string, err error) (*ToolResult, bool) {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return &ToolResult{Success: false, Error: "文件不存在: " + path}, true
	case errors.Is(err, git.ErrPathEscape):
		return &ToolResult{Success: false, Error: "路径非法: " + path}, true
	default:
		return nil, false
	}
}

// approvalPayload 构造审批事件负载。title 非空时附带展示（标题将被修改）。
func approvalPayload(path, current, proposed string, a *EditArgs) map[string]any {
	payload := map[string]any{
		"original":    current,
		"modified":    proposed,
		"path":        path,
		"change_type": a.ChangeType,
		"reason":      a.Reason,
	}
	if a.Title != "" {
		payload["title"] = a.Title
	}
	return payload
}

// requestApproval 阻塞等待用户审批。
// res 非 nil 表示审批被拒绝或操作被中断，调用方直接返回该结果；
// err 表示审批通道自身的系统错误。
func requestApproval(ctx context.Context, tc ToolContext, payload map[string]any) (feedback string, res *ToolResult, err error) {
	if tc.Approver == nil {
		return "", nil, nil
	}
	if tc.EmitApproval != nil {
		tc.EmitApproval(tc.ToolID, "file_edit", payload)
	}
	ap, err := tc.Approver.RequestApproval(ctx, tc.ToolID, payload)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", &ToolResult{Success: false, Error: "操作被中断"}, nil
		}
		return "", nil, fmt.Errorf("approval: %w", err)
	}
	if !ap.Approved {
		info := "你的修改被用户拒绝"
		if ap.Feedback != "" {
			info += "。用户反馈：" + ap.Feedback
		}
		return "", &ToolResult{
			Success: false,
			Error:   "审批未通过",
			Data: map[string]any{
				"path":        payload["path"],
				"change_type": payload["change_type"],
				"approved":    false,
			},
			Inject: []InjectMessage{{Role: "user", Content: info}},
		}, nil
	}
	return ap.Feedback, nil, nil
}

// writeApproved 写入前的并发冲突检查 + 落盘 + file:changed 事件。
// res 非 nil 表示业务失败（并发冲突/路径非法），err 表示系统错误。
func writeApproved(ctx context.Context, tc ToolContext, path, current, proposed string) (res *ToolResult, err error) {
	// 写入前重读对比，阻止并发冲突
	if fresh, err := git.ReadFile(tc.NovelID, path); err == nil && fresh != current {
		return &ToolResult{Success: false, Error: "文件已被修改，请重新读取最新内容后重试"}, nil
	}
	if err := git.WriteFile(tc.NovelID, path, proposed); err != nil {
		if errors.Is(err, git.ErrPathEscape) {
			return &ToolResult{Success: false, Error: "路径非法: " + path}, nil
		}
		return nil, fmt.Errorf("write file: %w", err)
	}
	emitFileChanged(ctx, tc.NovelID, path)
	return nil, nil
}

// emitFileChanged 推送 file:changed 事件。wails runtime 在上下文缺失时
// 会 log.Fatalf 终止进程，因此无 wails 运行时上下文（如测试环境）时静默跳过。
func emitFileChanged(ctx context.Context, novelID int64, path string) {
	if ctx == nil || ctx.Value("events") == nil {
		return
	}
	wails.EventsEmit(ctx, "file:changed", map[string]any{
		"novel_id": novelID,
		"path":     path,
	})
}

// physicalRWPath 章节正文的物理扁平路径（isOutline=false 时为章节，true 时为大纲）。
func physicalRWPath(isOutline bool, id int64) string {
	if isOutline {
		return git.OutlinePath(id)
	}
	return git.ChapterPath(id)
}

// editPlainFile 编辑通用文件（goink.md、技能文件），无章节记录联动。
func (t *EditTool) editPlainFile(ctx context.Context, a *EditArgs, tc ToolContext) (*ToolResult, error) {
	current, err := readFileForEdit(tc.NovelID, a.Path, a.ChangeType)
	if err != nil {
		if res, handled := fileReadError(a.Path, err); handled {
			return res, nil
		}
		return nil, fmt.Errorf("read file %s: %w", a.Path, err)
	}

	proposed, err := applyChange(a, current)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("编辑操作失败: %s", err.Error())}, nil
	}
	if proposed == current {
		return &ToolResult{Success: true, Data: map[string]any{"path": a.Path, "message": "内容未变化，跳过"}}, nil
	}

	// skill 格式校验（审批前，格式不对直接返回 LLM 修正）
	if isSkillPath(a.Path) {
		if _, err := skill.ParseBytes([]byte(proposed), ""); err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("skill 格式错误: %s", err.Error())}, nil
		}
	}

	feedback, res, err := requestApproval(ctx, tc, approvalPayload(a.Path, current, proposed, a))
	if err != nil {
		return nil, err
	}
	if res != nil {
		return res, nil
	}

	if res, err := writeApproved(ctx, tc, a.Path, current, proposed); res != nil || err != nil {
		return res, err
	}

	var injects []InjectMessage
	if feedback != "" {
		injects = append(injects, InjectMessage{Role: "user", Content: "用户通过了审批并反馈：" + feedback})
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
	return &ToolResult{Success: true, Data: data, Inject: injects}, nil
}

// editChapterLike 编辑章节正文或大纲（含 new.md 新建通道）。
//
// 路径与存储的关系：
//   - 物理文件永远按 id 扁平落盘（chapters/id_{id}.md / outlines/id_{id}.md），
//     两级路径 chapters/{vid}/id_{id}.md 仅为容错别名，读写时归一化为扁平路径
//   - 新建走 new.md 通道：先建章节记录拿 id，再写物理文件；写文件失败补偿删记录
func (t *EditTool) editChapterLike(ctx context.Context, a *EditArgs, tc ToolContext, ref chapterRef, isOutline bool) (*ToolResult, error) {
	physical := physicalRWPath(isOutline, ref.ID)

	var (
		ch  *chapter.Chapter
		vid int64
	)
	if ref.IsNew {
		// 新建通道：只能 full_replace；卷可选——带卷校验归属，
		// 不带卷由 createChapterRecord 解析默认卷（最后一卷，无卷则未分卷）
		if a.ChangeType != "full_replace" {
			return &ToolResult{Success: false, Error: "new.md 仅支持 full_replace（新建文件无原文可查改）"}, nil
		}
		if ref.Vid != 0 {
			v, err := volume.NewStore(tc.DB, tc.LoggerOrDefault()).GetByID(ctx, nil, tc.NovelID, ref.Vid)
			if err != nil {
				return &ToolResult{Success: false, Error: err.Error()}, nil
			}
			vid = v.ID
		}
	} else {
		// 已有章节：记录必须存在，新建走 new.md 通道
		rec, err := getChapterRecord(ctx, tc.DB, tc.NovelID, ref.ID)
		if err != nil {
			return nil, fmt.Errorf("query chapter record: %w", err)
		}
		if rec == nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("章节记录不存在: id=%d，新建请用 chapters/{卷ID}/new.md 或 chapters/new.md", ref.ID)}, nil
		}
		ch = rec
	}

	// 读取当前内容（new 通道视为空文件）
	current := ""
	if !ref.IsNew {
		content, err := git.ReadFile(tc.NovelID, physical)
		if err != nil {
			// 大纲先行等工作流：记录存在但文件尚未落盘，full_replace 视为空文件创建
			if !errors.Is(err, os.ErrNotExist) || a.ChangeType != "full_replace" {
				if res, handled := fileReadError(physical, err); handled {
					return res, nil
				}
				return nil, fmt.Errorf("read file %s: %w", physical, err)
			}
		} else {
			current = content
		}
	}

	proposed, err := applyChange(a, current)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("编辑操作失败: %s", err.Error())}, nil
	}
	if proposed == current {
		// 内容未变：title 有变化时仍执行标题更新，否则直接跳过
		if !ref.IsNew && a.Title != "" && ch.Title != a.Title {
			if err := updateChapterTitle(ctx, tc.DB, ch, a.Title); err != nil {
				return nil, err
			}
			return &ToolResult{Success: true, Data: map[string]any{
				"path":    physical,
				"title":   a.Title,
				"message": "内容未变化，仅更新标题",
			}}, nil
		}
		return &ToolResult{Success: true, Data: map[string]any{"path": physical, "message": "内容未变化，跳过"}}, nil
	}

	// 审批（new 通道展示虚拟路径，已有章节展示物理路径）
	payloadPath := physical
	if ref.IsNew {
		payloadPath = a.Path
	}
	feedback, res, err := requestApproval(ctx, tc, approvalPayload(payloadPath, current, proposed, a))
	if err != nil {
		return nil, err
	}
	if res != nil {
		return res, nil
	}

	// 并发冲突检查（new 通道写全新文件，跳过）。
	// 必须在 DB 变更前执行，避免写入被拒时留下脏记录。
	if !ref.IsNew {
		if fresh, err := git.ReadFile(tc.NovelID, physical); err == nil && fresh != current {
			return &ToolResult{Success: false, Error: "文件已被修改，请重新读取最新内容后重试"}, nil
		}
	}

	// DB 记录维护：新建则建记录拿 id；已有且传 title 则更新标题
	if ref.IsNew {
		created, err := createChapterRecord(ctx, tc, vid, a.Title)
		if err != nil {
			return nil, fmt.Errorf("create chapter record: %w", err)
		}
		ch = created
		// 物理路径按刚分配的章节 id 重新计算（ref.ID 对新建为 0）
		physical = physicalRWPath(isOutline, ch.ID)
	} else if a.Title != "" && ch.Title != a.Title {
		if err := updateChapterTitle(ctx, tc.DB, ch, a.Title); err != nil {
			return nil, err
		}
	}

	// 写入物理文件；新建通道失败时补偿删除刚建的记录
	if err := git.WriteFile(tc.NovelID, physical, proposed); err != nil {
		if ref.IsNew {
			if delErr := deleteChapterRecord(ctx, tc.DB, ch.ID); delErr != nil {
				return nil, fmt.Errorf("write file: %w（补偿删除章节记录失败 record_id=%d: %v）", err, ch.ID, delErr)
			}
		}
		if errors.Is(err, git.ErrPathEscape) {
			return &ToolResult{Success: false, Error: "路径非法: " + physical}, nil
		}
		return nil, fmt.Errorf("write file: %w", err)
	}

	emitFileChanged(ctx, tc.NovelID, physical)

	// 正文（非大纲）落盘后的维护链路：向量/搜索缓存/字数/写作日志
	if !isOutline {
		maintainChapterAfterEdit(ctx, tc, ch, proposed)
	}

	data := map[string]any{
		"path":        physical,
		"change_type": a.ChangeType,
		"approved":    true,
	}
	if ref.IsNew {
		data["chapter_id"] = ch.ID
		data["chapter_number"] = ch.ChapterNumber
		var volID int64 // 未分卷为 0；默认卷解析发生在事务内，须从记录取
		if ch.VolumeID != nil {
			volID = *ch.VolumeID
		}
		data["volume_id"] = volID
	}

	var injects []InjectMessage
	if feedback != "" {
		injects = append(injects, InjectMessage{Role: "user", Content: "用户通过了审批并反馈：" + feedback})
	}
	// 章节正文全量替换且内容较长时注入维护提醒
	if !isOutline && a.ChangeType == "full_replace" && len([]rune(proposed)) > 500 {
		injects = append(injects, InjectMessage{
			Role: "user",
			Content: fmt.Sprintf("你刚刚完成了《%s》（第%d章）的全量替换。请执行以下维护操作：\n1. 检查并更新角色设定（性格变化、新能力、身份转变等）\n2. 更新故事时间线（伏笔回收、新伏笔记录、章节计划推进）\n3. 更新读者认知（新悬念、已回收悬念）\n4. 更新故事弧线节点进度\n完成后向用户汇报修改摘要。",
				ch.Title, ch.ChapterNumber),
		})
	}
	if a.ChangeType == "line_range_replace" {
		afterEnd := a.StartLine + strings.Count(a.NewContent, "\n")
		data["before"] = linePreview(current, a.StartLine, a.EndLine)
		data["after"] = linePreview(proposed, a.StartLine, afterEnd)
	}
	return &ToolResult{Success: true, Data: data, Inject: injects}, nil
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
		if a.NewContent == "" {
			return "", fmt.Errorf("full_replace 模式需要提供 new_content；若要清空整个文件请用 line_range_replace(1, 总行数, \"\")")
		}
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

// ── 路径解析 ──────────────────────────────────────────────

// chapterRef 是章节/大纲虚拟路径的解析结果。
//
// v1.6.0 起章节文件按 id 命名，支持两种虚拟路径形态：
//   - 扁平主格式：chapters/id_{id}.md
//   - 两级容错格式：chapters/{vid}/id_{id}.md（vid 仅为容错别名）
//
// 新建走 new.md 通道：chapters/{vid}/new.md 或 chapters/new.md（不带卷默认最后一卷，
// 整本书无卷则未分卷）。outlines 同理。
type chapterRef struct {
	ID    int64 // 章节记录 id；new 通道为 0
	Vid   int64 // 路径中的卷 id；扁平格式或不带卷的 new 通道为 0
	IsNew bool  // 是否 new.md 新建通道
}

var (
	chapterPathRe = regexp.MustCompile(`^chapters/(?:([0-9]+)/)?(?:id_([0-9]+)|new)\.md$`)
	outlinePathRe = regexp.MustCompile(`^outlines/(?:([0-9]+)/)?(?:id_([0-9]+)|new)\.md$`)
	plainPathRe   = regexp.MustCompile(`^(goink\.md|skills/[^/]+\.md|~/.goink/skills/[^/]+\.md)$`)
)

// parseRWPath 解析章节/大纲路径；非章节类路径返回 ok=false。
func parseRWPath(p string) (ref chapterRef, isOutline bool, ok bool) {
	if ref, ok := parseChapterRef(p); ok {
		return ref, false, true
	}
	if ref, ok := parseOutlineRef(p); ok {
		return ref, true, true
	}
	return chapterRef{}, false, false
}

// parseChapterRef 解析章节正文路径，非法返回 false。
// 捕获组：m[1]=卷 id（可选），m[2]=章节 id（new 通道为空）。
func parseChapterRef(p string) (chapterRef, bool) {
	m := chapterPathRe.FindStringSubmatch(p)
	if m == nil {
		return chapterRef{}, false
	}
	return buildChapterRef(m)
}

// parseOutlineRef 解析章节大纲路径，非法返回 false。
func parseOutlineRef(p string) (chapterRef, bool) {
	m := outlinePathRe.FindStringSubmatch(p)
	if m == nil {
		return chapterRef{}, false
	}
	return buildChapterRef(m)
}

// buildChapterRef 从正则捕获组构造章节引用。
// 数字段超出 int64 范围时解析失败，按非法路径处理。
func buildChapterRef(m []string) (chapterRef, bool) {
	ref := chapterRef{}
	if m[1] != "" {
		v, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return chapterRef{}, false
		}
		ref.Vid = v
	}
	if m[2] == "" {
		ref.IsNew = true
	} else {
		id, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			return chapterRef{}, false
		}
		ref.ID = id
	}
	return ref, true
}

// validPath 校验 edit/read 支持的全部路径形态。
func validPath(p string) bool {
	if _, _, ok := parseRWPath(p); ok {
		return true
	}
	return plainPathRe.MatchString(p)
}

func isSkillPath(p string) bool {
	return strings.HasPrefix(p, "skills/") || strings.HasPrefix(p, "~/.goink/skills/")
}

// ── 章节记录 ──────────────────────────────────────────────

// getChapterRecord 按章节 id 取当前小说的章节记录，不存在返回 (nil, nil)。
func getChapterRecord(ctx context.Context, db *gorm.DB, novelID, id int64) (*chapter.Chapter, error) {
	var ch chapter.Chapter
	err := db.WithContext(ctx).Where("id = ? AND novel_id = ?", id, novelID).First(&ch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

// updateChapterTitle 更新已有章节记录的标题，并同步内存中的记录值。
func updateChapterTitle(ctx context.Context, db *gorm.DB, ch *chapter.Chapter, title string) error {
	if err := db.WithContext(ctx).Model(ch).Update("title", title).Error; err != nil {
		return fmt.Errorf("update chapter title: %w", err)
	}
	ch.Title = title
	return nil
}

// createChapterRecord 新建通道：分配 sort_order 与 chapter_number，创建章节记录。
//
// vid 语义：>0 为显式指定卷；0 为未指定卷——默认分进最后一卷，
// 整本书未建卷时创建未分卷章节（VolumeID=NULL）。
// sort_order 的分配规则见 volume.Store.AllocateChapterSortOrder。
func createChapterRecord(ctx context.Context, tc ToolContext, vid int64, title string) (*chapter.Chapter, error) {
	volStore := volume.NewStore(tc.DB, tc.LoggerOrDefault())

	var created *chapter.Chapter
	err := tc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 未指定卷：默认最后一卷；整本书无卷则未分卷
		targetVolumeID := vid
		if targetVolumeID == 0 {
			last, err := volStore.LastByNovel(ctx, tx, tc.NovelID)
			if err != nil {
				return err
			}
			if last != nil {
				targetVolumeID = last.ID
			}
		}

		pos, err := volStore.AllocateChapterSortOrder(ctx, tx, tc.NovelID, targetVolumeID)
		if err != nil {
			return err
		}

		// 章节号：全小说 max+1（过渡期字段，唯一索引兜底并发）
		var nextNum int
		if err := tx.WithContext(ctx).Model(&chapter.Chapter{}).
			Select("COALESCE(MAX(chapter_number), 0)").
			Where("novel_id = ?", tc.NovelID).
			Scan(&nextNum).Error; err != nil {
			return err
		}
		if title == "" {
			title = fmt.Sprintf("第%d章", nextNum+1)
		}
		ch := &chapter.Chapter{
			NovelID:       tc.NovelID,
			ChapterNumber: nextNum + 1,
			SortOrder:     pos,
			Title:         title,
		}
		if targetVolumeID != 0 {
			ch.VolumeID = &targetVolumeID
		}
		if err := tx.Create(ch).Error; err != nil {
			return err
		}
		created = ch
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// deleteChapterRecord 补偿：新建通道写文件失败时删除刚建的记录，避免孤儿记录。
// 事务内腾位产生的 sort_order 空洞无害（仅作排序键），不回滚。
func deleteChapterRecord(ctx context.Context, db *gorm.DB, id int64) error {
	return db.WithContext(ctx).Delete(&chapter.Chapter{}, id).Error
}

// maintainChapterAfterEdit 章节正文落盘后的维护链路：向量刷新、搜索缓存、字数统计。
// 过渡期（v1.6）：向量/搜索/写作日志仍按 chapter_number 键控，用记录中的
// ChapterNumber 桥接；4.3 整体切换 chapter_id 后移除桥接。
func maintainChapterAfterEdit(ctx context.Context, tc ToolContext, ch *chapter.Chapter, content string) {
	rag.SubmitRefresh(tc.NovelID, ch.ChapterNumber, content)
	if tc.SearchService != nil {
		tc.SearchService.UpdateCachedChapter(tc.NovelID, ch.ChapterNumber, content)
	}

	stats := text.ComputeStats(content)

	var oldWC int
	tc.DB.WithContext(ctx).Model(&chapter.Chapter{}).
		Select("COALESCE(word_count, 0)").
		Where("id = ?", ch.ID).
		Scan(&oldWC)
	if delta := stats.WordCount - oldWC; delta != 0 {
		tc.DB.WithContext(ctx).Create(&writing.WritingLog{
			Date:          time.Now().Format("2006-01-02"),
			NovelID:       tc.NovelID,
			ChapterNumber: ch.ChapterNumber,
			WordDelta:     delta,
		})
	}

	tc.DB.WithContext(ctx).Model(&chapter.Chapter{}).
		Where("id = ?", ch.ID).
		Update("word_count", stats.WordCount)
}

// ── 工具描述 ──────────────────────────────────────────────

const editDescription = `编辑小说文件（章节正文、大纲、故事状态 goink.md 或技能文件）。支持三种编辑模式：full_replace（全文替换）、search_replace（查找替换）、line_range_replace（行范围替换）。

新建章节或大纲：path 使用 new.md 形式（格式见 path 参数说明），且只能 full_replace；成功后返回值携带分配的 chapter_id、chapter_number、volume_id 与物理路径，后续编辑一律使用返回的物理路径。

各模式必填参数：
- full_replace：new_content
- search_replace：search_text + new_content
- line_range_replace：start_line + end_line + new_content
所有模式都必填：path、change_type、reason
（new_content 传空的具体行为见 new_content 参数说明）

模式选择策略：
- search_replace 连续两次因"未找到匹配"失败，直接换 line_range_replace，不要在同一种模式上反复重试
- line_range_replace 使用前务必重新 read 确认行号（行号可能因前序操作偏移）；执行后会返回 before/after 上下文，若 before 与预期不符说明行号偏移，需重新 read 修正后再试

所有修改会先生成 git diff 提交用户审批，审批通过后才写入文件。被拒绝时返回用户反馈，可根据反馈修正后重试。`

// ── read ──────────────────────────────────────────────────

// ReadArgs 是 read 工具的参数。
type ReadArgs struct {
	Path         string `json:"path" jsonschema:"required,description=要读取的文件路径。章节正文：chapters/id_{章节id}.md（主格式）或 chapters/{卷id}/id_{章节id}.md（等价容错写法）；章节大纲：outlines/id_{章节id}.md 或 outlines/{卷id}/id_{章节id}.md；故事状态 goink.md；小说级技能 skills/<name>.md；用户级技能 ~/.goink/skills/<name>.md；内置技能 /builtin/skills/<name>.md（只读）" validate:"required"`
	IncludeLines *bool  `json:"include_lines" jsonschema:"default=true,description=是否包含行号前缀（如 123|）。默认 true，用于精确引用和行范围编辑。传 false 获取纯文本"`
	StartLine    int    `json:"start_line" jsonschema:"default=1,description=起始行号 1-based 含此行，必须 <= end_line" validate:"omitempty,min=1"`
	EndLine      int    `json:"end_line" jsonschema:"default=2000,description=结束行号 1-based 含此行，超出自动截到文末。不传或传 0 均按默认 2000 处理" validate:"omitempty,min=0"`
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

	// 章节/大纲按 id 扁平路径读取（两级路径归一化）
	if ref, isOutline, ok := parseRWPath(a.Path); ok {
		return t.readChapterLike(ctx, a, tc, ref, isOutline)
	}
	if !validPath(a.Path) {
		return &ToolResult{Success: false, Error: invalidPathHint}, nil
	}

	content, err := git.ReadFile(tc.NovelID, a.Path)
	if err != nil {
		if res, handled := fileReadError(a.Path, err); handled {
			return res, nil
		}
		return nil, fmt.Errorf("read file %s: %w", a.Path, err)
	}
	return renderReadResult(a, a.Path, a.Path, content)
}

// readChapterLike 读取章节正文或大纲。
// 物理文件永远按 id 扁平路径读，两级路径仅作容错别名归一化。
func (t *ReadTool) readChapterLike(ctx context.Context, a *ReadArgs, tc ToolContext, ref chapterRef, isOutline bool) (*ToolResult, error) {
	if ref.IsNew {
		return &ToolResult{Success: false, Error: "new.md 仅用于 edit 新建章节/大纲，不可读取"}, nil
	}

	physical := git.ChapterPath(ref.ID)
	suffix := ""
	if isOutline {
		physical = git.OutlinePath(ref.ID)
		suffix = "（大纲）"
	}

	content, err := git.ReadFile(tc.NovelID, physical)
	if err != nil {
		if res, handled := fileReadError(physical, err); handled {
			return res, nil
		}
		return nil, fmt.Errorf("read file %s: %w", physical, err)
	}

	display := physical
	ch, err := getChapterRecord(ctx, tc.DB, tc.NovelID, ref.ID)
	if err != nil {
		return nil, fmt.Errorf("query chapter record: %w", err)
	}
	if ch != nil {
		display = ch.Title + suffix
	}

	return renderReadResult(a, physical, display, content)
}

// renderReadResult 按行窗口切片并渲染读取结果（行号前缀、截断标记）。
func renderReadResult(a *ReadArgs, path, display, content string) (*ToolResult, error) {
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

	data := map[string]any{
		"path":        path,
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

特性：
- 默认添加行号前缀，方便后续 edit 工具进行 line_range_replace 和 search_replace
- 返回 total_lines 表示全文行数，用于判断是否被截断；可用 line_range_replace(1, total_lines, "") 清空整个文件
- start_line 和 end_line 支持行范围读取，用于翻页或精确引用`

// ── 注册 ──────────────────────────────────────────────────

// RegisterRWTools 注册读写工具。
func RegisterRWTools(r *Registry) {
	r.Register(&ReadTool{})
	r.Register(&EditTool{})
}
